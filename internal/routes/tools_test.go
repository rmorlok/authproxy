package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	asynqmock "github.com/rmorlok/authproxy/internal/apasynq/mock"
	auth2 "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	httpf2 "github.com/rmorlok/authproxy/internal/httpf"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/require"
)

// toolsTestSetup wires the Tool and ToolSet routes to a real core service,
// database, and encryption service so tests exercise compilation and persistence.
type toolsTestSetup struct {
	Gin      *gin.Engine
	AuthUtil *auth2.AuthTestUtil
	Db       database.DB
	Encrypt  encrypt.E
	Core     coreIface.C
}

// newToolsTestSetup creates the namespaces and connections used by the tests:
// root.acme holds two connections and root.other holds one more.
func newToolsTestSetup(t *testing.T) (*toolsTestSetup, map[string]*database.Connection) {
	t.Helper()
	cfg := config.FromRoot(&sconfig.Root{})
	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
	cfg, rds := apredis.MustApplyTestConfig(cfg)
	cfg, auth, authUtil := auth2.TestAuthServiceWithDb(sconfig.ServiceIdApi, cfg, db)
	h := httpf2.CreateFactory(cfg, rds, nil, aplog.NewNoopLogger())
	cfg, e := encrypt.NewTestEncryptService(cfg, db)
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// ToolSet changes request background reconciliation, which these tests do not run.
	ac := asynqmock.NewMockClient(ctrl)
	ac.EXPECT().EnqueueContext(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	c := core.NewCoreService(cfg, db, e, rds, h, ac, test_utils.NewTestLogger())
	require.NoError(t, c.Migrate(context.Background()))

	r := apgin.ForTest(nil)
	NewToolsRoutes(cfg, auth, c).Register(r)
	NewToolSetsRoutes(cfg, auth, c).Register(r)
	NewConnectionsRoutes(cfg, auth, db, rds, c, h, e, aplog.NewNoopLogger()).Register(r)

	ctx := context.Background()
	require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{Path: "root.acme"}))
	require.NoError(t, db.CreateNamespace(ctx, &database.Namespace{Path: "root.other"}))

	connections := map[string]*database.Connection{}
	for key, namespace := range map[string]string{
		"calendar": "root.acme",
		"crm":      "root.acme",
		"other":    "root.other",
	} {
		connection := &database.Connection{
			Id:                  apid.New(apid.PrefixConnection),
			Namespace:           namespace,
			Name:                scommon.ResourceName("account-" + key),
			State:               database.ConnectionStateConfigured,
			ConnectorId:         apid.New(apid.PrefixConnector),
			ConnectorGeneration: 1,
		}
		require.NoError(t, db.CreateConnection(ctx, connection))
		connections[key] = connection
	}

	return &toolsTestSetup{Gin: r, AuthUtil: authUtil, Db: db, Encrypt: e, Core: c}, connections
}

// do sends a signed JSON request with the supplied permissions.
func (s *toolsTestSetup) do(
	t *testing.T,
	method, path string,
	body any,
	permissions []aschema.Permission,
) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := s.AuthUtil.NewSignedRequestForActorExternalId(method, path, reader, "root", "tool-admin", permissions)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	s.Gin.ServeHTTP(w, req)
	return w
}

// toolBody returns a valid create request for an HTTP Tool on a connection.
func toolBody(namespace, name string, connectionId apid.ID, labels map[string]string) map[string]any {
	metadata := map[string]any{"namespace": namespace, "name": name}
	if labels != nil {
		metadata["labels"] = labels
	}
	return map[string]any{
		"apiVersion": string(meta.APIVersionV1Alpha1),
		"kind":       string(toolschema.ToolKind),
		"metadata":   metadata,
		"spec": map[string]any{
			"connectionRef": map[string]any{
				"apiVersion": string(meta.APIVersionV1Alpha1),
				"kind":       "Connection",
				"id":         connectionId.String(),
			},
			"description": "List calendars available to this account.",
			"verbs":       []string{"tool:calendar.list", "tool:readonly"},
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
			"proxyHttp": map[string]any{
				"method": "GET",
				"url":    "https://{{cfg.apiHost}}/v1/calendars",
				"response": map[string]any{
					"transformJavascript": "({ count: response.body.items.length })",
				},
			},
		},
	}
}

