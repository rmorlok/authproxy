package httpcodec

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

func TestEncodeParametersScalarAndRepeatedValues(t *testing.T) {
	// Authored numbers retain their precision and spelling; arrays repeat keys.
	fields := map[string]common.RawJSON{
		"text":   common.RawJSON(`"Ada & Bob=100% + ☃"`),
		"bool":   common.RawJSON(`false`),
		"number": common.RawJSON(`9007199254740993.0100`),
		"exp":    common.RawJSON(`1e+20`),
		"zero":   common.RawJSON(`-0`),
		"null":   common.RawJSON(`null`),
		"empty":  common.RawJSON(`""`),
		"repeat": common.RawJSON(`["first",true,42,null,""]`),
		"omit":   common.RawJSON(`[]`),
	}
	encoded, err := EncodeParameters(context.Background(), fields, 4096)
	require.NoError(t, err)
	require.Equal(t, "bool=false&empty=&exp=1e%2B20&null=null&number=9007199254740993.0100&repeat=first&repeat=true&repeat=42&repeat=null&repeat=&text=Ada+%26+Bob%3D100%25+%2B+%E2%98%83&zero=-0", encoded)

	// The same bytes round-trip as a query and as an ordinary URL-encoded form.
	query, err := url.ParseQuery(encoded)
	require.NoError(t, err)
	require.Equal(t, []string{"first", "true", "42", "null", ""}, query["repeat"])
	require.Equal(t, "Ada & Bob=100% + ☃", query.Get("text"))
	require.NotContains(t, query, "omit")
}

func TestEncodeParametersLiteralNamesAndInertData(t *testing.T) {
	// Both names and values are literal data, including template-like text.
	encoded, err := EncodeParameters(context.Background(), map[string]common.RawJSON{
		"":                      common.RawJSON(`""`),
		"$value":                common.RawJSON(`"{{cfg.secret}}"`),
		"a&injected=true +%☃\n": common.RawJSON(`"%2F&admin=true\r\n"`),
	}, 4096)
	require.NoError(t, err)
	parsed, err := url.ParseQuery(encoded)
	require.NoError(t, err)
	require.Equal(t, url.Values{
		"":                      {""},
		"$value":                {"{{cfg.secret}}"},
		"a&injected=true +%☃\n": {"%2F&admin=true\r\n"},
	}, parsed)
	require.Contains(t, encoded, "%252F") // A supplied percent escape remains literal.
	require.NotContains(t, encoded, "\r")
	require.NotContains(t, encoded, "\n")
}

func TestEncodeParametersRejectsInvalidValues(t *testing.T) {
	for name, raw := range map[string]string{
		"object":          `{"secret":"private-config"}`,
		"directive":       `{"$value":"cfg.secret"}`,
		"literal":         `{"$literal":"value"}`,
		"nested array":    `[["private-config"]]`,
		"repeated object": `["okay",{"secret":"private-config"}]`,
		"empty JSON":      ``,
		"broken JSON":     `"private-config`,
		"two values":      `1,2`,
		"trailing data":   `true false`,
		"invalid UTF-8":   string([]byte{'"', 0xff, '"'}),
	} {
		t.Run(name, func(t *testing.T) {
			// Invalid fields fail the entire encoding and never expose their values.
			encoded, err := EncodeParameters(context.Background(), map[string]common.RawJSON{
				"a": common.RawJSON(`"valid"`), "z": common.RawJSON(raw),
			}, 4096)
			require.Error(t, err)
			require.Empty(t, encoded)
			require.NotContains(t, err.Error(), "private-config")
		})
	}

	// Invalid names are also rejected instead of silently replacing bad bytes.
	encoded, err := EncodeParameters(context.Background(), map[string]common.RawJSON{
		string([]byte{0xff}): common.RawJSON(`true`),
	}, 4096)
	require.Error(t, err)
	require.Empty(t, encoded)
}

