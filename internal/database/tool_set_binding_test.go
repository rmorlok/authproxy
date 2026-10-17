package database

import (
	"context"
	"math"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/stretchr/testify/require"
)

// toolSetBindingParentsForTest creates matching parents without publishing any
// generation. Reloading the parents captures their persisted metadata and times.
func toolSetBindingParentsForTest(t *testing.T) (context.Context, DB, *ToolSet, *Connection) {
	t.Helper()
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.WithFixedClock(context.Background(), time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{
		Path:   "root.product",
		Labels: Labels{"team": "platform"},
	}))
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.product.owner"}))

	toolSet := toolSetForDatabaseTest("root.product", "tools")
	toolSet.ConnectionSelector.MatchLabels = map[string]string{"env": "prod"}
	require.NoError(t, db.CreateToolSet(ctx, toolSet))

	connection := &Connection{
		Id:                  apid.New(apid.PrefixConnection),
		Namespace:           "root.product.owner",
		Name:                "account",
		State:               ConnectionStateSetup,
		ConnectorId:         apid.New(apid.PrefixConnector),
		ConnectorGeneration: 1,
		Labels:              Labels{"env": "prod"},
		Annotations:         Annotations{"note": "unchanged"},
	}
	require.NoError(t, db.CreateConnection(ctx, connection))

	toolSet, err := db.GetToolSet(ctx, toolSet.Id)
	require.NoError(t, err)

	connection, err = db.GetConnection(ctx, connection.Id)
	require.NoError(t, err)

	return ctx, db, toolSet, connection
}

// TestToolSetBindingCreateAndRead establishes a pending identity without changing
// either parent, requiring a generation, or implying connection readiness.
func TestToolSetBindingCreateAndRead(t *testing.T) {
	ctx, db, toolSet, connection := toolSetBindingParentsForTest(t)
	createdAt := apctx.GetClock(ctx).Now().Add(time.Hour)

	// Create the binding
	binding, err := db.CreateToolSetBinding(apctx.WithFixedClock(ctx, createdAt), toolSet.Id, connection.Id, 1)
	require.NoError(t, err)
	require.NoError(t, binding.Validate())
	require.True(t, binding.Id.HasPrefix(apid.PrefixToolSetBinding))

	// Validate id format
	parsed, err := apid.Parse(binding.Id.String())
	require.NoError(t, err)
	require.Equal(t, binding.Id, parsed)
	require.Equal(t, uint64(1), binding.Epoch)
	require.True(t, createdAt.Equal(binding.CreatedAt))
	require.True(t, createdAt.Equal(binding.UpdatedAt))
	require.Nil(t, binding.DeletedAt)

	// Get the binding directly
	byID, err := db.GetToolSetBinding(ctx, binding.Id)
	require.NoError(t, err)

	// Get the binding for the toolset/connection pair
	byPair, err := db.GetToolSetBindingForConnection(ctx, toolSet.Id, connection.Id)
	require.NoError(t, err)
	for _, stored := range []*ToolSetBinding{byID, byPair} {
		require.Equal(t, binding.Id, stored.Id)
		require.Equal(t, toolSet.Id, stored.ToolSetId)
		require.Equal(t, connection.Id, stored.ConnectionId)
		require.Equal(t, uint64(1), stored.Epoch)
		require.True(t, createdAt.Equal(stored.CreatedAt))
	}

	// Verify we can't create another beinding between these two
	_, err = db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, 1)
	require.ErrorIs(t, err, ErrDuplicate)

	// Previous one still there
	storedToolSet, err := db.GetToolSet(ctx, toolSet.Id)
	require.NoError(t, err)
	require.Equal(t, toolSet, storedToolSet)

	// Get the connection
	storedConnection, err := db.GetConnection(ctx, connection.Id)
	require.NoError(t, err)
	require.Equal(t, connection, storedConnection)

	// Multiple toolsets can bind to same connection
	otherToolSet := toolSetForDatabaseTest(toolSet.Namespace, "other-tools")
	require.NoError(t, db.CreateToolSet(ctx, otherToolSet))
	_, err = db.CreateToolSetBinding(ctx, otherToolSet.Id, connection.Id, 1)
	require.NoError(t, err, "multiple ToolSets can attach to one connection")

	otherConnection := *connection
	otherConnection.Id = apid.New(apid.PrefixConnection)
	otherConnection.Name = "other-account"
	require.NoError(t, db.CreateConnection(ctx, &otherConnection))

	// Toolsets can bind to multiple connections
	_, err = db.CreateToolSetBinding(ctx, toolSet.Id, otherConnection.Id, 1)
	require.NoError(t, err, "a ToolSet can attach to multiple connections")
}