// patchBody wraps metadata and spec fields in a ToolPatch envelope.
func patchBody(metadata, spec map[string]any) map[string]any {
	if metadata == nil {
		metadata = map[string]any{}
	}
	if spec == nil {
		spec = map[string]any{}
	}
	return map[string]any{
		"apiVersion": string(meta.APIVersionV1Alpha1),
		"kind":       string(toolschema.ToolKind),
		"metadata":   metadata,
		"spec":       spec,
	}
}

// decodeTool reads a Tool response body.
func decodeTool(t *testing.T, w *httptest.ResponseRecorder) toolschema.Tool {
	t.Helper()
	var tool toolschema.Tool
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &tool), w.Body.String())
	return tool
}

// installGeneratedToolForTest installs one ToolSet-generated Tool directly in
// storage, because no ToolSet controller exists yet to materialize it.
func installGeneratedToolForTest(t *testing.T, s *toolsTestSetup, connection *database.Connection) apid.ID {
	t.Helper()
	ctx := context.Background()
	toolSet := &database.ToolSet{
		Id:                 apid.New(apid.PrefixToolSet),
		Namespace:          connection.Namespace,
		Name:               "calendar-tools",
		ConnectionSelector: toolsetschema.ConnectionSelector{MatchLabels: map[string]string{}},
	}
	require.NoError(t, s.Db.CreateToolSet(ctx, toolSet))

	opaque, err := s.Encrypt.EncryptStringForNamespace(ctx, connection.Namespace, "{}")
	require.NoError(t, err)
	generation, err := s.Db.CreateToolSetGeneration(ctx, toolSet.Id, opaque)
	require.NoError(t, err)
	_, err = s.Db.PublishToolSetGeneration(ctx, toolSet.Id, generation.Generation, generation.Revision)
	require.NoError(t, err)
	binding, err := s.Db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, 1)
	require.NoError(t, err)

	definition := toolschema.ToolDefinition{
		Description: "Generated lookup.",
		Verbs:       []string{"tool:lookup"},
		InputSchema: []byte(`{"type":"object"}`),
		ProxyHTTP:   &toolschema.ProxyHTTP{Method: "GET", URL: "https://example.com/lookup"},
	}
	plaintext, err := json.Marshal(definition)
	require.NoError(t, err)
	encrypted, err := s.Encrypt.EncryptForNamespace(ctx, connection.Namespace, plaintext)
	require.NoError(t, err)

	toolId := apid.New(apid.PrefixTool)
	_, err = s.Db.InstallExplicitToolSetInventory(ctx, database.ToolSetInventoryInstall{
		BindingId:                binding.Id,
		ExpectedEpoch:            binding.Epoch,
		ExpectedSelectorRevision: 1,
		Generation:               generation.Generation,
		Tools: []database.PreparedToolSetTool{{
			Tool: database.Tool{
				Id:           toolId,
				Namespace:    connection.Namespace,
				Name:         "generated-lookup",
				ConnectionId: connection.Id,
				BindingId:    binding.Id,
				SourceKey:    "lookup",
				Description:  definition.Description,
				Verbs:        definition.Verbs,
			},
			EncryptedDefinition: encrypted,
			DefinitionDigest:    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		}},
	})
	require.NoError(t, err)
	return toolId
}

