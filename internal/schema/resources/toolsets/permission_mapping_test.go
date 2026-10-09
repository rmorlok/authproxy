package toolsets

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestPermissionMappingValidContracts keeps stable keys exact and accepts
// intentional overlap between exact keys and patterns without catalog lookup.
func TestPermissionMappingValidContracts(t *testing.T) {
	for _, match := range []SourceKeyMatch{
		{SourceKeys: []string{"list_calendars", "GET /calendars/{id}"}},
		{SourceKeyPatterns: []string{`calendar_.*`, `(?m)^list$`, `GET /calendars/[^/]+`}},
		{SourceKeys: []string{" padded key ", "Read", "read"}, SourceKeyPatterns: []string{` padded key `, `.*`}},
		{SourceKeys: []string{}, SourceKeyPatterns: []string{`list|search`}},
		{SourceKeys: []string{"list"}, SourceKeyPatterns: []string{}},
	} {
		mapping := &PermissionMapping{Match: match, AddVerbs: []string{"tool:calendar.list", "tool:readonly"}}
		before := mapping.Clone()
		require.NoError(t, mapping.Validate(nil))
		require.Equal(t, before, mapping, "validation must not normalize identities or aliases")
	}
}

// TestPermissionMappingValidation checks required values and nested diagnostic
// paths without treating patterns or aliases as executable authorization logic.
func TestPermissionMappingValidation(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*PermissionMapping)
		path string
	}{
		{"missing match", func(p *PermissionMapping) { p.Match = SourceKeyMatch{} }, "match"},
		{"empty match", func(p *PermissionMapping) {
			p.Match = SourceKeyMatch{SourceKeys: []string{}, SourceKeyPatterns: []string{}}
		}, "match"},
		{"blank key", func(p *PermissionMapping) { p.Match.SourceKeys = []string{" \t\n"} }, "match.sourceKeys[0]"},
		{"duplicate key", func(p *PermissionMapping) { p.Match.SourceKeys = []string{"list", "list"} }, "match.sourceKeys[1]"},
		{"blank pattern", func(p *PermissionMapping) { p.Match.SourceKeyPatterns = []string{" "} }, "match.sourceKeyPatterns[0]"},
		{"duplicate pattern", func(p *PermissionMapping) { p.Match.SourceKeyPatterns = []string{".*", ".*"} }, "match.sourceKeyPatterns[1]"},
		{"missing aliases", func(p *PermissionMapping) { p.AddVerbs = nil }, "addVerbs"},
		{"empty aliases", func(p *PermissionMapping) { p.AddVerbs = []string{} }, "addVerbs"},
		{"blank alias", func(p *PermissionMapping) { p.AddVerbs = []string{"\t"} }, "addVerbs[0]"},
		{"padded alias", func(p *PermissionMapping) { p.AddVerbs = []string{" tool:read"} }, "addVerbs[0]"},
		{"duplicate alias", func(p *PermissionMapping) { p.AddVerbs = []string{"tool:read", "tool:read"} }, "addVerbs[1]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			mapping := &PermissionMapping{Match: SourceKeyMatch{SourceKeys: []string{"list"}}, AddVerbs: []string{"tool:read"}}
			test.edit(mapping)
			require.ErrorContains(t, mapping.Validate(&common.ValidationContext{Path: "definition.permissionMappings[2]"}), "definition.permissionMappings[2]."+test.path)
		})
	}
	require.Error(t, (*PermissionMapping)(nil).Validate(nil))
	require.Error(t, (*SourceKeyMatch)(nil).Validate(nil))
}

// TestSourceKeyMatchRegexSyntax rejects unsupported RE2 syntax and expressions
// that only become syntactically valid by escaping the full-string wrapper.
func TestSourceKeyMatchRegexSyntax(t *testing.T) {
	for _, pattern := range []string{`[`, `(?=list)`, `(list)\1`, `foo)|bar(`} {
		matcher := SourceKeyMatch{SourceKeyPatterns: []string{pattern}}
		require.ErrorContains(t, matcher.Validate(nil), "sourceKeyPatterns[0]", pattern)
	}
	for _, pattern := range []string{`list|search`, `(?i)read`, `(?m)^read$`, `(?s).*`, `\Aread\z`} {
		matcher := SourceKeyMatch{SourceKeyPatterns: []string{pattern}}
		require.NoError(t, matcher.Validate(nil), pattern)
	}
}

