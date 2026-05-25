// Package embed exposes the built demo-shell SPA as an embedded filesystem.
//
// The contents come from `vite build` (outDir=../backend/embed/dist).
// A placeholder file + .gitkeep live in dist/ so the //go:embed directive
// has at least one file on a fresh checkout — the real production build
// overwrites them in CI / `make demo-shell`.
package embed

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
