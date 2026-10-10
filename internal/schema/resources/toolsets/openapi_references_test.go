package toolsets

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// openAPIReferencesForTest includes opaque JSON values and identities that do
// not imply HTTP fetching or local filesystem access.
func openAPIReferencesForTest() *OpenAPIReferenceConfig {
	return &OpenAPIReferenceConfig{
		BaseURI: util.ToPtr("https://example.test/spec/root.json"),
		Bundle: map[string]common.RawJSON{
			"urn:example:record":         common.RawJSON(`{"$id":"other.json","$self":"urn:provider:self","unknown":{"large":9007199254740993,"precise":1.2300}}`),
			"file:///schemas/allow.json": common.RawJSON(`true`),
			"urn:example:null":           common.RawJSON(`null`),
			"urn:example:array":          common.RawJSON(`[false,"opaque",4]`),
			"urn:example:string":         common.RawJSON(`"literal"`),
			"urn:example:number":         common.RawJSON(`18446744073709551615`),
		},
	}
}

// TestOpenAPIReferenceConfigValidation checks optional settings and JSON shape
// without interpreting a bundled document's version, dialect, or own identities.
func TestOpenAPIReferenceConfigValidation(t *testing.T) {
	for _, config := range []*OpenAPIReferenceConfig{nil, {}, {Bundle: map[string]common.RawJSON{}}, openAPIReferencesForTest()} {
		before := config.Clone()
		require.NoError(t, config.Validate(nil))
		require.Equal(t, before, config, "validation preserves authored content")
	}
	for _, raw := range []common.RawJSON{nil, {}, common.RawJSON(" "), common.RawJSON("{bad"), common.RawJSON("{} {}")} {
		config := &OpenAPIReferenceConfig{Bundle: map[string]common.RawJSON{"urn:example:invalid": raw}}
		require.ErrorContains(t, config.Validate(&common.ValidationContext{Path: "references"}), "references.bundle")
	}
}

// TestOpenAPIReferenceConfigIdentities applies the same literal absolute URI
// rules to the optional base and every bundle identity, independent of scheme.
func TestOpenAPIReferenceConfigIdentities(t *testing.T) {
	for _, test := range []struct {
		uri   string
		valid bool
	}{
		{"https://example.test/schema.json?revision=2", true},
		{"urn:example:schemas:record", true},
		{"file:///offline/schemas/record.json", true},
		{"https://example.test/%23literal", true},
		{"", false}, {"schema.json", false}, {"//example.test/schema.json", false},
		{"urn:example:record#", false}, {"https://example.test/schema#/part", false},
		{"https://reader:secret@example.test/schema", false},
		{" https://example.test/schema", false}, {"urn:example:with space", false},
		{"urn:example:record\n", false}, {"urn:example:\u00a0record", false},
		{"urn:example:\x7frecord", false}, {"urn:example:{{cfg.name}}", false},
		{"urn:example:record}}", false},
	} {
		t.Run(test.uri, func(t *testing.T) {
			for _, config := range []*OpenAPIReferenceConfig{
				{BaseURI: util.ToPtr(test.uri)},
				{Bundle: map[string]common.RawJSON{test.uri: common.RawJSON(`{}`)}},
			} {
				if test.valid {
					require.NoError(t, config.Validate(nil))
				} else {
					require.Error(t, config.Validate(nil))
				}
			}
		})
	}
}

// TestOpenAPIReferenceConfigRoundTrip preserves native bundle values and numeric
// tokens through both codecs. Omitted and empty bundles have the same meaning.
func TestOpenAPIReferenceConfigRoundTrip(t *testing.T) {
	for _, format := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{{"JSON", json.Marshal, util.DecodeJSONStrict}, {"YAML", yaml.Marshal, util.DecodeYAMLStrict}} {
		t.Run(format.name, func(t *testing.T) {
			original := openAPIReferencesForTest()
			encoded, err := format.encode(original)
			require.NoError(t, err)
			var decoded OpenAPIReferenceConfig
			require.NoError(t, format.decode(encoded, &decoded))
			require.NoError(t, decoded.Validate(nil))
			require.Equal(t, original.BaseURI, decoded.BaseURI)
			for identity, raw := range original.Bundle {
				require.JSONEq(t, string(raw), string(decoded.Bundle[identity]), identity)
			}
			require.Contains(t, string(decoded.Bundle["urn:example:record"]), "9007199254740993")
			require.Contains(t, string(decoded.Bundle["urn:example:record"]), "1.2300")
			require.Equal(t, common.RawJSON(`18446744073709551615`), decoded.Bundle["urn:example:number"])
			require.Equal(t, common.RawJSON(`null`), decoded.Bundle["urn:example:null"])
			encoded, err = format.encode(&OpenAPIReferenceConfig{Bundle: map[string]common.RawJSON{}})
			require.NoError(t, err)
			require.NoError(t, format.decode(encoded, &decoded))
			require.Nil(t, decoded.BaseURI)
			require.Empty(t, decoded.Bundle)
			require.NoError(t, decoded.Validate(nil))
		})
	}
}