// TestToolSetBindingMembership uses canonical namespace and label matching,
// including required empty values and inherited/system labels.
func TestToolSetBindingMembership(t *testing.T) {
	for _, test := range []struct {
		name      string
		namespace string
		scope     string
		labels    map[string]string
		match     bool
	}{
		{"owner namespace", "root.product", "", map[string]string{}, true},
		{"descendant namespace", "root.product.owner", "", map[string]string{}, true},
		{"similar prefix", "root.product-other", "", map[string]string{}, false},
		{"ancestor", "root", "", map[string]string{}, false},
		{"explicit exact excludes descendants", "root.product.owner", "root.product", map[string]string{}, false},
		{"explicit child", "root.product.owner", "root.product.owner", map[string]string{}, true},
		{"explicit subtree", "root.product.owner.child", "root.product.owner.**", map[string]string{}, true},
		{"wrong label", "root.product.owner", "", map[string]string{"env": "dev"}, false},
		{"empty present", "root.product.owner", "", map[string]string{"flag": ""}, true},
		{"empty missing", "root.product.owner", "", map[string]string{"absent": ""}, false},
		{"system label", "root.product.owner", "", map[string]string{"apxy/cxn/-/ns": "root.product.owner"}, true},
		{"inherited label", "root.product.owner", "", map[string]string{"apxy/ns/team": "platform"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, db, toolSet, connection := toolSetBindingParentsForTest(t)
			candidate := *connection
			candidate.Id = apid.New(apid.PrefixConnection)
			candidate.Name = "other"
			candidate.Namespace = test.namespace
			candidate.Labels = Labels{"env": "prod", "flag": ""}
			require.NoError(t, db.CreateConnection(ctx, &candidate))
			selector := toolsetschema.ConnectionSelector{
				Namespace:   test.scope,
				MatchLabels: test.labels,
			}
			updated, err := db.UpdateToolSetConnectionSelector(ctx, toolSet.Id, 1, selector)
			require.NoError(t, err)
			_, err = db.CreateToolSetBinding(ctx, toolSet.Id, candidate.Id, updated.SelectorRevision)
			if test.match {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrToolSetBindingMembershipConflict)
			}
		})
	}
}

