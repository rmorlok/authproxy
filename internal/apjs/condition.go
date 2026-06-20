package apjs

import (
	"fmt"

	"github.com/dop251/goja"
)

// EvaluateBoolean runs a JavaScript expression with the supplied variables in
// scope and returns its boolean result. The expression must evaluate to a
// boolean; undefined, null, and other result types are rejected so callers can
// fail closed instead of guessing intent.
func EvaluateBoolean(expression string, vars map[string]any) (bool, error) {
	vm := goja.New()

	for name, value := range vars {
		if err := vm.Set(name, value); err != nil {
			return false, fmt.Errorf("failed to set %q in JS runtime: %w", name, err)
		}
	}

	result, err := vm.RunString(expression)
	if err != nil {
		return false, fmt.Errorf("JS expression error: %w", err)
	}
	if goja.IsUndefined(result) || goja.IsNull(result) {
		return false, fmt.Errorf("JS expression returned %s", result)
	}

	exported := result.Export()
	b, ok := exported.(bool)
	if !ok {
		return false, fmt.Errorf("JS expression must return a boolean, got %T", exported)
	}
	return b, nil
}
