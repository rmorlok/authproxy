package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

// bindToolSet creates a primary ToolSet and a pending assignment to a connection.
func bindToolSet(t *testing.T, s *toolsTestSetup, namespace, name string, connectionId apid.ID) string {
	t.Helper()
	w := s.do(t, http.MethodPost, "/toolsets", toolSetBody(namespace, name, "primary"), aschema.AllPermissions())
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	toolSetId := decodeToolSet(t, w).Metadata.ID

	ctx := context.Background()
	stored, err := s.Db.GetToolSet(ctx, apid.ID(toolSetId))
	require.NoError(t, err)
	_, err = s.Db.CreateToolSetBinding(ctx, stored.Id, connectionId, stored.SelectorRevision)
	require.NoError(t, err)
	return toolSetId
}

// decodeBindingList reads a binding list response body.
func decodeBindingList(t *testing.T, w *httptest.ResponseRecorder) schemaapi.ConnectionToolSetBindingList {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list schemaapi.ConnectionToolSetBindingList
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list), w.Body.String())
	return list
}

// boundToolSetIds summarizes a binding list by its parent ToolSets.
func boundToolSetIds(list schemaapi.ConnectionToolSetBindingList) []string {
	ids := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		ids = append(ids, item.Spec.ToolSetRef.ID)
	}
	return ids
}

// connectionGet grants connections:get on one namespace matcher.
func connectionGet(namespace string) []aschema.Permission {
	return []aschema.Permission{{Namespace: namespace, Resources: []string{"connections"}, Verbs: []string{"get"}}}
}

// toolSetGet grants toolsets:get on a namespace matcher, optionally for fixed IDs.
func toolSetGet(namespace string, ids ...string) []aschema.Permission {
	return []aschema.Permission{{Namespace: namespace, Resources: []string{"toolsets"}, Verbs: []string{"get"}, ResourceIds: ids}}
}

