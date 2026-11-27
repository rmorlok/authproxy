package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/cli/apply"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/stretchr/testify/require"
)

// newToolApplyClient connects the real CLI apply transport to the authenticated
// management routes, with the same API prefix used by a running service.
func newToolApplyClient(t *testing.T, s *toolsTestSetup) *apply.Client {
	t.Helper()
	server := httptest.NewServer(http.StripPrefix("/api/v1", s.Gin))
	t.Cleanup(server.Close)
	token, err := s.AuthUtil.GenerateBearerToken(context.Background(), "apply-admin", "root", aschema.AllPermissions())
	require.NoError(t, err)
	client, err := apply.NewClient(apply.ClientOptions{
		APIURL: server.URL, Signer: jwt.NewSigner(token), Timeout: apply.DefaultRequestTimeout,
	})
	require.NoError(t, err)
	return client
}

// loadToolApplyManifest uses the CLI loader so field presence and typed schema
// decoding match a manifest supplied to ap apply.
func loadToolApplyManifest(t *testing.T, manifest map[string]any) apply.Document {
	t.Helper()
	encoded, err := json.Marshal(manifest)
	require.NoError(t, err)
	documents, err := apply.Load(context.Background(), apply.Options{
		Filenames: []string{"-"}, Stdin: bytes.NewReader(encoded), Validation: apply.ValidationStrict,
	})
	require.NoError(t, err)
	require.Len(t, documents, 1)
	return documents[0]
}

