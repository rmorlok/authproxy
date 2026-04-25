package util

// UnionBoolMaps merges any number of map[K]bool inputs into a single map containing
// every key that was present in any input, all set to true. Useful when bool maps
// are being used as sets and a combined set is needed.
func UnionBoolMaps[K comparable](maps ...map[K]bool) map[K]bool {
	total := 0
	for _, m := range maps {
		total += len(m)
	}
	out := make(map[K]bool, total)
	for _, m := range maps {
		for k := range m {
			out[k] = true
		}
	}
	return out
}
