package database

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
)

// bindingToolSetIDsForTest projects listed assignments by their parent ToolSet.
func bindingToolSetIDsForTest(bindings []ToolSetBindingListing) []apid.ID {
	ids := make([]apid.ID, 0, len(bindings))
	for i := range bindings {
		ids = append(ids, bindings[i].ToolSetId)
	}
	return ids
}

// TestListToolSetBindings pages one connection's assignments, scoped by the parent
// ToolSet's namespace, and keeps its position through a keyset cursor.
func TestListToolSetBindings(t *testing.T) {
	ctx, db, first, connection := toolSetBindingParentsForTest(t)

	// A second ToolSet, owned by the connection's own namespace, joins the ancestor.
	second := toolSetForDatabaseTest("root.product.owner", "more-tools")
	second.ConnectionSelector.MatchLabels = map[string]string{"env": "prod"}
	require.NoError(t, db.CreateToolSet(ctx, second))
	second, err := db.GetToolSet(ctx, second.Id)
	require.NoError(t, err)

	for _, toolSet := range []*ToolSet{first, second} {
		_, err := db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, toolSet.SelectorRevision)
		require.NoError(t, err)
	}

	// list starts a listing of this test's connection with broad ToolSet scope.
	list := func() ListToolSetBindingsBuilder {
		return db.ListToolSetBindingsBuilder().
			ForConnectionId(connection.Id).
			ForToolSetNamespaceMatchers([]string{"root.**"})
	}

	t.Run("reads both parents' identity with the binding", func(t *testing.T) {
		page := list().ForToolSetNamespaceMatchers([]string{"root.product"}).FetchPage(ctx)
		require.NoError(t, page.Error)
		require.Len(t, page.Results, 1)

		require.Equal(t, first.Id, page.Results[0].ToolSetId)
		require.Equal(t, "tools", string(page.Results[0].ToolSetName))
		require.Equal(t, "root.product", page.Results[0].ToolSetNamespace)
		require.Equal(t, "root.product.owner", page.Results[0].ConnectionNamespace)
	})

	t.Run("an empty scope matches nothing", func(t *testing.T) {
		page := db.ListToolSetBindingsBuilder().ForConnectionId(connection.Id).FetchPage(ctx)
		require.NoError(t, page.Error)
		require.Empty(t, page.Results)
	})

	t.Run("cursor visits every binding once", func(t *testing.T) {
		page := list().Limit(1).FetchPage(ctx)
		require.NoError(t, page.Error)
		require.True(t, page.HasMore)
		seen := bindingToolSetIDsForTest(page.Results)

		// The cursor keeps the connection, scope, and position.
		listing, err := db.ListToolSetBindingsFromCursor(ctx, page.Cursor)
		require.NoError(t, err)
		page = listing.FetchPage(ctx)
		require.NoError(t, page.Error)
		require.False(t, page.HasMore)
		require.Empty(t, page.Cursor)

		seen = append(seen, bindingToolSetIDsForTest(page.Results)...)
		require.ElementsMatch(t, []apid.ID{first.Id, second.Id}, seen)
	})

	t.Run("enumerate visits every page and stops on request", func(t *testing.T) {
		var seen []apid.ID
		err := list().Limit(1).Enumerate(ctx, func(page pagination.PageResult[ToolSetBindingListing]) (pagination.KeepGoing, error) {
			seen = append(seen, bindingToolSetIDsForTest(page.Results)...)
			return pagination.Continue, nil
		})
		require.NoError(t, err)
		require.ElementsMatch(t, []apid.ID{first.Id, second.Id}, seen)

		pages := 0
		err = list().Limit(1).Enumerate(ctx, func(pagination.PageResult[ToolSetBindingListing]) (pagination.KeepGoing, error) {
			pages++
			return pagination.Stop, nil
		})
		require.NoError(t, err)
		require.Equal(t, 1, pages)
	})

	t.Run("invalid filters report ErrInvalidListQuery", func(t *testing.T) {
		for _, listing := range []ListToolSetBindingsBuilder{
			db.ListToolSetBindingsBuilder().ForToolSetNamespaceMatchers([]string{"root"}),
			list().ForConnectionId(apid.New(apid.PrefixTool)),
			list().ForToolSetNamespaceMatchers([]string{"not a namespace"}),
		} {
			require.ErrorIs(t, listing.FetchPage(ctx).Error, ErrInvalidListQuery)
		}

		// A malformed cursor fails before any listing exists.
		_, err := db.ListToolSetBindingsFromCursor(ctx, "not-a-cursor")
		require.Error(t, err)
	})

	t.Run("a deleted parent hides its binding", func(t *testing.T) {
		require.NoError(t, db.DeleteToolSet(ctx, second.Id))

		page := list().FetchPage(ctx)
		require.NoError(t, page.Error)
		require.Equal(t, []apid.ID{first.Id}, bindingToolSetIDsForTest(page.Results))
	})
}

