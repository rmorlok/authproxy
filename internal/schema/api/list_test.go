package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestManagedListConstructorsUseV1Alpha1Envelope(t *testing.T) {
	tests := []struct {
		name         string
		expectedKind string
		value        any
	}{
		{name: "actors", expectedKind: "ActorList", value: NewListActorsResponseJson(nil, "next")},
		{name: "namespaces", expectedKind: "NamespaceList", value: NewListNamespacesResponseJson(nil, "next")},
		{name: "connectors", expectedKind: "ConnectorList", value: NewListConnectorsResponseJson(nil, "next")},
		{name: "connector generations", expectedKind: "ConnectorList", value: NewListConnectorVersionsResponseJson(nil, "next")},
		{name: "connections", expectedKind: "ConnectionList", value: NewListConnectionResponseJson(nil, "next")},
		{name: "keys", expectedKind: "KeyList", value: NewListKeysResponseJson(nil, "next")},
		{name: "rate limits", expectedKind: "RateLimitList", value: NewListRateLimitsResponseJson(nil, "next")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, err := json.Marshal(tt.value)
			require.NoError(t, err)
			var jsonValue map[string]any
			require.NoError(t, json.Unmarshal(jsonData, &jsonValue))
			require.Equal(t, "authproxy.net/v1alpha1", jsonValue["apiVersion"])
			require.Equal(t, tt.expectedKind, jsonValue["kind"])
			require.Equal(t, map[string]any{"continue": "next"}, jsonValue["metadata"])
			require.Equal(t, []any{}, jsonValue["items"])
			require.NotContains(t, jsonValue, "cursor")

			yamlData, err := yaml.Marshal(tt.value)
			require.NoError(t, err)
			var yamlValue map[string]any
			require.NoError(t, yaml.Unmarshal(yamlData, &yamlValue))
			require.Equal(t, "authproxy.net/v1alpha1", yamlValue["apiVersion"])
			require.Equal(t, tt.expectedKind, yamlValue["kind"])
			require.Equal(t, map[string]any{"continue": "next"}, yamlValue["metadata"])
			require.Equal(t, []any{}, yamlValue["items"])
		})
	}
}