// TestToolSetBindingCurrentPolicyFences rechecks persisted connection labels as
// well as the selector revision before creating or retiring an assignment.
func TestToolSetBindingCurrentPolicyFences(t *testing.T) {
	ctx, db, toolSet, connection := toolSetBindingParentsForTest(t)
	updated, err := db.UpdateToolSetConnectionSelector(ctx, toolSet.Id, 1, toolSet.ConnectionSelector)
	require.NoError(t, err)
	_, err = db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, 1)
	require.ErrorIs(t, err, ErrToolSetSelectorRevisionConflict)
	_, err = db.UpdateConnectionLabels(ctx, connection.Id, map[string]string{"env": "dev"})
	require.NoError(t, err)
	_, err = db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, updated.SelectorRevision)
	require.ErrorIs(t, err, ErrToolSetBindingMembershipConflict)
	_, err = db.UpdateConnectionLabels(ctx, connection.Id, map[string]string{"env": "prod"})
	require.NoError(t, err)
	binding, err := db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, updated.SelectorRevision)
	require.NoError(t, err)

	selector := toolsetschema.ConnectionSelector{MatchLabels: map[string]string{"env": "dev"}}
	updated, err = db.UpdateToolSetConnectionSelector(ctx, toolSet.Id, updated.SelectorRevision, selector)
	require.NoError(t, err)
	require.ErrorIs(t, db.DeleteToolSetBinding(ctx, binding.Id, 1, 2), ErrToolSetSelectorRevisionConflict)
	_, err = db.UpdateConnectionLabels(ctx, connection.Id, map[string]string{"env": "dev"})
	require.NoError(t, err)
	require.ErrorIs(t, db.DeleteToolSetBinding(ctx, binding.Id, 1, updated.SelectorRevision), ErrToolSetBindingMembershipConflict)
	_, err = db.UpdateConnectionLabels(ctx, connection.Id, map[string]string{"env": "prod"})
	require.NoError(t, err)
	require.NoError(t, db.DeleteToolSetBinding(ctx, binding.Id, 1, updated.SelectorRevision))
}

// TestToolSetBindingRemovalAndRecreation retains old identity tombstones and
// prevents work carrying an old binding ID from affecting a replacement pair.
func TestToolSetBindingRemovalAndRecreation(t *testing.T) {
	ctx, db, toolSet, connection := toolSetBindingParentsForTest(t)
	first, err := db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, 1)
	require.NoError(t, err)
	otherToolSet := toolSetForDatabaseTest(toolSet.Namespace, "other-tools")
	require.NoError(t, db.CreateToolSet(ctx, otherToolSet))
	sibling, err := db.CreateToolSetBinding(ctx, otherToolSet.Id, connection.Id, 1)
	require.NoError(t, err)
	assertSiblingUnchanged := func() {
		t.Helper()
		stored, err := db.GetToolSetBindingForConnection(ctx, otherToolSet.Id, connection.Id)
		require.NoError(t, err)
		require.Equal(t, sibling.Id, stored.Id)
		require.Equal(t, sibling.Epoch, stored.Epoch)
		require.True(t, sibling.CreatedAt.Equal(stored.CreatedAt))
		require.True(t, sibling.UpdatedAt.Equal(stored.UpdatedAt))
		require.Nil(t, stored.DeletedAt)
	}
	require.ErrorIs(t, db.DeleteToolSetBinding(ctx, first.Id, 2, 1), ErrToolSetBindingEpochConflict)
	require.ErrorIs(t, db.DeleteToolSetBinding(ctx, first.Id, 1, 1), ErrToolSetBindingMembershipConflict)
	_, err = db.UpdateConnectionLabels(ctx, connection.Id, map[string]string{})
	require.NoError(t, err)
	deletedAt := first.CreatedAt.Add(time.Hour)
	require.NoError(t, db.DeleteToolSetBinding(apctx.WithFixedClock(ctx, deletedAt), first.Id, 1, 1))
	_, err = db.GetToolSetBinding(ctx, first.Id)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = db.GetToolSetBindingForConnection(ctx, toolSet.Id, connection.Id)
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, db.DeleteToolSetBinding(ctx, first.Id, 1, 1), ErrNotFound)
	assertSiblingUnchanged()

	s := db.(*service)
	var tombstone ToolSetBinding
	err = s.sq.Select(tombstone.cols()...).
		From(ToolSetBindingsTable).
		Where(sq.Eq{"id": first.Id}).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(tombstone.fields()...)
	require.NoError(t, err)
	require.Equal(t, uint64(2), tombstone.Epoch)
	require.True(t, first.CreatedAt.Equal(tombstone.CreatedAt))
	require.True(t, deletedAt.Equal(tombstone.UpdatedAt))
	require.NotNil(t, tombstone.DeletedAt)
	require.True(t, deletedAt.Equal(*tombstone.DeletedAt))

	_, err = db.UpdateConnectionLabels(ctx, connection.Id, map[string]string{"env": "prod"})
	require.NoError(t, err)
	second, err := db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, 1)
	require.NoError(t, err)
	require.NotEqual(t, first.Id, second.Id)
	require.Equal(t, uint64(1), second.Epoch)
	require.ErrorIs(t, db.DeleteToolSetBinding(ctx, first.Id, 1, 1), ErrNotFound)
	stored, err := db.GetToolSetBindingForConnection(ctx, toolSet.Id, connection.Id)
	require.NoError(t, err)
	require.Equal(t, second.Id, stored.Id)
	assertSiblingUnchanged()
	_, err = s.sq.Insert(ToolSetBindingsTable).
		Columns(tombstone.cols()...).
		Values(tombstone.values()...).
		RunWith(s.db).
		ExecContext(ctx)
	require.Error(t, err, "deleted rows retain their primary keys without participating in live-pair uniqueness")
}

