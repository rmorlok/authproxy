package database

import (
	"context"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
)

// TestListToolSets pages logical ToolSets with namespace, name, and label
// filters, and keeps filters fixed across cursor continuation.
func TestListToolSets(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	for _, path := range []string{"root.owner", "root.owner.child", "root.owner_x"} {
		require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: path}))
	}

	create := func(namespace string, name scommon.ResourceName, labels Labels) *ToolSet {
		toolSet := toolSetForDatabaseTest(namespace, name)
		toolSet.Labels = labels
		require.NoError(t, db.CreateToolSet(ctx, toolSet))
		return toolSet
	}
	owner := create("root.owner", "calendar", Labels{"purpose": "calendar"})
	other := create("root.owner", "crm", nil)
	child := create("root.owner.child", "calendar", nil)
	sibling := create("root.owner_x", "calendar", nil)

	ids := func(page pagination.PageResult[ToolSet]) []apid.ID {
		require.NoError(t, page.Error)
		result := make([]apid.ID, 0, len(page.Results))
		for _, item := range page.Results {
			result = append(result, item.Id)
		}
		return result
	}

	t.Run("empty scope matches nothing", func(t *testing.T) {
		require.Empty(t, ids(db.ListToolSetsBuilder().Limit(10).FetchPage(ctx)))
	})

	t.Run("descendant matcher excludes similarly named siblings", func(t *testing.T) {
		page := db.
			ListToolSetsBuilder().
			ForNamespaceMatcher("root.owner.**").
			Limit(10).
			FetchPage(ctx)
		require.ElementsMatch(t, []apid.ID{owner.Id, other.Id, child.Id}, ids(page))
	})

	t.Run("name and label filters intersect", func(t *testing.T) {
		named := func() ListToolSetsBuilder {
			return db.
				ListToolSetsBuilder().
				ForNamespaceMatcher("root.**").
				ForName("calendar").
				Limit(10)
		}

		require.ElementsMatch(t, []apid.ID{owner.Id, child.Id, sibling.Id}, ids(named().FetchPage(ctx)))
		require.Equal(t, []apid.ID{owner.Id}, ids(named().ForLabelSelector("purpose=calendar").FetchPage(ctx)))
	})

	t.Run("cursor preserves filters", func(t *testing.T) {
		page := db.
			ListToolSetsBuilder().
			ForNamespaceMatcher("root.owner.**").
			Limit(2).
			FetchPage(ctx)
		require.Len(t, ids(page), 2)
		require.NotEmpty(t, page.Cursor)

		listing, err := db.ListToolSetsFromCursor(ctx, page.Cursor)
		require.NoError(t, err)
		next := listing.FetchPage(ctx)
		require.Len(t, ids(next), 1)
		require.Empty(t, next.Cursor)
		require.ElementsMatch(t, []apid.ID{owner.Id, other.Id, child.Id}, append(ids(page), ids(next)...))
	})

	t.Run("enumerate visits every page", func(t *testing.T) {
		var seen []apid.ID
		err := db.
			ListToolSetsBuilder().
			ForNamespaceMatcher("root.owner.**").
			Limit(1).
			Enumerate(ctx, func(page pagination.PageResult[ToolSet]) (pagination.KeepGoing, error) {
				seen = append(seen, ids(page)...)
				return pagination.Continue, nil
			})
		require.NoError(t, err)
		require.ElementsMatch(t, []apid.ID{owner.Id, other.Id, child.Id}, seen)
	})

	t.Run("deleted ToolSets are omitted", func(t *testing.T) {
		require.NoError(t, db.DeleteToolSet(ctx, other.Id))
		page := db.ListToolSetsBuilder().ForNamespaceMatcher("root.owner").Limit(10).FetchPage(ctx)
		require.Equal(t, []apid.ID{owner.Id}, ids(page))
	})

	t.Run("invalid queries are rejected", func(t *testing.T) {
		page := db.
			ListToolSetsBuilder().
			ForNamespaceMatcher("not a matcher").
			FetchPage(ctx)
		require.ErrorIs(t, page.Error, ErrInvalidListQuery)

		_, err := db.ListToolSetsFromCursor(ctx, "garbage")
		require.Error(t, err)
	})
}

