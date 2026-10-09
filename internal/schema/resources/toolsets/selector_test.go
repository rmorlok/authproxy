package toolsets_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestConnectionSelectorNamespaceScope verifies existing matcher semantics:
// default subtree scope includes its owner, while exact matchers stay exact.
func TestConnectionSelectorNamespaceScope(t *testing.T) {
	for _, test := range []struct {
		name, owner, scope, want string
		matches, excludes        []string
	}{
		{
			name:     "default subtree",
			owner:    "root.team",
			scope:    "",
			want:     "root.team.**",
			matches:  []string{"root.team", "root.team.user", "root.team.user.child"},
			excludes: []string{"root", "root.other", "root.teamwork"},
		},
		{
			name:     "exact owner",
			owner:    "root.team",
			scope:    "root.team",
			want:     "root.team",
			matches:  []string{"root.team"},
			excludes: []string{"root.team.user", "root.teamwork"},
		},
		{
			name:     "narrower subtree",
			owner:    "root.team",
			scope:    "root.team.user.**",
			want:     "root.team.user.**",
			matches:  []string{"root.team.user", "root.team.user.child"},
			excludes: []string{"root.team", "root.team.user2"},
		},
		{
			name:     "exact descendant",
			owner:    "root.team",
			scope:    "root.team.user",
			want:     "root.team.user",
			matches:  []string{"root.team.user"},
			excludes: []string{"root.team", "root.team.user.child"}},
		{
			name:     "root owner",
			owner:    "root",
			scope:    "",
			want:     "root.**",
			matches:  []string{"root", "root.team", "root.team.user"},
			excludes: []string{"rooted"},
		},
		{
			name:     "literal underscore",
			owner:    "root.team_a",
			scope:    "",
			want:     "root.team_a.**",
			matches:  []string{"root.team_a", "root.team_a.user"},
			excludes: []string{"root.teamXa", "root.teamXa.user"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			selector := &toolsets.ConnectionSelector{Namespace: test.scope, MatchLabels: map[string]string{}}
			matcher, labels, err := selector.Compile(test.owner)
			require.NoError(t, err)
			assert.Equal(t, test.want, matcher)
			assert.Empty(t, labels)
			for _, path := range test.matches {
				assert.True(t, namespace.Matches(matcher, path), path)
			}
			for _, path := range test.excludes {
				assert.False(t, namespace.Matches(matcher, path), path)
			}
		})
	}
}

// TestConnectionSelectorRejectsScopeExpansion rejects malformed matchers and
// subtree escapes before returning anything a caller could use for selection.
func TestConnectionSelectorRejectsScopeExpansion(t *testing.T) {
	for _, test := range []struct{ owner, scope string }{
		{"root.team", "root.**"}, {"root.team", "root.other.**"},
		{"root.team", "root.teamwork.**"}, {"root.team", "root.team.*"},
		{"root.team", "root.team**"}, {"root.team", "root.team.**.user"},
		{"root.team", " root.team"}, {"root.team", "**"},
		{"", "root.team"}, {"root.**", ""}, {"invalid", ""},
		{namespace.SkipPermissionChecks, ""},
	} {
		t.Run(test.owner+"/"+test.scope, func(t *testing.T) {
			selector := &toolsets.ConnectionSelector{Namespace: test.scope, MatchLabels: map[string]string{}}
			matcher, labels, err := selector.Compile(test.owner)
			require.Error(t, err)
			assert.Empty(t, matcher)
			assert.Empty(t, labels)
		})
	}
}

