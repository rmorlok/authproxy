package toolsets

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// fetchedDocumentForTest describes an authenticated document fetch; its source
// connection is intentionally in a namespace unrelated to a target ToolSet.
func fetchedDocumentForTest() *OpenAPIDocument {
	address := "https://provider.example/openapi.json?version=next"
	ref := connection.NewConnectionReference(apid.ID("cxn_01example0000001"))
	ref.Namespace = "root.specification_accounts"
	return &OpenAPIDocument{URL: &address, FetchConnectionRef: &ref}
}

// TestOpenAPIDocumentShape checks acquisition exclusivity and opaque JSON-object
// shape without treating the document as a Tool input schema or parsing OpenAPI.
func TestOpenAPIDocumentShape(t *testing.T) {
	require.Error(t, (*OpenAPIDocument)(nil).Validate(nil))
	require.ErrorContains(t, (&OpenAPIDocument{}).Validate(nil), "exactly one")
	for _, raw := range []string{
		`{}`, ` {"openapi":"unrecognized-future-version","x-provider":{"anything":[null,true,4]}} `,
		`{"swagger":"2.0","$ref":"https://unreachable.invalid/external.json"}`,
		`{"components":{"schemas":{"Example":{"$schema":"vendor-dialect","type":"array"}}}}`,
	} {
		document := &OpenAPIDocument{Inline: common.RawJSON(raw)}
		require.NoError(t, document.Validate(nil), raw)
		require.Equal(t, raw, string(document.Inline), "validation must not rewrite document contents")
	}
	for _, raw := range []string{"", " ", "null", "true", "[]", `"text"`, "{malformed", "{} {}"} {
		document := &OpenAPIDocument{Inline: common.RawJSON(raw)}
		require.ErrorContains(t, document.Validate(&common.ValidationContext{Path: "source.openapi.document"}), "source.openapi.document.inline", raw)
	}
	document := fetchedDocumentForTest()
	document.Inline = common.RawJSON(`{}`)
	require.ErrorContains(t, document.Validate(nil), "exactly one")
	document.URL = nil
	require.ErrorContains(t, document.Validate(nil), "fetchConnectionRef")
}

// TestOpenAPIDocumentLiteralURL distinguishes acquisition addresses from runtime
// endpoint templates and does not reject valid queries or encoded delimiters.
func TestOpenAPIDocumentLiteralURL(t *testing.T) {
	for _, address := range []string{
		"https://example.test/openapi.json", "HTTP://example.test/spec.yaml",
		"https://[::1]:8443/spec", "https://example.test/@vendor/spec?version=next",
		"https://example.test/spec%23literal.json", "https://example.test/a path/spec",
	} {
		document := &OpenAPIDocument{URL: &address}
		require.NoError(t, document.Validate(nil), address)
		require.Equal(t, address, *document.URL)
	}
	for _, address := range []string{
		"", " ", "/openapi.json", "//example.test/spec", "https:/spec", "https://",
		"https://:443/spec", "file:///tmp/spec.json", "ftp://example.test/spec",
		"https://user:password@example.test/spec", "https://@example.test/spec",
		"https://example.test/spec#", "https://example.test/spec#/paths",
		" https://example.test/spec", "https://example.test/spec\n",
		"https://example.test/a\nb", "https://example.test/spec%zz", "https://example.test:bad/spec",
		"https://{{cfg.host}}/spec", "https://example.test/{{cfg.version}}/spec",
		"https://example.test/spec}}",
	} {
		document := &OpenAPIDocument{URL: &address}
		require.ErrorContains(t, document.Validate(nil), "url", address)
	}
	address := "https://user:private-password@example.test/spec?token=private-token"
	err := (&OpenAPIDocument{URL: &address}).Validate(nil)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "private-password")
	require.NotContains(t, err.Error(), "private-token")
}

