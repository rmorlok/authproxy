// Package marketplaceembed exposes the built marketplace UI as an embedded filesystem.
//
// The contents come from `vite build` (outDir=embed/dist). The dist/ directory
// always contains a placeholder.html and .gitkeep so the //go:embed directive
// has at least one file on a fresh checkout — the real production build
// overwrites them in CI (see Dockerfile node stage).
package marketplaceembed

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS returns a filesystem rooted at the built dist/ directory.
func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
