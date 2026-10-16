package database

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
)

const toolDefinitionDigestForTest = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// standaloneToolConnectionForTest creates a connection without a real provider;
// definition compilation and upstream execution are outside storage tests.
func standaloneToolConnectionForTest(t *testing.T, ctx context.Context, db DB, namespace string) *Connection {
	t.Helper()

	connection := &Connection{
		Id:                  apid.New(apid.PrefixConnection),
		Namespace:           namespace,
		State:               ConnectionStateConfigured,
		ConnectorId:         apid.New(apid.PrefixConnector),
		ConnectorGeneration: 1,
	}

	require.NoError(t, db.CreateConnection(ctx, connection))

	return connection
}

// standaloneToolForTest creates a standalone identity and opaque encrypted first
// revision. No ToolSet, binding, compiler, or provider is needed to persist it.
func standaloneToolForTest(t *testing.T) (context.Context, DB, *Tool, *ToolRevision) {
	t.Helper()
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.WithFixedClock(context.Background(), time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC))
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.owner"}))

	connection := standaloneToolConnectionForTest(t, ctx, db, "root.owner")
	tool := &Tool{
		Id:           apid.New(apid.PrefixTool),
		Namespace:    connection.Namespace,
		ConnectionId: connection.Id,
		Description:  "Read an example resource.",
		Verbs:        []string{"example:read"},
	}

	revision, err := db.CreateStandaloneTool(ctx, tool, encfield.EncryptedField{
		ID:   apid.New(apid.PrefixDataEncryptionKey),
		Data: "ZGVmaW5pdGlvbg==",
	}, toolDefinitionDigestForTest)
	require.NoError(t, err)

	return ctx, db, tool, revision
}

// countToolRevisionsForTest counts physical history, including tombstones, so
// failed writes cannot conceal an orphan revision behind the public read API.
func countToolRevisionsForTest(t *testing.T, ctx context.Context, db DB, id apid.ID) int {
	t.Helper()
	s := db.(*service)
	var count int

	err := s.sq.Select("COUNT(*)").
		From(ToolRevisionsTable).
		Where(sq.Eq{"tool_id": id}).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(&count)
	require.NoError(t, err)

	return count
}

// TestStandaloneToolRevisionLifecycle advances the active pointer immediately
// while retaining immutable historical definition, connection, and timestamps.
func TestStandaloneToolRevisionLifecycle(t *testing.T) {
	ctx, db, tool, first := standaloneToolForTest(t)
	now := apctx.GetClock(ctx).Now()
	require.NoError(t, tool.Validate())
	require.NoError(t, first.Validate())
	require.Equal(t, common.ResourceName(tool.Id.String()), tool.Name)
	require.Equal(t, uint64(1), tool.ActiveRevision)
	require.Equal(t, tool.Id, first.ToolId)
	require.Equal(t, tool.ConnectionId, first.ConnectionId)
	require.Equal(t, uint64(1), first.Revision)
	require.True(t, now.Equal(tool.CreatedAt))
	require.True(t, now.Equal(tool.UpdatedAt))
	require.True(t, now.Equal(first.CreatedAt))
	require.NotNil(t, first.EncryptedAt)
	require.True(t, now.Equal(*first.EncryptedAt))

	updatedAt := now.Add(time.Hour)
	definition := first.EncryptedDefinition
	definition.Data = "bmV3LWRlZmluaXRpb24="
	tool.Name = "renamed"

	second, err := db.UpdateStandaloneTool(
		apctx.WithFixedClock(ctx, updatedAt),
		tool,
		1, // expected revision
		definition,
		strings.Repeat("b", 64),
	)
	require.NoError(t, err)
	require.Equal(t, uint64(2), tool.ActiveRevision)
	require.Equal(t, uint64(2), second.Revision)
	require.True(t, now.Equal(tool.CreatedAt))
	require.True(t, updatedAt.Equal(tool.UpdatedAt))
	require.True(t, updatedAt.Equal(second.CreatedAt))
	require.True(t, updatedAt.Equal(*second.EncryptedAt))

	current, err := db.GetCurrentToolRevision(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, second.Revision, current.Revision)
	require.Equal(t, definition, current.EncryptedDefinition)
	require.Equal(t, second.DefinitionDigest, current.DefinitionDigest)

	previous, err := db.GetToolRevision(ctx, tool.Id, 1)
	require.NoError(t, err)
	require.Equal(t, first.EncryptedDefinition, previous.EncryptedDefinition)
	require.Equal(t, first.DefinitionDigest, previous.DefinitionDigest)
	require.Equal(t, first.ConnectionId, previous.ConnectionId)
	require.True(t, now.Equal(previous.CreatedAt))
	require.True(t, now.Equal(*previous.EncryptedAt))

	before := *tool
	_, err = db.UpdateStandaloneTool(ctx, tool, 1, definition, second.DefinitionDigest)
	require.ErrorIs(t, err, ErrToolRevisionConflict)
	require.Equal(t, before, *tool)
	third, err := db.UpdateStandaloneTool(ctx, tool, 2, definition, second.DefinitionDigest)
	require.NoError(t, err)
	require.Equal(t, uint64(3), third.Revision, "accepted equivalent updates still fence stale work")
	_, err = db.GetToolRevision(ctx, tool.Id, 4)
	require.ErrorIs(t, err, ErrNotFound)
}