// TestConnectionSelectorCompilesForExistingLabelEngine guards compatibility
// with the shared database parser without importing it from production schemas.
func TestConnectionSelectorCompilesForExistingLabelEngine(t *testing.T) {
	selector := &toolsets.ConnectionSelector{MatchLabels: map[string]string{"z": "two", "a": "one", "empty": ""}}
	_, text, err := selector.Compile("root.team")
	require.NoError(t, err)
	assert.Equal(t, "a=one,empty=,z=two", text)
	parsed, err := database.ParseLabelSelector(text)
	require.NoError(t, err)
	assert.Equal(t, database.LabelSelector{
		{Key: "a", Operator: database.LabelOperatorEqual, Value: "one"},
		{Key: "empty", Operator: database.LabelOperatorEqual, Value: ""},
		{Key: "z", Operator: database.LabelOperatorEqual, Value: "two"},
	}, parsed)
	assert.True(t, parsed.Matches(map[string]string{"a": "one", "z": "two", "empty": "", "extra": "allowed"}))
	assert.False(t, parsed.Matches(map[string]string{"a": "one", "z": "two"}), "empty equality still requires the key")
	assert.False(t, parsed.Matches(map[string]string{"a": "one", "z": "other", "empty": ""}), "every requirement must match")
	assert.False(t, parsed.Matches(nil))
	_, again, err := (&toolsets.ConnectionSelector{MatchLabels: map[string]string{"empty": "", "a": "one", "z": "two"}}).Compile("root.team")
	require.NoError(t, err)
	assert.Equal(t, text, again)
}

// TestConnectionSelectorReadsSystemLabels allows inherited labels and their
// existing longer value limit without treating selection as a label write.
func TestConnectionSelectorReadsSystemLabels(t *testing.T) {
	labels := map[string]string{
		"apxy/cxn/-/id":                "cxn_example",
		"apxy/cxr/vendor.example/type": "calendar",
		"apxy/ns/-/ns":                 "root." + strings.Repeat("tenant_", 20),
	}
	selector := &toolsets.ConnectionSelector{MatchLabels: labels}
	_, text, err := selector.Compile("root")
	require.NoError(t, err)
	parsed, err := database.ParseLabelSelector(text)
	require.NoError(t, err)
	assert.True(t, parsed.Matches(labels))
	assert.Equal(t, labels, selector.MatchLabels)
	_, _, err = (&toolsets.ConnectionSelector{MatchLabels: map[string]string{"ordinary": labels["apxy/ns/-/ns"]}}).Compile("root")
	require.Error(t, err)
}

// TestConnectionSelectorRejectsLabelSyntax prevents equality values or keys
// from injecting additional requirements into the compiled matcher string.
func TestConnectionSelectorRejectsLabelSyntax(t *testing.T) {
	for _, labels := range []map[string]string{
		{"": "value"}, {"a,b": "value"}, {"a=b": "value"}, {"!a": "value"},
		{"a": "one,b=two"}, {"a": "one=two"}, {"a": " one"}, {"a": "one\ntwo"},
		{"apxy//id": "value"}, {"a/b/c": "value"},
		{"a": strings.Repeat("x", 64)}, {"apxy/ns/-/ns": strings.Repeat("x", 254)},
	} {
		selector := &toolsets.ConnectionSelector{MatchLabels: labels}
		err := selector.ValidateForNamespace("root", &common.ValidationContext{Path: "spec.connectionSelector"})
		require.ErrorContains(t, err, "spec.connectionSelector.matchLabels")
		matcher, text, err := selector.Compile("root")
		require.Error(t, err)
		assert.Empty(t, matcher)
		assert.Empty(t, text)
	}
}

// TestConnectionSelectorRequiresDeliberateLabelMap distinguishes an intentional
// all-in-scope selector from a missing or null selection policy in JSON/YAML.
func TestConnectionSelectorRequiresDeliberateLabelMap(t *testing.T) {
	for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
		for _, input := range []string{`{}`, `{"matchLabels":null}`} {
			var selector toolsets.ConnectionSelector
			require.NoError(t, decode([]byte(input), &selector))
			require.Nil(t, selector.MatchLabels)
			require.ErrorContains(t, selector.ValidateForNamespace("root", nil), "matchLabels")
		}
		for _, input := range []string{`{"matchLabels":{}}`, `{"namespace":"","matchLabels":{}}`} {
			var selector toolsets.ConnectionSelector
			require.NoError(t, decode([]byte(input), &selector))
			require.NotNil(t, selector.MatchLabels)
			matcher, text, err := selector.Compile("root")
			require.NoError(t, err)
			assert.Equal(t, "root.**", matcher)
			parsed, err := database.ParseLabelSelector(text)
			require.NoError(t, err)
			assert.True(t, parsed.Matches(nil))
		}
	}
	var absent *toolsets.ConnectionSelector
	_, _, err := absent.Compile("root")
	require.ErrorContains(t, err, "connection selector is required")
}

