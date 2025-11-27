package util

// ZipToMap converts two slices into a map.
// If duplicate keys exist in 'keys', the last occurrence wins.
// It stops when the shorter slice is exhausted.
func ZipToMap[K comparable, V any](keys []K, values []V) map[K]V {
	// Determine the length of the shorter slice to avoid panic
	n := min(len(keys), len(values))

	result := make(map[K]V, n)
	for i := 0; i < n; i++ {
		result[keys[i]] = values[i]
	}
	return result
}
