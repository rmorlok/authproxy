package database

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/stretchr/testify/require"
)

// toolSetForDatabaseTest provides an explicit match-all selector without
// manufacturing a definition generation or an installed connection binding.
func toolSetForDatabaseTest(namespace string, name scommon.ResourceName) *ToolSet {
	return &ToolSet{
		Id:        apid.New(apid.PrefixToolSet),
		Namespace: namespace,
		Name:      name,
		ConnectionSelector: toolsetschema.ConnectionSelector{
			MatchLabels: map[string]string{},
		},
	}
}

// TestToolSetCreateAndRead verifies logical defaults, detached selector storage,
// namespace inheritance, and the injected clock independently of generations.
func TestToolSetCreateAndRead(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	ctx := apctx.WithFixedClock(context.Background(), now)

	require.NoError(t, db.CreateNamespace(ctx, &Namespace{
		Path: "root.product", Labels: Labels{"team": "platform"},
	}))

	toolSet := toolSetForDatabaseTest("root.product", "")
	toolSet.Labels = Labels{"purpose": "calendar"}
	toolSet.Annotations = Annotations{"example.com/owner": "integrations"}
	toolSet.SelectorRevision = 99
	toolSet.CreatedAt, toolSet.UpdatedAt = now.Add(-time.Hour), now.Add(-time.Minute)
	toolSet.DeletedAt = &now

	// First create
	require.NoError(t, db.CreateToolSet(ctx, toolSet))
	require.NoError(t, toolSet.Validate())
	require.Equal(t, scommon.ResourceName(toolSet.Id.String()), toolSet.Name)
	require.Equal(t, uint64(1), toolSet.SelectorRevision)
	require.True(t, now.Equal(toolSet.CreatedAt))
	require.True(t, now.Equal(toolSet.UpdatedAt))
	require.Nil(t, toolSet.DeletedAt)

	// Then get
	byID, err := db.GetToolSet(ctx, toolSet.Id)
	require.NoError(t, err)

	// Then get by name, which is the same as id because unspecified
	byName, err := db.GetToolSetByName(ctx, toolSet.Namespace, toolSet.Name)
	require.NoError(t, err)

	for _, stored := range []*ToolSet{byID, byName} {
		require.Equal(t, toolSet.Id, stored.Id)
		require.Equal(t, toolSet.ConnectionSelector, stored.ConnectionSelector)
		require.NotNil(t, stored.ConnectionSelector.MatchLabels)
		require.Empty(t, stored.ConnectionSelector.MatchLabels)
		require.Equal(t, toolSet.Annotations, stored.Annotations)
		require.Equal(t, "platform", stored.Labels["apxy/ns/team"])
		require.Equal(t, "calendar", stored.Labels["purpose"])
		require.Equal(t, toolSet.Id.String(), stored.Labels["apxy/tls/-/id"])
		require.Equal(t, string(toolSet.Name), stored.Labels["apxy/tls/-/name"])
		require.Equal(t, toolSet.Namespace, stored.Labels["apxy/tls/-/ns"])
		require.True(t, now.Equal(stored.CreatedAt))
		require.True(t, now.Equal(stored.UpdatedAt))
	}

	// Make an in-memory change and convert it still loads from db
	byID.ConnectionSelector.MatchLabels["changed"] = "local"
	stored, err := db.GetToolSet(ctx, toolSet.Id)
	require.NoError(t, err)
	require.Empty(t, stored.ConnectionSelector.MatchLabels)
}

// TestToolSetCreateFailurePreservesCaller prevents rejected records from picking
// up defaults or inherited metadata that were never committed to the database.
func TestToolSetCreateFailurePreservesCaller(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()

	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.deleted"}))
	require.NoError(t, db.DeleteNamespace(ctx, "root.deleted"))
	require.Error(t, db.CreateToolSet(ctx, nil))

	for _, test := range []struct {
		name   string
		change func(*ToolSet)
	}{
		{"missing ID", func(s *ToolSet) { s.Id = apid.Nil }},
		{"wrong ID type", func(s *ToolSet) { s.Id = apid.New(apid.PrefixActor) }},
		{"invalid namespace", func(s *ToolSet) { s.Namespace = "root..product" }},
		{"missing namespace", func(s *ToolSet) { s.Namespace = "root.absent" }},
		{"deleted namespace", func(s *ToolSet) { s.Namespace = "root.deleted" }},
		{"invalid name", func(s *ToolSet) { s.Name = "invalid/name" }},
		{"invalid labels", func(s *ToolSet) { s.Labels = Labels{"invalid/key/name": "x"} }},
		{"invalid annotations", func(s *ToolSet) { s.Annotations = Annotations{"invalid/key/name": "x"} }},
		{"absent selector labels", func(s *ToolSet) { s.ConnectionSelector.MatchLabels = nil }},
		{"selector outside owner", func(s *ToolSet) { s.Namespace, s.ConnectionSelector.Namespace = "root.product", "root.**" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := toolSetForDatabaseTest("root", "")
			candidate.Labels, candidate.Annotations = Labels{"team": "product"}, Annotations{"note": "unchanged"}
			test.change(candidate)
			before := *candidate
			before.Labels, before.Annotations = candidate.Labels.Copy(), candidate.Annotations.Copy()
			before.ConnectionSelector = *candidate.ConnectionSelector.Clone()
			require.Error(t, db.CreateToolSet(ctx, candidate))
			require.Equal(t, before, *candidate)
		})
	}
}

