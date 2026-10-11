package toolsets

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// openAPIReferenceFetchForTest uses an acquisition connection outside the
// ToolSet namespace, just as an entry document's authenticated fetch can.
func openAPIReferenceFetchForTest() OpenAPIReferenceFetch {
	document := fetchedDocumentForTest()
	return OpenAPIReferenceFetch{URL: *document.URL, FetchConnectionRef: document.FetchConnectionRef}
}

// TestOpenAPIReferenceFetchValidation applies acquisition rules rather than the
// broader offline URI-identity rules, without resolving connection references.
func TestOpenAPIReferenceFetchValidation(t *testing.T) {
	require.Error(t, (*OpenAPIReferenceFetch)(nil).Validate(nil))
	fetch := openAPIReferenceFetchForTest()
	require.NoError(t, fetch.Validate(nil))
	fetch.FetchConnectionRef.ID = ""
	fetch.FetchConnectionRef.Name = "specification-account"
	require.NoError(t, fetch.Validate(nil))
	fetch.FetchConnectionRef = nil
	require.NoError(t, fetch.Validate(nil), "authentication is optional")
	for _, address := range []string{"", "schema.json", "file:///schemas/item.json", "https://reader:secret@example.test/schema", "https://example.test/schema#", "https://{{cfg.host}}/schema"} {
		fetch.URL = address
		require.ErrorContains(t, fetch.Validate(nil), "url", address)
	}
	for _, change := range []func(*meta.ObjectReference){
		func(ref *meta.ObjectReference) { ref.Kind = "Tool" },
		func(ref *meta.ObjectReference) { ref.ID = "tls_01example0000001" },
		func(ref *meta.ObjectReference) { ref.Generation = 1 },
		func(ref *meta.ObjectReference) { ref.Namespace = "root.**" },
	} {
		fetch := openAPIReferenceFetchForTest()
		change(fetch.FetchConnectionRef)
		require.ErrorContains(t, fetch.Validate(nil), "fetchConnectionRef")
	}
}

// TestOpenAPIReferenceFetchCollisions rejects ambiguous exact acquisition
// sources without exposing private query values in collision diagnostics.
func TestOpenAPIReferenceFetchCollisions(t *testing.T) {
	address := "https://example.test/schema?token=private-value"
	for _, test := range []struct {
		name, message string
		config        OpenAPIReferenceConfig
	}{
		{"duplicate", "must not be repeated", OpenAPIReferenceConfig{Fetch: []OpenAPIReferenceFetch{
			{URL: address}, {URL: address, FetchConnectionRef: fetchedDocumentForTest().FetchConnectionRef},
		}}},
		{"bundle overlap", "must not also appear in bundle", OpenAPIReferenceConfig{
			Bundle: map[string]common.RawJSON{address: common.RawJSON(`true`)},
			Fetch:  []OpenAPIReferenceFetch{{URL: address}},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := test.config.Clone()
			err := test.config.Validate(&common.ValidationContext{Path: "references"})
			require.ErrorContains(t, err, test.message)
			require.Contains(t, err.Error(), "references.fetch")
			require.NotContains(t, err.Error(), "private-value")
			require.Equal(t, before, &test.config)
		})
	}
}

// TestOpenAPIReferenceFetchRoundTrip preserves URL spellings and independent
// authentication; normalized URI collisions remain the importer's concern.
func TestOpenAPIReferenceFetchRoundTrip(t *testing.T) {
	first, second := openAPIReferenceFetchForTest(), openAPIReferenceFetchForTest()
	first.URL, second.URL = "HTTPS://EXAMPLE.TEST/Item?name=Item", "https://example.test/Item?name=Item"
	second.FetchConnectionRef.ID = "cxn_01example0000002"
	original := &OpenAPIReferenceConfig{Fetch: []OpenAPIReferenceFetch{
		first, second, {URL: "https://example.test/item?name=Item"}, {URL: "https://example.test/item?name=item"},
	}}
	for _, format := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{{"JSON", json.Marshal, util.DecodeJSONStrict}, {"YAML", yaml.Marshal, util.DecodeYAMLStrict}} {
		t.Run(format.name, func(t *testing.T) {
			encoded, err := format.encode(original)
			require.NoError(t, err)
			var decoded OpenAPIReferenceConfig
			require.NoError(t, format.decode(encoded, &decoded))
			require.NoError(t, decoded.Validate(nil))
			require.Equal(t, original, &decoded)
			for _, input := range []string{`{}`, `{"fetch":[]}`} {
				require.NoError(t, format.decode([]byte(input), &decoded))
				require.Empty(t, decoded.Fetch)
				require.NoError(t, decoded.Validate(nil))
			}
		})
	}
}

