package util

import (
	"slices"

	"github.com/rmorlok/authproxy/internal/schema/common"
)

// CloneRawJSONMap copies both the map and each raw JSON byte slice, retaining
// nil versus explicit empty values for validation and patch semantics.
func CloneRawJSONMap(values map[string]common.RawJSON) map[string]common.RawJSON {
	if values == nil {
		return nil
	}
	clone := make(map[string]common.RawJSON, len(values))
	for key, value := range values {
		clone[key] = slices.Clone(value)
	}
	return clone
}

// CloneValue copies a pointer to a scalar or shallow struct. Callers must
// separately detach any nested pointers, maps, or slices in structured values.
func CloneValue[T any](value *T) *T {
	if value == nil {
		return nil
	}

	clone := *value

	return &clone
}
