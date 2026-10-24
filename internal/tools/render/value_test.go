package render

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

func TestRenderValuePreservesTypes(t *testing.T) {
	// Typed references preserve all JSON kinds and authored numeric precision.
	data, err := NewContext(Input{Params: map[string]any{
		"false": false, "null": nil, "number": json.Number("9007199254740993.0100"),
		"array": []any{false, nil, "x"}, "object": map[string]any{"count": 2},
		"string": `quote" and <&>`,
	}}, testLimits())
	require.NoError(t, err)
	source := common.RawJSON(`{
		"boolean":{"$value":"params.false"}, "null":{"$value":"params.null"},
		"number":{"$value":"params.number"}, "authored":9007199254740993.0100,
		"array":{"$value":"params.array"}, "object":{"$value":"params.object"},
		"string":{"$value":"params.string"}, "template":"prefix {{params.string}}",
		"nested":[{"value":"{{params.false}}"}]
	}`)
	result, err := RenderValue(context.Background(), source, data, testLimits())
	require.NoError(t, err)
	require.JSONEq(t, `{
		"boolean":false,"null":null,"number":9007199254740993.0100,"authored":9007199254740993.0100,
		"array":[false,null,"x"],"object":{"count":2},"string":"quote\" and <&>",
		"template":"prefix quote\" and <&>","nested":[{"value":"false"}]
	}`, string(result))
	require.Equal(t, 2, strings.Count(string(result), "9007199254740993.0100"))
	require.Contains(t, string(result), "<&>")

	// Returned bytes are independent; corrupting them cannot corrupt future renders.
	result[0] = '!'
	again, err := RenderValue(context.Background(), source, data, testLimits())
	require.NoError(t, err)
	require.True(t, json.Valid(again))
}

func TestRenderValueLiteralAndInertData(t *testing.T) {
	// Caller values that look executable are inserted as data without a second pass.
	data, err := NewContext(Input{Params: map[string]any{
		"data": map[string]any{"$value": "cfg.secret", "text": "{{cfg.secret}}"},
	}}, testLimits())
	require.NoError(t, err)
	source := common.RawJSON(`{
		"injected":{"$value":"params.data"},
		"literal":{"$literal":{"$value":"cfg.secret","text":"{{>private-file}}"}},
		"literalScalar":{"$literal":"{{cfg.secret}}"},
		"ordinary":{"$value":"not.a.reference","another":true},
		"{{cfg.secret}}":"literal key"
	}`)
	result, err := RenderValue(context.Background(), source, data, testLimits())
	require.NoError(t, err)
	require.JSONEq(t, `{
		"injected":{"$value":"cfg.secret","text":"{{cfg.secret}}"},
		"literal":{"$value":"cfg.secret","text":"{{>private-file}}"},
		"literalScalar":"{{cfg.secret}}", "ordinary":{"$value":"not.a.reference","another":true},
		"{{cfg.secret}}":"literal key"
	}`, string(result))
}

func TestRenderValueFailures(t *testing.T) {
	data, err := NewContext(Input{Cfg: map[string]any{"secret": "private-configuration"}}, testLimits())
	require.NoError(t, err)
	for _, source := range []string{
		``, `{"bad":`, `true false`, `{"$value":42}`, `{"$value":""}`,
		`{"$value":"params.absent"}`, `"{{>partial}}"`, `"{{cfg.secret.child}}"`,
		string([]byte{'"', 0xff, '"'}),
	} {
		t.Run(source, func(t *testing.T) {
			// Invalid source, directives, and strict reference errors return no partial body.
			result, err := RenderValue(context.Background(), common.RawJSON(source), data, testLimits())
			require.Error(t, err)
			require.Nil(t, result)
			require.NotContains(t, err.Error(), "private-configuration")
		})
	}
}

func TestRenderValueLimitsAndCancellation(t *testing.T) {
	data, err := NewContext(Input{Params: map[string]any{
		"large": strings.Repeat("x", 60), "escaped": strings.Repeat("\n", 50),
	}}, testLimits())
	require.NoError(t, err)
	cases := []struct {
		name   string
		source string
		limits Limits
	}{
		{"source", `"` + strings.Repeat("x", 100) + `"`, Limits{MaxBytes: 100, MaxDepth: 32, MaxSteps: 10000}},
		{"injected output", `[{"$value":"params.large"},{"$value":"params.large"}]`, Limits{MaxBytes: 100, MaxDepth: 32, MaxSteps: 10000}},
		{"quoted output", `"{{params.escaped}}"`, Limits{MaxBytes: 100, MaxDepth: 32, MaxSteps: 10000}},
		{"depth", `[[[0]]]`, Limits{MaxBytes: 100, MaxDepth: 2, MaxSteps: 10000}},
		{"shared work", `["{{params.large}}","{{params.large}}"]`, Limits{MaxBytes: 1000, MaxDepth: 32, MaxSteps: 4}},
		{"invalid limits", `null`, Limits{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Expansion and quoting must fit final-output limits, not only source limits.
			result, err := RenderValue(context.Background(), common.RawJSON(tc.source), data, tc.limits)
			require.Error(t, err)
			require.Nil(t, result)
		})
	}

	// Cancellation also rejects empty/literal work before it can produce a body.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = RenderValue(ctx, common.RawJSON(`null`), data, testLimits())
	require.ErrorIs(t, err, context.Canceled)
}
