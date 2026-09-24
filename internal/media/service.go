// Package media turns uploaded images into the WebP variants hyl serves.
//
// Every upload is decoded in memory by libvips, auto-rotated, and written
// twice: a full variant capped at 2048 px on the long edge and a thumbnail
// capped at 400 px. Only the encoded WebP bytes are kept, never the original
// upload. The layout below <mediaDir> is owned by this package alone.
package media

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/davidbyttow/govips/v2/vips"
)

// Limits and encoding parameters shared by both variants.
const (
	// MaxInputBytes is how much of an upload is read before decoding. Larger
	// input is rejected instead of being handed to libvips.
	MaxInputBytes int64 = 20 << 20

	// FullEdge and ThumbEdge are the long-edge caps of the two variants.
	FullEdge  = 2048
	ThumbEdge = 400

	webpQuality         = 80
	webpReductionEffort = 4

	dirMode  = 0o755
	fileMode = 0o644
)

// libvips lifecycle. Startup happens once per process; Shutdown is final,
// because govips cannot be stopped and started again.
var (
	startOnce sync.Once
	startErr  error
	stopOnce  sync.Once
	stopped   atomic.Bool
)

// start initializes libvips on first use and reports why processing is
// impossible once Shutdown has run.
func start() error {
	if stopped.Load() {
		return errors.New("media: libvips has been shut down")
	}
	startOnce.Do(func() {
		startErr = vips.Startup(&vips.Config{ConcurrencyLevel: runtime.NumCPU()})
	})
	return startErr
}

// Shutdown releases libvips. Call it once from main after the last image was
// processed: govips cannot be restarted, so Shutdown is idempotent and any
// later ProcessImage fails instead of touching a dead library.
func Shutdown() {
	stopOnce.Do(func() {
		stopped.Store(true)
		vips.Shutdown()
	})
}

// Processed describes the two WebP variants written for one uploaded image.
type Processed struct {
	FullPath  string
	ThumbPath string
	Width     int
	Height    int
	Bytes     int64
}

// Service owns the media directory layout and the libvips lifecycle.
type Service struct {
	dir string
}

// New prepares <mediaDir>/{avatar,activity} and returns the service.
func New(mediaDir string) (*Service, error) {
	if mediaDir == "" {
		return nil, errors.New("media: media directory is required")
	}
	dir, err := filepath.Abs(mediaDir)
	if err != nil {
		return nil, fmt.Errorf("media: resolve %q: %w", mediaDir, err)
	}
	for kind, sub := range kindDirs {
		path := filepath.Join(dir, sub)
		if err := os.MkdirAll(path, dirMode); err != nil {
			return nil, fmt.Errorf("media: create directory for kind %q: %w", kind, err)
		}
	}
	if err := start(); err != nil {
		return nil, err
	}
	return &Service{dir: dir}, nil
}

// ProcessImage decodes src, auto-rotates it, and writes <mediaDir>/<kind>/<id>.webp
// (long edge <= 2048) and <mediaDir>/<kind>/<id>_thumb.webp (long edge <= 400),
// both WebP quality 80 with metadata stripped and reduction effort 4. `id` is
// the media row id and `kind` is KindAvatar or KindActivity.
func (s *Service) ProcessImage(src io.Reader, kind string, id int64) (Processed, error) {
	sub, ok := kindDir(kind)
	if !ok {
		return Processed{}, fmt.Errorf("media: unknown kind %q", kind)
	}
	if src == nil {
		return Processed{}, errors.New("media: nil source")
	}
	if err := start(); err != nil {
		return Processed{}, err
	}

	raw, err := readBounded(src, MaxInputBytes)
	if err != nil {
		return Processed{}, err
	}
	full, err := render(raw, FullEdge)
	if err != nil {
		return Processed{}, err
	}
	thumb, err := render(raw, ThumbEdge)
	if err != nil {
		return Processed{}, err
	}

	fullPath := filepath.Join(s.dir, sub, fileName(id, VariantFull))
	thumbPath := filepath.Join(s.dir, sub, fileName(id, VariantThumb))
	if err := writeFile(fullPath, full.data); err != nil {
		return Processed{}, err
	}
	if err := writeFile(thumbPath, thumb.data); err != nil {
		// Never leave a full variant behind without its thumbnail.
		os.Remove(fullPath)
		return Processed{}, err
	}

	return Processed{
		FullPath:  fullPath,
		ThumbPath: thumbPath,
		Width:     full.width,
		Height:    full.height,
		Bytes:     int64(len(full.data)),
	}, nil
}

