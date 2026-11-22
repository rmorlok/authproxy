package runtime

import (
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
)

// HostBinding declares one global owned by the trusted adapter. Compilation
// rejects authored declarations of this name; initialization freezes its
// caller-installed value, or undefined when the capability was not supplied.
type HostBinding struct {
	// Name must be a unique literal JavaScript identifier other than execute.
	// Escaped spellings are rejected so the lexical and property names agree.
	Name string

	// PreserveEnumerable retains an existing property's enumeration flag. Use
	// it for already-bound JSON aliases. Otherwise the host stays hidden from
	// global enumeration. An absent binding is non-enumerable in either case.
	PreserveEnumerable bool
}

// copyHostBindings validates trusted host policy before composing source probes
// and keeps a private copy so later caller edits cannot change initialization.
func copyHostBindings(bindings []HostBinding) ([]HostBinding, error) {
	copied := slices.Clone(bindings)
	seen := make(map[string]struct{}, len(copied))
	for _, binding := range copied {
		if binding.Name == "execute" || !validHostName(binding.Name) {
			return nil, ErrInvalidInput
		}
		if _, duplicate := seen[binding.Name]; duplicate {
			return nil, ErrInvalidInput
		}
		seen[binding.Name] = struct{}{}
	}
	return copied, nil
}

// validHostName requires exactly one uninitialized identifier declaration with
// the supplied spelling. Compiler validation also rejects strict-mode keywords;
// AST checks reject extra declarations, patterns, escapes, and injected source.
func validHostName(name string) bool {
	if name == "" || !utf8.ValidString(name) {
		return false
	}
	parsed, err := parser.ParseFile(nil, "host-name.js", "let "+name+";", 0, parser.WithDisableSourceMaps)
	if err != nil || len(parsed.Body) != 1 {
		return false
	}
	declaration, ok := parsed.Body[0].(*ast.LexicalDeclaration)
	if !ok || len(declaration.List) != 1 || declaration.List[0].Initializer != nil {
		return false
	}
	identifier, ok := declaration.List[0].Target.(*ast.Identifier)
	if !ok || identifier.Name.String() != name {
		return false
	}
	_, err = goja.CompileAST(parsed, true)
	return err == nil
}

// checkHostBindings uses the JavaScript compiler's scope rules to find global
// declaration conflicts. The appended lexical declaration is a compile-time
// probe, never executed. Appending preserves directives such as "use strict".
func checkHostBindings(source string, bindings []HostBinding, strict, reserveExecute bool) error {
	names := make([]string, 0, len(bindings)+1)
	reserved := make(map[string]struct{}, len(bindings)+1)
	for _, binding := range bindings {
		names = append(names, binding.Name)
		reserved[binding.Name] = struct{}{}
	}
	if reserveExecute {
		names = append(names, "execute")
		reserved["execute"] = struct{}{}
	}
	if len(names) == 0 {
		return nil
	}
	parsed, err := parser.ParseFile(
		nil, // fileSet
		"async-bindings.js",
		source+"\n;let "+strings.Join(names, ",")+";",
		0, // mode
		parser.WithDisableSourceMaps,
	)
	if err != nil {
		return ErrInvalidSource
	}
	if !strict && hasReservedConditionalFunction(parsed, reserved) {
		return ErrInvalidSource
	}
	if _, err := goja.CompileAST(parsed, strict); err != nil {
		return ErrInvalidSource
	}
	return nil
}

// hasReservedConditionalFunction rejects Goja's legacy unbraced conditional
// function declarations when they claim a reserved name. In non-strict
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
func hasReservedConditionalFunction(program *ast.Program, reserved map[string]struct{}) bool {
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
						if _, found := reserved[name]; found {
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
