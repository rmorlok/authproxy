package connection

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

func TestConnectionSchema(t *testing.T) {
	compiler := jsonschemav5.NewCompiler()
	for _, path := range []string{
		"../../common/schema.json",
		"../meta/schema.json",
		"../namespace/schema.json",
		"./schema.json",
	} {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var envelope struct {
			ID string `json:"$id"`
		}
		require.NoError(t, json.Unmarshal(data, &envelope))
		require.NoError(t, compiler.AddResource(envelope.ID, bytes.NewReader(data)))
	}

	schema, err := compiler.Compile(SchemaIDConnection)
	require.NoError(t, err)

	valid := map[string]any{
		"apiVersion": "authproxy.net/v1alpha1",
		"kind":       "Connection",
		"metadata": map[string]any{
			"id":        "cxn_test0000000000001",
			"name":      "production",
			"namespace": "root.acme",
		},
		"spec": map[string]any{
			"connectorRef": map[string]any{
				"apiVersion": "authproxy.net/v1alpha1",
				"kind":       "Connector",
				"id":         "cxr_test0000000000001",
				"generation": 2,
			},
			"actorRef": map[string]any{
				"apiVersion": "authproxy.net/v1alpha1",
				"kind":       "Actor",
				"id":         "act_test0000000000001",
			},
		},
		"status": map[string]any{
			"lifecycle":               map[string]any{"state": "configured"},
			"health":                  map[string]any{"state": "healthy"},
			"configurationConfigured": true,
		},
	}
	require.NoError(t, schema.Validate(valid))

	legacy := map[string]any{
		"id":          "cxn_test0000000000001",
		"namespace":   "root.acme",
		"state":       "configured",
		"healthState": "healthy",
	}
	require.Error(t, schema.Validate(legacy))
}