// TestPermissionMappingStrictDecoding rejects nulls even where another valid
// match could hide them, and permits only canonical property names and types.
func TestPermissionMappingStrictDecoding(t *testing.T) {
	for _, input := range []string{
		`{"match":null,"addVerbs":["tool:read"]}`,
		`{"match":{"sourceKeys":null,"sourceKeyPatterns":[".*"]},"addVerbs":["tool:read"]}`,
		`{"match":{"sourceKeys":["list"],"sourceKeyPatterns":null},"addVerbs":["tool:read"]}`,
		`{"match":{"sourceKeys":[null]},"addVerbs":["tool:read"]}`,
		`{"match":{"sourceKeyPatterns":[null]},"addVerbs":["tool:read"]}`,
		`{"match":{"sourceKeys":["list"]},"addVerbs":null}`,
		`{"match":{"sourceKeys":["list"]},"addVerbs":[null]}`,
		`{"match":{"sourceKeys":["list"]},"AddVerbs":["tool:read"]}`,
		`{"Match":{"sourceKeys":["list"]},"addVerbs":["tool:read"]}`,
		`{"match":{"SourceKeys":["list"]},"addVerbs":["tool:read"]}`,
		`{"match":{"sourceKeyPatterns":[true]},"addVerbs":["tool:read"]}`,
		`{"match":{"sourceKeys":"list"},"addVerbs":["tool:read"]}`,
		`{"match":{"sourceKeys":["list"],"unknown":[]},"addVerbs":["tool:read"]}`,
		`{"match":{"sourceKeys":["list"]},"addVerbs":["tool:read"],"unknown":true}`,
	} {
		for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
			var mapping PermissionMapping
			require.Error(t, decode([]byte(input), &mapping), input)
		}
	}
	for _, input := range []string{`{}`, `{"match":{},"addVerbs":[]}`} {
		var mapping PermissionMapping
		require.NoError(t, util.DecodeJSONStrict([]byte(input), &mapping))
		require.Error(t, mapping.Validate(nil))
	}
	var mapping PermissionMapping
	require.Error(t, util.DecodeJSONStrict([]byte(`null`), &mapping))
	require.Error(t, util.DecodeJSONStrict([]byte(`{} {}`), &mapping))
	// yaml.v3 skips custom unmarshalling for a root null. Validation still
	// rejects the fresh zero value because match and aliases are required.
	var nullMapping PermissionMapping
	require.NoError(t, util.DecodeYAMLStrict([]byte(`null`), &nullMapping))
	require.Error(t, nullMapping.Validate(nil))
}

// TestPermissionMappingYAMLAliasesAndMerges proves null presence survives both
// forms of indirection instead of becoming an omitted matcher or empty alias.
func TestPermissionMappingYAMLAliasesAndMerges(t *testing.T) {
	for _, input := range []string{
		"<<: {match: null}\naddVerbs: [tool:read]",
		"match:\n  <<: {sourceKeys: null}\n  sourceKeyPatterns: ['.*']\naddVerbs: [tool:read]",
		"match:\n  sourceKeys: [list]\n  <<: {sourceKeyPatterns: null}\naddVerbs: [tool:read]",
		"match:\n  sourceKeys: &clear null\n  sourceKeyPatterns: *clear\naddVerbs: [tool:read]",
		"match:\n  sourceKeys: [&clear null]\naddVerbs: [*clear]",
		"match: {sourceKeys: [list]}\n<<: {addVerbs: null}",
	} {
		var mapping PermissionMapping
		require.Error(t, util.DecodeYAMLStrict([]byte(input), &mapping), input)
	}
	var mapping PermissionMapping
	input := "match:\n  <<: {sourceKeys: [list]}\n  sourceKeyPatterns: ['list_.*']\naddVerbs: [tool:read]"
	require.NoError(t, util.DecodeYAMLStrict([]byte(input), &mapping))
	require.NoError(t, mapping.Validate(nil))
	for _, marshal := range []func(any) ([]byte, error){json.Marshal, yaml.Marshal} {
		emitted, err := marshal(mapping)
		require.NoError(t, err)
		var roundTrip PermissionMapping
		require.NoError(t, util.DecodeYAMLStrict(emitted, &roundTrip))
		require.Equal(t, mapping, roundTrip)
	}
}

// TestPermissionMappingCloneAndDecodeOwnership verifies all slices detach and
// failed decoding is atomic while successful decoding clears stale fields.
func TestPermissionMappingCloneAndDecodeOwnership(t *testing.T) {
	require.Nil(t, (*PermissionMapping)(nil).Clone())
	require.Nil(t, (*SourceKeyMatch)(nil).Clone())
	for _, list := range [][]string{nil, {}} {
		mapping := &PermissionMapping{Match: SourceKeyMatch{SourceKeys: list, SourceKeyPatterns: list}, AddVerbs: list}
		clone := mapping.Clone()
		require.Equal(t, mapping, clone)
		require.NotSame(t, mapping, clone)
	}
	mapping := &PermissionMapping{Match: SourceKeyMatch{SourceKeys: []string{"list"}, SourceKeyPatterns: []string{`list_.*`}}, AddVerbs: []string{"tool:read"}}
	clone := mapping.Clone()
	clone.Match.SourceKeys[0] = "changed"
	clone.Match.SourceKeyPatterns[0] = "changed"
	clone.AddVerbs[0] = "changed"
	require.Equal(t, "list", mapping.Match.SourceKeys[0])
	require.Equal(t, `list_.*`, mapping.Match.SourceKeyPatterns[0])
	require.Equal(t, "tool:read", mapping.AddVerbs[0])

	before := mapping.Clone()
	require.Error(t, json.Unmarshal([]byte(`{"match":{"sourceKeys":["new"]},"addVerbs":null}`), mapping))
	require.Equal(t, before, mapping)
	require.Error(t, json.Unmarshal([]byte(`{"sourceKeys":["new"],"sourceKeyPatterns":null}`), &mapping.Match))
	require.Equal(t, before, mapping)
	require.Error(t, json.Unmarshal([]byte(`null`), &mapping.Match))
	require.NoError(t, json.Unmarshal([]byte(`{"sourceKeys":[]}`), &mapping.Match))
	require.NotNil(t, mapping.Match.SourceKeys)
	require.Empty(t, mapping.Match.SourceKeys)
	require.Nil(t, mapping.Match.SourceKeyPatterns)
	require.NoError(t, json.Unmarshal([]byte(`{}`), mapping))
	require.Equal(t, &PermissionMapping{}, mapping)
}
