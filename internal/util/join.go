package util

import "strings"

// StringsJoin joins a slice of string-like values into a single string. It is equivalent to strings.Join, but
// allows for a more specific type for the slice.
func StringsJoin[T ~string](strs []T, sep string) string {
	strsVer := make([]string, 0, len(strs))
	for _, s := range strs {
		strsVer = append(strsVer, string(s))
	}

	return strings.Join(strsVer, sep)
}
