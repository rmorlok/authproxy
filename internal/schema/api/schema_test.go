package api

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

type schemaID struct {
	ID string `json:"$id"`
}

func loadSchema(t *testing.T, c *jsonschemav5.Compiler, path string) string {
	schemaBytes, err := os.ReadFile(path)
	require.NoError(t, err)

	var id schemaID
	require.NoError(t, json.Unmarshal(schemaBytes, &id))

	require.NoError(t, c.AddResource(id.ID, bytes.NewReader(schemaBytes)))

	return id.ID
}

func compileRefSchema(t *testing.T, ref string) *jsonschemav5.Schema {
	c := jsonschemav5.NewCompiler()

	_ = loadSchema(t, c, "../resources/namespace/schema.json")
	sid := loadSchema(t, c, "./schema.json")
	require.Equal(t, SchemaIdAPI, sid)

	const testSchemaID = "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/api/test.json"
	require.NoError(t, c.AddResource(testSchemaID, strings.NewReader(strings.TrimSpace(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/api/test.json",
  "$ref": "`+ref+`"
}`))))

	schema, err := c.Compile(testSchemaID)
	require.NoError(t, err)
	return schema
}

func TestSchemaSamples(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		file string
	}{
		{name: "initiate connection", ref: "./schema.json#/$defs/InitiateConnectionRequest", file: "valid-initiate-connection.json"},
		{name: "setup redirect", ref: "./schema.json#/$defs/ConnectionSetupRedirect", file: "valid-connection-setup-redirect.json"},
		{name: "setup form", ref: "./schema.json#/$defs/ConnectionSetupForm", file: "valid-connection-setup-form.json"},
		{name: "setup complete", ref: "./schema.json#/$defs/ConnectionSetupComplete", file: "valid-connection-setup-complete.json"},
		{name: "setup verifying", ref: "./schema.json#/$defs/ConnectionSetupVerifying", file: "valid-connection-setup-verifying.json"},
		{name: "setup error", ref: "./schema.json#/$defs/ConnectionSetupError", file: "valid-connection-setup-error.json"},
		{name: "submit connection", ref: "./schema.json#/$defs/SubmitConnectionRequest", file: "valid-submit-connection.json"},
		{name: "data source option", ref: "./schema.json#/$defs/DataSourceOption", file: "valid-data-source-option.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := compileRefSchema(t, tt.ref)

			data, err := os.ReadFile(filepath.Join("test_data", tt.file))
			require.NoError(t, err)

			var v any
			require.NoError(t, json.Unmarshal(data, &v))
			require.NoError(t, schema.Validate(v))
		})
	}
}

func TestInitiateConnectionRequestValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		req := InitiateConnectionRequest{
			ConnectorId:   "cxr_test0000000000001",
			IntoNamespace: "root.acme",
			ReturnToUrl:   "https://example.com/callback",
		}
		require.NoError(t, req.Validate())
		require.True(t, req.HasIntoNamespace())
		require.False(t, req.HasVersion())
	})

	t.Run("requires connector id", func(t *testing.T) {
		req := InitiateConnectionRequest{ReturnToUrl: "https://example.com/callback"}
		require.ErrorContains(t, req.Validate(), "connector_id is required")
	})

	t.Run("validates namespace", func(t *testing.T) {
		req := InitiateConnectionRequest{
			ConnectorId:   "cxr_test0000000000001",
			IntoNamespace: "not-rooted",
			ReturnToUrl:   "https://example.com/callback",
		}
		require.Error(t, req.Validate())
	})
}
