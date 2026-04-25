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

// ExtractVariables parses the template and returns the de-duplicated list of variable
// names referenced by it (including dotted paths like "cfg.tenant"). Section and inverted
// section tags are included alongside variable tags. Returns an error if the template is
// malformed.
func ExtractVariables(template string) ([]string, error) {
	if !ContainsMustache(template) {
		return nil, nil
	}

	tmpl, err := mustache.ParseString(template)
	if err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	var out []string
	collectTagNames(tmpl.Tags(), seen, &out)
	return out, nil
}

func collectTagNames(tags []mustache.Tag, seen map[string]struct{}, out *[]string) {
	for _, t := range tags {
		switch t.Type() {
		case mustache.Variable:
			name := t.Name()
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				*out = append(*out, name)
			}
		case mustache.Section, mustache.InvertedSection:
			name := t.Name()
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				*out = append(*out, name)
			}
			collectTagNames(t.Tags(), seen, out)
		}
	}
}
