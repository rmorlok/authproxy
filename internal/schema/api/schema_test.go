package api

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
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
	_ = loadSchema(t, c, "../auth/schema.json")
	_ = loadSchema(t, c, "../config/schema.json")
	_ = loadSchema(t, c, "../resources/meta/schema.json")
	_ = loadSchema(t, c, "../resources/namespace/schema.json")
	_ = loadSchema(t, c, "../resources/connectors/schema-oauth.json")
	_ = loadSchema(t, c, "../resources/connectors/schema.json")
	_ = loadSchema(t, c, "../resources/key/schema.json")
	_ = loadSchema(t, c, "../resources/actor/schema.json")
	_ = loadSchema(t, c, "../resources/connection/schema.json")
	_ = loadSchema(t, c, "../resources/rate_limit/schema.json")
	_ = loadSchema(t, c, "./v1alpha1/schema.json")
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
		{name: "error response", ref: "./schema.json#/$defs/ErrorResponse", file: "valid-error-response.json"},
		{name: "initiate connection", ref: "./schema.json#/$defs/ConnectionInitiateAction", file: "valid-initiate-connection.json"},
		{name: "setup redirect", ref: "./schema.json#/$defs/ConnectionSetupAction", file: "valid-connection-setup-redirect.json"},
		{name: "setup form", ref: "./schema.json#/$defs/ConnectionSetupAction", file: "valid-connection-setup-form.json"},
		{name: "setup complete", ref: "./schema.json#/$defs/ConnectionSetupAction", file: "valid-connection-setup-complete.json"},
		{name: "setup verifying", ref: "./schema.json#/$defs/ConnectionSetupAction", file: "valid-connection-setup-verifying.json"},
		{name: "setup error", ref: "./schema.json#/$defs/ConnectionSetupAction", file: "valid-connection-setup-error.json"},
		{name: "submit connection", ref: "./schema.json#/$defs/ConnectionSetupSubmitAction", file: "valid-submit-connection.json"},
		{name: "data source option", ref: "./schema.json#/$defs/DataSourceOption", file: "valid-data-source-option.json"},
		{name: "connection", ref: "./schema.json#/$defs/Connection", file: "valid-connection.json"},
		{name: "list connections", ref: "./schema.json#/$defs/ListConnectionResponse", file: "valid-list-connections.json"},
		{name: "disconnect connection request", ref: "./schema.json#/$defs/ConnectionDisconnectActionRequest", file: "valid-disconnect-connection-request.json"},
		{name: "disconnect response", ref: "./schema.json#/$defs/ConnectionDisconnectActionResponse", file: "valid-disconnect-response.json"},
		{name: "migrate connection version request", ref: "./schema.json#/$defs/ConnectionVersionMigrationActionRequest", file: "valid-migrate-connection-version-request.json"},
		{name: "migrate connection version response", ref: "./schema.json#/$defs/ConnectionVersionMigrationActionResponse", file: "valid-migrate-connection-version-response.json"},
		{name: "force connection state", ref: "./schema.json#/$defs/ConnectionForceStateActionRequest", file: "valid-force-connection-state.json"},
		{name: "update connection", ref: "./schema.json#/$defs/ConnectionPatch", file: "valid-update-connection.json"},
		{name: "proxy response", ref: "./schema.json#/$defs/ProxyResponse", file: "valid-proxy-response.json"},
		{name: "list notifications", ref: "./schema.json#/$defs/ListNotificationsResponse", file: "valid-list-notifications.json"},
		{name: "search resources", ref: "./schema.json#/$defs/SearchResourcesResponse", file: "valid-search-resources.json"},
		{name: "list namespaces", ref: "./schema.json#/$defs/ListNamespacesResponse", file: "valid-list-namespaces.json"},
		{name: "actor", ref: "./schema.json#/$defs/Actor", file: "valid-actor.json"},
		{name: "create actor", ref: "./schema.json#/$defs/CreateActorRequest", file: "valid-create-actor.json"},
		{name: "update actor", ref: "./schema.json#/$defs/UpdateActorRequest", file: "valid-update-actor.json"},
		{name: "list actors", ref: "./schema.json#/$defs/ListActorsResponse", file: "valid-list-actors.json"},
		{name: "metrics query", ref: "./schema.json#/$defs/MetricsQueryRequest", file: "valid-metrics-query.json"},
		{name: "metrics schema", ref: "./schema.json#/$defs/MetricsSchemaResponse", file: "valid-metrics-schema.json"},
		{name: "connector", ref: "./schema.json#/$defs/Connector", file: "valid-connector.json"},
		{name: "list connectors", ref: "./schema.json#/$defs/ListConnectorsResponse", file: "valid-list-connectors.json"},
		{name: "connector generation", ref: "./schema.json#/$defs/Connector", file: "valid-connector-version.json"},
		{name: "list connector versions", ref: "./schema.json#/$defs/ListConnectorVersionsResponse", file: "valid-list-connector-versions.json"},
		{name: "create connector", ref: "./schema.json#/$defs/CreateConnectorRequest", file: "valid-create-connector.json"},
		{name: "update connector", ref: "./schema.json#/$defs/UpdateConnectorRequest", file: "valid-update-connector.json"},
		{name: "create connector version", ref: "./schema.json#/$defs/CreateConnectorVersionRequest", file: "valid-create-connector-version.json"},
		{name: "update connector version", ref: "./schema.json#/$defs/UpdateConnectorVersionRequest", file: "valid-update-connector.json"},
		{name: "connector lifecycle request", ref: "./schema.json#/$defs/ConnectorLifecycleRequest", file: "valid-connector-lifecycle-request.json"},
		{name: "connector lifecycle response", ref: "./schema.json#/$defs/ConnectorLifecycleResponse", file: "valid-connector-lifecycle-response.json"},
		{name: "force connector version state", ref: "./schema.json#/$defs/ForceConnectorVersionStateRequest", file: "valid-force-connector-version-state.json"},
		{name: "rate limit", ref: "./schema.json#/$defs/RateLimit", file: "valid-rate-limit.json"},
		{name: "list rate limits", ref: "./schema.json#/$defs/ListRateLimitsResponse", file: "valid-list-rate-limits.json"},
		{name: "create rate limit", ref: "./schema.json#/$defs/CreateRateLimitRequest", file: "valid-create-rate-limit.json"},
		{name: "update rate limit", ref: "./schema.json#/$defs/UpdateRateLimitRequest", file: "valid-update-rate-limit.json"},
		{name: "dry-run request", ref: "./schema.json#/$defs/DryRunRequest", file: "valid-dry-run-request.json"},
		{name: "dry-run response", ref: "./schema.json#/$defs/DryRunResponse", file: "valid-dry-run-response.json"},
		{name: "key", ref: "./schema.json#/$defs/Key", file: "valid-key.json"},
		{name: "list keys", ref: "./schema.json#/$defs/ListKeysResponse", file: "valid-list-keys.json"},
		{name: "create key", ref: "./schema.json#/$defs/CreateKeyRequest", file: "valid-create-key.json"},
		{name: "update key", ref: "./schema.json#/$defs/UpdateKeyRequest", file: "valid-update-key.json"},
		{name: "session initiate params", ref: "./schema.json#/$defs/SessionInitiateParams", file: "valid-session-initiate-params.json"},
		{name: "session initiate failure", ref: "./schema.json#/$defs/SessionInitiateFailureResponse", file: "valid-session-initiate-failure-response.json"},
		{name: "session initiate success", ref: "./schema.json#/$defs/SessionInitiateSuccessResponse", file: "valid-session-initiate-success-response.json"},
		{name: "key value", ref: "./schema.json#/$defs/KeyValue", file: "valid-key-value.json"},
		{name: "put key value", ref: "./schema.json#/$defs/PutKeyValueRequest", file: "valid-put-key-value.json"},
		{name: "request event", ref: "./schema.json#/$defs/RequestEvent", file: "valid-request-event.json"},
		{name: "list request events", ref: "./schema.json#/$defs/ListRequestEventsResponse", file: "valid-list-request-events.json"},
		{name: "task info", ref: "./schema.json#/$defs/TaskInfo", file: "valid-task-info.json"},
		{name: "list queues", ref: "./schema.json#/$defs/ListQueuesResponse", file: "valid-list-queues.json"},
		{name: "list monitoring tasks", ref: "./schema.json#/$defs/ListMonitoringTasksResponse", file: "valid-list-monitoring-tasks.json"},
		{name: "list servers", ref: "./schema.json#/$defs/ListServersResponse", file: "valid-list-servers.json"},
		{name: "list scheduler entries", ref: "./schema.json#/$defs/ListSchedulerEntriesResponse", file: "valid-list-scheduler-entries.json"},
		{name: "list queue history", ref: "./schema.json#/$defs/ListQueueHistoryResponse", file: "valid-list-queue-history.json"},
		{name: "bulk action", ref: "./schema.json#/$defs/BulkActionResponse", file: "valid-bulk-action-response.json"},
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

