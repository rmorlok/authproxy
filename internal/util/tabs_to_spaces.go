package util

import "strings"

func TabsToSpaces(s string, n int) string {
	return strings.ReplaceAll(s, "\t", strings.Repeat(" ", n))
}