func TestTools(t *testing.T) {
	all := aschema.AllPermissions()

	t.Run("create publishes revision 1 with a resolved connection", func(t *testing.T) {
		s, cxns := newToolsTestSetup(t)
		w := s.do(t, http.MethodPost, "/tools", toolBody("root.acme", "list-calendars", cxns["calendar"].Id, map[string]string{"capability": "calendars"}), all)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		tool := decodeTool(t, w)
		require.True(t, tool.GetId().HasPrefix(apid.PrefixTool))
		require.Equal(t, uint64(1), tool.Status.Revision)
		require.Nil(t, tool.Status.ManagedBy)
		require.Equal(t, cxns["calendar"].Id.String(), tool.Spec.ConnectionRef.ID)
		require.Equal(t, "calendars", tool.Metadata.Labels["capability"])
		require.Equal(t, cxns["calendar"].Id.String(), tool.Metadata.Labels["apxy/cxn/-/id"], "connection identity label is derived")
		require.Equal(t, []string{"tool:calendar.list", "tool:readonly"}, tool.Spec.Verbs)
		require.Equal(t, "GET", tool.Spec.ProxyHTTP.Method)

		got := s.do(t, http.MethodGet, "/tools/"+tool.Metadata.ID, nil, all)
		require.Equal(t, http.StatusOK, got.Code, got.Body.String())
		require.Equal(t, tool.Spec.ProxyHTTP.URL, decodeTool(t, got).Spec.ProxyHTTP.URL)
	})

	t.Run("create resolves a connection by name in the tool namespace", func(t *testing.T) {
		s, cxns := newToolsTestSetup(t)
		body := toolBody("root.acme", "by-name", cxns["crm"].Id, nil)
		body["spec"].(map[string]any)["connectionRef"] = map[string]any{
			"apiVersion": string(meta.APIVersionV1Alpha1),
			"kind":       "Connection",
			"namespace":  "root.acme",
			"name":       string(cxns["crm"].Name),
		}
		w := s.do(t, http.MethodPost, "/tools", body, all)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Equal(t, cxns["crm"].Id.String(), decodeTool(t, w).Spec.ConnectionRef.ID)
	})

	t.Run("create rejects invalid or unsupported definitions before storing", func(t *testing.T) {
		s, cxns := newToolsTestSetup(t)

		crossNamespace := toolBody("root.acme", "cross", cxns["other"].Id, nil)
		w := s.do(t, http.MethodPost, "/tools", crossNamespace, all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "tool namespace")

		missing := toolBody("root.acme", "missing", apid.New(apid.PrefixConnection), nil)
		w = s.do(t, http.MethodPost, "/tools", missing, all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

		badTemplate := toolBody("root.acme", "bad-template", cxns["calendar"].Id, nil)
		badTemplate["spec"].(map[string]any)["proxyHttp"].(map[string]any)["url"] = "https://{{#cfg.apiHost}}/v1"
		w = s.do(t, http.MethodPost, "/tools", badTemplate, all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "proxyHttp")

		badTransform := toolBody("root.acme", "bad-transform", cxns["calendar"].Id, nil)
		badTransform["spec"].(map[string]any)["proxyHttp"].(map[string]any)["response"] = map[string]any{"transformJavascript": "({"}
		w = s.do(t, http.MethodPost, "/tools", badTransform, all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

		javascript := toolBody("root.acme", "js", cxns["calendar"].Id, nil)
		spec := javascript["spec"].(map[string]any)
		delete(spec, "proxyHttp")
		spec["javascript"] = "async function execute(params, context) { return {}; }"
		w = s.do(t, http.MethodPost, "/tools", javascript, all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "not supported yet")

		withStatus := toolBody("root.acme", "status", cxns["calendar"].Id, nil)
		withStatus["status"] = map[string]any{"revision": 9}
		w = s.do(t, http.MethodPost, "/tools", withStatus, all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

		list := s.do(t, http.MethodGet, "/tools", nil, all)
		require.Equal(t, http.StatusOK, list.Code, list.Body.String())
		var resp schemaapi.ListToolsResponseJson
		require.NoError(t, json.Unmarshal(list.Body.Bytes(), &resp))
		require.Empty(t, resp.Items, "rejected definitions are never stored")
	})

	t.Run("create enforces names and permissions", func(t *testing.T) {
		s, cxns := newToolsTestSetup(t)
		body := toolBody("root.acme", "dupe", cxns["calendar"].Id, nil)
		require.Equal(t, http.StatusOK, s.do(t, http.MethodPost, "/tools", body, all).Code)

		w := s.do(t, http.MethodPost, "/tools", body, all)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())

		w = s.do(t, http.MethodPost, "/tools", toolBody("root.acme", "other-verb", cxns["calendar"].Id, nil),
			aschema.PermissionsSingle("root.**", "tools", "get"))
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

		w = s.do(t, http.MethodPost, "/tools", toolBody("root.acme", "other-ns", cxns["calendar"].Id, nil),
			aschema.PermissionsSingle("root.other.**", "tools", "create"))
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

		w = s.do(t, http.MethodPost, "/tools", toolBody("root.acme", "proxy-only", cxns["calendar"].Id, nil),
			aschema.PermissionsSingle("root.**", "connections", "proxy"))
		require.Equal(t, http.StatusForbidden, w.Code, "connection grants do not authorize tool management")
	})

	t.Run("get enforces permissions and reports missing tools", func(t *testing.T) {
		s, cxns := newToolsTestSetup(t)
		created := decodeTool(t, s.do(t, http.MethodPost, "/tools", toolBody("root.acme", "get-me", cxns["calendar"].Id, nil), all))
		path := "/tools/" + created.Metadata.ID

		req, err := http.NewRequest(http.MethodGet, path, nil)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		s.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)

		require.Equal(t, http.StatusForbidden, s.do(t, http.MethodGet, path, nil, aschema.PermissionsSingle("root.other.**", "tools", "get")).Code)
		require.Equal(t, http.StatusForbidden, s.do(t, http.MethodGet, path, nil, aschema.PermissionsSingle("root.**", "tool:calendar.list", "*")).Code)
		require.Equal(t, http.StatusOK, s.do(t, http.MethodGet, path, nil, aschema.PermissionsSingle("root.acme", "tools", "get")).Code)
		require.Equal(t, http.StatusNotFound, s.do(t, http.MethodGet, "/tools/"+apid.New(apid.PrefixTool).String(), nil, all).Code)
		require.Equal(t, http.StatusBadRequest, s.do(t, http.MethodGet, "/tools/"+cxns["calendar"].Id.String(), nil, all).Code)
	})

	t.Run("update publishes a new revision immediately and retains history", func(t *testing.T) {
		s, cxns := newToolsTestSetup(t)
		created := decodeTool(t, s.do(t, http.MethodPost, "/tools", toolBody("root.acme", "editable", cxns["calendar"].Id, map[string]string{"keep": "yes"}), all))
		path := "/tools/" + created.Metadata.ID

		w := s.do(t, http.MethodPatch, path, patchBody(
			map[string]any{"name": "renamed"},
			map[string]any{
				"description":   "Updated description.",
				"connectionRef": map[string]any{"apiVersion": string(meta.APIVersionV1Alpha1), "kind": "Connection", "id": cxns["crm"].Id.String()},
				"hints":         map[string]any{"readOnly": true},
			},
		), all)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		updated := decodeTool(t, w)
		require.Equal(t, uint64(2), updated.Status.Revision)
		require.Equal(t, "renamed", string(updated.Metadata.Name))
		require.Equal(t, "Updated description.", updated.Spec.Description)
		require.Equal(t, cxns["crm"].Id.String(), updated.Spec.ConnectionRef.ID)
		require.Equal(t, cxns["crm"].Id.String(), updated.Metadata.Labels["apxy/cxn/-/id"], "retargeting re-derives the connection label")
		require.Equal(t, "yes", updated.Metadata.Labels["keep"])
		require.Equal(t, created.Spec.ProxyHTTP.URL, updated.Spec.ProxyHTTP.URL, "omitted fields are preserved")

		id := apid.ID(created.Metadata.ID)
		first, err := s.Db.GetToolRevision(context.Background(), id, 1)
		require.NoError(t, err)
		require.Equal(t, cxns["calendar"].Id, first.ConnectionId, "revision 1 keeps its original connection")
		current, err := s.Db.GetCurrentToolRevision(context.Background(), id)
		require.NoError(t, err)
		require.Equal(t, uint64(2), current.Revision)
		require.NotEqual(t, first.DefinitionDigest, current.DefinitionDigest)
	})

	t.Run("failed updates leave the active revision intact", func(t *testing.T) {
		s, cxns := newToolsTestSetup(t)
		created := decodeTool(t, s.do(t, http.MethodPost, "/tools", toolBody("root.acme", "stable", cxns["calendar"].Id, nil), all))
		path := "/tools/" + created.Metadata.ID

		for name, body := range map[string]map[string]any{
			"cross namespace connection": patchBody(nil, map[string]any{
				"connectionRef": map[string]any{"apiVersion": string(meta.APIVersionV1Alpha1), "kind": "Connection", "id": cxns["other"].Id.String()},
			}),
			"uncompilable template": patchBody(nil, map[string]any{
				"proxyHttp": map[string]any{"method": "GET", "url": "https://{{#cfg.apiHost}}"},
			}),
			"javascript executor": patchBody(nil, map[string]any{
				"proxyHttp":  nil,
				"javascript": "async function execute() { return 1; }",
			}),
			"namespace change": patchBody(map[string]any{"namespace": "root.other"}, nil),
		} {
			w := s.do(t, http.MethodPatch, path, body, all)
			require.Equal(t, http.StatusBadRequest, w.Code, "%s: %s", name, w.Body.String())
		}

		got := decodeTool(t, s.do(t, http.MethodGet, path, nil, all))
		require.Equal(t, uint64(1), got.Status.Revision)
		require.Equal(t, cxns["calendar"].Id.String(), got.Spec.ConnectionRef.ID)

		require.Equal(t, http.StatusForbidden, s.do(t, http.MethodPatch, path, patchBody(nil, nil), aschema.PermissionsSingle("root.**", "tools", "get")).Code)
		require.Equal(t, http.StatusNotFound, s.do(t, http.MethodPatch, "/tools/"+apid.New(apid.PrefixTool).String(), patchBody(nil, nil), all).Code)
	})

	t.Run("stale revisions cannot overwrite or delete a newer revision", func(t *testing.T) {
		s, cxns := newToolsTestSetup(t)
		ctx := context.Background()
		created := decodeTool(t, s.do(t, http.MethodPost, "/tools", toolBody("root.acme", "raced", cxns["calendar"].Id, nil), all))
		id := apid.ID(created.Metadata.ID)

		// Simulate two writers that both observed revision 1.
		first := created.Clone()
		first.Status = &toolschema.ToolStatus{Revision: 1}
		first.Spec.Description = "first writer"
		_, err := s.Core.UpdateTool(ctx, id, 1, first)
		require.NoError(t, err)

		second := created.Clone()
		second.Status = &toolschema.ToolStatus{Revision: 1}
		second.Spec.Description = "second writer"
		_, err = s.Core.UpdateTool(ctx, id, 1, second)
		require.ErrorIs(t, err, core.ErrToolRevisionConflict)
		require.ErrorIs(t, s.Core.DeleteTool(ctx, id, 1), core.ErrToolRevisionConflict)

		got, err := s.Core.GetTool(ctx, id)
		require.NoError(t, err)
		require.Equal(t, uint64(2), got.GetActiveRevision())
		require.Equal(t, "first writer", got.GetResource().Spec.Description)
	})

	t.Run("write responses match stored timestamps", func(t *testing.T) {
		s, cxns := newToolsTestSetup(t)

		// A nanosecond clock exposes sub-microsecond values PostgreSQL cannot store.
		ctx := apctx.WithFixedClock(context.Background(), time.Date(2026, 10, 30, 12, 0, 0, 123456789, time.UTC))

		// The create response carries the same timestamps as a later read.
		var body toolschema.Tool
		encoded, err := json.Marshal(toolBody("root.acme", "precise", cxns["calendar"].Id, nil))
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(encoded, &body))
		created, err := s.Core.CreateTool(ctx, &body)
		require.NoError(t, err)
		stored, err := s.Core.GetTool(ctx, created.GetId())
		require.NoError(t, err)
		require.Equal(t, stored.GetResource().Metadata, created.GetResource().Metadata)

		// An update built from the create response passes the immutability check
		// and returns the stored timestamps too.
		candidate := created.GetResource()
		candidate.Spec.Description = "changed"
		updated, err := s.Core.UpdateTool(ctx, created.GetId(), created.GetActiveRevision(), candidate)
		require.NoError(t, err)
		stored, err = s.Core.GetTool(ctx, created.GetId())
		require.NoError(t, err)
		require.Equal(t, stored.GetResource().Metadata, updated.GetResource().Metadata)
	})

	t.Run("list orders results and rejects invalid ordering", func(t *testing.T) {
		s, cxns := newToolsTestSetup(t)

		// Create Tools out of alphabetical order.
		for _, name := range []string{"bravo", "alpha", "charlie"} {
			w := s.do(t, http.MethodPost, "/tools", toolBody("root.acme", name, cxns["calendar"].Id, nil), all)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		}

		names := func(path string) []string {
			w := s.do(t, http.MethodGet, path, nil, all)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var resp schemaapi.ListToolsResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			result := make([]string, 0, len(resp.Items))
			for _, item := range resp.Items {
				result = append(result, string(item.Metadata.Name))
			}
			return result
		}

		// The field defaults to ascending, and an explicit direction is honored.
		require.Equal(t, []string{"alpha", "bravo", "charlie"}, names("/tools?orderBy=name"))
		require.Equal(t, []string{"charlie", "bravo", "alpha"}, names("/tools?orderBy="+url.QueryEscape("name desc")))

		// Unknown fields and directions are client errors.
		require.Equal(t, http.StatusBadRequest, s.do(t, http.MethodGet, "/tools?orderBy=bogus", nil, all).Code)
		require.Equal(t, http.StatusBadRequest, s.do(t, http.MethodGet, "/tools?orderBy="+url.QueryEscape("name sideways"), nil, all).Code)
	})

	t.Run("list filters by scope, connection, name, and labels with pagination", func(t *testing.T) {
		s, cxns := newToolsTestSetup(t)
		for i, cxn := range []string{"calendar", "calendar", "crm"} {
			labels := map[string]string{"index": fmt.Sprint(i)}
			w := s.do(t, http.MethodPost, "/tools", toolBody("root.acme", fmt.Sprintf("tool-%d", i), cxns[cxn].Id, labels), all)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		}
		require.Equal(t, http.StatusOK, s.do(t, http.MethodPost, "/tools", toolBody("root.other", "elsewhere", cxns["other"].Id, nil), all).Code)

		list := func(query string, permissions []aschema.Permission) schemaapi.ListToolsResponseJson {
			w := s.do(t, http.MethodGet, "/tools"+query, nil, permissions)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var resp schemaapi.ListToolsResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			return resp
		}
		names := func(resp schemaapi.ListToolsResponseJson) []string {
			var result []string
			for _, item := range resp.Items {
				result = append(result, string(item.Metadata.Name))
			}
			return result
		}

		require.ElementsMatch(t, []string{"tool-0", "tool-1", "tool-2", "elsewhere"}, names(list("", all)))
		require.ElementsMatch(t, []string{"tool-0", "tool-1", "tool-2"}, names(list("", aschema.PermissionsSingle("root.acme.**", "tools", "list"))))
		require.ElementsMatch(t, []string{"elsewhere"}, names(list("?namespace=root.other", all)))
		require.ElementsMatch(t, []string{"tool-0", "tool-1"}, names(list("?connectionId="+cxns["calendar"].Id.String(), all)))
		require.ElementsMatch(t, []string{"tool-2"}, names(list("?name=tool-2", all)))
		require.ElementsMatch(t, []string{"tool-1"}, names(list("?labelSelector=index%3D1", all)))

		page := list("?limit=2", all)
		require.Len(t, page.Items, 2)
		require.NotEmpty(t, page.Metadata.Continue)
		seen := names(page)
		for page.Metadata.Continue != "" {
			page = list("?cursor="+url.QueryEscape(page.Metadata.Continue), all)
			seen = append(seen, names(page)...)
		}
		require.ElementsMatch(t, []string{"tool-0", "tool-1", "tool-2", "elsewhere"}, seen)

		for _, query := range []string{"?limit=0", "?name=Not_Valid!", "?connectionId=tol_notaconnection", "?cursor=garbage"} {
			w := s.do(t, http.MethodGet, "/tools"+query, nil, all)
			require.Equal(t, http.StatusBadRequest, w.Code, "%s: %s", query, w.Body.String())
		}
	})

	t.Run("delete is idempotent and frees the name", func(t *testing.T) {
		s, cxns := newToolsTestSetup(t)
		body := toolBody("root.acme", "short-lived", cxns["calendar"].Id, nil)
		created := decodeTool(t, s.do(t, http.MethodPost, "/tools", body, all))
		path := "/tools/" + created.Metadata.ID

		require.Equal(t, http.StatusForbidden, s.do(t, http.MethodDelete, path, nil, aschema.PermissionsSingle("root.**", "tools", "update")).Code)
		require.Equal(t, http.StatusNoContent, s.do(t, http.MethodDelete, path, nil, all).Code)
		require.Equal(t, http.StatusNotFound, s.do(t, http.MethodGet, path, nil, all).Code)
		require.Equal(t, http.StatusNoContent, s.do(t, http.MethodDelete, path, nil, all).Code)

		recreated := s.do(t, http.MethodPost, "/tools", body, all)
		require.Equal(t, http.StatusOK, recreated.Code, recreated.Body.String())
		require.NotEqual(t, created.Metadata.ID, decodeTool(t, recreated).Metadata.ID, "deleted IDs are never reused")
	})

	t.Run("generated tools are readable but only changed through their ToolSet", func(t *testing.T) {
		s, cxns := newToolsTestSetup(t)
		id := installGeneratedToolForTest(t, s, cxns["calendar"])
		path := "/tools/" + id.String()

		w := s.do(t, http.MethodGet, path, nil, all)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		tool := decodeTool(t, w)
		require.NotNil(t, tool.Status.ManagedBy)
		require.Equal(t, "lookup", tool.Status.ManagedBy.SourceKey)
		require.Equal(t, uint64(1), tool.Status.ManagedBy.ToolSetRef.Generation)

		w = s.do(t, http.MethodPatch, path, patchBody(nil, map[string]any{"description": "changed"}), all)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), tool.Status.ManagedBy.ToolSetRef.ID)

		w = s.do(t, http.MethodDelete, path, nil, all)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		require.Equal(t, http.StatusOK, s.do(t, http.MethodGet, path, nil, all).Code)

		w = s.do(t, http.MethodGet, "/tools?connectionId="+cxns["calendar"].Id.String(), nil, all)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), id.String())
	})
}