// TestToolSetBindingDeletedParentCleanup hides assignments immediately while
// allowing later cleanup without requiring a usable parent selector revision.
func TestToolSetBindingDeletedParentCleanup(t *testing.T) {
	for _, parent := range []string{"toolset", "connection"} {
		t.Run(parent, func(t *testing.T) {
			ctx, db, toolSet, connection := toolSetBindingParentsForTest(t)
			binding, err := db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, 1)
			require.NoError(t, err)
			if parent == "toolset" {
				require.NoError(t, db.DeleteToolSet(ctx, toolSet.Id))
			} else {
				require.NoError(t, db.DeleteConnection(ctx, connection.Id))
			}
			_, err = db.GetToolSetBinding(ctx, binding.Id)
			require.ErrorIs(t, err, ErrNotFound)
			_, err = db.GetToolSetBindingForConnection(ctx, toolSet.Id, connection.Id)
			require.ErrorIs(t, err, ErrNotFound)
			_, err = db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, 1)
			require.ErrorIs(t, err, ErrNotFound)
			require.ErrorIs(t, db.DeleteToolSetBinding(ctx, binding.Id, 2, 0), ErrToolSetBindingEpochConflict)
			require.NoError(t, db.DeleteToolSetBinding(ctx, binding.Id, 1, 0))
			require.ErrorIs(t, db.DeleteToolSetBinding(ctx, binding.Id, 1, 0), ErrNotFound)
		})
	}
}

// TestToolSetBindingConnectionReadiness keeps pending membership independent of
// setup completion and transient operational health.
func TestToolSetBindingConnectionReadiness(t *testing.T) {
	for _, state := range []ConnectionState{ConnectionStateSetup, ConnectionStateConfigured} {
		t.Run(string(state), func(t *testing.T) {
			ctx, db, toolSet, connection := toolSetBindingParentsForTest(t)
			require.NoError(t, db.SetConnectionState(ctx, connection.Id, state))
			s := db.(*service)
			_, err := s.sq.Update(ConnectionsTable).
				Set("health_state", ConnectionHealthStateUnhealthy).
				Where(sq.Eq{"id": connection.Id}).
				RunWith(s.db).
				ExecContext(ctx)
			require.NoError(t, err)
			binding, err := db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, 1)
			require.NoError(t, err)
			_, err = db.GetToolSetBinding(ctx, binding.Id)
			require.NoError(t, err)
		})
	}
}