// TestConnectionSelectorStrictDecoding rejects unsupported operators, null
// equality values, unknown fields, and malformed collection or scalar types.
func TestConnectionSelectorStrictDecoding(t *testing.T) {
	for _, input := range []string{
		`{"matchLabels":{},"matchExpressions":[]}`, `{"matchlabels":{}}`,
		`{"namespace":null,"matchLabels":{}}`, `{"namespace":1,"matchLabels":{}}`,
		`{"matchLabels":{"env":null}}`, `{"matchLabels":{"env":true}}`,
		`{"matchLabels":{"env":["prod"]}}`, `{"matchLabels":[]}`,
	} {
		for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
			var selector toolsets.ConnectionSelector
			require.Error(t, decode([]byte(input), &selector), input)
		}
	}
	var selector toolsets.ConnectionSelector
	require.Error(t, util.DecodeJSONStrict([]byte(`{"matchLabels":{}} {}`), &selector))
	require.Error(t, util.DecodeJSONStrict([]byte(`null`), &selector))
	// YAML bypasses custom unmarshalling for a top-level null. The zero
	// selector still fails validation instead of becoming an all-match policy.
	var nullSelector toolsets.ConnectionSelector
	require.NoError(t, util.DecodeYAMLStrict([]byte(`null`), &nullSelector))
	require.Error(t, nullSelector.ValidateForNamespace("root", nil))
}

// TestConnectionSelectorYAMLAliasesAndMerges applies ordinary YAML composition
// before validation, preserving exact empty values and detecting aliased nulls.
func TestConnectionSelectorYAMLAliasesAndMerges(t *testing.T) {
	var selector toolsets.ConnectionSelector
	input := "namespace: &empty ''\nmatchLabels:\n  <<: &base {env: prod}\n  blank: *empty\n"
	require.NoError(t, util.DecodeYAMLStrict([]byte(input), &selector))
	matcher, text, err := selector.Compile("root.team")
	require.NoError(t, err)
	assert.Equal(t, "root.team.**", matcher)
	assert.Equal(t, "blank=,env=prod", text)
	encoded, err := yaml.Marshal(selector)
	require.NoError(t, err)
	var roundTrip toolsets.ConnectionSelector
	require.NoError(t, util.DecodeYAMLStrict(encoded, &roundTrip))
	assert.Equal(t, selector, roundTrip)
	for _, invalid := range []string{
		"matchLabels:\n  <<: &base {env: null}\n",
		"matchLabels:\n  first: &missing null\n  second: *missing\n",
	} {
		require.Error(t, util.DecodeYAMLStrict([]byte(invalid), &roundTrip))
	}
}

// TestConnectionSelectorCloneAndDecodeOwnership preserves nil/empty maps and
// ensures a failed decode or changed clone cannot rewrite an existing policy.
func TestConnectionSelectorCloneAndDecodeOwnership(t *testing.T) {
	var absent *toolsets.ConnectionSelector
	assert.Nil(t, absent.Clone())
	assert.Nil(t, (&toolsets.ConnectionSelector{}).Clone().MatchLabels)
	assert.NotNil(t, (&toolsets.ConnectionSelector{MatchLabels: map[string]string{}}).Clone().MatchLabels)
	original := &toolsets.ConnectionSelector{Namespace: "root.team", MatchLabels: map[string]string{"env": "prod"}}
	clone := original.Clone()
	clone.Namespace = "root.team.child"
	clone.MatchLabels["env"] = "changed"
	assert.Equal(t, "root.team", original.Namespace)
	assert.Equal(t, "prod", original.MatchLabels["env"])
	before, err := json.Marshal(original)
	require.NoError(t, err)
	require.Error(t, json.Unmarshal([]byte(`{"namespace":"root.other","matchLabels":{"env":null}}`), original))
	after, err := json.Marshal(original)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	require.NoError(t, json.Unmarshal([]byte(`{"matchLabels":{}}`), original))
	assert.Empty(t, original.Namespace)
	assert.NotNil(t, original.MatchLabels)
	assert.Empty(t, original.MatchLabels)
}