// TestListToolSetBindingsReportsInstallation shows the applied generation after
// an inventory install, which is what the status projection reads.
func TestListToolSetBindingsReportsInstallation(t *testing.T) {
	fixture := explicitInventoryForTest(t, 1)
	_, err := fixture.db.InstallExplicitToolSetInventory(fixture.ctx, fixture.request)
	require.NoError(t, err)

	page := fixture.db.ListToolSetBindingsBuilder().
		ForConnectionId(fixture.connection.Id).
		ForToolSetNamespaceMatchers([]string{"root.**"}).
		FetchPage(fixture.ctx)
	require.NoError(t, page.Error)
	require.Len(t, page.Results, 1)
	require.NotNil(t, page.Results[0].AppliedGeneration)
	require.Equal(t, fixture.request.Generation, *page.Results[0].AppliedGeneration)
}

// TestListToolSetBindingsForToolSet pages one ToolSet's assignments by connection
// and keeps its position when bindings are removed between pages.
func TestListToolSetBindingsForToolSet(t *testing.T) {
	ctx, db, toolSet, connection := toolSetBindingParentsForTest(t)

	// Bind three connections, the first being the shared fixture's.
	connections := []*Connection{connection}
	for _, name := range []string{"second", "third"} {
		other := *connection
		other.Id = apid.New(apid.PrefixConnection)
		other.Name = common.ResourceName(name)
		require.NoError(t, db.CreateConnection(ctx, &other))
		connections = append(connections, &other)
	}
	for _, parent := range connections {
		_, err := db.CreateToolSetBinding(ctx, toolSet.Id, parent.Id, toolSet.SelectorRevision)
		require.NoError(t, err)
	}

	builder := db.ListToolSetBindingsBuilder().
		ForToolSetId(toolSet.Id).
		ForToolSetNamespaceMatchers([]string{toolSet.Namespace}).
		Limit(1)

	// The first page holds one binding and offers a continuation.
	first := builder.FetchPage(ctx)
	require.NoError(t, first.Error)
	require.Len(t, first.Results, 1)
	require.True(t, first.HasMore)

	// Removing the listed binding must not make the next page skip a connection.
	listed := first.Results[0]
	_, err := db.UpdateConnectionLabels(ctx, listed.ConnectionId, map[string]string{"env": "staging"})
	require.NoError(t, err)
	require.NoError(t, db.DeleteToolSetBinding(ctx, listed.Id, listed.Epoch, toolSet.SelectorRevision))

	// Continuing from the cursor visits both remaining connections exactly once.
	// ID order follows the database's collation, so only membership is asserted.
	resumed, err := db.ListToolSetBindingsFromCursor(ctx, first.Cursor)
	require.NoError(t, err)
	remaining := []apid.ID{}
	require.NoError(t, resumed.Enumerate(ctx, func(page pagination.PageResult[ToolSetBindingListing]) (pagination.KeepGoing, error) {
		for i := range page.Results {
			remaining = append(remaining, page.Results[i].ConnectionId)
		}
		return pagination.Continue, nil
	}))

	expected := []apid.ID{}
	for _, parent := range connections {
		if parent.Id != listed.ConnectionId {
			expected = append(expected, parent.Id)
		}
	}
	require.ElementsMatch(t, expected, remaining)

	// Choosing no parent, or both, is an invalid query.
	neither := db.ListToolSetBindingsBuilder().
		ForToolSetNamespaceMatchers([]string{"root.**"}).
		FetchPage(ctx)
	require.ErrorIs(t, neither.Error, ErrInvalidListQuery)
	both := db.ListToolSetBindingsBuilder().
		ForConnectionId(connection.Id).
		ForToolSetId(toolSet.Id).
		ForToolSetNamespaceMatchers([]string{"root.**"}).
		FetchPage(ctx)
	require.ErrorIs(t, both.Error, ErrInvalidListQuery)
}