func TestEncodeParametersLimits(t *testing.T) {
	cases := []struct {
		name   string
		fields map[string]common.RawJSON
		limit  int
		want   string
		fail   string
	}{
		{"no fields", nil, 2, "", ""},
		{"zero limit", nil, 0, "", "positive"},
		{"negative limit", nil, -1, "", "positive"},
		{"array framing", nil, 1, "", "source byte limit"},
		{"many fields", map[string]common.RawJSON{"a": {}, "b": {}, "c": {}}, 2, "", "source byte limit"},
		{"key size", map[string]common.RawJSON{"long": common.RawJSON(`0`)}, 5, "", "source byte limit"},
		{"value size", map[string]common.RawJSON{"x": common.RawJSON(`123`)}, 5, "", "source byte limit"},
		{"aggregate source", map[string]common.RawJSON{"a": common.RawJSON(`0`), "b": common.RawJSON(`0`)}, 6, "", "source byte limit"},
		{"source exact", map[string]common.RawJSON{"x": common.RawJSON(`"ab"`)}, 7, "x=ab", ""},
		{"source one over", map[string]common.RawJSON{"x": common.RawJSON(`"ab"`)}, 6, "", "source byte limit"},
		{"escape exact", map[string]common.RawJSON{"x": common.RawJSON(`"&&&"`)}, 11, "x=%26%26%26", ""},
		{"escape one over", map[string]common.RawJSON{"x": common.RawJSON(`"&&&"`)}, 10, "", "encoded parameter byte limit"},
		{"escaped key", map[string]common.RawJSON{"&&&": common.RawJSON(`""`)}, 9, "", "encoded parameter byte limit"},
		{"repeated key", map[string]common.RawJSON{"abcdef": common.RawJSON(`[0,0,0]`)}, 15, "", "encoded parameter byte limit"},
		{"separator exact", map[string]common.RawJSON{"a": common.RawJSON(`"&&"`), "b": common.RawJSON(`"&&"`)}, 17, "a=%26%26&b=%26%26", ""},
		{"separator one over", map[string]common.RawJSON{"a": common.RawJSON(`"&&"`), "b": common.RawJSON(`"&&"`)}, 16, "", "encoded parameter byte limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Source and encoded output have separate aggregate checks at the same cap.
			encoded, err := EncodeParameters(context.Background(), tc.fields, tc.limit)
			if tc.fail != "" {
				require.ErrorContains(t, err, tc.fail)
				require.Empty(t, encoded)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, encoded)
		})
	}
}

func TestEncodeParametersCanceled(t *testing.T) {
	// Cancellation is preserved for callers to classify; no partial data escapes.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	encoded, err := EncodeParameters(ctx, map[string]common.RawJSON{"x": common.RawJSON(`1`)}, 4096)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, encoded)

	// A missing context is a caller error rather than a panic.
	encoded, err = EncodeParameters(nil, nil, 4096)
	require.Error(t, err)
	require.Empty(t, encoded)
}

func FuzzEncodeParameters(f *testing.F) {
	f.Add("field", `"value"`)
	f.Add("a&b", `["+","%",null,false,9007199254740993]`)
	f.Add("nested", `[[1]]`)
	f.Add("$value", `{"$value":"cfg.secret"}`)
	f.Fuzz(func(t *testing.T, name, raw string) {
		// Every successful output stays bounded and parses without injecting keys.
		const limit = 4096
		encoded, err := EncodeParameters(context.Background(), map[string]common.RawJSON{name: common.RawJSON(raw)}, limit)
		if err != nil {
			require.Empty(t, encoded)
			return
		}
		require.LessOrEqual(t, len(encoded), limit)
		values, err := url.ParseQuery(encoded)
		require.NoError(t, err)
		for key := range values {
			require.Equal(t, name, key)
		}
		var input any
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		require.NoError(t, decoder.Decode(&input))
		if repeated, ok := input.([]any); ok {
			require.Len(t, values[name], len(repeated))
		} else {
			require.Len(t, values[name], 1)
		}
	})
}