// applyToolManifest resolves, plans, and executes one Tool against real routes.
// Batch dependency ordering is covered by the apply package; these fixtures
// already contain every namespace and connection needed by the manifest.
func applyToolManifest(t *testing.T, client *apply.Client, manifest map[string]any) (*toolschema.Tool, apply.Operation) {
	t.Helper()
	ctx := context.Background()
	targets, err := client.ResolveBatch(ctx, []apply.Document{loadToolApplyManifest(t, manifest)})
	require.NoError(t, err)
	plan, err := apply.Reconcile(targets[0], apply.ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	var live *apply.LiveResource
	switch plan.Operation {
	case apply.OperationCreate:
		live, err = client.Create(ctx, plan.Document)
	case apply.OperationUpdate:
		live, err = client.Update(ctx, plan.Target, plan.Patch)
	case apply.OperationUnchanged:
		live = plan.Target.Current
	default:
		t.Fatalf("unexpected apply operation: %s", plan.Operation)
	}
	require.NoError(t, err)
	tool, ok := live.Resource.(*toolschema.Tool)
	require.True(t, ok)
	return tool, plan.Operation
}

// namedToolConnectionReference authors the namespaced identity that the server
// resolves to the ID-only reference returned in a Tool response.
func namedToolConnectionReference(connection *database.Connection) map[string]any {
	return map[string]any{
		"apiVersion": "authproxy.net/v1alpha1", "kind": "Connection",
		"namespace": connection.Namespace, "name": string(connection.Name),
	}
}

// TestToolApplyLifecycle exercises HTTP and JavaScript manifests through the CLI
// planner, HTTP transport, publication compiler, and encrypted revision storage.
func TestToolApplyLifecycle(t *testing.T) {
	for _, executor := range []string{"http", "javascript"} {
		t.Run(executor, func(t *testing.T) {
			// Real connector definitions permit named connection lookups and JavaScript publication.
			s, connections := newToolsTestSetup(t)
			connection := connections["calendar"]
			installToolJavascriptConnector(t, s, connection, 1, "", database.ConnectorGenerationStatePrimary)
			client := newToolApplyClient(t, s)
			manifest := toolBody(connection.Namespace, "applied-"+executor, connection.Id, nil)
			spec := manifest["spec"].(map[string]any)
			spec["connectionRef"] = namedToolConnectionReference(connection)
			if executor == "javascript" {
				delete(spec, "proxyHttp")
				spec["javascript"] = `async function execute() { return {version: 1}; }`
			}

			// First apply creates a standalone revision. Recording its assigned ID in history
			// on the next apply is a metadata update, which also advances the Tool revision.
			created, operation := applyToolManifest(t, client, manifest)
			require.Equal(t, apply.OperationCreate, operation)
			require.Equal(t, uint64(1), created.Status.Revision)
			require.Nil(t, created.Status.ManagedBy)
			require.Equal(t, connection.Id.String(), created.Spec.ConnectionRef.ID)
			reapplied, operation := applyToolManifest(t, client, manifest)
			require.Equal(t, apply.OperationUpdate, operation)
			require.Equal(t, created.Metadata.ID, reapplied.Metadata.ID)
			require.Equal(t, uint64(2), reapplied.Status.Revision)
			require.Equal(t, created.Spec, reapplied.Spec)
			require.Equal(t, namedToolConnectionReference(connection), spec["connectionRef"], "resolution must not rewrite the authored manifest")

			// Once history contains the server identity, an identical named manifest performs no write.
			reapplied, operation = applyToolManifest(t, client, manifest)
			require.Equal(t, apply.OperationUnchanged, operation)
			require.Equal(t, uint64(2), reapplied.Status.Revision)

			// An executor change takes effect immediately and preserves the old executable snapshot.
			if executor == "http" {
				spec["proxyHttp"].(map[string]any)["url"] = "https://example.com/v2/calendars"
			} else {
				spec["javascript"] = `async function execute() { return {version: 2}; }`
			}
			updated, operation := applyToolManifest(t, client, manifest)
			require.Equal(t, apply.OperationUpdate, operation)
			require.Equal(t, uint64(3), updated.Status.Revision)
			history, err := s.Db.GetToolRevision(context.Background(), apid.ID(created.Metadata.ID), 1)
			require.NoError(t, err)
			plaintext, err := s.Encrypt.Decrypt(context.Background(), history.EncryptedDefinition)
			require.NoError(t, err)
			var previous toolschema.ToolDefinition
			require.NoError(t, json.Unmarshal(plaintext, &previous))
			if executor == "http" {
				require.Equal(t, created.Spec.ProxyHTTP.URL, previous.ProxyHTTP.URL)
				require.Equal(t, "https://example.com/v2/calendars", updated.Spec.ProxyHTTP.URL)
			} else {
				require.Equal(t, *created.Spec.Javascript, *previous.Javascript)
				require.Equal(t, spec["javascript"], *updated.Spec.Javascript)
			}
			reapplied, operation = applyToolManifest(t, client, manifest)
			require.Equal(t, apply.OperationUnchanged, operation)
			require.Equal(t, uint64(3), reapplied.Status.Revision)
		})
	}
}

// TestToolApplyChangesExecutor switches between executor types and clears
// previously managed optional fields when they disappear from the manifest.
func TestToolApplyChangesExecutor(t *testing.T) {
	// Explicit false values must be preserved until the author removes those fields.
	s, connections := newToolsTestSetup(t)
	connection := connections["calendar"]
	installToolJavascriptConnector(t, s, connection, 1, "", database.ConnectorGenerationStatePrimary)
	client := newToolApplyClient(t, s)
	manifest := toolBody(connection.Namespace, "switched-tool", connection.Id, nil)
	spec := manifest["spec"].(map[string]any)
	spec["outputSchema"] = false
	spec["hints"] = map[string]any{"readOnly": false}
	created, _ := applyToolManifest(t, client, manifest)
	require.JSONEq(t, "false", string(created.Spec.OutputSchema))
	require.NotNil(t, created.Spec.Hints.ReadOnly)
	require.False(t, *created.Spec.Hints.ReadOnly)

	// Omitting the old managed executor clears it in the same patch that installs JavaScript.
	delete(spec, "proxyHttp")
	spec["javascript"] = `async function execute() { return null; }`
	javascript, operation := applyToolManifest(t, client, manifest)
	require.Equal(t, apply.OperationUpdate, operation)
	require.Equal(t, uint64(2), javascript.Status.Revision)
	require.Nil(t, javascript.Spec.ProxyHTTP)
	require.Equal(t, spec["javascript"], *javascript.Spec.Javascript)
	require.JSONEq(t, "false", string(javascript.Spec.OutputSchema))
	require.False(t, *javascript.Spec.Hints.ReadOnly)

	// Switching back also clears the optional schema and hints instead of restoring old values.
	delete(spec, "javascript")
	delete(spec, "outputSchema")
	delete(spec, "hints")
	spec["proxyHttp"] = map[string]any{"method": "GET", "url": "https://example.com/lookup"}
	httpTool, operation := applyToolManifest(t, client, manifest)
	require.Equal(t, apply.OperationUpdate, operation)
	require.Equal(t, uint64(3), httpTool.Status.Revision)
	require.Nil(t, httpTool.Spec.Javascript)
	require.Equal(t, "https://example.com/lookup", httpTool.Spec.ProxyHTTP.URL)
	require.Empty(t, httpTool.Spec.OutputSchema)
	require.Nil(t, httpTool.Spec.Hints)

	// The replacement is stable after rereading the server's canonical representation.
	reapplied, operation := applyToolManifest(t, client, manifest)
	require.Equal(t, apply.OperationUnchanged, operation)
	require.Equal(t, httpTool.Status.Revision, reapplied.Status.Revision)
}

// TestToolApplyConnectionRetarget replaces a named connection reference without
// retaining the previous ID and rejects connections outside the Tool namespace.
func TestToolApplyConnectionRetarget(t *testing.T) {
	// Install all connector definitions so lookups exercise the real connection projections.
	s, connections := newToolsTestSetup(t)
	for _, connection := range connections {
		installToolJavascriptConnector(t, s, connection, 1, "", database.ConnectorGenerationStatePrimary)
	}
	client := newToolApplyClient(t, s)
	manifest := toolBody("root.acme", "retargeted-tool", connections["calendar"].Id, nil)
	spec := manifest["spec"].(map[string]any)
	spec["connectionRef"] = namedToolConnectionReference(connections["calendar"])
	created, _ := applyToolManifest(t, client, manifest)

	// A different name selects a new connection and publishes exactly one new revision.
	spec["connectionRef"] = namedToolConnectionReference(connections["crm"])
	updated, operation := applyToolManifest(t, client, manifest)
	require.Equal(t, apply.OperationUpdate, operation)
	require.Equal(t, uint64(2), updated.Status.Revision)
	require.Equal(t, connections["crm"].Id.String(), updated.Spec.ConnectionRef.ID)
	historical, err := s.Db.GetToolRevision(context.Background(), apid.ID(created.Metadata.ID), 1)
	require.NoError(t, err)
	require.Equal(t, connections["calendar"].Id, historical.ConnectionId)

	// Both named and ID references remain constrained to the Tool's namespace.
	for _, ref := range []map[string]any{
		namedToolConnectionReference(connections["other"]),
		{"apiVersion": "authproxy.net/v1alpha1", "kind": "Connection", "id": connections["other"].Id.String()},
	} {
		spec["connectionRef"] = ref
		_, err = client.ResolveBatch(context.Background(), []apply.Document{loadToolApplyManifest(t, manifest)})
		require.Error(t, err)
		current, err := s.Db.GetCurrentToolRevision(context.Background(), apid.ID(created.Metadata.ID))
		require.NoError(t, err)
		require.Equal(t, uint64(2), current.Revision)
		require.Equal(t, connections["crm"].Id, current.ConnectionId)
	}
}

// TestToolApplyInvalidPublicationKeepsHistory proves a failed server-side
// compilation cannot advance either the active revision or last-applied state.
func TestToolApplyInvalidPublicationKeepsHistory(t *testing.T) {
	// Publish a valid executable before submitting source that fails during initialization.
	s, connections := newToolsTestSetup(t)
	connection := connections["calendar"]
	installToolJavascriptConnector(t, s, connection, 1, "", database.ConnectorGenerationStatePrimary)
	client := newToolApplyClient(t, s)
	manifest := javascriptToolBody(connection, "stable-applied-tool", `async function execute() { return true; }`)
	created, _ := applyToolManifest(t, client, manifest)
	manifest["spec"].(map[string]any)["javascript"] = `throw "private-init-marker"; async function execute() {}`
	targets, err := client.ResolveBatch(context.Background(), []apply.Document{loadToolApplyManifest(t, manifest)})
	require.NoError(t, err)
	plan, err := apply.Reconcile(targets[0], apply.ReconcileOptions{Overwrite: true})
	require.NoError(t, err)

	// The route returns a safe validation error; the failed apply does not save its history.
	_, err = client.Update(context.Background(), plan.Target, plan.Patch)
	var apiError *apply.APIError
	require.ErrorAs(t, err, &apiError)
	require.Equal(t, http.StatusBadRequest, apiError.StatusCode)
	require.NotContains(t, err.Error(), "private-init-marker")
	w := s.do(t, http.MethodGet, "/tools/"+created.Metadata.ID, nil, aschema.AllPermissions())
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	current := decodeTool(t, w)
	require.Equal(t, created.Status.Revision, current.Status.Revision)
	require.Equal(t, *created.Spec.Javascript, *current.Spec.Javascript)
	require.Equal(t, created.Metadata.Annotations[apply.LastAppliedAnnotation], current.Metadata.Annotations[apply.LastAppliedAnnotation])
}

// TestToolApplyGeneratedOwnership rejects adoption of a ToolSet-owned Tool and
// points the author to the ToolSet instead of changing generated inventory.
func TestToolApplyGeneratedOwnership(t *testing.T) {
	// Generated inventory has an owning ToolSet even when the submitted manifest is minimal.
	s, connections := newToolsTestSetup(t)
	toolID := installGeneratedToolForTest(t, s, connections["calendar"])
	client := newToolApplyClient(t, s)
	w := s.do(t, http.MethodGet, "/tools/"+toolID.String(), nil, aschema.AllPermissions())
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	generated := decodeTool(t, w)
	doc := loadToolApplyManifest(t, map[string]any{
		"apiVersion": "authproxy.net/v1alpha1", "kind": "Tool",
		"metadata": map[string]any{"id": toolID.String()},
	})

	// Ownership is rejected during planning, before even a history-only write can occur.
	_, err := client.ResolveBatch(context.Background(), []apply.Document{doc})
	require.ErrorContains(t, err, generated.Status.ManagedBy.ToolSetRef.ID)
	w = s.do(t, http.MethodGet, "/tools/"+toolID.String(), nil, aschema.AllPermissions())
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	current := decodeTool(t, w)
	require.Equal(t, generated.Status.Revision, current.Status.Revision)
	require.NotContains(t, current.Metadata.Annotations, apply.LastAppliedAnnotation)
}