// TestToolSetNamesAndDeletion keeps names unique among live namespace peers
// while retaining deleted IDs and timestamps for later dependent storage.
func TestToolSetNamesAndDeletion(t *testing.T) {
	_, db, rawDB := MustApplyBlankTestDbConfigRaw(t, nil)
	createdAt := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	ctx := apctx.WithFixedClock(context.Background(), createdAt)

	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.other"}))

	// Create with an explicit name
	first := toolSetForDatabaseTest("root", "shared")

	require.NoError(t, db.CreateToolSet(ctx, first))

	// Creating another toolset in the same namespace with the same name is an
	// error
	conflict := toolSetForDatabaseTest("root", "shared")
	require.ErrorIs(t, db.CreateToolSet(ctx, conflict), ErrDuplicate)
	require.Zero(t, conflict.SelectorRevision)
	require.True(t, conflict.CreatedAt.IsZero())

	// It's ok to create a toolset with the same name in a different namespace
	other := toolSetForDatabaseTest("root.other", "shared")
	require.NoError(t, db.CreateToolSet(ctx, other))

	// Delete the toolset
	deletedAt := createdAt.Add(time.Hour)
	ctx = apctx.WithFixedClock(ctx, deletedAt)
	require.NoError(t, db.DeleteToolSet(ctx, first.Id))

	// Now should be not found
	_, err := db.GetToolSet(ctx, first.Id)
	require.ErrorIs(t, err, ErrNotFound)

	// Should not find by name
	_, err = db.GetToolSetByName(ctx, "root", "shared")
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, db.DeleteToolSet(ctx, first.Id), ErrNotFound)

	// Cannot make updates
	_, err = db.UpdateToolSetConnectionSelector(ctx, first.Id, 1, first.ConnectionSelector)
	require.ErrorIs(t, err, ErrNotFound)

	// Check the times in the database
	var storedCreatedAt, storedUpdatedAt, storedDeletedAt time.Time
	require.NoError(t, rawDB.QueryRow(fmt.Sprintf(
		"SELECT created_at, updated_at, deleted_at FROM tool_sets WHERE id = '%s'", first.Id,
	)).Scan(&storedCreatedAt, &storedUpdatedAt, &storedDeletedAt))
	require.True(t, createdAt.Equal(storedCreatedAt))
	require.True(t, deletedAt.Equal(storedUpdatedAt))
	require.True(t, deletedAt.Equal(storedDeletedAt))

	// Create another in the original namespce with the same name (allowed)
	replacement := toolSetForDatabaseTest("root", "shared")
	require.NoError(t, db.CreateToolSet(ctx, replacement))

	// Try to recreate first toolset (same id)
	first.Name = "different-name"
	require.ErrorIs(t, db.CreateToolSet(ctx, first), ErrDuplicate, "soft deletion must not resurrect an old identity")

	// Should still get the one that was created after the delete
	storedOther, err := db.GetToolSetByName(ctx, "root.other", "shared")
	require.NoError(t, err)
	require.Equal(t, other.Id, storedOther.Id)
}