// TestStandaloneToolRevisionRollback rejects a duplicate live name without
// leaving a revision, changing the pointer, or mutating the caller's candidate.
func TestStandaloneToolRevisionRollback(t *testing.T) {
	ctx, db, tool, first := standaloneToolForTest(t)
	other := &Tool{
		Id:           apid.New(apid.PrefixTool),
		Namespace:    tool.Namespace,
		Name:         "occupied",
		ConnectionId: tool.ConnectionId,
		Description:  tool.Description,
		Verbs:        tool.Verbs,
	}
	_, err := db.CreateStandaloneTool(ctx, other, first.EncryptedDefinition, first.DefinitionDigest)
	require.NoError(t, err)

	candidate := *tool
	candidate.Name = other.Name
	before := candidate

	_, err = db.UpdateStandaloneTool(ctx, &candidate, 1, first.EncryptedDefinition, first.DefinitionDigest)
	require.ErrorIs(t, err, ErrDuplicate)
	require.Equal(t, before, candidate)
	require.Equal(t, 1, countToolRevisionsForTest(t, ctx, db, tool.Id))

	stored, err := db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, tool.Name, stored.Name)
	require.Equal(t, uint64(1), stored.ActiveRevision)
	require.True(t, tool.UpdatedAt.Equal(stored.UpdatedAt))

	candidate.Id = apid.New(apid.PrefixTool)
	candidate.ActiveRevision = 99
	candidate.DeletedAt = &candidate.CreatedAt
	before = candidate

	_, err = db.CreateStandaloneTool(ctx, &candidate, first.EncryptedDefinition, first.DefinitionDigest)
	require.ErrorIs(t, err, ErrDuplicate)
	require.Equal(t, before, candidate)
	require.Zero(t, countToolRevisionsForTest(t, ctx, db, candidate.Id))

	require.NoError(t, db.DeleteStandaloneTool(ctx, other.Id, 1))
	tool.Name = other.Name
	_, err = db.UpdateStandaloneTool(ctx, tool, 1, first.EncryptedDefinition, first.DefinitionDigest)
	require.NoError(t, err)
	require.Equal(t, uint64(2), tool.ActiveRevision)
}

