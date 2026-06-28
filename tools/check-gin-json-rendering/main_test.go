package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCheckDetectsGinJSONRenderingMethods(t *testing.T) {
	root := testModule(t, map[string]string{
		"internal/routes/handlers.go": `
package routes

import "github.com/gin-gonic/gin"

func handler(c *gin.Context) {
	c.JSON(200, gin.H{"ok": true})
	c.PureJSON(200, gin.H{"ok": true})
	c.IndentedJSON(200, gin.H{"ok": true})
	c.SecureJSON(200, gin.H{"ok": true})
	c.AsciiJSON(200, gin.H{"ok": true})
}
`,
	})

	violations, err := check(root, []string{"./internal/routes/..."})
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}

	got := violationMethods(violations)
	want := []string{"AsciiJSON", "IndentedJSON", "JSON", "PureJSON", "SecureJSON"}
	if !slices.Equal(got, want) {
		t.Fatalf("methods = %v, want %v", got, want)
	}
}

func TestCheckIgnoresAllowedAndNonGinJSONCalls(t *testing.T) {
	root := testModule(t, map[string]string{
		"internal/routes/handlers.go": `
package routes

import "github.com/gin-gonic/gin"

type localRenderer struct{}

func (localRenderer) JSON(code int, obj any) {}

func handler(c *gin.Context, local localRenderer) {
	local.JSON(200, nil)

	// apiredact:allow-gin-json: this endpoint intentionally returns a fixed health payload with no route data
	c.JSON(200, gin.H{"ok": true})
	c.PureJSON(200, gin.H{"ok": true}) // apiredact:allow-gin-json: this endpoint intentionally bypasses API serialization
}
`,
	})

	violations, err := check(root, []string{"./internal/routes/..."})
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
}

func TestCheckRequiresEscapeHatchReason(t *testing.T) {
	root := testModule(t, map[string]string{
		"internal/routes/handlers.go": `
package routes

import "github.com/gin-gonic/gin"

func handler(c *gin.Context) {
	// apiredact:allow-gin-json
	c.JSON(200, gin.H{"ok": true})
}
`,
	})

	violations, err := check(root, []string{"./internal/routes/..."})
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("violations = %d, want 1", len(violations))
	}
	if !strings.Contains(violations[0].Message, "must include a reason") {
		t.Fatalf("message = %q, want missing reason guidance", violations[0].Message)
	}
}

func TestCheckSkipsTestsAndFilesOutsideRoutes(t *testing.T) {
	root := testModule(t, map[string]string{
		"internal/routes/handlers.go": `
package routes

func handler() {}
`,
		"internal/routes/handlers_test.go": `
package routes

import "github.com/gin-gonic/gin"

func testHandler(c *gin.Context) {
	c.JSON(200, gin.H{"ok": true})
}
`,
		"internal/notroutes/handlers.go": `
package notroutes

import "github.com/gin-gonic/gin"

func handler(c *gin.Context) {
	c.JSON(200, gin.H{"ok": true})
}
`,
	})

	violations, err := check(root, []string{"./..."})
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
}

func violationMethods(violations []violation) []string {
	methods := make([]string, 0, len(violations))
	for _, violation := range violations {
		methods = append(methods, violation.Method)
	}
	slices.Sort(methods)
	return methods
}

func testModule(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	baseFiles := map[string]string{
		"go.mod": `
module example.com/app

go 1.24

require github.com/gin-gonic/gin v0.0.0

replace github.com/gin-gonic/gin => ./testgin
`,
		"testgin/go.mod": `
module github.com/gin-gonic/gin

go 1.24
`,
		"testgin/context.go": `
package gin

type H map[string]any

type Context struct{}

func (*Context) JSON(code int, obj any) {}
func (*Context) PureJSON(code int, obj any) {}
func (*Context) IndentedJSON(code int, obj any) {}
func (*Context) SecureJSON(code int, obj any) {}
func (*Context) AsciiJSON(code int, obj any) {}
`,
	}
	for path, contents := range files {
		baseFiles[path] = contents
	}

	for path, contents := range baseFiles {
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("create dir for %s: %v", path, err)
		}
		if err := os.WriteFile(fullPath, []byte(strings.TrimSpace(contents)+"\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	return root
}
