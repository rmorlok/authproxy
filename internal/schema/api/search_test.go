package api

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestNewSearchResourcesResponseJson(t *testing.T) {
	response := NewSearchResourcesResponseJson(nil, nil, []meta.Kind{"Actor"})
	require.Equal(t, meta.NewTypeMeta(SearchResultListKind), response.TypeMeta)
	require.Empty(t, response.Items)
	require.Empty(t, response.Metadata.TruncatedKinds)
	require.Equal(t, []meta.Kind{"Actor"}, response.Metadata.IncompleteKinds)

	jsonData, err := json.Marshal(response)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"apiVersion":"authproxy.net/v1alpha1",
		"kind":"SearchResultList",
		"metadata":{"truncatedKinds":[],"incompleteKinds":["Actor"]},
		"items":[]
	}`, string(jsonData))

	yamlData, err := yaml.Marshal(response)
	require.NoError(t, err)
	require.Contains(t, string(yamlData), "kind: SearchResultList")
}