// TestStandaloneToolRetargetHistory retains the old connection identity through
// its purge, while purging the current connection removes its Tool and history.
func TestStandaloneToolRetargetHistory(t *testing.T) {
	ctx, db, tool, first := standaloneToolForTest(t)

	originalConnection := tool.ConnectionId

	other := standaloneToolConnectionForTest(t, ctx, db, tool.Namespace)

	tool.ConnectionId = other.Id

	second, err := db.UpdateStandaloneTool(ctx, tool, 1, first.EncryptedDefinition, first.DefinitionDigest)
	require.NoError(t, err)
	require.Equal(t, other.Id, second.ConnectionId)
	require.NoError(t, db.DeleteConnection(ctx, originalConnection))

	_, err = db.PurgeSoftDeletedRecords(ctx, apctx.GetClock(ctx).Now().Add(time.Second))
	require.NoError(t, err)

	s := db.(*service)
	var oldConnections int
	err = s.sq.Select("COUNT(*)").
		From(ConnectionsTable).
		Where(sq.Eq{"id": originalConnection}).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(&oldConnections)
	require.NoError(t, err)
	require.Zero(t, oldConnections, "history survives physical removal of its former connection")

	history, err := db.GetToolRevision(ctx, tool.Id, 1)
	require.NoError(t, err)
	require.Equal(t, originalConnection, history.ConnectionId)

	current, err := db.GetCurrentToolRevision(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, other.Id, current.ConnectionId)
	require.Equal(t, uint64(2), current.Revision)

	require.NoError(t, db.DeleteConnection(ctx, other.Id))

	_, err = db.GetCurrentToolRevision(ctx, tool.Id)
	require.ErrorIs(t, err, ErrNotFound)

	_, err = db.GetTool(ctx, tool.Id)
	require.NoError(t, err, "management identity remains visible for cleanup")

	_, err = db.GetToolRevision(ctx, tool.Id, 2)
	require.NoError(t, err, "history is inspectable but cannot supply an execution snapshot")

	_, err = db.PurgeSoftDeletedRecords(ctx, apctx.GetClock(ctx).Now().Add(time.Second))
	require.NoError(t, err)

	_, err = db.GetTool(ctx, tool.Id)
	require.ErrorIs(t, err, ErrNotFound)

	var remainingTools int
	err = s.sq.Select("COUNT(*)").
		From(ToolsTable).
		Where(sq.Eq{"id": tool.Id}).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(&remainingTools)
	require.NoError(t, err)
	require.Zero(t, remainingTools, "purging the current connection physically removes its Tool")
	require.Zero(t, countToolRevisionsForTest(t, ctx, db, tool.Id))
}

// TestStandaloneToolDeletion releases the name but retains the old ID and
// revision rows, including cleanup after the current connection is deleted.
func TestStandaloneToolDeletion(t *testing.T) {
	ctx, db, tool, first := standaloneToolForTest(t)
	_, err := db.UpdateStandaloneTool(ctx, tool, 1, first.EncryptedDefinition, first.DefinitionDigest)
	require.NoError(t, err)
	require.ErrorIs(t, db.DeleteStandaloneTool(ctx, tool.Id, 1), ErrToolRevisionConflict)
	require.NoError(t, db.DeleteConnection(ctx, tool.ConnectionId))

	deletedAt := tool.UpdatedAt.Add(time.Hour)

	require.NoError(t, db.DeleteStandaloneTool(apctx.WithFixedClock(ctx, deletedAt), tool.Id, 2))

	_, err = db.GetTool(ctx, tool.Id)
	require.ErrorIs(t, err, ErrNotFound)

	_, err = db.GetCurrentToolRevision(ctx, tool.Id)
	require.ErrorIs(t, err, ErrNotFound)

	_, err = db.GetToolRevision(ctx, tool.Id, 1)
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, db.DeleteStandaloneTool(ctx, tool.Id, 2), ErrNotFound)
	require.Equal(t, 2, countToolRevisionsForTest(t, ctx, db, tool.Id))

	s := db.(*service)
	var tombstone Tool
	err = s.sq.Select(tombstone.cols()...).
		From(ToolsTable).
		Where(sq.Eq{"id": tool.Id}).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(tombstone.fields()...)
	require.NoError(t, err)
	require.Equal(t, uint64(2), tombstone.ActiveRevision)
	require.NotNil(t, tombstone.DeletedAt)
	require.True(t, deletedAt.Equal(*tombstone.DeletedAt))
	require.True(t, deletedAt.Equal(tombstone.UpdatedAt))

	connection := standaloneToolConnectionForTest(t, ctx, db, tool.Namespace)
	replacement := *tool
	replacement.Id = apid.New(apid.PrefixTool)
	replacement.ConnectionId = connection.Id

	_, err = db.CreateStandaloneTool(ctx, &replacement, first.EncryptedDefinition, first.DefinitionDigest)
	require.NoError(t, err)
	require.Equal(t, uint64(1), replacement.ActiveRevision)
	tool.ConnectionId = connection.Id
	tool.Name = "unused-name"

	_, err = db.CreateStandaloneTool(ctx, tool, first.EncryptedDefinition, first.DefinitionDigest)
	require.ErrorIs(t, err, ErrDuplicate)
}

