package database

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
)

// TestListTools pages management identities with namespace, name, connection,
// and label filters, and keeps filters fixed across cursor continuation.
func TestListTools(t *testing.T) {
	ctx, db, base, definition := standaloneToolForTest(t)
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.owner.child"}))
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.owner_x"}))
	childConnection := standaloneToolConnectionForTest(t, ctx, db, "root.owner.child")
	otherConnection := standaloneToolConnectionForTest(t, ctx, db, "root.owner_x")

	create := func(namespace string, connection apid.ID, name common.ResourceName, labels Labels) *Tool {
		tool := &Tool{
			Id:           apid.New(apid.PrefixTool),
			Namespace:    namespace,
			Name:         name,
			ConnectionId: connection,
			Description:  base.Description,
			Verbs:        base.Verbs,
			Labels:       labels,
		}
		_, err := db.CreateStandaloneTool(ctx, tool, definition.EncryptedDefinition, definition.DefinitionDigest)
		require.NoError(t, err)
		return tool
	}

	tagged := create(base.Namespace, base.ConnectionId, "tagged", Labels{"purpose": "lookup"})
	child := create("root.owner.child", childConnection.Id, "child", nil)
	sibling := create("root.owner_x", otherConnection.Id, "sibling", nil)

	ids := func(page ToolListPage) []apid.ID {
		return mapToolIDsForTest(page.Items)
	}

	t.Run("empty scope matches nothing", func(t *testing.T) {
		page, err := db.ListTools(ctx, ToolListQuery{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, page.Items)
	})

	t.Run("descendant matcher excludes similarly named siblings", func(t *testing.T) {
		page, err := db.ListTools(ctx, ToolListQuery{NamespaceMatchers: []string{"root.owner.**"}, Limit: 10})
		require.NoError(t, err)
		require.ElementsMatch(t, []apid.ID{base.Id, tagged.Id, child.Id}, ids(page))
		require.NotContains(t, ids(page), sibling.Id)
	})

	t.Run("exact namespace, name, connection, and label filters intersect", func(t *testing.T) {
		page, err := db.ListTools(ctx, ToolListQuery{NamespaceMatchers: []string{"root.owner"}, Limit: 10})
		require.NoError(t, err)
		require.ElementsMatch(t, []apid.ID{base.Id, tagged.Id}, ids(page))

		page, err = db.ListTools(ctx, ToolListQuery{NamespaceMatchers: []string{"root.**"}, Name: "child", Limit: 10})
		require.NoError(t, err)
		require.Equal(t, []apid.ID{child.Id}, ids(page))

		page, err = db.ListTools(ctx, ToolListQuery{NamespaceMatchers: []string{"root.**"}, ConnectionId: otherConnection.Id, Limit: 10})
		require.NoError(t, err)
		require.Equal(t, []apid.ID{sibling.Id}, ids(page))

		page, err = db.ListTools(ctx, ToolListQuery{NamespaceMatchers: []string{"root.**"}, LabelSelector: "purpose=lookup", Limit: 10})
		require.NoError(t, err)
		require.Equal(t, []apid.ID{tagged.Id}, ids(page))

		page, err = db.ListTools(ctx, ToolListQuery{
			NamespaceMatchers: []string{"root.**"},
			LabelSelector:     "apxy/cxn/-/id=" + childConnection.Id.String(),
			Limit:             10,
		})
		require.NoError(t, err)
		require.Equal(t, []apid.ID{child.Id}, ids(page))
	})

	t.Run("cursor preserves filters and visits every tool once", func(t *testing.T) {
		query := ToolListQuery{NamespaceMatchers: []string{"root.**"}, Limit: 1}
		page, err := db.ListTools(ctx, query)
		require.NoError(t, err)
		seen := ids(page)
		for page.Cursor != "" {
			page, err = db.ListToolsFromCursor(ctx, page.Cursor)
			require.NoError(t, err)
			require.LessOrEqual(t, len(page.Items), 1)
			seen = append(seen, ids(page)...)
		}
		require.ElementsMatch(t, []apid.ID{base.Id, tagged.Id, child.Id, sibling.Id}, seen)
	})

	t.Run("deleted tools are omitted", func(t *testing.T) {
		require.NoError(t, db.DeleteStandaloneTool(ctx, tagged.Id, tagged.ActiveRevision))
		page, err := db.ListTools(ctx, ToolListQuery{NamespaceMatchers: []string{"root.owner"}, Limit: 10})
		require.NoError(t, err)
		require.Equal(t, []apid.ID{base.Id}, ids(page))
	})

	t.Run("invalid queries fail", func(t *testing.T) {
		for _, query := range []ToolListQuery{
			{NamespaceMatchers: []string{"root"}},
			{NamespaceMatchers: []string{"root"}, Limit: MaxToolListLimit + 1},
			{NamespaceMatchers: []string{"not a namespace"}, Limit: 1},
			{NamespaceMatchers: []string{"root"}, ConnectionId: apid.New(apid.PrefixTool), Limit: 1},
			{NamespaceMatchers: []string{"root"}, LabelSelector: "=", Limit: 1},
		} {
			_, err := db.ListTools(ctx, query)
			require.Error(t, err, "%+v", query)
		}
		_, err := db.ListToolsFromCursor(ctx, "not-a-cursor")
		require.Error(t, err)
	})
}

// mapToolIDsForTest projects listed identities for order-insensitive checks.
func mapToolIDsForTest(tools []Tool) []apid.ID {
	result := make([]apid.ID, 0, len(tools))
	for _, tool := range tools {
		result = append(result, tool.Id)
	}
	return result
}

// TestListToolsOrdering sorts management Tools, breaks ties by ID, keeps the
// ordering across cursor pages, and rejects unknown columns.
func TestListToolsOrdering(t *testing.T) {
	ctx, db, base, definition := standaloneToolForTest(t)

	// Create Tools whose names sort differently from their creation order.
	create := func(name common.ResourceName, offset time.Duration) *Tool {
		tool := &Tool{
			Id:           apid.New(apid.PrefixTool),
			Namespace:    base.Namespace,
			Name:         name,
			ConnectionId: base.ConnectionId,
			Description:  base.Description,
			Verbs:        base.Verbs,
		}
		at := apctx.WithFixedClock(ctx, apctx.GetClock(ctx).Now().Add(offset))
		_, err := db.CreateStandaloneTool(at, tool, definition.EncryptedDefinition, definition.DefinitionDigest)
		require.NoError(t, err)
		return tool
	}
	zulu := create("zulu", time.Hour)
	alpha := create("alpha", 2*time.Hour)

	list := func(field ToolOrderByField, order pagination.OrderBy, limit int) ToolListPage {
		page, err := db.ListTools(ctx, ToolListQuery{
			NamespaceMatchers: []string{base.Namespace},
			Limit:             limit,
			OrderByField:      field,
			OrderBy:           order,
		})
		require.NoError(t, err)
		return page
	}

	// created_at sorts in both directions; the base Tool is oldest.
	require.Equal(t, []apid.ID{base.Id, zulu.Id, alpha.Id}, mapToolIDsForTest(list(ToolOrderByCreatedAt, pagination.OrderByAsc, 10).Items))
	require.Equal(t, []apid.ID{alpha.Id, zulu.Id, base.Id}, mapToolIDsForTest(list(ToolOrderByCreatedAt, pagination.OrderByDesc, 10).Items))

	// A descending name sort continues correctly through the cursor.
	page := list(ToolOrderByName, pagination.OrderByDesc, 1)
	require.Equal(t, []apid.ID{zulu.Id}, mapToolIDsForTest(page.Items))
	next, err := db.ListToolsFromCursor(ctx, page.Cursor)
	require.NoError(t, err)
	require.NotContains(t, mapToolIDsForTest(next.Items), zulu.Id)

	// Equal namespaces tie-break in the database's own ID order, which follows
	// its collation rather than Go's byte order.
	require.Equal(t,
		mapToolIDsForTest(list(ToolOrderById, pagination.OrderByAsc, 10).Items),
		mapToolIDsForTest(list(ToolOrderByNamespace, pagination.OrderByAsc, 10).Items),
	)

	// Unknown columns never reach SQL.
	_, err = db.ListTools(ctx, ToolListQuery{
		NamespaceMatchers: []string{base.Namespace},
		Limit:             1,
		OrderByField:      "name; DROP TABLE tools",
	})
	require.Error(t, err)
}