func TestManagedListSchemasRejectLegacyTopLevelCursor(t *testing.T) {
	schema := compileRefSchema(t, "./schema.json#/$defs/ListActorsResponse")
	require.Error(t, schema.Validate(map[string]any{
		"items":  []any{},
		"cursor": "next-page",
	}))
}

func TestConnectorSchemasRejectLegacyFlatShape(t *testing.T) {
	schema := compileRefSchema(t, "./schema.json#/$defs/CreateConnectorRequest")
	require.Error(t, schema.Validate(map[string]any{
		"namespace": "root",
		"definition": map[string]any{
			"displayName": "Legacy",
		},
	}))
}

func TestConnectionSchemasRejectLegacyFlatShapes(t *testing.T) {
	t.Run("resource", func(t *testing.T) {
		schema := compileRefSchema(t, "./schema.json#/$defs/Connection")
		require.Error(t, schema.Validate(map[string]any{
			"id":          "cxn_test0000000000001",
			"namespace":   "root",
			"state":       "configured",
			"healthState": "healthy",
		}))
	})

	t.Run("initiate action", func(t *testing.T) {
		schema := compileRefSchema(t, "./schema.json#/$defs/ConnectionInitiateAction")
		require.Error(t, schema.Validate(map[string]any{
			"connectorId": "cxr_test0000000000001",
			"returnToUrl": "https://example.com/callback",
		}))
	})

	t.Run("patch", func(t *testing.T) {
		schema := compileRefSchema(t, "./schema.json#/$defs/ConnectionPatch")
		require.Error(t, schema.Validate(map[string]any{
			"name": "legacy-name",
		}))
	})
}

func TestInitiateConnectionRequestValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		req := ConnectionInitiateAction{Action: apiv1alpha1.NewActionRequest(
			ConnectionInitiateActionKind,
			meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       connectorschema.ConnectorKind,
				ID:         "cxr_test0000000000001",
			},
			ConnectionInitiateSpec{
				IntoNamespace: "root.acme",
				ReturnToURL:   "https://example.com/callback",
			},
		)}
		require.NoError(t, req.ValidateRequest(ConnectionInitiateActionKind))
	})

	t.Run("requires connector target", func(t *testing.T) {
		req := ConnectionInitiateAction{Action: apiv1alpha1.NewActionRequest(
			ConnectionInitiateActionKind,
			meta.ObjectReference{},
			ConnectionInitiateSpec{ReturnToURL: "https://example.com/callback"},
		)}
		require.ErrorContains(t, req.ValidateRequest(ConnectionInitiateActionKind), "apiVersion")
	})

	t.Run("validates namespace", func(t *testing.T) {
		req := ConnectionInitiateAction{Action: apiv1alpha1.NewActionRequest(
			ConnectionInitiateActionKind,
			meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       connectorschema.ConnectorKind,
				ID:         "cxr_test0000000000001",
			},
			ConnectionInitiateSpec{
				IntoNamespace: "not-rooted",
				ReturnToURL:   "https://example.com/callback",
			},
		)}
		require.Error(t, req.ValidateRequest(ConnectionInitiateActionKind))
	})

	t.Run("rejects connector generation zero only by omission semantics", func(t *testing.T) {
		req := ConnectionInitiateAction{Action: apiv1alpha1.NewActionRequest(
			ConnectionInitiateActionKind,
			meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       connectorschema.ConnectorKind,
				ID:         "cxr_test0000000000001",
			},
			ConnectionInitiateSpec{ReturnToURL: "https://example.com/callback"},
		)}
		require.NoError(t, req.ValidateRequest(ConnectionInitiateActionKind))
	})
}