// TestToolSetSelectorCompareAndSwap fences stale selection policies and preserves
// namespace scope, exact system-label requirements, and unrelated metadata.
func TestToolSetSelectorCompareAndSwap(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	createdAt := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	ctx := apctx.WithFixedClock(context.Background(), createdAt)

	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.product"}))

	// Create initial toolset with the test default selector
	toolSet := toolSetForDatabaseTest("root.product", "calendar")
	toolSet.Labels, toolSet.Annotations = Labels{"team": "support"}, Annotations{"note": "stable"}
	require.NoError(t, db.CreateToolSet(ctx, toolSet))

	// Update the selector
	selector := toolsetschema.ConnectionSelector{
		Namespace: "root.product.user-1.**",
		MatchLabels: map[string]string{
			"apxy/cxn/-/id": "cxn_01example0000001",
			"empty":         "",
		},
	}
	updatedAt := createdAt.Add(time.Minute)
	ctx = apctx.WithFixedClock(ctx, updatedAt)
	updated, err := db.UpdateToolSetConnectionSelector(ctx, toolSet.Id, 1, selector)
	require.NoError(t, err)
	require.Equal(t, uint64(2), updated.SelectorRevision)
	require.Equal(t, selector, updated.ConnectionSelector)
	require.Equal(t, toolSet.Labels, updated.Labels)
	require.Equal(t, toolSet.Annotations, updated.Annotations)
	require.Equal(t, toolSet.Name, updated.Name)
	require.Equal(t, toolSet.Namespace, updated.Namespace)
	require.True(t, createdAt.Equal(updated.CreatedAt))
	require.True(t, updatedAt.Equal(updated.UpdatedAt))

	// Try update back with initial revision
	_, err = db.UpdateToolSetConnectionSelector(ctx, toolSet.Id, 1, toolSet.ConnectionSelector)
	require.ErrorIs(t, err, ErrToolSetSelectorRevisionConflict)

	stored, err := db.GetToolSet(ctx, toolSet.Id)
	require.NoError(t, err)
	require.Equal(t, selector, stored.ConnectionSelector)
	require.Equal(t, uint64(2), stored.SelectorRevision)

	updated, err = db.UpdateToolSetConnectionSelector(ctx, toolSet.Id, 2, selector)
	require.NoError(t, err)
	require.Equal(t, uint64(3), updated.SelectorRevision, "accepted equivalent replacements still fence older jobs")

	// Attempt updates with invalid selectors
	for _, invalid := range []toolsetschema.ConnectionSelector{
		{
			MatchLabels: nil,
		},
		{
			Namespace:   "root.**",
			MatchLabels: map[string]string{},
		},
		{
			Namespace:   "root.other",
			MatchLabels: map[string]string{},
		},
	} {
		_, err := db.UpdateToolSetConnectionSelector(ctx, toolSet.Id, 3, invalid)
		require.Error(t, err)
	}

	for _, revision := range []uint64{0, math.MaxInt64, math.MaxUint64} {
		_, err := db.UpdateToolSetConnectionSelector(ctx, toolSet.Id, revision, selector)
		require.Error(t, err)
	}

	// Do a valid update that still works
	updated, err = db.UpdateToolSetConnectionSelector(ctx, toolSet.Id, 3, toolSet.ConnectionSelector)
	require.NoError(t, err)
	require.Equal(t, uint64(4), updated.SelectorRevision)
	require.NotNil(t, updated.ConnectionSelector.MatchLabels)
	require.Empty(t, updated.ConnectionSelector.MatchLabels)
}

// TestToolSetConcurrentSelectorUpdates admits one writer per observed revision
// and ensures the losing policy cannot overwrite the committed replacement.
func TestToolSetConcurrentSelectorUpdates(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()

	// Create a toolset
	toolSet := toolSetForDatabaseTest("root", "concurrent")
	require.NoError(t, db.CreateToolSet(ctx, toolSet))

	start := make(chan struct{})
	type outcome struct {
		choice string
		value  *ToolSet
		err    error
	}
	results := make(chan outcome, 2)
	for _, choice := range []string{"first", "second"} {
		go func(choice string) {
			<-start
			value, err := db.UpdateToolSetConnectionSelector(
				ctx,
				toolSet.Id,
				1,
				toolsetschema.ConnectionSelector{
					MatchLabels: map[string]string{
						"choice": choice,
					},
				},
			)
			results <- outcome{choice, value, err}
		}(choice)
	}
	close(start)
	winner := ""
	for range 2 {
		result := <-results
		if result.err != nil {
			require.ErrorIs(t, result.err, ErrToolSetSelectorRevisionConflict)
			continue
		}
		require.Empty(t, winner, "only one writer may commit revision 2")
		winner = result.choice
		require.Equal(t, uint64(2), result.value.SelectorRevision)
	}
	require.NotEmpty(t, winner)
	stored, err := db.GetToolSet(ctx, toolSet.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(2), stored.SelectorRevision)
	require.Equal(t, winner, stored.ConnectionSelector.MatchLabels["choice"])
}

