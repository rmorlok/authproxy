package apgin

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/contrib/static"
)

// embedServeFileSystem adapts an fs.FS (typically an embed.FS sub-FS) for use
// with gin-contrib/static. It mirrors the index-friendly behavior of
// static.LocalFile(path, true): directory paths report Exists=true so that
// http.FileSystem's directory handling can serve their index.html.
type embedServeFileSystem struct {
	fs     fs.FS
	httpFS http.FileSystem
}

// NewEmbedServeFileSystem wraps an fs.FS as a static.ServeFileSystem.
func NewEmbedServeFileSystem(efs fs.FS) static.ServeFileSystem {
	return &embedServeFileSystem{fs: efs, httpFS: http.FS(efs)}
}

func (e *embedServeFileSystem) Open(name string) (http.File, error) {
	return e.httpFS.Open(name)
}

func (e *embedServeFileSystem) Exists(prefix string, urlPath string) bool {
	p := strings.TrimPrefix(urlPath, prefix)
	if len(p) == len(urlPath) {
		// urlPath did not start with prefix.
		return false
	}
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		// Mount root — let the SPA fallback render index.html.
		return false
	}
	f, err := e.fs.Open(p)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}