// TestListToolSetGenerations pages one ToolSet's generations newest first and
// hides the history of a deleted parent.
func TestListToolSetGenerations(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.owner"}))

	toolSet := toolSetForDatabaseTest("root.owner", "calendar")
	require.NoError(t, db.CreateToolSet(ctx, toolSet))
	for i := 0; i < 3; i++ {
		generation, err := db.CreateToolSetGeneration(ctx, toolSet.Id, encfield.EncryptedField{
			ID:   apid.New(apid.PrefixDataEncryptionKey),
			Data: "b3BhcXVl",
		})
		require.NoError(t, err)
		_, err = db.PublishToolSetGeneration(ctx, toolSet.Id, generation.Generation, generation.Revision)
		require.NoError(t, err)
	}

	// The first page is newest first and reports more results.
	page := db.
		ListToolSetGenerationsBuilder().
		ForToolSetId(toolSet.Id).
		Limit(2).
		FetchPage(ctx)
	require.Equal(t, []uint64{3, 2}, generationNumbersForTest(t, page))
	require.NotEmpty(t, page.Cursor)

	// The cursor resumes on the last generation and ends the listing.
	listing, err := db.ListToolSetGenerationsFromCursor(ctx, page.Cursor)
	require.NoError(t, err)
	next := listing.FetchPage(ctx)
	require.Equal(t, []uint64{1}, generationNumbersForTest(t, next))
	require.Empty(t, next.Cursor)

	// Enumerate walks every generation in the same order.
	var seen []uint64
	err = db.
		ListToolSetGenerationsBuilder().
		ForToolSetId(toolSet.Id).
		Limit(1).
		Enumerate(ctx, func(page pagination.PageResult[ToolSetGeneration]) (pagination.KeepGoing, error) {
			seen = append(seen, generationNumbersForTest(t, page)...)
			return pagination.Continue, nil
		})
	require.NoError(t, err)
	require.Equal(t, []uint64{3, 2, 1}, seen)

	// A listing without a parent is invalid input.
	require.ErrorIs(t, db.ListToolSetGenerationsBuilder().FetchPage(ctx).Error, ErrInvalidListQuery)

	// Deleting the parent hides its history, even from an existing cursor.
	require.NoError(t, db.DeleteToolSet(ctx, toolSet.Id))
	require.ErrorIs(t, db.ListToolSetGenerationsBuilder().ForToolSetId(toolSet.Id).FetchPage(ctx).Error, ErrNotFound)
}

// generationNumbersForTest projects a page's generation numbers in order.
func generationNumbersForTest(t *testing.T, page pagination.PageResult[ToolSetGeneration]) []uint64 {
	t.Helper()
	require.NoError(t, page.Error)
	result := make([]uint64, 0, len(page.Results))
	for _, item := range page.Results {
		result = append(result, item.Generation)
	}
	return result
}

// TestUpdateToolSetMetadata replaces user metadata while retaining system
// labels, re-deriving identity labels, and leaving the selector untouched.
func TestUpdateToolSetMetadata(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	created := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	ctx := apctx.WithFixedClock(context.Background(), created)
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.owner", Labels: Labels{"team": "platform"}}))

	toolSet := toolSetForDatabaseTest("root.owner", "calendar")
	toolSet.Labels = Labels{"purpose": "calendar"}
	toolSet.Annotations = Annotations{"example.com/owner": "integrations"}
	require.NoError(t, db.CreateToolSet(ctx, toolSet))
	taken := toolSetForDatabaseTest("root.owner", "taken")
	require.NoError(t, db.CreateToolSet(ctx, taken))

	updatedAt := created.Add(time.Minute)
	ctx = apctx.WithFixedClock(context.Background(), updatedAt)
	updated, err := db.UpdateToolSetMetadata(ctx, toolSet.Id, "scheduling", map[string]string{"tier": "gold"}, nil)
	require.NoError(t, err)
	require.Equal(t, scommon.ResourceName("scheduling"), updated.Name)
	require.True(t, updatedAt.Equal(updated.UpdatedAt))

	stored, err := db.GetToolSet(ctx, toolSet.Id)
	require.NoError(t, err)
	require.Equal(t, "gold", stored.Labels["tier"])
	require.NotContains(t, stored.Labels, "purpose")
	require.Equal(t, "platform", stored.Labels["apxy/ns/team"])
	require.Equal(t, "scheduling", stored.Labels["apxy/tls/-/name"])
	require.Empty(t, stored.Annotations)
	require.Equal(t, uint64(1), stored.SelectorRevision)
	require.Equal(t, toolSet.ConnectionSelector, stored.ConnectionSelector)

	_, err = db.UpdateToolSetMetadata(ctx, toolSet.Id, "taken", nil, nil)
	require.ErrorIs(t, err, ErrDuplicate)

	_, err = db.UpdateToolSetMetadata(ctx, toolSet.Id, "scheduling", map[string]string{"apxy/tls/-/name": "spoof"}, nil)
	require.Error(t, err)

	require.NoError(t, db.DeleteToolSet(ctx, taken.Id))
	_, err = db.UpdateToolSetMetadata(ctx, taken.Id, "taken", nil, nil)
	require.ErrorIs(t, err, ErrNotFound)
}