// Remove deletes both variants for one media id, ignoring files that are absent.
func (s *Service) Remove(kind string, id int64) error {
	sub, ok := kindDir(kind)
	if !ok {
		return fmt.Errorf("media: unknown kind %q", kind)
	}
	for _, variant := range []string{VariantFull, VariantThumb} {
		path := filepath.Join(s.dir, sub, fileName(id, variant))
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("media: remove %s: %w", path, err)
		}
	}
	return nil
}

// Path returns the absolute path of one variant of a stored image; variant is
// VariantFull or VariantThumb. An unknown kind yields the empty string.
func (s *Service) Path(kind string, id int64, variant string) string {
	sub, ok := kindDir(kind)
	if !ok {
		return ""
	}
	return filepath.Join(s.dir, sub, fileName(id, variant))
}

// rendered is one encoded variant together with the pixel dimensions it was
// encoded from.
type rendered struct {
	data          []byte
	width, height int
}

// render decodes raw, honours the EXIF orientation, shrinks the image to fit
// edge×edge, drops the metadata and encodes the result as WebP.
//
// The scaling step is a contain: vips.InterestingNone tells libvips to fit the
// image inside the box instead of cropping it to fill (which is what an
// interestingness value such as InterestingAttention asks for), so the long edge
// ends up at most `edge` px and the aspect ratio is untouched. Square display
// crops belong to the client, and the full variant is what the lightbox shows.
func render(raw []byte, edge int) (rendered, error) {
	img, err := vips.NewImageFromBuffer(raw)
	if err != nil {
		return rendered{}, fmt.Errorf("media: decode image: %w", err)
	}
	defer img.Close()

	if err := img.AutoRotate(); err != nil {
		return rendered{}, fmt.Errorf("media: auto-rotate: %w", err)
	}
	if err := img.ThumbnailWithSize(edge, edge, vips.InterestingNone, vips.SizeDown); err != nil {
		return rendered{}, fmt.Errorf("media: shrink to %d px: %w", edge, err)
	}
	if err := img.RemoveMetadata(); err != nil {
		return rendered{}, fmt.Errorf("media: strip metadata: %w", err)
	}
	data, _, err := img.ExportWebp(&vips.WebpExportParams{
		Quality:         webpQuality,
		StripMetadata:   true,
		ReductionEffort: webpReductionEffort,
	})
	if err != nil {
		return rendered{}, fmt.Errorf("media: encode webp: %w", err)
	}
	return rendered{data: data, width: img.Width(), height: img.Height()}, nil
}

// readBounded reads at most max bytes of r and rejects larger input.
func readBounded(r io.Reader, max int64) ([]byte, error) {
	buf, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, fmt.Errorf("media: read upload: %w", err)
	}
	if int64(len(buf)) > max {
		return nil, fmt.Errorf("media: upload exceeds the %d byte limit", max)
	}
	if len(buf) == 0 {
		return nil, errors.New("media: upload is empty")
	}
	return buf, nil
}

// writeFile atomically replaces path with data: it writes a temporary file in
// the same directory and renames it into place, so a half-written image is
// never visible to a reader.
func writeFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("media: create temporary file in %s: %w", dir, err)
	}
	tmp := f.Name()
	discard := func(cause error) error {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("media: write %s: %w", path, cause)
	}
	if _, err := f.Write(data); err != nil {
		return discard(err)
	}
	if err := f.Chmod(fileMode); err != nil {
		return discard(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("media: write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("media: rename into %s: %w", path, err)
	}
	return nil
}