// TestOpenAPIReferenceFetchStrictDecoding rejects erased presence and unknown
// controls, preserving both a fetch entry and its containing config on failure.
func TestOpenAPIReferenceFetchStrictDecoding(t *testing.T) {
	for _, format := range []struct {
		name   string
		decode func([]byte, any) error
	}{{"JSON", util.DecodeJSONStrict}, {"YAML", util.DecodeYAMLStrict}} {
		t.Run(format.name, func(t *testing.T) {
			for _, input := range []string{
				`{"fetch":null}`, `{"fetch":{}}`, `{"Fetch":[]}`, `{"fetch":[null]}`,
				`{"fetch":[[]]}`, `{"fetch":[{"url":null}]}`, `{"fetch":[{"url":4}]}`,
				`{"fetch":[{"URL":"https://example.test/schema"}]}`, `{"fetch":[{"unknown":true}]}`,
				`{"fetch":[{"fetchConnectionRef":null}]}`, `{"fetch":[{"fetchConnectionRef":{"generation":0}}]}`,
				`{"fetch":[{"fetchConnectionRef":{"generation":null}}]}`, `{"fetch":[{"fetchConnectionRef":{"Generation":0}}]}`,
				`{"fetch":[{"fetchConnectionRef":{"id":null}}]}`, `{"fetch":[{"fetchConnectionRef":{"unknown":true}}]}`,
			} {
				config := &OpenAPIReferenceConfig{Fetch: []OpenAPIReferenceFetch{openAPIReferenceFetchForTest()}}
				before := config.Clone()
				require.Error(t, format.decode([]byte(input), config), input)
				require.Equal(t, before, config, "failed decoding must be atomic")
			}
			fetch := openAPIReferenceFetchForTest()
			before := openAPIReferenceFetchForTest()
			require.Error(t, format.decode([]byte(`{"url":"https://changed.test/schema","fetchConnectionRef":{"id":null}}`), &fetch))
			require.Equal(t, before, fetch)
			require.NoError(t, format.decode([]byte(`{"url":"https://changed.test/schema"}`), &fetch))
			require.Nil(t, fetch.FetchConnectionRef, "replacement must clear old authentication")
			require.NoError(t, fetch.Validate(nil))
			var missingURL OpenAPIReferenceFetch
			require.NoError(t, format.decode([]byte(`{}`), &missingURL))
			require.ErrorContains(t, missingURL.Validate(nil), "url")
		})
	}
}

// TestOpenAPIReferenceFetchYAMLComposition supports ordinary YAML composition
// while still rejecting nulls supplied through aliases and merged objects.
func TestOpenAPIReferenceFetchYAMLComposition(t *testing.T) {
	var config OpenAPIReferenceConfig
	require.NoError(t, util.DecodeYAMLStrict([]byte(`
<<:
  fetch:
    - &authenticated
      url: https://example.test/item
      fetchConnectionRef: {apiVersion: authproxy.net/v1alpha1, kind: Connection, id: cxn_01example0000001}
    - <<: *authenticated
      url: https://example.test/other
`), &config))
	require.NoError(t, config.Validate(nil))
	require.Len(t, config.Fetch, 2)
	require.Equal(t, config.Fetch[0].FetchConnectionRef, config.Fetch[1].FetchConnectionRef)
	require.NotEqual(t, config.Fetch[0].URL, config.Fetch[1].URL)
	for _, input := range []string{
		"<<: {fetch: null}", "fetch: [&nothing null, *nothing]",
		"fetch: [{url: &nothing null, fetchConnectionRef: *nothing}]",
		"fetch: [{<<: {fetchConnectionRef: {generation: null}}}]",
	} {
		before := config.Clone()
		require.Error(t, util.DecodeYAMLStrict([]byte(input), &config), input)
		require.Equal(t, before, &config)
	}
}

// TestOpenAPIReferenceFetchCheckedFields prevents an earlier connection object
// from merging unchecked fields into the final JSON acquisition settings.
func TestOpenAPIReferenceFetchCheckedFields(t *testing.T) {
	var config OpenAPIReferenceConfig
	require.NoError(t, util.DecodeJSONStrict([]byte(`{"fetch":[null],"fetch":[{"url":"https://example.test/schema","fetchConnectionRef":{"generation":1,"namespace":"root.discarded"},"fetchConnectionRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","id":"cxn_01example0000001"}}]}`), &config))
	require.NoError(t, config.Validate(nil))
	require.Len(t, config.Fetch, 1)
	require.Empty(t, config.Fetch[0].FetchConnectionRef.Namespace)
	require.Zero(t, config.Fetch[0].FetchConnectionRef.Generation)
}

// TestOpenAPIReferenceFetchClone detaches acquisition settings while preserving
// nil, empty, and invalid states for subsequent validation diagnostics.
func TestOpenAPIReferenceFetchClone(t *testing.T) {
	for _, fetches := range [][]OpenAPIReferenceFetch{nil, {}, {{}}} {
		original := &OpenAPIReferenceConfig{Fetch: fetches}
		require.Equal(t, original, original.Clone())
	}
	original := &OpenAPIReferenceConfig{Fetch: []OpenAPIReferenceFetch{openAPIReferenceFetchForTest(), {URL: "https://example.test/public"}}}
	clone := original.Clone()
	require.Equal(t, original, clone)
	clone.Fetch[0].URL = "https://changed.test/schema"
	clone.Fetch[0].FetchConnectionRef.Namespace = "root.changed"
	clone.Fetch[1].FetchConnectionRef = fetchedDocumentForTest().FetchConnectionRef
	require.Equal(t, openAPIReferenceFetchForTest(), original.Fetch[0])
	require.Nil(t, original.Fetch[1].FetchConnectionRef)
}
