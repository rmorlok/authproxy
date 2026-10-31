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

	// fetch returns one page's IDs and fails the test on a listing error.
	fetch := func(listing ListToolsExecutor) []apid.ID {
		page := listing.FetchPage(ctx)
		require.NoError(t, page.Error)
		return mapToolIDsForTest(page.Results)
	}

	t.Run("empty scope matches nothing", func(t *testing.T) {
		require.Empty(t, fetch(db.ListToolsBuilder().Limit(10)))
	})

	t.Run("descendant matcher excludes similarly named siblings", func(t *testing.T) {
		ids := fetch(db.ListToolsBuilder().ForNamespaceMatcher("root.owner.**").Limit(10))
		require.ElementsMatch(t, []apid.ID{base.Id, tagged.Id, child.Id}, ids)
		require.NotContains(t, ids, sibling.Id)
	})

	t.Run("exact namespace, name, connection, and label filters intersect", func(t *testing.T) {
		all := func() ListToolsBuilder { return db.ListToolsBuilder().ForNamespaceMatcher("root.**").Limit(10) }

		// An exact matcher leaves out descendants.
		require.ElementsMatch(t, []apid.ID{base.Id, tagged.Id}, fetch(db.ListToolsBuilder().ForNamespaceMatcher("root.owner").Limit(10)))

		// Each optional filter narrows the full scope to one Tool.
		require.Equal(t, []apid.ID{child.Id}, fetch(all().ForName("child")))
		require.Equal(t, []apid.ID{sibling.Id}, fetch(all().ForConnectionId(otherConnection.Id)))
		require.Equal(t, []apid.ID{tagged.Id}, fetch(all().ForLabelSelector("purpose=lookup")))
		require.Equal(t, []apid.ID{child.Id}, fetch(all().ForLabelSelector("apxy/cxn/-/id="+childConnection.Id.String())))
	})

	t.Run("a later scope replaces an earlier one", func(t *testing.T) {
		// Scope is replaced, so a second call can never widen the first.
		ids := fetch(db.ListToolsBuilder().ForNamespaceMatcher("root.**").ForNamespaceMatchers([]string{"root.owner_x"}).Limit(10))
		require.Equal(t, []apid.ID{sibling.Id}, ids)
	})

	t.Run("cursor preserves filters and visits every tool once", func(t *testing.T) {
		page := db.ListToolsBuilder().ForNamespaceMatcher("root.**").Limit(1).FetchPage(ctx)
		require.NoError(t, page.Error)
		seen := mapToolIDsForTest(page.Results)
		for page.HasMore {
			listing, err := db.ListToolsFromCursor(ctx, page.Cursor)
			require.NoError(t, err)
			page = listing.FetchPage(ctx)
			require.NoError(t, page.Error)
			require.LessOrEqual(t, len(page.Results), 1)
			seen = append(seen, mapToolIDsForTest(page.Results)...)
		}
		require.ElementsMatch(t, []apid.ID{base.Id, tagged.Id, child.Id, sibling.Id}, seen)
		require.Empty(t, page.Cursor)
	})

	t.Run("enumerate visits every page", func(t *testing.T) {
		var seen []apid.ID
		err := db.ListToolsBuilder().ForNamespaceMatcher("root.**").Limit(1).
			Enumerate(ctx, func(page pagination.PageResult[Tool]) (pagination.KeepGoing, error) {
				seen = append(seen, mapToolIDsForTest(page.Results)...)
				return pagination.Continue, nil
			})
		require.NoError(t, err)
		require.ElementsMatch(t, []apid.ID{base.Id, tagged.Id, child.Id, sibling.Id}, seen)
	})

	t.Run("enumerate stops when the callback does", func(t *testing.T) {
		pages := 0
		err := db.ListToolsBuilder().ForNamespaceMatcher("root.**").Limit(1).
			Enumerate(ctx, func(pagination.PageResult[Tool]) (pagination.KeepGoing, error) {
				pages++
				return pagination.Stop, nil
			})
		require.NoError(t, err)
		require.Equal(t, 1, pages)
	})

	t.Run("deleted tools are omitted", func(t *testing.T) {
		require.NoError(t, db.DeleteStandaloneTool(ctx, tagged.Id, tagged.ActiveRevision))
		require.Equal(t, []apid.ID{base.Id}, fetch(db.ListToolsBuilder().ForNamespaceMatcher("root.owner").Limit(10)))
	})

	t.Run("invalid filters report ErrInvalidListQuery", func(t *testing.T) {
		for _, listing := range []ListToolsBuilder{
			db.ListToolsBuilder().ForNamespaceMatcher("not a namespace"),
			db.ListToolsBuilder().ForNamespaceMatcher("root").ForConnectionId(apid.New(apid.PrefixTool)),
			db.ListToolsBuilder().ForNamespaceMatcher("root").ForLabelSelector("="),
		} {
			require.ErrorIs(t, listing.FetchPage(ctx).Error, ErrInvalidListQuery)
		}

		// A malformed cursor fails before any listing exists.
		_, err := db.ListToolsFromCursor(ctx, "not-a-cursor")
		require.Error(t, err)
	})
}

