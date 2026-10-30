package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/stretchr/testify/require"
)

// toolSetTemplate returns one explicit HTTP Tool template with the given key.
func toolSetTemplate(key, url string) map[string]any {
	return map[string]any{
		"key": key,
		"spec": map[string]any{
			"description": "List calendars available to this account.",
			"verbs":       []string{"tool:calendar.list"},
			"inputSchema": map[string]any{"type": "object"},
			"proxyHttp":   map[string]any{"method": "GET", "url": url},
		},
	}
}

// toolSetDefinition wraps templates in an explicit-source definition.
func toolSetDefinition(templates ...map[string]any) map[string]any {
	if templates == nil {
		templates = []map[string]any{}
	}
	return map[string]any{"source": map[string]any{"explicit": map[string]any{"tools": templates}}}
}

// toolSetBody returns a valid create request with a match-all selector.
func toolSetBody(namespace, name, desiredState string) map[string]any {
	spec := map[string]any{
		"connectionSelector": map[string]any{"matchLabels": map[string]string{}},
		"definition":         toolSetDefinition(toolSetTemplate("list-calendars", "https://{{cfg.apiHost}}/v1/calendars")),
	}
	if desiredState != "" {
		spec["release"] = map[string]any{"desiredState": desiredState}
	}
	return map[string]any{
		"apiVersion": string(meta.APIVersionV1Alpha1),
		"kind":       string(toolsetschema.ToolSetKind),
		"metadata":   map[string]any{"namespace": namespace, "name": name},
		"spec":       spec,
	}
}

// toolSetPatchBody wraps metadata and spec fields in a ToolSetPatch envelope.
func toolSetPatchBody(metadata, spec map[string]any) map[string]any {
	body := patchBody(metadata, spec)
	body["kind"] = string(toolsetschema.ToolSetKind)
	return body
}

// decodeToolSet reads a ToolSet response body.
func decodeToolSet(t *testing.T, w *httptest.ResponseRecorder) toolsetschema.ToolSet {
	t.Helper()
	var toolSet toolsetschema.ToolSet
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &toolSet), w.Body.String())
	return toolSet
}

// decodeToolSetList reads a ToolSetList response body.
func decodeToolSetList(t *testing.T, w *httptest.ResponseRecorder) schemaapi.ListToolSetsResponseJson {
	t.Helper()
	var list schemaapi.ListToolSetsResponseJson
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list), w.Body.String())
	return list
}

// generationStates summarizes a generation list as "number:state" pairs.
func generationStates(list schemaapi.ListToolSetsResponseJson) []string {
	result := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		result = append(result, fmt.Sprintf("%d:%s", item.Metadata.Generation, item.Status.Release.State))
	}
	return result
}

// firstTemplateURL returns the URL of a ToolSet's first explicit template.
func firstTemplateURL(toolSet toolsetschema.ToolSet) string {
	return toolSet.Spec.Definition.Source.Explicit.Tools[0].Spec.ProxyHTTP.URL
}

