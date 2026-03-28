package aptmpl

import (
	"github.com/cbroglie/mustache"
)

// RenderMustache renders a mustache template string with the given data context.
// Used to interpolate configuration values into OAuth endpoints, URLs, etc.
func RenderMustache(template string, data map[string]any) (string, error) {
	return mustache.Render(template, data)
}

// ContainsMustache returns true if the string contains mustache syntax ({{ }}).
func ContainsMustache(s string) bool {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '{' && s[i+1] == '{' {
			return true
		}
	}
	return false
}
