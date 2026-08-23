package www

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var assets embed.FS

// PublicFS is the pre-processed file system containing the production assets.
var PublicFS fs.FS

func init() {
	// Create a sub-file system starting at the 'dist' directory.
	if sub, err := fs.Sub(assets, "dist"); err == nil {
		PublicFS = sub
	} else {
		// This should only happen if 'dist' directory is missing during build.
		// We use an empty FS as fallback to avoid panic, though the UI won't work.
		PublicFS = fs.FS(nil)
	}
}
