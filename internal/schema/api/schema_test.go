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

	_ = loadSchema(t, c, "../common/schema.json")
	_ = loadSchema(t, c, "../resources/namespace/schema.json")
	_ = loadSchema(t, c, "../resources/connectors/schema-oauth.json")
	_ = loadSchema(t, c, "../resources/connectors/schema.json")
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
		{name: "namespace", ref: "./schema.json#/$defs/Namespace", file: "valid-namespace.json"},
		{name: "create namespace", ref: "./schema.json#/$defs/CreateNamespaceRequest", file: "valid-create-namespace.json"},
		{name: "update namespace", ref: "./schema.json#/$defs/UpdateNamespaceRequest", file: "valid-update-namespace.json"},
		{name: "list namespaces", ref: "./schema.json#/$defs/ListNamespacesResponse", file: "valid-list-namespaces.json"},
		{name: "set namespace encryption key", ref: "./schema.json#/$defs/SetNamespaceEncryptionKeyRequest", file: "valid-set-namespace-encryption-key.json"},
		{name: "namespace encryption key", ref: "./schema.json#/$defs/NamespaceEncryptionKey", file: "valid-namespace-encryption-key.json"},
		{name: "actor", ref: "./schema.json#/$defs/Actor", file: "valid-actor.json"},
		{name: "create actor", ref: "./schema.json#/$defs/CreateActorRequest", file: "valid-create-actor.json"},
		{name: "update actor", ref: "./schema.json#/$defs/UpdateActorRequest", file: "valid-update-actor.json"},
		{name: "list actors", ref: "./schema.json#/$defs/ListActorsResponse", file: "valid-list-actors.json"},
		{name: "connector", ref: "./schema.json#/$defs/Connector", file: "valid-connector.json"},
		{name: "list connectors", ref: "./schema.json#/$defs/ListConnectorsResponse", file: "valid-list-connectors.json"},
		{name: "connector version", ref: "./schema.json#/$defs/ConnectorVersion", file: "valid-connector-version.json"},
		{name: "list connector versions", ref: "./schema.json#/$defs/ListConnectorVersionsResponse", file: "valid-list-connector-versions.json"},
		{name: "create connector", ref: "./schema.json#/$defs/CreateConnectorRequest", file: "valid-create-connector.json"},
		{name: "update connector", ref: "./schema.json#/$defs/UpdateConnectorRequest", file: "valid-update-connector.json"},
		{name: "create connector version", ref: "./schema.json#/$defs/CreateConnectorVersionRequest", file: "valid-create-connector-version.json"},
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
