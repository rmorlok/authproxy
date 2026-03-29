package apjs

import (
	"fmt"

	"github.com/dop251/goja"
)

// DataSourceOption represents a single option for a data source dropdown/select.
type DataSourceOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// TransformJSON runs a JavaScript expression against input data and returns the result
// as a slice of DataSourceOption. The input data is available as the variable "data"
// in the JS expression. The expression should return an array of objects with "value"
// and "label" string fields.
func TransformJSON(expression string, data any) ([]DataSourceOption, error) {
	vm := goja.New()

	if err := vm.Set("data", data); err != nil {
		return nil, fmt.Errorf("failed to set data in JS runtime: %w", err)
	}

	result, err := vm.RunString(expression)
	if err != nil {
		return nil, fmt.Errorf("JS expression error: %w", err)
	}

	if goja.IsUndefined(result) || goja.IsNull(result) {
		return nil, fmt.Errorf("JS expression returned %s", result)
	}

	exported := result.Export()
	items, ok := exported.([]any)
	if !ok {
		return nil, fmt.Errorf("JS expression must return an array, got %T", exported)
	}

	options := make([]DataSourceOption, 0, len(items))
	for i, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("array element %d is not an object", i)
		}

		value, hasValue := obj["value"]
		label, hasLabel := obj["label"]
		if !hasValue || !hasLabel {
			return nil, fmt.Errorf("array element %d missing required 'value' or 'label' field", i)
		}

		options = append(options, DataSourceOption{
			Value: fmt.Sprint(value),
			Label: fmt.Sprint(label),
		})
	}

	return options, nil
}