// TestListToolSetsOrdering sorts logical ToolSets by each supported column
// family, breaks ties by ID, and keeps the ordering across cursor pages.
func TestListToolSetsOrdering(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	base := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	for _, path := range []string{"root.a", "root.b"} {
		require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: path}))
	}

	// Create ToolSets one minute apart, so creation order differs from name order.
	create := func(minutes int, namespace string, name scommon.ResourceName) *ToolSet {
		toolSet := toolSetForDatabaseTest(namespace, name)
		at := base.Add(time.Duration(minutes) * time.Minute)
		require.NoError(t, db.CreateToolSet(apctx.WithFixedClock(ctx, at), toolSet))
		return toolSet
	}
	first := create(0, "root.b", "beta")
	second := create(1, "root.a", "gamma")
	third := create(2, "root.a", "alpha")

	list := func(field ToolSetOrderByField, order pagination.OrderBy) []apid.ID {
		page := db.ListToolSetsBuilder().
			ForNamespaceMatcher("root.**").
			Limit(10).
			OrderBy(field, order).
			FetchPage(ctx)
		require.NoError(t, page.Error)
		result := make([]apid.ID, 0, len(page.Results))
		for _, item := range page.Results {
			result = append(result, item.Id)
		}
		return result
	}

	// Each column sorts in both directions.
	require.Equal(t, []apid.ID{first.Id, second.Id, third.Id}, list(ToolSetOrderByCreatedAt, pagination.OrderByAsc))
	require.Equal(t, []apid.ID{third.Id, second.Id, first.Id}, list(ToolSetOrderByCreatedAt, pagination.OrderByDesc))
	require.Equal(t, []apid.ID{third.Id, first.Id, second.Id}, list(ToolSetOrderByName, pagination.OrderByAsc))

	// A metadata write moves a ToolSet to the end of updated_at order.
	_, err := db.UpdateToolSetMetadata(apctx.WithFixedClock(ctx, base.Add(time.Hour)), first.Id, "beta", nil, nil)
	require.NoError(t, err)
	require.Equal(t, first.Id, list(ToolSetOrderByUpdatedAt, pagination.OrderByDesc)[0])

	// Equal namespaces tie-break in the database's own ID order, which follows
	// its collation rather than Go's byte order, across pages with no duplicates.
	var tied []apid.ID
	for _, id := range list(ToolSetOrderById, pagination.OrderByAsc) {
		if id != first.Id {
			tied = append(tied, id)
		}
	}
	page := db.ListToolSetsBuilder().
		ForNamespaceMatcher("root.**").
		Limit(2).
		OrderBy(ToolSetOrderByNamespace, pagination.OrderByDesc).
		FetchPage(ctx)
	require.NoError(t, page.Error)
	require.Len(t, page.Results, 2)
	require.Equal(t, first.Id, page.Results[0].Id)
	listing, err := db.ListToolSetsFromCursor(ctx, page.Cursor)
	require.NoError(t, err)
	next := listing.FetchPage(ctx)
	require.NoError(t, next.Error)
	require.Len(t, next.Results, 1)
	require.Empty(t, next.Cursor)
	require.Equal(t, tied, []apid.ID{page.Results[1].Id, next.Results[0].Id})

	// Unknown columns never reach SQL.
	invalid := db.ListToolSetsBuilder().
		ForNamespaceMatcher("root.**").
		OrderBy("name; DROP TABLE tool_sets", pagination.OrderByAsc).
		FetchPage(ctx)
	require.ErrorIs(t, invalid.Error, ErrInvalidListQuery)
}

// TestListToolSetGenerationsOrdering overrides the newest-first default and
// keeps the requested order through a cursor.
func TestListToolSetGenerationsOrdering(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.owner"}))
	toolSet := toolSetForDatabaseTest("root.owner", "calendar")
	require.NoError(t, db.CreateToolSet(ctx, toolSet))

	// Publish generations 1 and 2, then leave generation 3 as a draft.
	definition := encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "b3BhcXVl"}
	for i := 0; i < 3; i++ {
		generation, err := db.CreateToolSetGeneration(ctx, toolSet.Id, definition)
		require.NoError(t, err)
		if i < 2 {
			_, err = db.PublishToolSetGeneration(ctx, toolSet.Id, generation.Generation, generation.Revision)
			require.NoError(t, err)
		}
	}

	// Ascending generation order continues correctly through the cursor.
	page := db.ListToolSetGenerationsBuilder().
		ForToolSetId(toolSet.Id).
		Limit(2).
		OrderBy(ToolSetGenerationOrderByGeneration, pagination.OrderByAsc).
		FetchPage(ctx)
	require.Equal(t, []uint64{1, 2}, generationNumbersForTest(t, page))
	listing, err := db.ListToolSetGenerationsFromCursor(ctx, page.Cursor)
	require.NoError(t, err)
	require.Equal(t, []uint64{3}, generationNumbersForTest(t, listing.FetchPage(ctx)))

	// State sorts as text: active, draft, primary.
	page = db.ListToolSetGenerationsBuilder().
		ForToolSetId(toolSet.Id).
		Limit(10).
		OrderBy(ToolSetGenerationOrderByState, pagination.OrderByAsc).
		FetchPage(ctx)
	require.Equal(t, []uint64{1, 3, 2}, generationNumbersForTest(t, page))

	// Unknown columns never reach SQL.
	invalid := db.ListToolSetGenerationsBuilder().
		ForToolSetId(toolSet.Id).
		OrderBy("bogus", pagination.OrderByAsc).
		FetchPage(ctx)
	require.ErrorIs(t, invalid.Error, ErrInvalidListQuery)
}
