package database

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
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