// TestStandaloneToolDeletedNamespace hides management and revision reads even
// when the Tool and its connection have not yet been cleaned up.
func TestStandaloneToolDeletedNamespace(t *testing.T) {
	ctx, db, tool, _ := standaloneToolForTest(t)
	require.NoError(t, db.DeleteNamespace(ctx, tool.Namespace))
	_, err := db.GetTool(ctx, tool.Id)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = db.GetCurrentToolRevision(ctx, tool.Id)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = db.GetToolRevision(ctx, tool.Id, 1)
	require.ErrorIs(t, err, ErrNotFound)
}

// TestStandaloneToolConcurrentUpdates accepts exactly one candidate for the
// same active revision, retaining only its newly appended revision and pointer.
func TestStandaloneToolConcurrentUpdates(t *testing.T) {
	ctx, db, tool, first := standaloneToolForTest(t)
	start := make(chan struct{})
	type outcome struct {
		tool     Tool
		revision ToolRevision
		err      error
	}
	results := make(chan outcome, 2)
	for _, input := range []struct {
		name   common.ResourceName
		data   string
		digest string
	}{
		{"winner-a", "d2lubmVyLWE=", strings.Repeat("a", 64)},
		{"winner-b", "d2lubmVyLWI=", strings.Repeat("b", 64)},
	} {
		go func() {
			candidate := *tool
			candidate.Name = input.name
			snapshot := *first
			snapshot.Revision = 2
			snapshot.EncryptedDefinition.Data = input.data
			snapshot.DefinitionDigest = input.digest
			<-start
			_, err := db.UpdateStandaloneTool(ctx, &candidate, 1, snapshot.EncryptedDefinition, snapshot.DefinitionDigest)
			results <- outcome{
				tool:     candidate,
				revision: snapshot,
				err:      err,
			}
		}()
	}
	close(start)
	var winner outcome
	for range 2 {
		result := <-results
		if result.err == nil {
			require.True(t, winner.tool.Id.IsNil(), "only one writer can win")
			winner = result
		} else {
			require.ErrorIs(t, result.err, ErrToolRevisionConflict)
			require.Equal(t, uint64(1), result.tool.ActiveRevision)
		}
	}
	require.False(t, winner.tool.Id.IsNil())
	stored, err := db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, winner.tool.Name, stored.Name)
	require.Equal(t, uint64(2), stored.ActiveRevision)
	current, err := db.GetCurrentToolRevision(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(2), current.Revision)
	require.Equal(t, winner.revision.EncryptedDefinition, current.EncryptedDefinition)
	require.Equal(t, winner.revision.DefinitionDigest, current.DefinitionDigest)
	require.Equal(t, winner.revision.ConnectionId, current.ConnectionId)
	require.Equal(t, 2, countToolRevisionsForTest(t, ctx, db, tool.Id))
}