// TestOpenAPIDocumentFetchReference validates canonical Connection references
// independently of ToolSet namespace ownership or execution connection binding.
func TestOpenAPIDocumentFetchReference(t *testing.T) {
	document := fetchedDocumentForTest()
	require.NoError(t, document.Validate(nil))
	document.FetchConnectionRef.ID = ""
	document.FetchConnectionRef.Name = "source-account"
	require.NoError(t, document.Validate(nil), "namespaced names are resolved by the importer")
	document.FetchConnectionRef.ID = "cxn_01example0000001"
	require.NoError(t, document.Validate(nil), "ID can coexist with namespace and name")
	for _, test := range []struct {
		field  string
		change func(*meta.ObjectReference)
	}{
		{"kind", func(r *meta.ObjectReference) { r.Kind = "Tool" }},
		{"apiVersion", func(r *meta.ObjectReference) { r.APIVersion = "authproxy.net/v2" }},
		{"id", func(r *meta.ObjectReference) { r.ID = "tls_01example0000001" }},
		{"namespace", func(r *meta.ObjectReference) { r.Namespace = "root.**" }},
		{"name", func(r *meta.ObjectReference) { r.Name = "bad name" }},
		{"generation", func(r *meta.ObjectReference) { r.Generation = 1 }},
		{"fetchConnectionRef", func(r *meta.ObjectReference) { r.ID = ""; r.Name = "" }},
	} {
		candidate := fetchedDocumentForTest()
		test.change(candidate.FetchConnectionRef)
		require.ErrorContains(t, candidate.Validate(nil), test.field)
	}
}

// TestOpenAPIDocumentStrictDecoding keeps authored controls closed while leaving
// inline provider content open, including keys that resemble control fields.
func TestOpenAPIDocumentStrictDecoding(t *testing.T) {
	for _, input := range []string{
		`{"inline":null}`, `{"url":null}`, `{"url":123}`, `{"fetchConnectionRef":null}`,
		`{"URL":"https://example.test/spec"}`, `{"baseUri":"https://example.test/spec"}`,
		`{"referenceBundle":{}}`, `{"fetchExternalReferences":true}`, `[]`,
		`{"fetchConnectionRef":{"generation":0}}`, `{"fetchConnectionRef":{"generation":null}}`,
		`{"fetchConnectionRef":{"Generation":0}}`, `{"fetchConnectionRef":{"id":null}}`,
		`{"fetchConnectionRef":{"apiVersion":null}}`, `{"fetchConnectionRef":{"namespace":null}}`,
		`{"fetchConnectionRef":{"name":null}}`, `{"fetchConnectionRef":{"kind":null}}`,
		`{"fetchConnectionRef":{"unknown":true}}`, `{"fetchConnectionRef":[]}`,
	} {
		for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
			var document OpenAPIDocument
			require.Error(t, decode([]byte(input), &document), input)
		}
	}
	var document OpenAPIDocument
	input := []byte(`{"inline":{"unknown":true,"generation":0,"url":null,"$ref":"https://unreachable.invalid/spec"}}`)
	require.NoError(t, util.DecodeJSONStrict(input, &document))
	require.NoError(t, document.Validate(nil))
	require.Error(t, util.DecodeJSONStrict([]byte(`null`), &document))
	require.Error(t, util.DecodeJSONStrict([]byte(`{} {}`), &document))
}

// TestOpenAPIDocumentRoundTrip retains large numeric document values rather than
// decoding them through float64 during shape validation or JSON serialization.
func TestOpenAPIDocumentRoundTrip(t *testing.T) {
	raw := common.RawJSON(`{"openapi":"3.1.0","x-large":9007199254740993,"x-unsigned":18446744073709551615,"components":{"schemas":{"Example":{"default":null}}}}`)
	for _, format := range []struct {
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{json.Marshal, util.DecodeJSONStrict}, {yaml.Marshal, util.DecodeYAMLStrict},
	} {
		for _, original := range []*OpenAPIDocument{{Inline: raw}, fetchedDocumentForTest()} {
			encoded, err := format.encode(original)
			require.NoError(t, err)
			var decoded OpenAPIDocument
			require.NoError(t, format.decode(encoded, &decoded))
			require.NoError(t, decoded.Validate(nil))
			if original.Inline != nil {
				require.Contains(t, string(decoded.Inline), "9007199254740993")
				require.Contains(t, string(decoded.Inline), "18446744073709551615")
			} else {
				require.Equal(t, original, &decoded)
			}
		}
	}
	// Native JSON can retain numeric syntax outside float64's finite range;
	// shape validation must not impose a floating-point representation on it.
	document := &OpenAPIDocument{Inline: common.RawJSON(`{"x-number":1e999}`)}
	require.NoError(t, document.Validate(nil))
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	require.True(t, bytes.Contains(encoded, []byte("1e999")))
}

