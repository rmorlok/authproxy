package apjs

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
)

var reservedRuntimeVars = map[string]struct{}{
	"cfg":         {},
	"labels":      {},
	"annotations": {},
	"data":        {},
}

// Library is connector-level JavaScript that can define shared constants and
// functions for later predicate and transform expressions. The source is
// compiled once, but each evaluation gets its own goja runtime.
type Library struct {
	source  string
	program *goja.Program
}

// CompileLibrary parses connector-level JavaScript into a reusable program.
// It does not run the library; call Validate to check top-level execution.
func CompileLibrary(source string) (*Library, error) {
	if strings.TrimSpace(source) == "" {
		return &Library{}, nil
	}

	program, err := goja.Compile("connector.js", source, false)
	if err != nil {
		return nil, fmt.Errorf("compile connector JavaScript library: %w", err)
	}

	return &Library{
		source:  source,
		program: program,
	}, nil
}

// CompileAndValidateLibrary compiles connector-level JavaScript and verifies
// that its top-level initialization runs without runtime variables.
func CompileAndValidateLibrary(source string) (*Library, error) {
	library, err := CompileLibrary(source)
	if err != nil {
		return nil, err
	}
	if err := library.Validate(); err != nil {
		return nil, err
	}
	return library, nil
}

// ValidateExpressionSyntax parses a connector-authored expression without
// executing it. This is useful for expressions that depend on runtime variables
// and cannot be safely evaluated while the connector definition is validated.
func ValidateExpressionSyntax(expression string) error {
	if strings.TrimSpace(expression) == "" {
		return fmt.Errorf("expression is required")
	}
	if _, err := goja.Compile("expression.js", expression, false); err != nil {
		return fmt.Errorf("compile JavaScript expression: %w", err)
	}
	return nil
}

// Source returns the source JavaScript used to compile the library.
func (l *Library) Source() string {
	if l == nil {
		return ""
	}
	return l.source
}

// Validate runs the connector library without any injected runtime variables.
// This catches top-level initialization errors and prevents connector code from
// claiming names that AuthProxy injects during predicate and transform calls.
func (l *Library) Validate() error {
	if l == nil {
		return nil
	}

	vm := goja.New()
	if err := l.run(vm); err != nil {
		return err
	}
	return validateReservedRuntimeVars(vm)
}

// NewContext builds a JavaScript evaluation context using this library and
// the supplied runtime variables.
func (l *Library) NewContext(vars map[string]any) Context {
	return Context{
		library: l,
		vars:    vars,
	}
}

func (l *Library) run(vm *goja.Runtime) error {
	if l == nil || l.program == nil {
		return nil
	}
	if _, err := vm.RunProgram(l.program); err != nil {
		return fmt.Errorf("JS library error: %w", err)
	}
	return nil
}

func validateReservedRuntimeVars(vm *goja.Runtime) error {
	for name := range reservedRuntimeVars {
		value := vm.Get(name)
		if value != nil && !goja.IsUndefined(value) {
			return fmt.Errorf("JS library defines reserved runtime variable %q", name)
		}
	}
	return nil
}
