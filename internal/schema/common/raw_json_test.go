package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestRawJSONYAMLRoundTrip retains structured schema objects in YAML exports.
func TestRawJSONYAMLRoundTrip(t *testing.T) {
	original := RawJSON(`{"type":"object","properties":{"tenant":{"type":"string"}}}`)

	encoded, err := yaml.Marshal(original)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "type: object")
	require.Contains(t, string(encoded), "tenant:")

	var decoded RawJSON
	require.NoError(t, yaml.Unmarshal(encoded, &decoded))
	require.JSONEq(t, string(original), string(decoded))
}

// TestRawJSONMarshalYAMLRejectsInvalidJSON rejects malformed input and trailing
// content rather than exporting a valid-looking prefix of the authored value.
func TestRawJSONMarshalYAMLRejectsInvalidJSON(t *testing.T) {
	for _, input := range []string{``, `{invalid`, `{} true`, `{} invalid`} {
		t.Run(input, func(t *testing.T) {
			_, err := yaml.Marshal(RawJSON(input))
			require.ErrorContains(t, err, "invalid raw JSON")
		})
	}
}

// TestRawJSONNilMarshalYAML preserves the shared nil-as-null contract.
func TestRawJSONNilMarshalYAML(t *testing.T) {
	encoded, err := yaml.Marshal(RawJSON(nil))
	require.NoError(t, err)
	require.Equal(t, "null\n", string(encoded))

	encodedJSON, err := json.Marshal(RawJSON(nil))
	require.NoError(t, err)
	require.JSONEq(t, "null", string(encodedJSON))
}

// TestRawJSONMarshalYAMLPreservesNumbersAndScalarTypes prevents export from
// silently rounding JSON numbers or turning quoted literals into YAML scalars.
func TestRawJSONMarshalYAMLPreservesNumbersAndScalarTypes(t *testing.T) {
	original := RawJSON(`{"integer":9007199254740993,"unsigned":18446744073709551615,"decimal":0.1234567890123456789,"text":"null","booleanText":"true","numberText":"123","items":[false,null]}`)
	encoded, err := yaml.Marshal(original)
	require.NoError(t, err)
	var document yaml.Node
	require.NoError(t, yaml.Unmarshal(encoded, &document))
	root := document.Content[0]
	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(root.Content); i += 2 {
		fields[root.Content[i].Value] = root.Content[i+1]
	}
	require.Equal(t, "9007199254740993", fields["integer"].Value)
	require.Equal(t, "18446744073709551615", fields["unsigned"].Value)
	require.Equal(t, "0.1234567890123456789", fields["decimal"].Value)
	for _, key := range []string{"text", "booleanText", "numberText"} {
		require.Equal(t, "!!str", fields[key].Tag)
	}
	require.Equal(t, "!!bool", fields["items"].Content[0].Tag)
	require.Equal(t, "!!null", fields["items"].Content[1].Tag)
}

// TestRawJSONMarshalYAMLPreservesOverflowExponent verifies YAML emission, not
// arbitrary-precision import, when a JSON number exceeds native float64 range.
func TestRawJSONMarshalYAMLPreservesOverflowExponent(t *testing.T) {
	encoded, err := yaml.Marshal(RawJSON(`{"value":1e10000}`))
	require.NoError(t, err)
	require.Contains(t, string(encoded), "!!float 1e10000")
	var document yaml.Node
	require.NoError(t, yaml.Unmarshal(encoded, &document))
	value := document.Content[0].Content[1]
	require.Equal(t, "!!float", value.Tag)
	require.Equal(t, "1e10000", value.Value)
}

// TestRawJSONMarshalYAMLOrdersObjects keeps nested object output stable even
// when the original JSON supplies its keys in a different order.
func TestRawJSONMarshalYAMLOrdersObjects(t *testing.T) {
	first, err := yaml.Marshal(RawJSON(`{"z":1,"a":{"z":2,"a":3}}`))
	require.NoError(t, err)
	second, err := yaml.Marshal(RawJSON(`{"a":{"a":3,"z":2},"z":1}`))
	require.NoError(t, err)
	require.Equal(t, string(first), string(second))
	require.Equal(t, "a:\n    a: 3\n    z: 2\nz: 1\n", string(first))
}

// TestRawJSONUnmarshalYAMLStringifiesScalarKeys accepts unquoted numeric keys,
// such as OpenAPI response codes, which yaml.v3 decodes as non-string keys.
func TestRawJSONUnmarshalYAMLStringifiesScalarKeys(t *testing.T) {
	var decoded RawJSON
	require.NoError(t, yaml.Unmarshal([]byte("responses:\n  200:\n    description: ok\n  true: yes\n"), &decoded))
	require.JSONEq(t, `{"responses":{"200":{"description":"ok"},"true":"yes"}}`, string(decoded))
}

// TestRawJSONUnmarshalYAMLPreservesNumberText keeps JSON-compatible numbers as
// authored instead of rounding through float64 or dropping trailing zeros.
func TestRawJSONUnmarshalYAMLPreservesNumberText(t *testing.T) {
	var decoded RawJSON
	require.NoError(t, yaml.Unmarshal([]byte("big: 12345678901234567890123\nversion: 1.0\nneg: -9007199254740993\nhex: 0x1F\n"), &decoded))
	require.Equal(t, `{"big":12345678901234567890123,"hex":31,"neg":-9007199254740993,"version":1.0}`, string(decoded))
}

// TestRawJSONUnmarshalYAMLMergeKeys applies merges with explicit keys winning
// and still rejects keys that collide once converted to JSON object keys.
func TestRawJSONUnmarshalYAMLMergeKeys(t *testing.T) {
	var decoded RawJSON
	require.NoError(t, yaml.Unmarshal([]byte("base: &base {a: 1, b: 2}\nother: &other {b: 3, c: 4}\nvalue:\n  <<: [*base, *other]\n  a: 5\n"), &decoded))
	require.JSONEq(t, `{"base":{"a":1,"b":2},"other":{"b":3,"c":4},"value":{"a":5,"b":2,"c":4}}`, string(decoded))

	require.ErrorContains(t, yaml.Unmarshal([]byte("1: a\n\"1\": b\n"), &decoded), "already defined")
}