func TestConnectionToolSetBindings(t *testing.T) {
	all := aschema.AllPermissions()

	t.Run("reports each pending assignment without internal identity", func(t *testing.T) {
		s, connections := newToolsTestSetup(t)
		calendar := connections["calendar"]

		// A connection with nothing assigned returns an empty array, not null.
		empty := s.do(t, http.MethodGet, "/connections/"+calendar.Id.String()+"/toolsets", nil, all)
		require.Equal(t, http.StatusOK, empty.Code, empty.Body.String())
		require.Contains(t, empty.Body.String(), `"items":[]`)

		toolSetId := bindToolSet(t, s, "root.acme", "calendar-tools", calendar.Id)

		w := s.do(t, http.MethodGet, "/connections/"+calendar.Id.String()+"/toolsets", nil, all)
		list := decodeBindingList(t, w)
		require.Len(t, list.Items, 1)
		binding := list.Items[0]

		// The projection identifies the pair by public references only.
		require.Equal(t, schemaapi.ConnectionToolSetBindingKind, binding.Kind)
		require.Equal(t, "root.acme", binding.Metadata.Namespace)
		require.Empty(t, binding.Metadata.ID)
		require.Equal(t, calendar.Id.String(), binding.Spec.ConnectionRef.ID)
		require.Equal(t, toolSetId, binding.Spec.ToolSetRef.ID)
		require.Equal(t, "calendar-tools", string(binding.Spec.ToolSetRef.Name))
		require.Equal(t, "root.acme", binding.Spec.ToolSetRef.Namespace)
		require.NotContains(t, w.Body.String(), string(apid.PrefixToolSetBinding), "the internal binding ID stays private")

		// Nothing has been installed, and that is reported rather than omitted.
		require.Nil(t, binding.Status.AppliedGeneration)
		require.Len(t, binding.Status.Conditions, 1)
		require.Equal(t, schemaapi.ToolSetBindingConditionInstalled, binding.Status.Conditions[0].Type)
		require.Equal(t, meta.ConditionFalse, binding.Status.Conditions[0].Status)
		require.Equal(t, schemaapi.ToolSetBindingReasonInventoryPending, binding.Status.Conditions[0].Reason)
	})

	t.Run("lists only the addressed connection", func(t *testing.T) {
		s, connections := newToolsTestSetup(t)
		calendarSet := bindToolSet(t, s, "root.acme", "calendar-tools", connections["calendar"].Id)
		bindToolSet(t, s, "root.acme", "crm-tools", connections["crm"].Id)

		w := s.do(t, http.MethodGet, "/connections/"+connections["calendar"].Id.String()+"/toolsets", nil, all)
		require.Equal(t, []string{calendarSet}, boundToolSetIds(decodeBindingList(t, w)))
	})

	t.Run("requires connection access", func(t *testing.T) {
		s, connections := newToolsTestSetup(t)
		bindToolSet(t, s, "root.acme", "calendar-tools", connections["calendar"].Id)
		path := "/connections/" + connections["calendar"].Id.String() + "/toolsets"

		// ToolSet read access alone cannot inspect a connection.
		require.Equal(t, http.StatusForbidden, s.do(t, http.MethodGet, path, nil, toolSetGet("root.**")).Code)

		// Connection access in another namespace does not reach this connection.
		denied := append(connectionGet("root.other"), toolSetGet("root.**")...)
		require.Equal(t, http.StatusForbidden, s.do(t, http.MethodGet, path, nil, denied).Code)
	})

	t.Run("omits ToolSets the caller cannot read", func(t *testing.T) {
		s, connections := newToolsTestSetup(t)
		calendar := connections["calendar"]
		shared := bindToolSet(t, s, "root", "shared-tools", calendar.Id)
		owned := bindToolSet(t, s, "root.acme", "calendar-tools", calendar.Id)
		path := "/connections/" + calendar.Id.String() + "/toolsets"

		// Connection access alone reveals no ToolSets, and is not an error.
		list := decodeBindingList(t, s.do(t, http.MethodGet, path, nil, connectionGet("root.**")))
		require.Empty(t, list.Items)

		// A namespace grant hides a ToolSet owned by an ancestor namespace.
		permissions := append(connectionGet("root.**"), toolSetGet("root.acme")...)
		list = decodeBindingList(t, s.do(t, http.MethodGet, path, nil, permissions))
		require.Equal(t, []string{owned}, boundToolSetIds(list))

		// An ID restriction is enforced row by row within an allowed namespace.
		permissions = append(connectionGet("root.**"), toolSetGet("root.**", shared)...)
		list = decodeBindingList(t, s.do(t, http.MethodGet, path, nil, permissions))
		require.Equal(t, []string{shared}, boundToolSetIds(list))
	})

	t.Run("pages every binding once with a keyset cursor", func(t *testing.T) {
		s, connections := newToolsTestSetup(t)
		calendar := connections["calendar"]
		path := "/connections/" + calendar.Id.String() + "/toolsets"

		var expected []string
		for i := range 3 {
			expected = append(expected, bindToolSet(t, s, "root.acme", fmt.Sprintf("tools-%d", i), calendar.Id))
		}

		// Walk one row at a time until the server stops issuing a cursor.
		var seen []string
		next := path + "?limit=1"
		for next != "" {
			list := decodeBindingList(t, s.do(t, http.MethodGet, next, nil, all))
			require.Len(t, list.Items, 1)
			seen = append(seen, boundToolSetIds(list)...)
			next = ""
			if list.Metadata.Continue != "" {
				next = path + "?cursor=" + url.QueryEscape(list.Metadata.Continue)
			}
		}
		require.ElementsMatch(t, expected, seen)
	})

	t.Run("a cursor cannot cross connections", func(t *testing.T) {
		s, connections := newToolsTestSetup(t)
		calendar, crm := connections["calendar"], connections["crm"]
		for i := range 2 {
			bindToolSet(t, s, "root.acme", fmt.Sprintf("tools-%d", i), calendar.Id)
		}

		// Spend a cursor from one connection on a different connection's route.
		list := decodeBindingList(t, s.do(t, http.MethodGet, "/connections/"+calendar.Id.String()+"/toolsets?limit=1", nil, all))
		require.NotEmpty(t, list.Metadata.Continue)
		w := s.do(t, http.MethodGet, "/connections/"+crm.Id.String()+"/toolsets?cursor="+url.QueryEscape(list.Metadata.Continue), nil, all)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})

	t.Run("rejects bad input and missing connections", func(t *testing.T) {
		s, connections := newToolsTestSetup(t)
		path := "/connections/" + connections["calendar"].Id.String() + "/toolsets"

		// Paging input is validated before anything is read.
		require.Equal(t, http.StatusBadRequest, s.do(t, http.MethodGet, path+"?limit=0", nil, all).Code)
		require.Equal(t, http.StatusBadRequest, s.do(t, http.MethodGet, path+"?cursor=nope", nil, all).Code)

		// Wrong ID types and unknown connections are distinguished.
		require.Equal(t, http.StatusBadRequest, s.do(t, http.MethodGet, "/connections/"+apid.New(apid.PrefixTool).String()+"/toolsets", nil, all).Code)
		require.Equal(t, http.StatusNotFound, s.do(t, http.MethodGet, "/connections/"+apid.New(apid.PrefixConnection).String()+"/toolsets", nil, all).Code)
	})

	t.Run("a deleted ToolSet disappears", func(t *testing.T) {
		s, connections := newToolsTestSetup(t)
		calendar := connections["calendar"]
		toolSetId := bindToolSet(t, s, "root.acme", "calendar-tools", calendar.Id)

		require.Equal(t, http.StatusNoContent, s.do(t, http.MethodDelete, "/toolsets/"+toolSetId, nil, all).Code)

		// The binding row awaits cleanup, but a deleted parent is never reported.
		w := s.do(t, http.MethodGet, "/connections/"+calendar.Id.String()+"/toolsets", nil, all)
		require.Empty(t, decodeBindingList(t, w).Items)
	})
}

