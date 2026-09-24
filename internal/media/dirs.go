package media

import "strconv"

// Variants accepted by Path.
const (
	VariantFull  = "full"
	VariantThumb = "thumb"
)

// File naming of the two variants written for one media id: the full image is
// <id>.webp and the thumbnail <id>_thumb.webp.
const (
	webpExt     = ".webp"
	thumbSuffix = "_thumb"
)

// kindDirs maps a media kind (KindAvatar/KindActivity, the values stored in
// media.kind) to the subdirectory under the media root that stores its files.
// It is the only place the on-disk layout is spelled out.
var kindDirs = map[string]string{
	KindAvatar:   "avatar",
	KindActivity: "activity",
}

// kindDir returns the subdirectory holding one media kind and reports whether
// the kind is known.
func kindDir(kind string) (string, bool) {
	dir, ok := kindDirs[kind]
	return dir, ok
}

// fileName renders the file name of one variant of a media id. Any variant
// other than VariantThumb resolves to the full image.
func fileName(id int64, variant string) string {
	base := strconv.FormatInt(id, 10)
	if variant == VariantThumb {
		return base + thumbSuffix + webpExt
	}
	return base + webpExt
}