// TestToolSetBindingValidation rejects invalid identities and counters before
// SQL execution, including an exhausted stored epoch that cannot be incremented.
func TestToolSetBindingValidation(t *testing.T) {
	ctx, db, toolSet, connection := toolSetBindingParentsForTest(t)
	binding, err := db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, 1)
	require.NoError(t, err)
	require.Error(t, (*ToolSetBinding)(nil).Validate())
	for _, change := range []func(*ToolSetBinding){
		func(b *ToolSetBinding) { b.Id = apid.Nil },
		func(b *ToolSetBinding) { b.Id = toolSet.Id },
		func(b *ToolSetBinding) { b.ToolSetId = apid.Nil },
		func(b *ToolSetBinding) { b.ToolSetId = connection.Id },
		func(b *ToolSetBinding) { b.ConnectionId = apid.Nil },
		func(b *ToolSetBinding) { b.ConnectionId = toolSet.Id },
		func(b *ToolSetBinding) { b.Epoch = 0 },
		func(b *ToolSetBinding) { b.Epoch = math.MaxInt64 + 1 },
	} {
		invalid := *binding
		change(&invalid)
		require.Error(t, invalid.Validate())
	}
	for _, id := range []apid.ID{apid.Nil, connection.Id} {
		_, err := db.GetToolSetBinding(ctx, id)
		require.Error(t, err)
		require.Error(t, db.DeleteToolSetBinding(ctx, id, 1, 1))
		_, err = db.CreateToolSetBinding(ctx, id, connection.Id, 1)
		require.Error(t, err)
		_, err = db.GetToolSetBindingForConnection(ctx, id, connection.Id)
		require.Error(t, err)
	}
	for _, id := range []apid.ID{apid.Nil, toolSet.Id} {
		_, err := db.CreateToolSetBinding(ctx, toolSet.Id, id, 1)
		require.Error(t, err)
		_, err = db.GetToolSetBindingForConnection(ctx, toolSet.Id, id)
		require.Error(t, err)
	}
	for _, revision := range []uint64{0, math.MaxInt64 + 1} {
		_, err := db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, revision)
		require.Error(t, err)
		require.Error(t, db.DeleteToolSetBinding(ctx, binding.Id, 1, revision))
	}
	for _, epoch := range []uint64{0, math.MaxInt64, math.MaxInt64 + 1} {
		require.Error(t, db.DeleteToolSetBinding(ctx, binding.Id, epoch, 1))
	}
	_, err = db.GetToolSetBinding(ctx, apid.New(apid.PrefixToolSetBinding))
	require.ErrorIs(t, err, ErrNotFound)
	_, err = db.CreateToolSetBinding(ctx, apid.New(apid.PrefixToolSet), connection.Id, 1)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = db.CreateToolSetBinding(ctx, toolSet.Id, apid.New(apid.PrefixConnection), 1)
	require.ErrorIs(t, err, ErrNotFound)

	_, err = db.UpdateConnectionLabels(ctx, connection.Id, map[string]string{})
	require.NoError(t, err)
	s := db.(*service)
	_, err = s.sq.Update(ToolSetBindingsTable).
		Set("epoch", int64(math.MaxInt64)).
		Where(sq.Eq{"id": binding.Id}).
		RunWith(s.db).
		ExecContext(ctx)
	require.NoError(t, err)
	exhausted, err := db.GetToolSetBinding(ctx, binding.Id)
	require.NoError(t, err)
	require.NoError(t, exhausted.Validate())
	require.Error(t, db.DeleteToolSetBinding(ctx, binding.Id, math.MaxInt64, 1))
	unchanged, err := db.GetToolSetBinding(ctx, binding.Id)
	require.NoError(t, err)
	require.Equal(t, exhausted, unchanged)
}

// TestToolSetBindingConcurrentWrites permits one live identity for a pair and
// exactly one retirement for the captured epoch under concurrent workers.
func TestToolSetBindingConcurrentWrites(t *testing.T) {
	ctx, db, toolSet, connection := toolSetBindingParentsForTest(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, 1)
			results <- err
		}()
	}
	close(start)
	successes := 0
	for range 2 {
		if err := <-results; err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ErrDuplicate)
		}
	}
	require.Equal(t, 1, successes)
	binding, err := db.GetToolSetBindingForConnection(ctx, toolSet.Id, connection.Id)
	require.NoError(t, err)
	_, err = db.UpdateConnectionLabels(ctx, connection.Id, map[string]string{})
	require.NoError(t, err)
	start = make(chan struct{})
	for range 2 {
		go func() {
			<-start
			results <- db.DeleteToolSetBinding(ctx, binding.Id, 1, 1)
		}()
	}
	close(start)
	successes = 0
	for range 2 {
		if err := <-results; err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ErrNotFound)
		}
	}
	require.Equal(t, 1, successes)
}