// TestStandaloneToolValidation rejects malformed storage inputs without changing
// the caller or creating history, and enforces immutable namespace ownership.
func TestStandaloneToolValidation(t *testing.T) {
	ctx, db, tool, first := standaloneToolForTest(t)
	require.Error(t, (*Tool)(nil).Validate())
	require.Error(t, (*ToolRevision)(nil).Validate())
	for _, change := range []func(*Tool){
		func(v *Tool) { v.Id = apid.Nil },
		func(v *Tool) { v.Id = v.ConnectionId },
		func(v *Tool) { v.Namespace = "root..bad" },
		func(v *Tool) { v.Name = "bad/name" },
		func(v *Tool) { v.ConnectionId = apid.Nil },
		func(v *Tool) { v.ConnectionId = v.Id },
		func(v *Tool) { v.ActiveRevision = 0 },
		func(v *Tool) { v.ActiveRevision = math.MaxInt64 + 1 },
	} {
		invalid := *tool
		change(&invalid)
		require.Error(t, invalid.Validate())
	}
	for _, change := range []func(*ToolRevision){
		func(v *ToolRevision) { v.ToolId = apid.Nil },
		func(v *ToolRevision) { v.ToolId = v.ConnectionId },
		func(v *ToolRevision) { v.ConnectionId = apid.Nil },
		func(v *ToolRevision) { v.ConnectionId = v.ToolId },
		func(v *ToolRevision) { v.Revision = 0 },
		func(v *ToolRevision) { v.Revision = math.MaxInt64 + 1 },
		func(v *ToolRevision) { v.EncryptedDefinition = encfield.EncryptedField{} },
		func(v *ToolRevision) { v.DefinitionDigest = "" },
		func(v *ToolRevision) { v.DefinitionDigest = strings.Repeat("a", 63) },
		func(v *ToolRevision) { v.DefinitionDigest = strings.Repeat("A", 64) },
		func(v *ToolRevision) { v.DefinitionDigest = strings.Repeat("g", 64) },
	} {
		invalid := *first
		change(&invalid)
		require.Error(t, invalid.Validate())
	}
	_, err := db.CreateStandaloneTool(ctx, nil, first.EncryptedDefinition, first.DefinitionDigest)
	require.Error(t, err)
	_, err = db.UpdateStandaloneTool(ctx, nil, 1, first.EncryptedDefinition, first.DefinitionDigest)
	require.Error(t, err)
	for _, id := range []apid.ID{apid.Nil, tool.ConnectionId} {
		_, err := db.GetTool(ctx, id)
		require.Error(t, err)
		_, err = db.GetCurrentToolRevision(ctx, id)
		require.Error(t, err)
		_, err = db.GetToolRevision(ctx, id, 1)
		require.Error(t, err)
		require.Error(t, db.DeleteStandaloneTool(ctx, id, 1))
	}
	for _, revision := range []uint64{0, math.MaxInt64 + 1} {
		_, err := db.GetToolRevision(ctx, tool.Id, revision)
		require.Error(t, err)
		_, err = db.UpdateStandaloneTool(ctx, tool, revision, first.EncryptedDefinition, first.DefinitionDigest)
		require.Error(t, err)
		require.Error(t, db.DeleteStandaloneTool(ctx, tool.Id, revision))
	}

	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.other"}))
	other := standaloneToolConnectionForTest(t, ctx, db, "root.other")
	for _, change := range []func(*Tool){
		func(v *Tool) { v.Namespace = other.Namespace },
		func(v *Tool) { v.ConnectionId = other.Id },
		func(v *Tool) { v.ConnectionId = apid.New(apid.PrefixConnection) },
	} {
		invalid := *tool
		change(&invalid)
		before := invalid
		_, err := db.UpdateStandaloneTool(ctx, &invalid, 1, first.EncryptedDefinition, first.DefinitionDigest)
		require.Error(t, err)
		require.Equal(t, before, invalid)
		invalid.Id = apid.New(apid.PrefixTool)
		before = invalid
		_, err = db.CreateStandaloneTool(ctx, &invalid, first.EncryptedDefinition, first.DefinitionDigest)
		require.Error(t, err)
		require.Equal(t, before, invalid)
		require.Zero(t, countToolRevisionsForTest(t, ctx, db, invalid.Id))
	}
	before := *tool
	_, err = db.UpdateStandaloneTool(ctx, tool, 1, encfield.EncryptedField{}, first.DefinitionDigest)
	require.Error(t, err)
	_, err = db.UpdateStandaloneTool(ctx, tool, 1, first.EncryptedDefinition, "invalid")
	require.Error(t, err)
	require.Equal(t, before, *tool)
	require.Equal(t, 1, countToolRevisionsForTest(t, ctx, db, tool.Id))
}

