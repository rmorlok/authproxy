package database

import (
	"context"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

// toolOwnershipBindingForTest creates a matching assignment without publishing
// an inventory. Ownership storage is deliberately tested before activation exists.
func toolOwnershipBindingForTest(
	t *testing.T,
	ctx context.Context,
	db DB,
	tool *Tool,
	name common.ResourceName,
) *ToolSetBinding {
	t.Helper()

	toolSet := toolSetForDatabaseTest(tool.Namespace, name)
	require.NoError(t, db.CreateToolSet(ctx, toolSet))

	binding, err := db.CreateToolSetBinding(ctx, toolSet.Id, tool.ConnectionId, 1)
	require.NoError(t, err)

	return binding
}

// generatedToolForTest assigns ownership directly after ordinary creation because
// production inventory installation is a later slice, not a standalone write.
func generatedToolForTest(
	t *testing.T,
	ctx context.Context,
	db DB,
	base *Tool,
	definition *ToolRevision,
	binding *ToolSetBinding,
	key string,
) *Tool {
	t.Helper()
	tool := createToolCandidateForTest(t, ctx, db, base, definition, nil, base.Verbs, base.Labels)
	s := db.(*service)

	_, err := s.sq.
		Update(ToolsTable).
		Set("binding_id", binding.Id).
		Set("source_key", key).
		Where(sq.Eq{"id": tool.Id}).
		RunWith(s.db).
		ExecContext(ctx)
	require.NoError(t, err)

	tool, err = db.GetTool(ctx, tool.Id)
	require.NoError(t, err)

	return tool
}

// TestGeneratedToolValidation requires paired ownership and a nonblank exact
// source key without normalizing provider-specific punctuation or whitespace.
func TestGeneratedToolValidation(t *testing.T) {
	_, _, base, _ := standaloneToolForTest(t)
	bindingID := apid.New(apid.PrefixToolSetBinding)
	for _, test := range []struct {
		binding apid.ID
		key     string
		valid   bool
	}{
		{apid.Nil, "", true},
		{bindingID, "lookup", true},
		{bindingID, "  GET /v1/{id}?a='b'%_\t", true},
		{apid.Nil, "lookup", false},
		{bindingID, "", false},
		{bindingID, " \t\n", false},
		{base.ConnectionId, "lookup", false},
	} {
		candidate := copyToolMetadataForTest(base)
		candidate.BindingId, candidate.SourceKey = test.binding, test.key
		before := copyToolMetadataForTest(&candidate)
		err := candidate.Validate()
		if test.valid {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
		require.Equal(t, before, candidate, "validation must not normalize source identity")
	}
}

// TestGeneratedToolStandaloneGuards preserves identities, metadata, and history
// when standalone writes attempt to author or clear managed ownership.
func TestGeneratedToolStandaloneGuards(t *testing.T) {
	ctx, db, base, definition := standaloneToolForTest(t)
	base = readToolRowForTest(t, ctx, db, base.Id)
	binding := toolOwnershipBindingForTest(t, ctx, db, base, "tools")
	generated := generatedToolForTest(t, ctx, db, base, definition, binding, "lookup")
	for _, strip := range []bool{false, true} {
		candidate := copyToolMetadataForTest(generated)
		candidate.Name = "changed"
		candidate.Labels, candidate.Annotations = Labels{"new": "label"}, Annotations{"new": "note"}
		if strip {
			candidate.BindingId, candidate.SourceKey = apid.Nil, ""
		}
		before := copyToolMetadataForTest(&candidate)
		_, err := db.UpdateStandaloneTool(
			ctx,
			&candidate,
			1,
			definition.EncryptedDefinition,
			definition.DefinitionDigest,
		)
		require.ErrorIs(t, err, ErrToolManaged)
		require.Equal(t, before, candidate)
	}

	candidate := copyToolMetadataForTest(base)
	candidate.BindingId, candidate.SourceKey = binding.Id, "new-operation"
	before := copyToolMetadataForTest(&candidate)
	_, err := db.UpdateStandaloneTool(
		ctx,
		&candidate,
		1,
		definition.EncryptedDefinition,
		definition.DefinitionDigest,
	)
	require.ErrorIs(t, err, ErrToolManaged)
	require.Equal(t, before, candidate)

	candidate.Id, candidate.Name = apid.New(apid.PrefixTool), "new-tool"
	before = copyToolMetadataForTest(&candidate)
	_, err = db.CreateStandaloneTool(ctx, &candidate, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.ErrorIs(t, err, ErrToolManaged)
	require.Equal(t, before, candidate)

	_, err = db.GetTool(ctx, candidate.Id)
	require.ErrorIs(t, err, ErrNotFound)
	require.Zero(t, countToolRevisionsForTest(t, ctx, db, candidate.Id))
	require.ErrorIs(t, db.DeleteStandaloneTool(ctx, generated.Id, 1), ErrToolManaged)

	stored, err := db.GetTool(ctx, generated.Id)
	require.NoError(t, err)
	require.Equal(t, generated, stored)

	unchanged, err := db.GetTool(ctx, base.Id)
	require.NoError(t, err)
	require.Equal(t, base, unchanged)

	for _, tool := range []*Tool{generated, base} {
		require.Equal(t, 1, countToolRevisionsForTest(t, ctx, db, tool.Id))
		history, err := db.GetToolRevision(ctx, tool.Id, 1)
		require.NoError(t, err, "generated history remains inspectable")
		require.Equal(t, definition.EncryptedDefinition, history.EncryptedDefinition)
		require.Equal(t, definition.DefinitionDigest, history.DefinitionDigest)
	}

	_, err = db.GetCurrentToolRevision(ctx, generated.Id)
	require.ErrorIs(t, err, ErrNotFound, "ownership alone must not activate execution")

	page, err := db.ListToolCandidates(ctx, ToolCandidateQuery{NamespaceMatchers: []string{base.Namespace}, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, []apid.ID{base.Id}, toolCandidateIDsForTest(page))
	require.Empty(t, page.Cursor)
}

// TestGeneratedToolSourceIdentity retains case, punctuation, and surrounding
// whitespace while reserving each binding/source pair even after soft deletion.
func TestGeneratedToolSourceIdentity(t *testing.T) {
	ctx, db, base, definition := standaloneToolForTest(t)
	binding := toolOwnershipBindingForTest(t, ctx, db, base, "tools")

	for _, key := range []string{
		"lookup",
		"Lookup",
		" lookup ",
		"GET /v1/{id}?a='b'%_\t",
	} {
		tool := generatedToolForTest(t, ctx, db, base, definition, binding, key)
		require.Equal(t, binding.Id, tool.BindingId)
		require.Equal(t, key, tool.SourceKey)
	}

	otherBinding := toolOwnershipBindingForTest(t, ctx, db, base, "other-tools")
	other := generatedToolForTest(t, ctx, db, base, definition, otherBinding, "lookup")
	s := db.(*service)
	_, err := s.sq.
		Update(ToolsTable).
		Set("deleted_at", apctx.GetClock(ctx).Now()).
		Where(sq.Eq{
			"binding_id": binding.Id,
			"source_key": "lookup",
		}).
		RunWith(s.db).
		ExecContext(ctx)
	require.NoError(t, err)
	_, err = s.sq.Update(ToolsTable).Set("binding_id", binding.Id).
		Where(sq.Eq{"id": other.Id}).RunWith(s.db).ExecContext(ctx)
	require.Error(t, err, "retired source keys cannot allocate a different Tool identity")

	stored, err := db.GetTool(ctx, other.Id)
	require.NoError(t, err)
	require.Equal(t, otherBinding.Id, stored.BindingId)
	require.Equal(t, 1, countToolRevisionsForTest(t, ctx, db, other.Id))
}

// TestGeneratedToolDatabaseConstraints rejects half-owned and orphan rows even
// when writes bypass model validation and the standalone mutation methods.
func TestGeneratedToolDatabaseConstraints(t *testing.T) {
	ctx, db, base, _ := standaloneToolForTest(t)
	binding := toolOwnershipBindingForTest(t, ctx, db, base, "tools")
	s := db.(*service)
	for _, fields := range []map[string]any{
		{"binding_id": nil, "source_key": "lookup"},
		{"binding_id": binding.Id, "source_key": ""},
		{"binding_id": binding.Id, "source_key": "   "},
		{"binding_id": apid.New(apid.PrefixToolSetBinding), "source_key": "lookup"},
	} {
		_, err := s.sq.Update(ToolsTable).SetMap(fields).
			Where(sq.Eq{"id": base.Id}).RunWith(s.db).ExecContext(ctx)
		require.Error(t, err)
		stored, err := db.GetTool(ctx, base.Id)
		require.NoError(t, err)
		require.True(t, stored.BindingId.IsNil())
		require.Empty(t, stored.SourceKey)
	}
}

// TestGeneratedToolParentPurge removes generated identities and their history
// through either ownership or connection cascades, without orphan revisions.
func TestGeneratedToolParentPurge(t *testing.T) {
	for _, parent := range []string{"binding", "connection"} {
		t.Run(parent, func(t *testing.T) {
			ctx, db, base, definition := standaloneToolForTest(t)
			binding := toolOwnershipBindingForTest(t, ctx, db, base, "tools")
			generated := generatedToolForTest(t, ctx, db, base, definition, binding, "lookup")
			s := db.(*service)
			if parent == "binding" {
				_, err := s.sq.
					Delete(ToolSetBindingsTable).
					Where(sq.Eq{"id": binding.Id}).
					RunWith(s.db).
					ExecContext(ctx)
				require.NoError(t, err)
				_, err = db.GetTool(ctx, base.Id)
				require.NoError(t, err, "removing a binding must preserve standalone siblings")
			} else {
				require.NoError(t, db.DeleteConnection(ctx, base.ConnectionId))
				_, err := db.PurgeSoftDeletedRecords(ctx, apctx.GetClock(ctx).Now().Add(time.Second))
				require.NoError(t, err)
				require.Zero(t, countToolRevisionsForTest(t, ctx, db, base.Id))
			}
			_, err := db.GetTool(ctx, generated.Id)
			require.ErrorIs(t, err, ErrNotFound)
			require.Zero(t, countToolRevisionsForTest(t, ctx, db, generated.Id))
			var bindings int
			err = s.sq.
				Select("COUNT(*)").
				From(ToolSetBindingsTable).
				Where(sq.Eq{"id": binding.Id}).
				RunWith(s.db).
				QueryRowContext(ctx).
				Scan(&bindings)
			require.NoError(t, err)
			require.Zero(t, bindings)
			_, err = db.GetToolSet(ctx, binding.ToolSetId)
			require.NoError(t, err)
		})
	}
}

// TestGeneratedToolMigration discards owned rows on downgrade rather than
// exposing them as standalone, while preserving independent rows and history.
func TestGeneratedToolMigration(t *testing.T) {
	ctx, db, base, definition := standaloneToolForTest(t)
	base.Labels, base.Annotations = Labels{"local": "retained"}, Annotations{"note": "retained"}
	_, err := db.UpdateStandaloneTool(ctx, base, 1, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	binding := toolOwnershipBindingForTest(t, ctx, db, base, "tools")
	generated := generatedToolForTest(t, ctx, db, base, definition, binding, "lookup")
	retired := generatedToolForTest(t, ctx, db, base, definition, binding, "retired")
	s := db.(*service)

	_, err = s.sq.
		Update(ToolsTable).
		Set("deleted_at", apctx.GetClock(ctx).Now()).
		Where(sq.Eq{"id": retired.Id}).
		RunWith(s.db).
		ExecContext(ctx)
	require.NoError(t, err)

	before, err := db.GetTool(ctx, base.Id)
	require.NoError(t, err)
	migrateDatabaseToVersion(t, s, 22)
	var remaining int
	err = s.sq.
		Select("COUNT(*)").
		From(ToolsTable).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(&remaining)
	require.NoError(t, err)
	require.Equal(t, 1, remaining)

	for _, tool := range []*Tool{generated, retired} {
		require.Zero(t, countToolRevisionsForTest(t, ctx, db, tool.Id))
	}
	require.Equal(t, 2, countToolRevisionsForTest(t, ctx, db, base.Id))

	var name common.ResourceName
	var revision uint64
	err = s.sq.
		Select("name", "active_revision").
		From(ToolsTable).
		Where(sq.Eq{"id": base.Id}).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(&name, &revision)
	require.NoError(t, err)
	require.Equal(t, base.Name, name)
	require.Equal(t, base.ActiveRevision, revision)

	migrateDatabaseToVersion(t, s, 23)
	stored, err := db.GetTool(ctx, base.Id)
	require.NoError(t, err)
	require.Equal(t, before, stored)

	current, err := db.GetCurrentToolRevision(ctx, base.Id)
	require.NoError(t, err)
	require.Equal(t, definition.EncryptedDefinition, current.EncryptedDefinition)
	require.Equal(t, definition.DefinitionDigest, current.DefinitionDigest)

	history, err := db.GetToolRevision(ctx, base.Id, 1)
	require.NoError(t, err)
	require.Equal(t, definition.EncryptedDefinition, history.EncryptedDefinition)

	for _, tool := range []*Tool{generated, retired} {
		_, err := db.GetTool(ctx, tool.Id)
		require.ErrorIs(t, err, ErrNotFound)
	}

	_, err = db.GetToolSetBinding(ctx, binding.Id)
	require.NoError(t, err, "the downgrade removes inventory, not membership")
}
