package util

// PrependAll prepends a prefix to all strings in a slice
func PrependAll(prefix string, strs []string) []string {
	return Map(strs, func(s string) string { return prefix + s })
}