// TestToolListPageSize selects the default for unset sizes and caps large ones.
func TestToolListPageSize(t *testing.T) {
	require.Equal(t, uint64(defaultToolListLimit), toolListPageSize(0, MaxToolListLimit))
	require.Equal(t, uint64(defaultToolListLimit), toolListPageSize(-1, MaxToolListLimit))
	require.Equal(t, uint64(7), toolListPageSize(7, MaxToolListLimit))
	require.Equal(t, uint64(MaxToolListLimit), toolListPageSize(MaxToolListLimit+1, MaxToolListLimit))
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

	list := func(field ToolOrderByField, order pagination.OrderBy, limit int32) pagination.PageResult[Tool] {
		page := db.ListToolsBuilder().
			ForNamespaceMatcher(base.Namespace).
			Limit(limit).
			OrderBy(field, order).
			FetchPage(ctx)
		require.NoError(t, page.Error)
		return page
	}

	// created_at sorts in both directions; the base Tool is oldest.
	require.Equal(t, []apid.ID{base.Id, zulu.Id, alpha.Id}, mapToolIDsForTest(list(ToolOrderByCreatedAt, pagination.OrderByAsc, 10).Results))
	require.Equal(t, []apid.ID{alpha.Id, zulu.Id, base.Id}, mapToolIDsForTest(list(ToolOrderByCreatedAt, pagination.OrderByDesc, 10).Results))

	// A descending name sort continues correctly through the cursor.
	page := list(ToolOrderByName, pagination.OrderByDesc, 1)
	require.Equal(t, []apid.ID{zulu.Id}, mapToolIDsForTest(page.Results))
	listing, err := db.ListToolsFromCursor(ctx, page.Cursor)
	require.NoError(t, err)
	next := listing.FetchPage(ctx)
	require.NoError(t, next.Error)
	require.NotContains(t, mapToolIDsForTest(next.Results), zulu.Id)

	// Equal namespaces tie-break in the database's own ID order, which follows
	// its collation rather than Go's byte order.
	require.Equal(t,
		mapToolIDsForTest(list(ToolOrderById, pagination.OrderByAsc, 10).Results),
		mapToolIDsForTest(list(ToolOrderByNamespace, pagination.OrderByAsc, 10).Results),
	)

	// Unknown columns never reach SQL.
	invalid := db.ListToolsBuilder().
		ForNamespaceMatcher(base.Namespace).
		OrderBy("name; DROP TABLE tools", pagination.OrderByAsc).
		FetchPage(ctx)
	require.ErrorIs(t, invalid.Error, ErrInvalidListQuery)
}
