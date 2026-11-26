package core

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

// TestInvokeToolWorkerLogger sends abnormal-worker diagnostics to the configured
// service sink with captured attribution, without exposing authored/private data.
func TestInvokeToolWorkerLogger(t *testing.T) {
	for _, caught := range []bool{false, true} {
		name := "uncaught failure"
		if caught {
			name = "caught failure"
		}
		t.Run(name, func(t *testing.T) {
			source := `async function execute(params, context) {
				await context.connections[0].fetch("https://example.test/private-path");
			}`
			if caught {
				source = `async function execute(params, context) {
					try { await context.connections[0].fetch("https://example.test/private-path"); }
					catch (_) { return {console: typeof console}; }
				}`
			}
			f := javaScriptToolFixture(t, source)
			var diagnostics bytes.Buffer
			f.service.logger = slog.New(slog.NewJSONHandler(&diagnostics, nil)).With("configured_sink", true)
			f.snapshot.Connection.Labels = database.Labels{"purpose": "private-label"}
			f.snapshot.Connection.Annotations = database.Annotations{"note": "private-annotation"}
			auth := toolInvocationAuth(toolInvocationGrant("tool:read"), nil)
			auth.GetActor().Labels = map[string]string{"team": "private-actor-label"}
			expectHTTPToolDefinition(t, f)
			expectJavaScriptToolPreparation(t, f, "", `{"setting":"private-configuration"}`)
			expectHTTPToolTransport(t, f, auth.GetActor().Labels, func(*http.Request) (*http.Response, error) {
				panic("private-panic-payload")
			})

			// A real configured-client worker emits its diagnostic before handing
			// completion to the VM, so returning synchronizes the log-buffer read.
			got, err := f.service.invokeTool(context.Background(), auth, f.snapshot.Revision.ToolId, nil, common.RawJSON(`{"marker":"private-argument"}`))
			require.NoError(t, err)
			if caught {
				// Supplying the diagnostic logger must not enable authored console logging.
				require.False(t, got.IsError)
				require.JSONEq(t, `{"console":"undefined"}`, string(got.StructuredContent))
			} else {
				requireHTTPToolFailure(t, f, got, "UPSTREAM_FAILED", api.ToolExecutionStateMayHaveExecuted)
			}

			// Only captured scalar identities and fixed diagnostics reach the service sink.
			raw := diagnostics.String()
			require.NotContains(t, raw, "private-")
			var record map[string]any
			require.NoError(t, json.Unmarshal([]byte(raw), &record))
			require.Equal(t, true, record["configured_sink"])
			require.Equal(t, "JavaScript fetch worker exited abnormally", record["msg"])
			require.Equal(t, "panic", record["kind"])
			require.Equal(t, got.ToolID, record["tool_id"])
			require.Equal(t, float64(got.Revision), record["tool_revision"])
			require.Equal(t, got.InvocationID, record["invocation_id"])
			require.Equal(t, got.Metadata.ConnectionID, record["connection_id"])
			require.Equal(t, f.snapshot.Connection.Namespace, record["namespace"])
			require.Equal(t, f.snapshot.Connector.Id.String(), record["connector_id"])
			require.Equal(t, float64(f.snapshot.Connector.Generation), record["connector_generation"])
			require.Equal(t, auth.GetActor().Id.String(), record["actor_id"])
			encoded, err := json.Marshal(got)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "private-")
		})
	}
}
