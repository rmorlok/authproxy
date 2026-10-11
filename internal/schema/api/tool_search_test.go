package api

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestToolSearchQueryValidation accepts omission and every supported detail
// level while keeping explicit limits within one bounded discovery page.
func TestToolSearchQueryValidation(t *testing.T) {
	require.Error(t, (*ToolSearchQuery)(nil).Validate())

	for _, detail := range []ToolSchemaDetail{"", ToolSchemaDetailNone, ToolSchemaDetailCompact, ToolSchemaDetailFull} {
		query := ToolSearchQuery{Schema: detail, ToolSetID: "tls_01example0000001"}
		require.NoError(t, query.Validate())
	}

	for _, test := range []struct {
		name  string
		query ToolSearchQuery
		field string
	}{
		{"omitted", ToolSearchQuery{}, ""},
		{"smallest page", ToolSearchQuery{Limit: util.ToPtr(1)}, ""},
		{"largest page", ToolSearchQuery{Limit: util.ToPtr(MaxToolSearchLimit)}, ""},
		{"zero limit", ToolSearchQuery{Limit: util.ToPtr(0)}, "$.limit"},
		{"negative limit", ToolSearchQuery{Limit: util.ToPtr(-1)}, "$.limit"},
		{"over limit", ToolSearchQuery{Limit: util.ToPtr(MaxToolSearchLimit + 1)}, "$.limit"},
		{"unknown schema", ToolSearchQuery{Schema: "preview"}, "$.schema"},
		{"miscased schema", ToolSearchQuery{Schema: "Full"}, "$.schema"},
		{"wrong resource", ToolSearchQuery{ToolSetID: "tol_01example0000001"}, "$.toolSetId"},
		{"invalid ID", ToolSearchQuery{ToolSetID: "unknown"}, "$.toolSetId"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.query.Validate()
			if test.field == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.field)
			}
		})
	}
}

// TestToolSearchQueryDefaults keeps caller storage separate and preserves exact
// search/cursor values, including the label used to select a connection.
func TestToolSearchQueryDefaults(t *testing.T) {
	original := ToolSearchQuery{
		Query:         "  calendar events  ",
		LabelSelector: "apxy/cxn/-/id=cxn_01example0000001,capability=calendar",
		ToolSetID:     "tls_01example0000001", IncludeUnavailable: true,
		Continue: " opaque+cursor/= ",
	}
	withDefaults := original.WithDefaults()
	require.Equal(t, ToolSchemaDetailNone, withDefaults.Schema)
	require.Equal(t, DefaultToolSearchLimit, *withDefaults.Limit)
	require.NoError(t, withDefaults.Validate())
	require.Empty(t, original.Schema)
	require.Nil(t, original.Limit)
	withDefaults.Schema, withDefaults.Limit = "", nil
	require.Equal(t, original, withDefaults)

	for _, limit := range []int{1, 29, MaxToolSearchLimit, 0} {
		original.Schema, original.Limit = ToolSchemaDetailFull, util.ToPtr(limit)
		copy := original.WithDefaults()
		require.Equal(t, original, copy)
		require.NotSame(t, original.Limit, copy.Limit)
		*copy.Limit = 17
		require.Equal(t, limit, *original.Limit)
	}

	invalid := ToolSearchQuery{Schema: "unknown", Limit: util.ToPtr(-1)}.WithDefaults()
	require.Equal(t, ToolSchemaDetail("unknown"), invalid.Schema)
	require.Equal(t, -1, *invalid.Limit)
	require.Error(t, invalid.Validate(), "defaults must not hide invalid supplied values")

	// The schema layer neither parses nor rewrites the shared selector grammar.
	query := ToolSearchQuery{LabelSelector: "provider in ("}
	require.NoError(t, query.Validate())
	require.Equal(t, query.LabelSelector, query.WithDefaults().LabelSelector)
}

// TestToolSearchQueryRoundTrip retains filters and continuation context across
// JSON/YAML transport without applying defaults or introducing connection fields.
func TestToolSearchQueryRoundTrip(t *testing.T) {
	for _, format := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{"JSON", json.Marshal, util.DecodeJSONStrict},
		{"YAML", yaml.Marshal, util.DecodeYAMLStrict},
	} {
		t.Run(format.name, func(t *testing.T) {
			original := ToolSearchQuery{
				Query: "events", LabelSelector: "apxy/cxn/-/id=cxn_01example0000001",
				ToolSetID: "tls_01example0000001", Schema: ToolSchemaDetailCompact,
				IncludeUnavailable: true, Limit: util.ToPtr(12), Continue: "token+/=",
			}
			data, err := format.encode(original)
			require.NoError(t, err)
			var decoded ToolSearchQuery
			require.NoError(t, format.decode(data, &decoded))
			require.Equal(t, original, decoded)
			require.NoError(t, decoded.Validate())

			var empty ToolSearchQuery
			require.NoError(t, format.decode([]byte(`{}`), &empty))
			require.Equal(t, ToolSearchQuery{}, empty)
			require.NoError(t, empty.Validate())

			for _, input := range []string{`{"connectionIds":["cxn_01example0000001"]}`, `{"unknown":true}`} {
				var query ToolSearchQuery
				require.Error(t, format.decode([]byte(input), &query))
			}
		})
	}
}
