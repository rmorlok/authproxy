package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/stretchr/testify/require"
)

// installToolJavascriptConnector stores one encrypted helper generation for the
// connection, independently of which generation is currently primary.
func installToolJavascriptConnector(
	t *testing.T,
	s *toolsTestSetup,
	connection *database.Connection,
	generation uint64,
	source string,
	state database.ConnectorGenerationState,
) {
	t.Helper()
	ctx := context.Background()
	definition, err := json.Marshal(map[string]any{
		"auth": map[string]any{"type": "no-auth"}, "javascript": source,
	})
	require.NoError(t, err)
	encrypted, err := s.Encrypt.EncryptForNamespace(ctx, connection.Namespace, definition)
	require.NoError(t, err)
	require.NoError(t, s.Db.UpsertConnectorGeneration(ctx, &database.ConnectorWithDefinition{
		Id: connection.ConnectorId, Namespace: connection.Namespace,
		Generation: generation, State: state, EncryptedDefinition: encrypted,
	}))
}

// javascriptToolBody replaces the HTTP executor in the common management fixture.
func javascriptToolBody(connection *database.Connection, name, source string) map[string]any {
	body := toolBody(connection.Namespace, name, connection.Id, nil)
	spec := body["spec"].(map[string]any)
	delete(spec, "proxyHttp")
	spec["javascript"] = source
	return body
}

// TestJavaScriptToolsPublishAndUpdate exercises real encryption and revision
// storage through management routes without ever calling the Tool entry point.
func TestJavaScriptToolsPublishAndUpdate(t *testing.T) {
	s, connections := newToolsTestSetup(t)
	connection := connections["calendar"]
	all := aschema.AllPermissions()
	ctx := context.Background()

	// A newer primary must not replace the helper generation pinned by the connection.
	installToolJavascriptConnector(t, s, connection, 1,
		`function selectedGeneration() { return "pinned"; }`, database.ConnectorGenerationStatePrimary)
	installToolJavascriptConnector(t, s, connection, 2,
		`throw "new primary must not initialize";`, database.ConnectorGenerationStatePrimary)
	source := `
		if (selectedGeneration() !== "pinned") throw "wrong helper";
		if (typeof params !== "undefined" || typeof cfg !== "undefined" ||
		    typeof context !== "undefined" || typeof fetch !== "undefined" ||
		    typeof console !== "undefined") throw "unexpected publication capability";
		const initialized = btoa("ready");
		async function execute() { throw "publication must not invoke execute"; }
	`
	w := s.do(t, http.MethodPost, "/tools", javascriptToolBody(connection, "javascript-lookup", source), all)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	created := decodeTool(t, w)
	require.Equal(t, uint64(1), created.Status.Revision)
	require.Equal(t, source, *created.Spec.Javascript)
	require.Nil(t, created.Spec.ProxyHTTP)
	require.Equal(t, connection.Id.String(), created.Spec.ConnectionRef.ID)

	// Reading back the definition returns the exact authored source from storage.
	path := "/tools/" + created.Metadata.ID
	w = s.do(t, http.MethodGet, path, nil, all)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, source, *decodeTool(t, w).Spec.Javascript)
	first, err := s.Db.GetCurrentToolRevision(ctx, apid.ID(created.Metadata.ID))
	require.NoError(t, err)

	// Updating source immediately selects revision 2 while preserving revision 1.
	replacement := `if (selectedGeneration() !== "pinned") throw "wrong helper";
		async function execute(params) { return {updated: params}; }`
	w = s.do(t, http.MethodPatch, path, patchBody(nil, map[string]any{"javascript": replacement}), all)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	updated := decodeTool(t, w)
	require.Equal(t, uint64(2), updated.Status.Revision)
	require.Equal(t, replacement, *updated.Spec.Javascript)
	w = s.do(t, http.MethodGet, path, nil, all)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, uint64(2), decodeTool(t, w).Status.Revision)
	require.Equal(t, replacement, *decodeTool(t, w).Spec.Javascript)
	current, err := s.Db.GetCurrentToolRevision(ctx, apid.ID(created.Metadata.ID))
	require.NoError(t, err)
	require.Equal(t, uint64(2), current.Revision)
	require.NotEqual(t, first.DefinitionDigest, current.DefinitionDigest)
	historical, err := s.Db.GetToolRevision(ctx, apid.ID(created.Metadata.ID), 1)
	require.NoError(t, err)
	plaintext, err := s.Encrypt.Decrypt(ctx, historical.EncryptedDefinition)
	require.NoError(t, err)
	var definition toolschema.ToolDefinition
	require.NoError(t, json.Unmarshal(plaintext, &definition))
	require.Equal(t, source, *definition.Javascript)
}