func TestToolSets(t *testing.T) {
	all := aschema.AllPermissions()

	t.Run("create stores a draft unless primary is requested", func(t *testing.T) {
		s, _ := newToolsTestSetup(t)

		w := s.do(t, http.MethodPost, "/toolsets", toolSetBody("root.acme", "drafted", ""), all)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		draft := decodeToolSet(t, w)
		require.Equal(t, uint64(1), draft.Metadata.Generation)
		require.Equal(t, toolsetschema.ToolSetReleaseStateDraft, draft.Spec.Release.DesiredState)
		require.Equal(t, toolsetschema.ToolSetReleaseStateDraft, draft.Status.Release.State)
		require.Equal(t, "root.acme.**", draft.Spec.ConnectionSelector.Namespace, "selector scope defaults to the owner subtree")
		require.Equal(t, draft.Metadata.ID, draft.Metadata.Labels["apxy/tls/-/id"])

		w = s.do(t, http.MethodPost, "/toolsets", toolSetBody("root.acme", "published", "primary"), all)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		published := decodeToolSet(t, w)
		require.Equal(t, toolsetschema.ToolSetReleaseStatePrimary, published.Status.Release.State)

		got := s.do(t, http.MethodGet, "/toolsets/"+published.Metadata.ID, nil, all)
		require.Equal(t, http.StatusOK, got.Code, got.Body.String())
		require.Equal(t, "https://{{cfg.apiHost}}/v1/calendars", firstTemplateURL(decodeToolSet(t, got)))
	})

	t.Run("create rejects invalid or unsupported definitions before storing", func(t *testing.T) {
		s, _ := newToolsTestSetup(t)

		badTemplate := toolSetBody("root.acme", "bad-template", "")
		badTemplate["spec"].(map[string]any)["definition"] = toolSetDefinition(toolSetTemplate("bad", "https://{{#cfg.apiHost}}/v1"))
		w := s.do(t, http.MethodPost, "/toolsets", badTemplate, all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "explicit.tools[0].spec.proxyHttp")

		javascript := toolSetBody("root.acme", "js", "")
		template := toolSetTemplate("js", "")
		spec := template["spec"].(map[string]any)
		delete(spec, "proxyHttp")
		spec["javascript"] = "async function execute(params, context) { return {}; }"
		javascript["spec"].(map[string]any)["definition"] = toolSetDefinition(template)
		w = s.do(t, http.MethodPost, "/toolsets", javascript, all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "not supported yet")

		mcp := toolSetBody("root.acme", "mcp", "")
		mcp["spec"].(map[string]any)["definition"] = map[string]any{
			"source": map[string]any{"mcp": map[string]any{"endpoint": "https://example.com/mcp", "transport": "streamableHttp"}},
		}
		w = s.do(t, http.MethodPost, "/toolsets", mcp, all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "not supported yet")

		ancestor := toolSetBody("root.acme", "ancestor", "")
		ancestor["spec"].(map[string]any)["connectionSelector"] = map[string]any{"namespace": "root.**", "matchLabels": map[string]string{}}
		w = s.do(t, http.MethodPost, "/toolsets", ancestor, all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

		missingNamespace := s.do(t, http.MethodPost, "/toolsets", toolSetBody("root.missing", "orphan", ""), all)
		require.Equal(t, http.StatusBadRequest, missingNamespace.Code, missingNamespace.Body.String())

		withStatus := toolSetBody("root.acme", "status", "")
		withStatus["status"] = map[string]any{"release": map[string]any{"state": "primary"}}
		w = s.do(t, http.MethodPost, "/toolsets", withStatus, all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

		list := s.do(t, http.MethodGet, "/toolsets", nil, all)
		require.Equal(t, http.StatusOK, list.Code, list.Body.String())
		require.Empty(t, decodeToolSetList(t, list).Items, "rejected definitions are never stored")
	})

	t.Run("create enforces names and permissions", func(t *testing.T) {
		s, _ := newToolsTestSetup(t)
		require.Equal(t, http.StatusCreated, s.do(t, http.MethodPost, "/toolsets", toolSetBody("root.acme", "dupe", ""), all).Code)

		w := s.do(t, http.MethodPost, "/toolsets", toolSetBody("root.acme", "dupe", ""), all)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())

		w = s.do(t, http.MethodPost, "/toolsets", toolSetBody("root.acme", "wrong-ns", ""),
			aschema.PermissionsSingle("root.other.**", "toolsets", "create"))
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

		w = s.do(t, http.MethodPost, "/toolsets", toolSetBody("root.acme", "tools-grant", ""),
			aschema.PermissionsSingle("root.**", "tools", "create"))
		require.Equal(t, http.StatusForbidden, w.Code, "tool grants do not authorize toolset management")

		w = s.do(t, http.MethodPost, "/toolsets", toolSetBody("root.acme", "granted", ""),
			aschema.PermissionsSingle("root.acme.**", "toolsets", "create"))
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		id := decodeToolSet(t, w).Metadata.ID

		w = s.do(t, http.MethodGet, "/toolsets/"+id, nil, aschema.PermissionsSingle("root.other.**", "toolsets", "get"))
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
		w = s.do(t, http.MethodGet, "/toolsets/"+id+"/generations", nil, aschema.PermissionsSingle("root.**", "toolsets", "get"))
		require.Equal(t, http.StatusForbidden, w.Code, "generation reads require list/generations")
		w = s.do(t, http.MethodGet, "/toolsets/"+id+"/generations", nil, aschema.PermissionsSingle("root.**", "toolsets", "list/generations"))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("logical metadata and selector updates never create generations", func(t *testing.T) {
		s, _ := newToolsTestSetup(t)
		created := decodeToolSet(t, s.do(t, http.MethodPost, "/toolsets", toolSetBody("root.acme", "calendar", "primary"), all))
		path := "/toolsets/" + created.Metadata.ID

		w := s.do(t, http.MethodPatch, path, toolSetPatchBody(
			map[string]any{"name": "scheduling", "labels": map[string]string{"tier": "gold"}},
			map[string]any{"connectionSelector": map[string]any{"matchLabels": map[string]string{"provider": "google"}}},
		), all)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		updated := decodeToolSet(t, w)
		require.Equal(t, "scheduling", string(updated.Metadata.Name))
		require.Equal(t, "gold", updated.Metadata.Labels["tier"])
		require.Equal(t, "scheduling", updated.Metadata.Labels["apxy/tls/-/name"])
		require.Equal(t, map[string]string{"provider": "google"}, updated.Spec.ConnectionSelector.MatchLabels)
		require.Equal(t, "root.acme.**", updated.Spec.ConnectionSelector.Namespace)
		require.Equal(t, uint64(1), updated.Metadata.Generation)
		require.Equal(t, toolsetschema.ToolSetReleaseStatePrimary, updated.Status.Release.State)

		generations := decodeToolSetList(t, s.do(t, http.MethodGet, path+"/generations", nil, all))
		require.Equal(t, []string{"1:primary"}, generationStates(generations))

		w = s.do(t, http.MethodPatch, path, toolSetPatchBody(nil, map[string]any{
			"connectionSelector": map[string]any{"namespace": "root.**", "matchLabels": map[string]string{}},
		}), all)
		require.Equal(t, http.StatusBadRequest, w.Code, "selectors cannot reach outside the owner subtree")

		w = s.do(t, http.MethodPatch, path, toolSetPatchBody(map[string]any{"generation": 1}, nil), all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})

	t.Run("definition changes go through a draft and publication", func(t *testing.T) {
		s, _ := newToolsTestSetup(t)
		created := decodeToolSet(t, s.do(t, http.MethodPost, "/toolsets", toolSetBody("root.acme", "calendar", "primary"), all))
		path := "/toolsets/" + created.Metadata.ID
		changed := toolSetDefinition(toolSetTemplate("list-calendars", "https://{{cfg.apiHost}}/v2/calendars"))

		// A definition change at the logical endpoint creates a draft; the primary
		// that new connections install is untouched.
		w := s.do(t, http.MethodPatch, path, toolSetPatchBody(nil, map[string]any{"definition": changed}), all)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		draft := decodeToolSet(t, w)
		require.Equal(t, uint64(2), draft.Metadata.Generation)
		require.Equal(t, toolsetschema.ToolSetReleaseStateDraft, draft.Status.Release.State)

		current := decodeToolSet(t, s.do(t, http.MethodGet, path, nil, all))
		require.Equal(t, uint64(1), current.Metadata.Generation)
		require.Equal(t, "https://{{cfg.apiHost}}/v1/calendars", firstTemplateURL(current))

		// A second definition change edits the same draft.
		again := toolSetDefinition(toolSetTemplate("list-calendars", "https://{{cfg.apiHost}}/v3/calendars"))
		w = s.do(t, http.MethodPatch, path, toolSetPatchBody(nil, map[string]any{"definition": again}), all)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Equal(t, uint64(2), decodeToolSet(t, w).Metadata.Generation)

		// An invalid edit is rejected and leaves the draft as it was.
		invalid := toolSetDefinition(toolSetTemplate("list-calendars", "https://{{#cfg.apiHost}}"))
		w = s.do(t, http.MethodPatch, path, toolSetPatchBody(nil, map[string]any{"definition": invalid}), all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		stored := decodeToolSet(t, s.do(t, http.MethodGet, path+"/generations/2", nil, all))
		require.Equal(t, "https://{{cfg.apiHost}}/v3/calendars", firstTemplateURL(stored))

		// At the logical endpoint, primary intent alone is already satisfied by the
		// existing primary, so it neither publishes the pending draft nor adds one.
		w = s.do(t, http.MethodPatch, path, toolSetPatchBody(nil, map[string]any{"release": map[string]any{"desiredState": "primary"}}), all)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Equal(t, uint64(1), decodeToolSet(t, w).Metadata.Generation)

		// Publishing the draft promotes it and demotes the previous primary.
		w = s.do(t, http.MethodPatch, path+"/generations/2", toolSetPatchBody(nil, map[string]any{"release": map[string]any{"desiredState": "primary"}}), all)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		published := decodeToolSet(t, w)
		require.Equal(t, uint64(2), published.Metadata.Generation)
		require.Equal(t, toolsetschema.ToolSetReleaseStatePrimary, published.Status.Release.State)

		generations := decodeToolSetList(t, s.do(t, http.MethodGet, path+"/generations", nil, all))
		require.Equal(t, []string{"2:primary", "1:active"}, generationStates(generations))
		for _, item := range generations.Items {
			require.Equal(t, toolsetschema.ToolSetReleaseStatePrimary, item.Spec.Release.DesiredState, "published generations keep primary intent")
		}

		// A definition with primary intent at the logical endpoint drafts and
		// publishes in one request.
		w = s.do(t, http.MethodPatch, path, toolSetPatchBody(nil, map[string]any{
			"definition": changed,
			"release":    map[string]any{"desiredState": "primary"},
		}), all)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Equal(t, uint64(3), decodeToolSet(t, w).Metadata.Generation)
		generations = decodeToolSetList(t, s.do(t, http.MethodGet, path+"/generations", nil, all))
		require.Equal(t, []string{"3:primary", "2:active", "1:active"}, generationStates(generations))
	})

	t.Run("generation endpoints edit drafts only and project the current selector", func(t *testing.T) {
		s, _ := newToolsTestSetup(t)
		created := decodeToolSet(t, s.do(t, http.MethodPost, "/toolsets", toolSetBody("root.acme", "calendar", "primary"), all))
		path := "/toolsets/" + created.Metadata.ID

		// An empty body clones the newest generation into a draft.
		w := s.do(t, http.MethodPost, path+"/generations", nil, all)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		draft := decodeToolSet(t, w)
		require.Equal(t, uint64(2), draft.Metadata.Generation)
		require.Equal(t, toolsetschema.ToolSetReleaseStateDraft, draft.Spec.Release.DesiredState)
		require.Equal(t, firstTemplateURL(created), firstTemplateURL(draft))

		w = s.do(t, http.MethodPost, path+"/generations", nil, all)
		require.Equal(t, http.StatusConflict, w.Code, "only one draft may exist")

		// Logical fields cannot be written through a generation.
		w = s.do(t, http.MethodPatch, path+"/generations/2", toolSetPatchBody(nil, map[string]any{
			"connectionSelector": map[string]any{"matchLabels": map[string]string{"provider": "google"}},
		}), all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		w = s.do(t, http.MethodPatch, path+"/generations/2", toolSetPatchBody(map[string]any{"name": "renamed"}, nil), all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

		// Published definitions are immutable, but an empty patch is a no-op.
		changed := toolSetDefinition(toolSetTemplate("list-calendars", "https://{{cfg.apiHost}}/v2/calendars"))
		w = s.do(t, http.MethodPatch, path+"/generations/1", toolSetPatchBody(nil, map[string]any{"definition": changed}), all)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		w = s.do(t, http.MethodPatch, path+"/generations/1", toolSetPatchBody(nil, nil), all)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		// Edit and publish the draft in one request.
		w = s.do(t, http.MethodPatch, path+"/generations/2", toolSetPatchBody(nil, map[string]any{
			"definition": changed,
			"release":    map[string]any{"desiredState": "primary"},
		}), all)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Equal(t, toolsetschema.ToolSetReleaseStatePrimary, decodeToolSet(t, w).Status.Release.State)

		// A later selector change is projected onto every generation.
		w = s.do(t, http.MethodPatch, path, toolSetPatchBody(nil, map[string]any{
			"connectionSelector": map[string]any{"matchLabels": map[string]string{"provider": "google"}},
		}), all)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		old := decodeToolSet(t, s.do(t, http.MethodGet, path+"/generations/1", nil, all))
		require.Equal(t, toolsetschema.ToolSetReleaseStateActive, old.Status.Release.State)
		require.Equal(t, map[string]string{"provider": "google"}, old.Spec.ConnectionSelector.MatchLabels)

		// A generation patch can create and publish a new draft directly.
		w = s.do(t, http.MethodPost, path+"/generations", toolSetPatchBody(nil, map[string]any{
			"definition": toolSetDefinition(),
			"release":    map[string]any{"desiredState": "primary"},
		}), all)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		require.Equal(t, uint64(3), decodeToolSet(t, w).Metadata.Generation)
		generations := decodeToolSetList(t, s.do(t, http.MethodGet, path+"/generations?limit=2", nil, all))
		require.Equal(t, []string{"3:primary", "2:active"}, generationStates(generations))
		require.NotEmpty(t, generations.Metadata.Continue)
		next := decodeToolSetList(t, s.do(t, http.MethodGet, path+"/generations?cursor="+url.QueryEscape(generations.Metadata.Continue), nil, all))
		require.Equal(t, []string{"1:active"}, generationStates(next))

		w = s.do(t, http.MethodGet, path+"/generations/9", nil, all)
		require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
		w = s.do(t, http.MethodGet, path+"/generations/zero", nil, all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})

	t.Run("list filters and paginates within the caller's scope", func(t *testing.T) {
		s, _ := newToolsTestSetup(t)
		for _, item := range []struct{ namespace, name string }{
			{"root.acme", "calendar"}, {"root.acme", "crm"}, {"root.acme", "mail"}, {"root.other", "calendar"},
		} {
			w := s.do(t, http.MethodPost, "/toolsets", toolSetBody(item.namespace, item.name, ""), all)
			require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		}

		page := decodeToolSetList(t, s.do(t, http.MethodGet, "/toolsets?namespace=root.acme&limit=2", nil, all))
		require.Len(t, page.Items, 2)
		require.NotEmpty(t, page.Metadata.Continue)
		next := decodeToolSetList(t, s.do(t, http.MethodGet, "/toolsets?cursor="+url.QueryEscape(page.Metadata.Continue), nil, all))
		require.Len(t, next.Items, 1)
		require.Empty(t, next.Metadata.Continue)

		named := decodeToolSetList(t, s.do(t, http.MethodGet, "/toolsets?name=calendar", nil, all))
		require.Len(t, named.Items, 2)

		scoped := decodeToolSetList(t, s.do(t, http.MethodGet, "/toolsets", nil, aschema.PermissionsSingle("root.other.**", "toolsets", "list")))
		require.Len(t, scoped.Items, 1)
		require.Equal(t, "root.other", scoped.Items[0].Metadata.Namespace)

		w := s.do(t, http.MethodGet, "/toolsets?limit=0", nil, all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})

	t.Run("write responses match stored timestamps", func(t *testing.T) {
		s, _ := newToolsTestSetup(t)

		// A nanosecond clock exposes sub-microsecond values PostgreSQL cannot store.
		ctx := apctx.WithFixedClock(context.Background(), time.Date(2026, 10, 30, 12, 0, 0, 123456789, time.UTC))

		// The create response carries the same timestamps as a later read.
		var body toolsetschema.ToolSet
		encoded, err := json.Marshal(toolSetBody("root.acme", "precise", "primary"))
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(encoded, &body))
		created, err := s.Core.CreateToolSet(ctx, &body)
		require.NoError(t, err)
		stored, err := s.Core.GetToolSet(ctx, created.GetId())
		require.NoError(t, err)
		require.Equal(t, stored.GetResource().Metadata, created.GetResource().Metadata)

		// A metadata update and a new draft do too.
		patch := toolsetschema.NewToolSetPatch()
		name := scommon.ResourceName("renamed")
		patch.Metadata.Name = &name
		updated, err := s.Core.UpdateToolSet(ctx, created.GetId(), patch)
		require.NoError(t, err)
		stored, err = s.Core.GetToolSet(ctx, created.GetId())
		require.NoError(t, err)
		require.Equal(t, stored.GetResource().Metadata, updated.GetResource().Metadata)

		draft, err := s.Core.CreateToolSetGeneration(ctx, created.GetId(), nil)
		require.NoError(t, err)
		stored, err = s.Core.GetToolSetGeneration(ctx, created.GetId(), draft.GetGeneration())
		require.NoError(t, err)
		require.Equal(t, stored.GetResource().Metadata, draft.GetResource().Metadata)
	})

	t.Run("delete is idempotent and frees the name", func(t *testing.T) {
		s, _ := newToolsTestSetup(t)
		created := decodeToolSet(t, s.do(t, http.MethodPost, "/toolsets", toolSetBody("root.acme", "calendar", "primary"), all))
		path := "/toolsets/" + created.Metadata.ID

		w := s.do(t, http.MethodDelete, path, nil, aschema.PermissionsSingle("root.other.**", "toolsets", "delete"))
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

		require.Equal(t, http.StatusNoContent, s.do(t, http.MethodDelete, path, nil, all).Code)
		require.Equal(t, http.StatusNoContent, s.do(t, http.MethodDelete, path, nil, all).Code)
		require.Equal(t, http.StatusNotFound, s.do(t, http.MethodGet, path, nil, all).Code)
		require.Equal(t, http.StatusNotFound, s.do(t, http.MethodGet, path+"/generations/1", nil, all).Code)

		w = s.do(t, http.MethodPost, "/toolsets", toolSetBody("root.acme", "calendar", ""), all)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	})
}
