package media

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidbyttow/govips/v2/vips"
)

// TestMain releases libvips once, the way main does.
func TestMain(m *testing.M) {
	code := m.Run()
	Shutdown()
	os.Exit(code)
}

// pngFixture builds a width×height PNG in memory.
func pngFixture(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = uint8(x), uint8(y), uint8(x^y), 0xff
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode fixture png: %v", err)
	}
	return buf.Bytes()
}

// openVariant checks that a stored variant is a non-empty WebP file, decodes it
// with vips and reports its size on disk.
func openVariant(t *testing.T, path string) (image.Point, int64) {
	t.Helper()
	img, err := vips.NewImageFromFile(path)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	defer img.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Size() == 0 {
		t.Fatalf("%s is empty", path)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	head := make([]byte, 12)
	if _, err := io.ReadFull(f, head); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(head[0:4]) != "RIFF" || string(head[8:12]) != "WEBP" {
		t.Fatalf("%s is not a WebP file: %q", path, head)
	}
	return image.Pt(img.Width(), img.Height()), info.Size()
}

func longEdge(p image.Point) int { return max(p.X, p.Y) }

func TestProcessImageWritesBothVariants(t *testing.T) {
	dir := t.TempDir()
	svc, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, sub := range []string{kindDirs[KindAvatar], kindDirs[KindActivity]} {
		info, err := os.Stat(filepath.Join(dir, sub))
		if err != nil || !info.IsDir() {
			t.Fatalf("New did not create %s: %v", sub, err)
		}
	}

	got, err := svc.ProcessImage(bytes.NewReader(pngFixture(t, 3000, 2000)), KindActivity, 42)
	if err != nil {
		t.Fatalf("ProcessImage: %v", err)
	}
	if want := svc.Path(KindActivity, 42, VariantFull); got.FullPath != want {
		t.Errorf("FullPath = %q, want %q", got.FullPath, want)
	}
	if want := svc.Path(KindActivity, 42, VariantThumb); got.ThumbPath != want {
		t.Errorf("ThumbPath = %q, want %q", got.ThumbPath, want)
	}
	if want := filepath.Join(dir, kindDirs[KindActivity]); filepath.Dir(got.FullPath) != want {
		t.Errorf("full variant written to %q, want %q", filepath.Dir(got.FullPath), want)
	}

	full, fullBytes := openVariant(t, got.FullPath)
	if full.X != FullEdge || full.Y != 1365 {
		t.Errorf("full variant = %v, want 2048x1365", full)
	}
	if got.Width != full.X || got.Height != full.Y {
		t.Errorf("Processed = %dx%d, want the full variant's %v", got.Width, got.Height, full)
	}
	if got.Bytes != fullBytes {
		t.Errorf("Processed.Bytes = %d, want %d", got.Bytes, fullBytes)
	}

	thumb, _ := openVariant(t, got.ThumbPath)
	if longEdge(thumb) != ThumbEdge {
		t.Errorf("thumbnail long edge = %d, want %d", longEdge(thumb), ThumbEdge)
	}
	if diff := math.Abs(float64(full.X)/float64(full.Y) - float64(thumb.X)/float64(thumb.Y)); diff > 0.01 {
		t.Errorf("thumbnail %v does not preserve the %v aspect ratio", thumb, full)
	}

	if err := svc.Remove(KindActivity, 42); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	for _, path := range []string{got.FullPath, got.ThumbPath} {
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("stat %s after Remove = %v, want not-exist", path, err)
		}
	}
	if err := svc.Remove(KindActivity, 42); err != nil {
		t.Errorf("Remove of absent files = %v, want nil", err)
	}
}

func TestProcessImageAvatarSmallerThanThumbnail(t *testing.T) {
	dir := t.TempDir()
	svc, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got, err := svc.ProcessImage(bytes.NewReader(pngFixture(t, 800, 600)), KindAvatar, 7)
	if err != nil {
		t.Fatalf("ProcessImage: %v", err)
	}
	if want := filepath.Join(dir, kindDirs[KindAvatar]); filepath.Dir(got.FullPath) != want {
		t.Errorf("avatar written to %q, want %q", filepath.Dir(got.FullPath), want)
	}
	full, _ := openVariant(t, got.FullPath)
	if full != image.Pt(800, 600) {
		t.Errorf("full variant = %v, want the 800x600 original", full)
	}
	thumb, _ := openVariant(t, got.ThumbPath)
	if thumb != image.Pt(ThumbEdge, 300) {
		t.Errorf("thumbnail = %v, want 400x300", thumb)
	}
}

func TestProcessImageRejectsUnknownKind(t *testing.T) {
	svc, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = svc.ProcessImage(infinity{}, "bogus", 1)
	if err == nil || !strings.Contains(err.Error(), "unknown kind") {
		t.Errorf("ProcessImage with unknown kind = %v, want an unknown-kind error", err)
	}
	if err := svc.Remove("bogus", 1); err == nil || !strings.Contains(err.Error(), "unknown kind") {
		t.Errorf("Remove with unknown kind = %v, want an unknown-kind error", err)
	}
	if got := svc.Path("bogus", 1, VariantFull); got != "" {
		t.Errorf("Path with unknown kind = %q, want empty", got)
	}
}

func TestProcessImageRejectsOversizedInput(t *testing.T) {
	svc, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = svc.ProcessImage(infinity{}, KindActivity, 1)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("ProcessImage of >%d bytes = %v, want a size-limit error", MaxInputBytes, err)
	}
	if got := svc.Path(KindActivity, 1, VariantFull); got != "" {
		if _, err := os.Stat(got); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("rejected upload left %s behind: %v", got, err)
		}
	}
}

// infinity is an endless stream of zero bytes, used to exercise the input bound
// without allocating a 20 MiB payload.
type infinity struct{}

func (infinity) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