// TestJavaScriptToolInvalidPublicationKeepsRevision proves compilation and
// initialization finish successfully before replacing any active definition.
func TestJavaScriptToolInvalidPublicationKeepsRevision(t *testing.T) {
	s, connections := newToolsTestSetup(t)
	connection := connections["calendar"]
	installToolJavascriptConnector(t, s, connection, 1, "", database.ConnectorGenerationStatePrimary)
	all := aschema.AllPermissions()
	const source = `async function execute() { return true; }`
	w := s.do(t, http.MethodPost, "/tools", javascriptToolBody(connection, "stable-javascript", source), all)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	created := decodeTool(t, w)
	path := "/tools/" + created.Metadata.ID
	before, err := s.Db.GetCurrentToolRevision(context.Background(), apid.ID(created.Metadata.ID))
	require.NoError(t, err)

	for name, spec := range map[string]map[string]any{
		"syntax":                   {"javascript": `async function execute( {`},
		"entry point":              {"javascript": `function execute() { return true; }`},
		"reserved name":            {"javascript": `const ToolError = 1; async function execute() {}`},
		"initialization throw":     {"javascript": `throw "private-init-marker"; async function execute() {}`},
		"initialization rejection": {"javascript": `Promise.reject("private-init-marker"); async function execute() {}`},
		"global HTTP":              {"javascript": `fetch("https://example.invalid"); async function execute() {}`},
		"connection HTTP":          {"javascript": `context.connections[0].fetch("https://example.invalid"); async function execute() {}`},
		"initialization timeout": {
			"javascript": `while (true) {} async function execute() {}`,
			"limits":     map[string]any{"timeoutMillis": 10},
		},
		"invalid schema": {"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "invalid-type"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			// Each failed complete replacement leaves the original current revision unchanged.
			w := s.do(t, http.MethodPatch, path, patchBody(nil, spec), all)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			require.NotContains(t, w.Body.String(), "private-init-marker")
			current, err := s.Db.GetCurrentToolRevision(context.Background(), apid.ID(created.Metadata.ID))
			require.NoError(t, err)
			require.Equal(t, uint64(1), current.Revision)
			require.Equal(t, before.DefinitionDigest, current.DefinitionDigest)
			w = s.do(t, http.MethodGet, path, nil, all)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Equal(t, source, *decodeTool(t, w).Spec.Javascript)
		})
	}
}

// TestJavaScriptToolRejectedCreateStoresNothing checks the same initialization
// gate applies before a new logical Tool or its first revision is persisted.
func TestJavaScriptToolRejectedCreateStoresNothing(t *testing.T) {
	s, connections := newToolsTestSetup(t)
	connection := connections["calendar"]
	installToolJavascriptConnector(t, s, connection, 1, "", database.ConnectorGenerationStatePrimary)
	all := aschema.AllPermissions()

	// A syntactically valid script still cannot publish after failed initialization.
	w := s.do(t, http.MethodPost, "/tools", javascriptToolBody(connection, "invalid-javascript",
		`throw "private-init-marker"; async function execute() {}`), all)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), "private-init-marker")

	// No partial Tool is visible through the management listing.
	w = s.do(t, http.MethodGet, "/tools", nil, all)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var listed schemaapi.ListToolsResponseJson
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	require.Empty(t, listed.Items)
}
