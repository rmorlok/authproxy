package apjs

import (
	"fmt"

	"github.com/dop251/goja"
)

// Context evaluates JavaScript expressions against a connector library and a
// caller-provided runtime variable set.
type Context struct {
	library *Library
	vars    map[string]any
}

// NewContext builds a JavaScript evaluation context with an optional library.
func NewContext(library *Library, vars map[string]any) Context {
	return Context{
		library: library,
		vars:    vars,
	}
}

// WithVar returns a copy of the context with one additional runtime variable.
func (c Context) WithVar(name string, value any) Context {
	vars := make(map[string]any, len(c.vars)+1)
	for k, v := range c.vars {
		vars[k] = v
	}
	vars[name] = value
	c.vars = vars
	return c
}

// EvaluateBoolean runs a JavaScript expression and requires a boolean result.
// Undefined, null, and other result types are rejected so callers can fail
// closed instead of guessing intent.
func (c Context) EvaluateBoolean(expression string) (bool, error) {
	result, err := c.runExpression(expression)
	if err != nil {
		return false, err
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

// TransformOptions runs a JavaScript expression and converts its result into
// data-source dropdown/select options.
func (c Context) TransformOptions(expression string) ([]DataSourceOption, error) {
	result, err := c.runExpression(expression)
	if err != nil {
		return nil, err
	}
	return dataSourceOptionsFromValue(result)
}

// TransformJSON runs a transform expression with data exposed as the runtime
// variable "data".
func (c Context) TransformJSON(expression string, data any) ([]DataSourceOption, error) {
	return c.WithVar("data", data).TransformOptions(expression)
}

// EvaluateObject runs a JavaScript expression and exports its result as a
// JSON-like object. Undefined/null are treated as an empty object so migration
// hooks can no-op by returning nothing.
func (c Context) EvaluateObject(expression string) (map[string]any, error) {
	result, err := c.runExpression(expression)
	if err != nil {
		return nil, err
	}
	if goja.IsUndefined(result) || goja.IsNull(result) {
		return map[string]any{}, nil
	}

	exported := result.Export()
	obj, ok := exported.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("JS expression must return an object, got %T", exported)
	}
	return obj, nil
}

func (c Context) runExpression(expression string) (goja.Value, error) {
	vm := goja.New()

	if c.library != nil {
		if err := c.library.run(vm); err != nil {
			return nil, err
		}
		if err := validateReservedRuntimeVars(vm); err != nil {
			return nil, err
		}
	}

	for name, value := range c.vars {
		if err := vm.Set(name, value); err != nil {
			return nil, fmt.Errorf("failed to set %q in JS runtime: %w", name, err)
		}
	}

	result, err := vm.RunString(expression)
	if err != nil {
		return nil, fmt.Errorf("JS expression error: %w", err)
	}
	return result, nil
}
