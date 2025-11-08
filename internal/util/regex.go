package util

import "strings"

// EscapeRegex escapes regex characters in a string
func EscapeRegex(s string) string {
	// Escape special regex characters to treat path as literal string
	escaped := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(
		strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(
			s,
			`\`, `\\`),
			`.`, `\.`),
			`+`, `\+`),
			`*`, `\*`),
		`?`, `\?`),
		`[`, `\[`),
		`]`, `\]`),
		`(`, `\(`)
	escaped = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(
		escaped,
		`)`, `\)`),
		`{`, `\{`),
		`}`, `\}`),
		`$`, `\$`)

	return escaped
}