// TestOpenAPIDocumentMarshalPreservesSourcePresence prevents invalid empty raw
// inline content from disappearing and silently selecting an accompanying URL.
func TestOpenAPIDocumentMarshalPreservesSourcePresence(t *testing.T) {
	for _, format := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{"JSON", json.Marshal, util.DecodeJSONStrict},
		{"YAML", yaml.Marshal, util.DecodeYAMLStrict},
	} {
		t.Run(format.name, func(t *testing.T) {
			document := fetchedDocumentForTest()
			document.Inline = common.RawJSON{}
			require.ErrorContains(t, document.Validate(nil), "exactly one")
			_, err := format.encode(document)
			require.Error(t, err, "an invalid inline source must not be omitted")
			require.NotNil(t, document.Inline)

			document.Inline = nil
			encoded, err := format.encode(document)
			require.NoError(t, err)
			var decoded OpenAPIDocument
			require.NoError(t, format.decode(encoded, &decoded))
			require.NoError(t, decoded.Validate(nil))
			require.Equal(t, document, &decoded)
		})
	}
}

// TestOpenAPIDocumentYAMLComposition shares strict JSON field checks after YAML
// aliases and merges resolve, including null and zero generation presence.
func TestOpenAPIDocumentYAMLComposition(t *testing.T) {
	var document OpenAPIDocument
	require.NoError(t, util.DecodeYAMLStrict([]byte("<<: {url: 'https://example.test/spec'}\nfetchConnectionRef:\n  apiVersion: authproxy.net/v1alpha1\n  kind: Connection\n  namespace: root.other\n  name: source\n"), &document))
	require.NoError(t, document.Validate(nil))
	for _, input := range []string{
		"<<: {inline: null}\n",
		"url: &missing null\nfetchConnectionRef: *missing\n",
		"fetchConnectionRef:\n  <<: {generation: 0}\n",
		"fetchConnectionRef:\n  id: &missing null\n  name: *missing\n",
	} {
		require.Error(t, util.DecodeYAMLStrict([]byte(input), &document), input)
	}
}

// TestOpenAPIDocumentCloneAndDecodeOwnership preserves raw source diagnostics and
// detaches every pointer without losing an explicitly invalid empty byte slice.
func TestOpenAPIDocumentCloneAndDecodeOwnership(t *testing.T) {
	original := fetchedDocumentForTest()
	original.Inline = common.RawJSON(" { malformed ")
	clone := original.Clone()
	require.Equal(t, original, clone)
	clone.Inline[0] = '['
	*clone.URL = "https://changed.test/spec"
	clone.FetchConnectionRef.Namespace = "root.changed"
	require.Equal(t, common.RawJSON(" { malformed "), original.Inline)
	require.Equal(t, *fetchedDocumentForTest().URL, *original.URL)
	require.Equal(t, "root.specification_accounts", original.FetchConnectionRef.Namespace)
	require.Nil(t, (*OpenAPIDocument)(nil).Clone())
	require.Nil(t, (&OpenAPIDocument{}).Clone().Inline)
	require.NotNil(t, (&OpenAPIDocument{Inline: common.RawJSON{}}).Clone().Inline)

	document := fetchedDocumentForTest()
	require.Error(t, util.DecodeJSONStrict([]byte(`{"url":"https://changed.test/spec","fetchConnectionRef":{"generation":0}}`), document))
	require.Equal(t, fetchedDocumentForTest(), document)
	require.NoError(t, util.DecodeJSONStrict([]byte(`{"inline":{}}`), document))
	require.Nil(t, document.URL)
	require.Nil(t, document.FetchConnectionRef)
	require.NoError(t, document.Validate(nil))
}