// TestToolSetBindingConstraints checks database enforcement independently of the
// creation API, including foreign keys and historical identity retention.
func TestToolSetBindingConstraints(t *testing.T) {
	ctx, db, toolSet, connection := toolSetBindingParentsForTest(t)
	binding, err := db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, 1)
	require.NoError(t, err)
	s := db.(*service)
	for _, test := range []struct {
		name   string
		change func(*ToolSetBinding)
	}{
		{"live pair", func(b *ToolSetBinding) {}},
		{"missing toolset", func(b *ToolSetBinding) { b.ToolSetId = apid.New(apid.PrefixToolSet) }},
		{"missing connection", func(b *ToolSetBinding) { b.ConnectionId = apid.New(apid.PrefixConnection) }},
		{"zero epoch", func(b *ToolSetBinding) { b.Epoch = 0; b.DeletedAt = &b.CreatedAt }},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := *binding
			invalid.Id = apid.New(apid.PrefixToolSetBinding)
			test.change(&invalid)
			_, err := s.sq.Insert(ToolSetBindingsTable).
				Columns(invalid.cols()...).
				Values(invalid.values()...).
				RunWith(s.db).
				ExecContext(ctx)
			require.Error(t, err)
		})
	}
}

// TestToolSetBindingMigrationReversibility preserves both parents when the
// binding-only migration is rolled back and then reapplied.
func TestToolSetBindingMigrationReversibility(t *testing.T) {
	ctx, db, toolSet, connection := toolSetBindingParentsForTest(t)
	binding, err := db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, 1)
	require.NoError(t, err)
	migrateDatabaseToVersion(t, db.(*service), 18)
	_, err = db.GetToolSet(ctx, toolSet.Id)
	require.NoError(t, err)
	_, err = db.GetConnection(ctx, connection.Id)
	require.NoError(t, err)
	migrateDatabaseToVersion(t, db.(*service), 24)
	_, err = db.GetToolSetBinding(ctx, binding.Id)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, 1)
	require.NoError(t, err)
}

// TestToolSetBindingConnectionPurge permits the existing connection retention
// job to remove both live and tombstoned assignments with their retired parent.
func TestToolSetBindingConnectionPurge(t *testing.T) {
	ctx, db, toolSet, connection := toolSetBindingParentsForTest(t)
	first, err := db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, 1)
	require.NoError(t, err)
	_, err = db.UpdateConnectionLabels(ctx, connection.Id, map[string]string{})
	require.NoError(t, err)
	require.NoError(t, db.DeleteToolSetBinding(ctx, first.Id, 1, 1))
	_, err = db.UpdateConnectionLabels(ctx, connection.Id, map[string]string{"env": "prod"})
	require.NoError(t, err)
	_, err = db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, 1)
	require.NoError(t, err)
	require.NoError(t, db.DeleteConnection(ctx, connection.Id))

	s := db.(*service)
	var before int
	err = s.sq.Select("COUNT(*)").
		From(ToolSetBindingsTable).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(&before)
	require.NoError(t, err)
	require.Equal(t, 2, before)
	deleted, err := db.PurgeSoftDeletedRecords(ctx, apctx.GetClock(ctx).Now().Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted, "the purge count includes the parent, not cascaded rows")
	for _, table := range []string{ConnectionsTable, ToolSetBindingsTable} {
		var remaining int
		err = s.sq.Select("COUNT(*)").
			From(table).
			RunWith(s.db).
			QueryRowContext(ctx).
			Scan(&remaining)
		require.NoError(t, err)
		require.Zero(t, remaining)
	}
	_, err = db.GetToolSet(ctx, toolSet.Id)
	require.NoError(t, err)
}