// TestStandaloneToolCounterExhaustion refuses to wrap an actual stored maximum
// revision while allowing deletion, which does not append another revision.
func TestStandaloneToolCounterExhaustion(t *testing.T) {
	ctx, db, tool, first := standaloneToolForTest(t)
	s := db.(*service)
	last := *first
	last.Revision = math.MaxInt64
	_, err := s.sq.Insert(ToolRevisionsTable).
		Columns(last.cols()...).
		Values(last.values()...).
		RunWith(s.db).
		ExecContext(ctx)
	require.NoError(t, err)
	_, err = s.sq.Update(ToolsTable).
		Set("active_revision", int64(math.MaxInt64)).
		Where(sq.Eq{"id": tool.Id}).
		RunWith(s.db).
		ExecContext(ctx)
	require.NoError(t, err)
	tool, err = db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.NoError(t, tool.Validate())
	require.NoError(t, last.Validate())
	before := *tool
	_, err = db.UpdateStandaloneTool(ctx, tool, math.MaxInt64, first.EncryptedDefinition, first.DefinitionDigest)
	require.ErrorIs(t, err, ErrToolRevisionConflict)
	require.Equal(t, before, *tool)
	current, err := db.GetCurrentToolRevision(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(math.MaxInt64), current.Revision)
	require.Equal(t, 2, countToolRevisionsForTest(t, ctx, db, tool.Id))
	require.NoError(t, db.DeleteStandaloneTool(ctx, tool.Id, math.MaxInt64))
}

// TestStandaloneToolRevisionConstraints ensures the active pointer always names
// an existing revision and SQL rejects invalid or orphaned history rows.
func TestStandaloneToolRevisionConstraints(t *testing.T) {
	ctx, db, tool, first := standaloneToolForTest(t)
	s := db.(*service)
	_, err := s.sq.Update(ToolsTable).
		Set("active_revision", 2).
		Where(sq.Eq{"id": tool.Id}).
		RunWith(s.db).
		ExecContext(ctx)
	require.Error(t, err, "a committed active pointer cannot refer to absent history")
	_, err = s.sq.Delete(ToolRevisionsTable).
		Where(sq.Eq{"tool_id": tool.Id, "revision": 1}).
		RunWith(s.db).
		ExecContext(ctx)
	require.Error(t, err, "an active revision cannot be physically deleted")
	for _, change := range []func(*ToolRevision){
		func(v *ToolRevision) { v.Revision = 1 },
		func(v *ToolRevision) { v.Revision = 0 },
		func(v *ToolRevision) { v.ToolId = apid.New(apid.PrefixTool) },
		func(v *ToolRevision) { v.EncryptedDefinition = encfield.EncryptedField{} },
		func(v *ToolRevision) { v.DefinitionDigest = "short" },
	} {
		invalid := *first
		invalid.Revision = 2
		change(&invalid)
		_, err = s.sq.Insert(ToolRevisionsTable).
			Columns(invalid.cols()...).
			Values(invalid.values()...).
			RunWith(s.db).
			ExecContext(ctx)
		require.Error(t, err)
	}
	current, err := db.GetCurrentToolRevision(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(1), current.Revision)
	require.Equal(t, 1, countToolRevisionsForTest(t, ctx, db, tool.Id))
}

