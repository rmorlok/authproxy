package apjs

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// EvaluateObjectJSONWithContext evaluates a connector statement list and returns
// a detached JSON object under explicit finite limits and caller cancellation.
// Root null/undefined becomes {}, preserving migration hooks' no-op result.
// Ordinary objects and null-prototype objects are supported; arrays and other
// root values are rejected. Nested values must follow the shared JSON contract:
// no getters, proxies, custom prototypes, cycles, functions, or undefined fields.
//
// Input copying, library initialization, expression execution, and output
// encoding share one deadline. Library source must also be bounded before its
// compilation. Inputs follow EvaluateBooleanWithContext's native-copy contract;
// authored mutations never affect caller maps and no I/O host is installed.
// Failures return nil and safe runtime classifications without authored details.
func (c Context) EvaluateObjectJSONWithContext(
	ctx context.Context,
	expression string,
	limits jsruntime.Limits,
) (json.RawMessage, error) {
	return c.evaluateJSONWithContext(
		ctx, expression, limits,
		func(_ context.Context, vm *goja.Runtime, value goja.Value) (goja.Value, error) {
			if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
				return vm.NewObject(), nil
			}
			// Inspect type metadata without exporting a graph or invoking proxy
			// traps. The shared encoder validates prototypes and every property.
			object, ok := value.(*goja.Object)
			if !ok || object.ExportType() != reflect.TypeFor[map[string]any]() || object.ClassName() != "Object" {
				return nil, jsruntime.ErrInvalidOutput
			}
			return object, nil
		},
	)
}
