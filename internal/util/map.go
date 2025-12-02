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

// FlatMap transforms a slice of type T into a slice of type V
// by applying a function that returns a slice of V, then flattening the result.
func FlatMap[T, V any](input []T, transform func(T) []V) []V {
	// Pre-allocating with a guess of 0 or similar to input length helps,
	// but exact size is unknown without running the transform first.
	result := make([]V, 0, len(input))

	for _, item := range input {
		// Append automatically flattens the returned slice into the result
		result = append(result, transform(item)...)
	}
	return result
}
