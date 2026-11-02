package javascript

import (
	"reflect"
	"slices"

	"github.com/dop251/goja/ast"
)

// hasReservedConditionalFunction rejects Goja's legacy unbraced conditional
// function declarations when they claim a host name or execute. In non-strict
// helper code, `if (ok) function fetch() {}` is compiled as an assignment that
// can reach the global object even from inside a function. The lexical probe
// changes name resolution and cannot detect this case. Ordinary declarations
// inside braces remain block-local and are allowed.
//
// Inspect the entire AST, including nested function bodies. Reflection is
// restricted to Goja AST structs so all expression/control-flow containers are
// covered without duplicating the parser's node taxonomy. Source-file metadata
// and other non-AST structs are not traversed. Shared declaration-list pointers
// are visited once rather than repeatedly walking the same source subtree.
func hasReservedConditionalFunction(program *ast.Program) bool {
	astPackage := reflect.TypeFor[ast.Program]().PkgPath()
	pending := []reflect.Value{reflect.ValueOf(program)}
	seen := make(map[any]struct{})

	for len(pending) != 0 {
		value := pending[len(pending)-1]
		pending = pending[:len(pending)-1]

		switch value.Kind() {
		case reflect.Interface:
			if !value.IsNil() {
				pending = append(pending, value.Elem())
			}
		case reflect.Pointer:
			if value.IsNil() || value.Type().Elem().PkgPath() != astPackage {
				continue
			}
			node := value.Interface()
			if _, visited := seen[node]; visited {
				continue
			}
			seen[node] = struct{}{}
			if statement, ok := node.(*ast.IfStatement); ok {
				for _, branch := range []ast.Statement{statement.Consequent, statement.Alternate} {
					if declaration, ok := branch.(*ast.FunctionDeclaration); ok && declaration.Function.Name != nil {
						name := declaration.Function.Name.Name.String()
						if name == "execute" || slices.Contains(toolHostNames, name) {
							return true
						}
					}
				}
			}
			pending = append(pending, value.Elem())
		case reflect.Struct:
			if value.Type().PkgPath() == astPackage {
				for i := 0; i < value.NumField(); i++ {
					pending = append(pending, value.Field(i))
				}
			}
		case reflect.Slice:
			for i := 0; i < value.Len(); i++ {
				pending = append(pending, value.Index(i))
			}
		}
	}
	
	return false
}
