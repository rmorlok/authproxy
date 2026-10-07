package util

import "slices"

// CloneRawJSONMap copies both the map and each raw JSON byte slice, retaining
// nil versus explicit empty values for validation and patch semantics.
// The byte-slice constraint preserves named raw JSON types without making
// this utility package depend on schema packages that themselves import util.
func CloneRawJSONMap[T ~[]byte](values map[string]T) map[string]T {
	if values == nil {
		return nil
	}
	clone := make(map[string]T, len(values))
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
