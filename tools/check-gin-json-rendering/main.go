package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

const (
	defaultPattern = "./internal/routes/..."
	ginPackage     = "github.com/gin-gonic/gin"
	allowMarker    = "apiredact:allow-gin-json"
)

var forbiddenMethods = map[string]struct{}{
	"JSON":         {},
	"PureJSON":     {},
	"IndentedJSON": {},
	"SecureJSON":   {},
	"AsciiJSON":    {},
}

type violation struct {
	Filename string
	Line     int
	Column   int
	Method   string
	Message  string
}

type allowDirective struct {
	valid bool
}

type allowStatus struct {
	allowed bool
	present bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	patterns := args
	if len(patterns) == 0 {
		patterns = []string{defaultPattern}
	}

	violations, err := check(".", patterns)
	if err != nil {
		fmt.Fprintf(stderr, "check-gin-json-rendering: %v\n", err)
		return 2
	}

	if len(violations) == 0 {
		fmt.Fprintln(stdout, "route JSON rendering guard passed")
		return 0
	}

	for _, v := range violations {
		fmt.Fprintf(stderr, "%s:%d:%d: %s\n", v.Filename, v.Line, v.Column, v.Message)
	}
	return 1
}

func check(dir string, patterns []string) ([]violation, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}

	fset := token.NewFileSet()
	cfg := &packages.Config{
		Dir:   absDir,
		Fset:  fset,
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}
	if loadErr := packageLoadError(pkgs); loadErr != nil {
		return nil, loadErr
	}

	var violations []violation
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			filename := fset.Position(file.Pos()).Filename
			if !shouldCheckFile(absDir, filename) {
				continue
			}

			allows := collectAllowDirectives(fset, file)
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}

				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if _, found := forbiddenMethods[sel.Sel.Name]; !found {
					return true
				}
				if !isGinRenderSelection(pkg, sel) {
					return true
				}

				pos := fset.Position(sel.Sel.Pos())
				status := allowForLine(allows, pos.Line)
				if status.allowed {
					return true
				}

				message := fmt.Sprintf("direct Gin %s rendering is forbidden in internal/routes; use apgin.APIJSON so apiredact tags are honored, or add // %s: <reason>", sel.Sel.Name, allowMarker)
				if status.present {
					message = fmt.Sprintf("%s escape hatch comment must include a reason for the direct Gin %s rendering call", allowMarker, sel.Sel.Name)
				}
				violations = append(violations, violation{
					Filename: displayPath(absDir, pos.Filename),
					Line:     pos.Line,
					Column:   pos.Column,
					Method:   sel.Sel.Name,
					Message:  message,
				})
				return true
			})
		}
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Filename != violations[j].Filename {
			return violations[i].Filename < violations[j].Filename
		}
		if violations[i].Line != violations[j].Line {
			return violations[i].Line < violations[j].Line
		}
		return violations[i].Column < violations[j].Column
	})

	return violations, nil
}

func packageLoadError(pkgs []*packages.Package) error {
	var messages []string
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		for _, err := range pkg.Errors {
			messages = append(messages, err.Error())
		}
	})
	if len(messages) == 0 {
		return nil
	}
	sort.Strings(messages)
	return fmt.Errorf("load packages:\n%s", strings.Join(messages, "\n"))
}

func shouldCheckFile(rootDir string, filename string) bool {
	if strings.HasSuffix(filename, "_test.go") {
		return false
	}

	rel, err := filepath.Rel(rootDir, filename)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel == "internal/routes" || strings.HasPrefix(rel, "internal/routes/")
}

func isGinRenderSelection(pkg *packages.Package, sel *ast.SelectorExpr) bool {
	selection := pkg.TypesInfo.Selections[sel]
	if selection == nil {
		return false
	}

	obj := selection.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	if obj.Pkg().Path() != ginPackage {
		return false
	}
	_, forbidden := forbiddenMethods[obj.Name()]
	return forbidden
}

func collectAllowDirectives(fset *token.FileSet, file *ast.File) map[int][]allowDirective {
	allows := make(map[int][]allowDirective)
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if !strings.Contains(comment.Text, allowMarker) {
				continue
			}

			directive := allowDirective{valid: allowCommentHasReason(comment.Text)}
			start := fset.Position(comment.Pos()).Line
			end := fset.Position(comment.End()).Line
			for line := start; line <= end; line++ {
				allows[line] = append(allows[line], directive)
			}
		}
	}
	return allows
}

func allowCommentHasReason(text string) bool {
	idx := strings.Index(text, allowMarker)
	if idx == -1 {
		return false
	}

	reason := strings.TrimSpace(text[idx+len(allowMarker):])
	reason = strings.TrimPrefix(reason, ":")
	reason = strings.TrimPrefix(reason, "-")
	reason = strings.TrimSpace(reason)
	reason = strings.TrimSuffix(reason, "*/")
	reason = strings.TrimSpace(reason)
	return reason != ""
}

func allowForLine(allows map[int][]allowDirective, line int) allowStatus {
	var status allowStatus
	for _, checkLine := range []int{line, line - 1} {
		for _, directive := range allows[checkLine] {
			status.present = true
			if directive.valid {
				status.allowed = true
				return status
			}
		}
	}
	return status
}

func displayPath(rootDir string, filename string) string {
	rel, err := filepath.Rel(rootDir, filename)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filename
	}
	return filepath.ToSlash(rel)
}