// TestOpenAPIReferenceConfigStrictDecodeAndOwnership keeps authoring controls
// closed and rejects duplicate decoded bundle identities before maps lose them.
func TestOpenAPIReferenceConfigStrictDecodeAndOwnership(t *testing.T) {
	for _, format := range []struct {
		name   string
		decode func([]byte, any) error
	}{{"JSON", util.DecodeJSONStrict}, {"YAML", util.DecodeYAMLStrict}} {
		t.Run(format.name, func(t *testing.T) {
			for _, input := range []string{
				`[]`, `true`, `{"unknown":true}`, `{"baseURI":"urn:example:root"}`, `{"Bundle":{}}`,
				`{"baseUri":null}`, `{"baseUri":4}`, `{"bundle":null}`, `{"bundle":[]}`, `{"bundle":"text"}`,
				`{"baseUri":"urn:example:changed","bundle":{"urn:example:record":true,"urn:example:record":false}}`,
				`{"bundle":{"urn:example:record":true,"urn:example:\u0072ecord":false}}`,
			} {
				config := openAPIReferencesForTest()
				before := config.Clone()
				require.Error(t, format.decode([]byte(input), config), input)
				require.Equal(t, before, config, "failed decoding must be atomic")
			}
			config := openAPIReferencesForTest()
			require.NoError(t, format.decode([]byte(`{}`), config))
			require.Equal(t, &OpenAPIReferenceConfig{}, config)
		})
	}
	var config OpenAPIReferenceConfig
	require.Error(t, util.DecodeJSONStrict([]byte(`null`), &config))
	// yaml.v3 skips custom decoding at root null; an omitted configuration is
	// valid, while null fields nested in an authored object are rejected above.
	require.NoError(t, util.DecodeYAMLStrict([]byte(`null`), &config))
	require.NoError(t, config.Validate(nil))
}

// TestOpenAPIReferenceConfigCheckedFields prevents unchecked entries in an
// earlier bundle object from merging into the final authored bundle.
func TestOpenAPIReferenceConfigCheckedFields(t *testing.T) {
	var config OpenAPIReferenceConfig
	require.NoError(t, util.DecodeJSONStrict([]byte(`{"bundle":{"urn:discarded":null},"bundle":{"urn:retained":false}}`), &config))
	require.Equal(t, map[string]common.RawJSON{"urn:retained": common.RawJSON(`false`)}, config.Bundle)
}

// TestOpenAPISourceNullReferences rejects null at the owned configuration
// boundary, even though individual opaque bundle payloads may contain null.
func TestOpenAPISourceNullReferences(t *testing.T) {
	for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
		var source OpenAPISource
		require.Error(t, decode([]byte(`{"document":{"inline":{}},"references":null}`), &source))
	}
}

// TestOpenAPIReferenceConfigYAMLComposition checks aliases and merged controls
// without confusing an opaque null bundle value with a forbidden null bundle.
func TestOpenAPIReferenceConfigYAMLComposition(t *testing.T) {
	var config OpenAPIReferenceConfig
	require.NoError(t, util.DecodeYAMLStrict([]byte(`
<<: {baseUri: "urn:example:root"}
bundle:
  "urn:example:record": &record {type: object, example: 9007199254740993}
  "urn:example:alias": *record
  "urn:example:null": &nothing null
  "urn:example:alias-null": *nothing
`), &config))
	require.NoError(t, config.Validate(nil))
	require.Equal(t, config.Bundle["urn:example:record"], config.Bundle["urn:example:alias"])
	require.Equal(t, common.RawJSON(`null`), config.Bundle["urn:example:alias-null"])
	for _, input := range []string{"<<: {baseUri: null}", "<<: {bundle: null}", "baseUri: &none null\nbundle: *none"} {
		before := config.Clone()
		require.Error(t, util.DecodeYAMLStrict([]byte(input), &config))
		require.Equal(t, before, &config)
	}
}

// TestOpenAPIReferenceConfigClone detaches maps and raw bytes, retaining nil,
// empty, and malformed values so cloning cannot erase validation diagnostics.
func TestOpenAPIReferenceConfigClone(t *testing.T) {
	require.Nil(t, (*OpenAPIReferenceConfig)(nil).Clone())
	for _, bundle := range []map[string]common.RawJSON{nil, {}, {
		"urn:example:nil": nil, "urn:example:empty": {}, "urn:example:bad": common.RawJSON(" { malformed "),
	}} {
		config := &OpenAPIReferenceConfig{BaseURI: util.ToPtr("urn:example:root"), Bundle: bundle}
		clone := config.Clone()
		require.Equal(t, config, clone)
		*clone.BaseURI = "urn:example:changed"
		require.Equal(t, "urn:example:root", *config.BaseURI)
		if bundle != nil {
			clone.Bundle["urn:example:added"] = common.RawJSON(`true`)
			require.NotContains(t, config.Bundle, "urn:example:added")
			if raw := clone.Bundle["urn:example:bad"]; len(raw) > 0 {
				raw[0] = '['
				require.Equal(t, common.RawJSON(" { malformed "), config.Bundle["urn:example:bad"])
			}
		}
	}
}
