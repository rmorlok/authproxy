package util

// Map applies a function to each element of the input slice and returns a new slice with the results
func Map[T any, U any](input []T, transform func(T) U) []U {
	// Create a new slice with the same length as input to hold the results
	result := make([]U, len(input))

	// Apply the transformation function to each element
	for i, v := range input {
		result[i] = transform(v)
	}

	return result
}

// GetKeys extracts all keys from a map into a slice
func GetKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