// TestToolSetLookupValidation rejects malformed identities and excludes missing
// rows without interpreting a missing policy as a stale selector revision.
func TestToolSetLookupValidation(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	selector := toolsetschema.ConnectionSelector{MatchLabels: map[string]string{}}
	for _, id := range []apid.ID{apid.Nil, apid.New(apid.PrefixConnection)} {
		_, err := db.GetToolSet(ctx, id)
		require.Error(t, err)
		require.Error(t, db.DeleteToolSet(ctx, id))
		_, err = db.UpdateToolSetConnectionSelector(ctx, id, 1, selector)
		require.Error(t, err)
	}
	missing := apid.New(apid.PrefixToolSet)
	_, err := db.GetToolSet(ctx, missing)
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, db.DeleteToolSet(ctx, missing), ErrNotFound)
	_, err = db.UpdateToolSetConnectionSelector(ctx, missing, 1, selector)
	require.ErrorIs(t, err, ErrNotFound)
	for _, identity := range [][2]string{{"root..invalid", "name"}, {"root", "invalid/name"}, {"root", ""}} {
		_, err := db.GetToolSetByName(ctx, identity[0], scommon.ResourceName(identity[1]))
		require.Error(t, err)
	}
}

// TestToolSetNamespaceLabelPropagation keeps inherited metadata current through
// both triggered propagation and bounded periodic drift repair, without changing
// the selection policy revision used to fence connection reconciliation.
func TestToolSetNamespaceLabelPropagation(t *testing.T) {
	_, db, rawDB := MustApplyBlankTestDbConfigRaw(t, nil)
	ctx := context.Background()
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.product", Labels: Labels{"team": "original"}}))
	var toolSets []*ToolSet
	for i := range 3 {
		toolSet := toolSetForDatabaseTest("root.product", scommon.ResourceName(fmt.Sprintf("tools-%d", i)))
		toolSet.Labels = Labels{"local": "preserved"}
		require.NoError(t, db.CreateToolSet(ctx, toolSet))
		toolSets = append(toolSets, toolSet)
	}
	_, err := db.UpdateNamespaceLabels(ctx, "root.product", map[string]string{"team": "updated"})
	require.NoError(t, err)
	require.NoError(t, db.RefreshNamespaceLabelsCarryForward(ctx, "root.product"))
	for _, toolSet := range toolSets {
		stored, err := db.GetToolSet(ctx, toolSet.Id)
		require.NoError(t, err)
		require.Equal(t, "updated", stored.Labels["apxy/ns/team"])
		require.Equal(t, toolSet.SelectorRevision, stored.SelectorRevision)
		require.Equal(t, toolSet.ConnectionSelector, stored.ConnectionSelector)
	}
	_, err = db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(db), 1, nil)
	require.NoError(t, err)
	for _, toolSet := range toolSets {
		_, err := rawDB.Exec(fmt.Sprintf("UPDATE tool_sets SET labels = '{\"local\":\"preserved\",\"apxy/ns/team\":\"drifted\"}' WHERE id = '%s'", toolSet.Id))
		require.NoError(t, err)
	}
	corrected, err := db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(db), 1, nil)
	require.NoError(t, err)
	require.Equal(t, int64(len(toolSets)), corrected)
	for _, toolSet := range toolSets {
		stored, err := db.GetToolSet(ctx, toolSet.Id)
		require.NoError(t, err)
		require.Equal(t, "updated", stored.Labels["apxy/ns/team"])
		require.Equal(t, "preserved", stored.Labels["local"])
		require.Equal(t, toolSet.Id.String(), stored.Labels["apxy/tls/-/id"])
		require.Equal(t, toolSet.SelectorRevision, stored.SelectorRevision)
		require.Equal(t, toolSet.ConnectionSelector, stored.ConnectionSelector)
	}
	corrected, err = db.ReconcileCarryForwardLabels(ctx, testReconcileCheckpointer(db), 1, nil)
	require.NoError(t, err)
	require.Zero(t, corrected)
}

// TestToolSetMigrationReversibility preserves unrelated namespace rows while
// removing and recreating the new logical-resource table on either provider.
func TestToolSetMigrationReversibility(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.preserved"}))
	before := toolSetForDatabaseTest("root.preserved", "before")
	require.NoError(t, db.CreateToolSet(ctx, before))
	migrateDatabaseToVersion(t, db.(*service), 16)
	_, err := db.GetNamespace(ctx, "root.preserved")
	require.NoError(t, err)
	migrateDatabaseToVersion(t, db.(*service), 17)
	_, err = db.GetToolSet(ctx, before.Id)
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, db.CreateToolSet(ctx, toolSetForDatabaseTest("root.preserved", "after")))
}
