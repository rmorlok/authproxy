package common

import (
	"context"
	"fmt"
	"os"

	"github.com/cbroglie/mustache"
)

// StringValueTemplatedEnvVars is a string value whose final value is produced by rendering a
// mustache template, substituting any `{{VAR}}` tokens with the values of the named environment
// variables. The primary value is considered "not present" if any of the referenced env vars
// are unset or empty; in that case the optional Default is used. If neither the templated value
// nor the default is available, HasValue returns false and GetValue returns an error.
type StringValueTemplatedEnvVars struct {
	Template string  `json:"template_env_vars" yaml:"template_env_vars"`
	Default  *string `json:"default,omitempty" yaml:"default,omitempty"`
}

// resolveEnvVars walks the template and looks up each referenced variable in the environment.
// Returns the lookup map, whether every variable resolved to a non-empty value, and any
// template-parsing error encountered.
func (t *StringValueTemplatedEnvVars) resolveEnvVars() (map[string]any, bool, error) {
	tmpl, err := mustache.ParseString(t.Template)
	if err != nil {
		return nil, false, fmt.Errorf("invalid template: %w", err)
	}

	data := map[string]any{}
	allPresent := true
	seen := map[string]struct{}{}
	for _, tag := range tmpl.Tags() {
		if tag.Type() != mustache.Variable {
			continue
		}
		name := tag.Name()
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}

		val, present := os.LookupEnv(name)
		if !present || len(val) == 0 {
			allPresent = false
			continue
		}
		data[name] = val
	}

	return data, allPresent, nil
}

func (t *StringValueTemplatedEnvVars) HasValue(ctx context.Context) bool {
	_, allPresent, err := t.resolveEnvVars()
	if err == nil && allPresent && len(t.Template) > 0 {
		return true
	}
	return t.Default != nil && len(*t.Default) > 0
}

func (t *StringValueTemplatedEnvVars) GetValue(ctx context.Context) (string, error) {
	data, allPresent, err := t.resolveEnvVars()
	if err != nil {
		return "", err
	}

	if allPresent && len(t.Template) > 0 {
		rendered, err := mustache.Render(t.Template, data)
		if err != nil {
			return "", fmt.Errorf("failed to render template: %w", err)
		}
		return rendered, nil
	}

	if t.Default != nil {
		return *t.Default, nil
	}
	return "", fmt.Errorf("one or more environment variables referenced by template '%s' do not have a value", t.Template)
}

func (t *StringValueTemplatedEnvVars) Clone() StringValueType {
	if t == nil {
		return nil
	}

	clone := *t
	return &clone
}

var _ StringValueType = (*StringValueTemplatedEnvVars)(nil)