// TestStandaloneToolRevisionReEncryption rotates one composite history identity
// without changing its sibling revision, active pointer, or definition digest.
func TestStandaloneToolRevisionReEncryption(t *testing.T) {
	ctx, db, tool, first := standaloneToolForTest(t)
	second, err := db.UpdateStandaloneTool(ctx, tool, 1, first.EncryptedDefinition, first.DefinitionDigest)
	require.NoError(t, err)
	s := db.(*service)
	targetKey := apid.New(apid.PrefixDataEncryptionKey)
	_, err = s.sq.Update(NamespacesTable).
		Set("target_data_encryption_key_id", targetKey).
		Where(sq.Eq{"path": tool.Namespace}).
		RunWith(s.db).
		ExecContext(ctx)
	require.NoError(t, err)
	var targets []ReEncryptionTarget
	err = db.EnumerateFieldsRequiringReEncryption(ctx, func(page []ReEncryptionTarget, _ bool) (pagination.KeepGoing, error) {
		for _, target := range page {
			if target.Table == ToolRevisionsTable {
				targets = append(targets, target)
			}
		}
		return pagination.Continue, nil
	})
	require.NoError(t, err)
	require.Len(t, targets, 2)
	target := targets[0]
	if target.PrimaryKeyValues[1] == int64(2) {
		target = targets[1]
	}
	require.Equal(t, []string{"tool_id", "revision"}, target.PrimaryKeyCols)
	require.Equal(t, []any{tool.Id.String(), int64(1)}, target.PrimaryKeyValues)
	require.Equal(t, targetKey, target.TargetDataEncryptionKeyId)
	require.Equal(t, "encrypted_definition", target.FieldColumn)
	update := ReEncryptedFieldUpdate{
		Table:            target.Table,
		PrimaryKeyCols:   target.PrimaryKeyCols,
		PrimaryKeyValues: target.PrimaryKeyValues,
		FieldColumn:      target.FieldColumn,
		ExpectedValue:    target.EncryptedFieldValue,
		NewValue: encfield.EncryptedField{
			ID:   targetKey,
			Data: "cm90YXRlZA==",
		},
	}
	rotatedAt := first.CreatedAt.Add(time.Hour)
	require.NoError(t, db.BatchUpdateReEncryptedFields(apctx.WithFixedClock(ctx, rotatedAt), []ReEncryptedFieldUpdate{update}))
	rotated, err := db.GetToolRevision(ctx, tool.Id, 1)
	require.NoError(t, err)
	require.Equal(t, update.NewValue, rotated.EncryptedDefinition)
	require.Equal(t, first.DefinitionDigest, rotated.DefinitionDigest)
	require.True(t, first.CreatedAt.Equal(rotated.CreatedAt))
	require.True(t, rotatedAt.Equal(*rotated.EncryptedAt))

	update.NewValue.Data = "b2xkLXdvcms="
	require.NoError(t, db.BatchUpdateReEncryptedFields(apctx.WithFixedClock(ctx, rotatedAt.Add(time.Hour)), []ReEncryptedFieldUpdate{update}))
	stillRotated, err := db.GetToolRevision(ctx, tool.Id, 1)
	require.NoError(t, err)
	require.Equal(t, rotated.EncryptedDefinition, stillRotated.EncryptedDefinition)
	require.True(t, rotatedAt.Equal(*stillRotated.EncryptedAt))
	current, err := db.GetCurrentToolRevision(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, second.Revision, current.Revision)
	require.Equal(t, second.EncryptedDefinition, current.EncryptedDefinition)
	require.True(t, second.EncryptedAt.Equal(*current.EncryptedAt))
	stored, err := db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, tool.ActiveRevision, stored.ActiveRevision)
	require.True(t, tool.UpdatedAt.Equal(stored.UpdatedAt))
}

// TestStandaloneToolMigrationReversibility drops the mutually dependent Tool
// tables without disturbing their connection, then permits fresh creation.
func TestStandaloneToolMigrationReversibility(t *testing.T) {
	ctx, db, tool, first := standaloneToolForTest(t)
	migrateDatabaseToVersion(t, db.(*service), 19)
	_, err := db.GetConnection(ctx, tool.ConnectionId)
	require.NoError(t, err)
	migrateDatabaseToVersion(t, db.(*service), 23)
	_, err = db.GetTool(ctx, tool.Id)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = db.CreateStandaloneTool(ctx, tool, first.EncryptedDefinition, first.DefinitionDigest)
	require.NoError(t, err)
}
