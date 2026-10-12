package database

import (
	"context"
	"math"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/config"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/stretchr/testify/require"
)

// toolSetGenerationForDatabaseTest creates a logical parent and its first draft
// with a fixed clock. Ciphertext is opaque to these persistence operations.
func toolSetGenerationForDatabaseTest(t *testing.T) (context.Context, DB, *ToolSet, *ToolSetGeneration) {
	t.Helper()
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.WithFixedClock(context.Background(), time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC))
	parent := toolSetForDatabaseTest("root", "tools")
	parent.Labels = Labels{"team": "support"}
	parent.Annotations = Annotations{"note": "logical"}
	require.NoError(t, db.CreateToolSet(ctx, parent))
	draft, err := db.CreateToolSetGeneration(ctx, parent.Id, encfield.EncryptedField{
		ID:   apid.New(apid.PrefixDataEncryptionKey),
		Data: "b3JpZ2luYWw=",
	})
	require.NoError(t, err)
	return ctx, db, parent, draft
}

// TestToolSetGenerationLifecycle keeps ordinary definition edits draft-only and
// switches primaries atomically without rewriting logical selection or metadata.
func TestToolSetGenerationLifecycle(t *testing.T) {
	// Create the toolset generation
	ctx, db, parent, first := toolSetGenerationForDatabaseTest(t)
	now := apctx.GetClock(ctx).Now()
	require.NoError(t, first.Validate())
	require.Equal(t, uint64(1), first.Generation)
	require.Equal(t, uint64(1), first.Revision)
	require.Equal(t, toolsetschema.ToolSetReleaseStateDraft, first.State)
	require.True(t, now.Equal(first.CreatedAt))
	require.True(t, now.Equal(first.UpdatedAt))
	require.NotNil(t, first.EncryptedAt)
	require.True(t, now.Equal(*first.EncryptedAt))

	// It won't let use create another of the same generation
	_, err := db.CreateToolSetGeneration(ctx, parent.Id, first.EncryptedDefinition)
	require.ErrorIs(t, err, ErrDuplicate)

	// Create a new generation with an update
	editedAt := now.Add(time.Minute)
	definition := first.EncryptedDefinition
	definition.Data = "cmVwbGFjZW1lbnQ="
	edited, err := db.UpdateToolSetGenerationDraft(apctx.WithFixedClock(ctx, editedAt), parent.Id, 1, 1, definition)
	require.NoError(t, err)
	require.Equal(t, uint64(2), edited.Revision)
	require.Equal(t, definition, edited.EncryptedDefinition)
	require.True(t, now.Equal(edited.CreatedAt))
	require.True(t, editedAt.Equal(edited.UpdatedAt))
	require.True(t, editedAt.Equal(*edited.EncryptedAt))

	// Try to update with that same generation is an error
	_, err = db.UpdateToolSetGenerationDraft(ctx, parent.Id, 1, 1, first.EncryptedDefinition)
	require.ErrorIs(t, err, ErrToolSetGenerationRevisionConflict)

	// Publish it
	publishedAt := now.Add(2 * time.Minute)
	published, err := db.PublishToolSetGeneration(apctx.WithFixedClock(ctx, publishedAt), parent.Id, 1, 2)
	require.NoError(t, err)
	require.Equal(t, toolsetschema.ToolSetReleaseStatePrimary, published.State)
	require.Equal(t, uint64(2), published.Revision)
	require.True(t, publishedAt.Equal(published.UpdatedAt))
	require.True(t, editedAt.Equal(*published.EncryptedAt))

	// Second publish fails
	_, err = db.PublishToolSetGeneration(ctx, parent.Id, 1, 2)
	require.ErrorIs(t, err, ErrToolSetGenerationRevisionConflict)

	// Cannot update same generation
	_, err = db.UpdateToolSetGenerationDraft(ctx, parent.Id, 1, 2, first.EncryptedDefinition)
	require.ErrorIs(t, err, ErrToolSetGenerationRevisionConflict)

	// Create another toolset generation
	second, err := db.CreateToolSetGeneration(ctx, parent.Id, first.EncryptedDefinition)
	require.NoError(t, err)
	require.Equal(t, uint64(2), second.Generation)

	// Allows update to draft
	second, err = db.UpdateToolSetGenerationDraft(ctx, parent.Id, 2, 1, second.EncryptedDefinition)
	require.NoError(t, err)
	require.Equal(t, uint64(2), second.Revision, "accepted equivalent edits still invalidate older publication work")

	_, err = db.PublishToolSetGeneration(ctx, parent.Id, 2, 1)
	require.ErrorIs(t, err, ErrToolSetGenerationRevisionConflict)
	current, err := db.GetToolSetGenerationByState(ctx, parent.Id, toolsetschema.ToolSetReleaseStatePrimary)
	require.NoError(t, err)
	require.Equal(t, uint64(1), current.Generation, "failed publication must retain the former primary")
	require.Equal(t, definition, current.EncryptedDefinition)

	_, err = db.PublishToolSetGeneration(ctx, parent.Id, 2, 2)
	require.NoError(t, err)
	current, err = db.GetToolSetGenerationByState(ctx, parent.Id, toolsetschema.ToolSetReleaseStatePrimary)
	require.NoError(t, err)
	require.Equal(t, uint64(2), current.Generation)
	previous, err := db.GetToolSetGeneration(ctx, parent.Id, 1)
	require.NoError(t, err)
	require.Equal(t, toolsetschema.ToolSetReleaseStateActive, previous.State)
	require.Equal(t, definition, previous.EncryptedDefinition)
	require.Equal(t, uint64(2), previous.Revision)
	_, err = db.UpdateToolSetGenerationDraft(ctx, parent.Id, 1, 2, first.EncryptedDefinition)
	require.ErrorIs(t, err, ErrToolSetGenerationRevisionConflict)
	_, err = db.GetToolSetGenerationByState(ctx, parent.Id, toolsetschema.ToolSetReleaseStateDraft)
	require.ErrorIs(t, err, ErrNotFound)

	logical, err := db.GetToolSet(ctx, parent.Id)
	require.NoError(t, err)
	require.Equal(t, parent.Name, logical.Name)
	require.Equal(t, parent.Namespace, logical.Namespace)
	require.Equal(t, parent.Labels, logical.Labels)
	require.Equal(t, parent.Annotations, logical.Annotations)
	require.Equal(t, parent.ConnectionSelector, logical.ConnectionSelector)
	require.Equal(t, parent.SelectorRevision, logical.SelectorRevision)
	require.True(t, parent.CreatedAt.Equal(logical.CreatedAt))
	require.True(t, parent.UpdatedAt.Equal(logical.UpdatedAt))
}

// TestToolSetGenerationValidation checks persisted counters and rejects invalid
// operation inputs without treating historical states as unique state lookups.
func TestToolSetGenerationValidation(t *testing.T) {
	ctx, db, parent, draft := toolSetGenerationForDatabaseTest(t)
	require.Error(t, (*ToolSetGeneration)(nil).Validate())
	for _, change := range []func(*ToolSetGeneration){
		func(g *ToolSetGeneration) { g.ToolSetId = apid.Nil },
		func(g *ToolSetGeneration) { g.ToolSetId = apid.New(apid.PrefixConnector) },
		func(g *ToolSetGeneration) { g.Generation = 0 },
		func(g *ToolSetGeneration) { g.Generation = math.MaxInt64 + 1 },
		func(g *ToolSetGeneration) { g.Revision = 0 },
		func(g *ToolSetGeneration) { g.Revision = math.MaxInt64 + 1 },
		func(g *ToolSetGeneration) { g.State = "unknown" },
		func(g *ToolSetGeneration) { g.EncryptedDefinition = encfield.EncryptedField{} },
	} {
		invalid := *draft
		change(&invalid)
		require.Error(t, invalid.Validate())
	}

	// Invalid id checks
	for _, id := range []apid.ID{
		apid.Nil,
		apid.New(apid.PrefixActor),
	} {
		_, err := db.CreateToolSetGeneration(ctx, id, draft.EncryptedDefinition)
		require.Error(t, err)

		_, err = db.GetToolSetGeneration(ctx, id, 1)
		require.Error(t, err)
	}

	// Invalid generation checks
	for _, generation := range []uint64{
		0,
		math.MaxInt64 + 1,
	} {
		_, err := db.GetToolSetGeneration(ctx, parent.Id, generation)
		require.Error(t, err)
		_, err = db.UpdateToolSetGenerationDraft(ctx, parent.Id, generation, 1, draft.EncryptedDefinition)
		require.Error(t, err)
		_, err = db.PublishToolSetGeneration(ctx, parent.Id, generation, 1)
		require.Error(t, err)
	}

	// Invalid revision checks
	for _, revision := range []uint64{
		0,
		math.MaxInt64 + 1,
	} {
		_, err := db.UpdateToolSetGenerationDraft(ctx, parent.Id, 1, revision, draft.EncryptedDefinition)
		require.Error(t, err)
		_, err = db.PublishToolSetGeneration(ctx, parent.Id, 1, revision)
		require.Error(t, err)
	}

	for _, state := range []toolsetschema.ToolSetReleaseState{
		"",
		"unknown",
		toolsetschema.ToolSetReleaseStateActive,
		toolsetschema.ToolSetReleaseStateArchived,
	} {
		_, err := db.GetToolSetGenerationByState(ctx, parent.Id, state)
		require.Error(t, err)
	}

	_, err := db.CreateToolSetGeneration(ctx, parent.Id, encfield.EncryptedField{})
	require.Error(t, err)

	_, err = db.UpdateToolSetGenerationDraft(ctx, parent.Id, 1, 1, encfield.EncryptedField{})
	require.Error(t, err)
}

// TestToolSetGenerationPublicationRollback injects a failure after demotion so
// publication must roll back both release state and the former primary's time.
func TestToolSetGenerationPublicationRollback(t *testing.T) {
	ctx, db, parent, first := toolSetGenerationForDatabaseTest(t)
	_, err := db.PublishToolSetGeneration(ctx, parent.Id, 1, 1)
	require.NoError(t, err)

	second, err := db.CreateToolSetGeneration(ctx, parent.Id, first.EncryptedDefinition)
	require.NoError(t, err)

	s := db.(*service)
	statements := []string{`CREATE TRIGGER reject_tool_set_publication
		BEFORE UPDATE OF state ON tool_set_generations
		WHEN NEW.generation = 2 AND NEW.state = 'primary'
		BEGIN SELECT RAISE(ABORT, 'test publication failure'); END`}
	if s.cfg.GetProvider() == config.DatabaseProviderPostgres {
		statements = []string{
			`CREATE FUNCTION reject_tool_set_publication() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN
				IF NEW.generation = 2 AND NEW.state = 'primary' THEN
					RAISE EXCEPTION 'test publication failure';
				END IF;
				RETURN NEW;
			END; $$`,
			`CREATE TRIGGER reject_tool_set_publication BEFORE UPDATE OF state ON tool_set_generations
			FOR EACH ROW EXECUTE FUNCTION reject_tool_set_publication()`,
		}
	}
	for _, statement := range statements {
		_, err := s.db.ExecContext(ctx, statement)
		require.NoError(t, err)
	}

	later := apctx.WithFixedClock(ctx, first.UpdatedAt.Add(time.Hour))
	_, err = db.PublishToolSetGeneration(later, parent.Id, 2, 1)
	require.ErrorContains(t, err, "test publication failure")

	current, err := db.GetToolSetGenerationByState(ctx, parent.Id, toolsetschema.ToolSetReleaseStatePrimary)
	require.NoError(t, err)
	require.Equal(t, uint64(1), current.Generation)
	require.True(t, first.UpdatedAt.Equal(current.UpdatedAt))

	pending, err := db.GetToolSetGenerationByState(ctx, parent.Id, toolsetschema.ToolSetReleaseStateDraft)
	require.NoError(t, err)
	require.Equal(t, uint64(2), pending.Generation)
	require.True(t, second.UpdatedAt.Equal(pending.UpdatedAt))
}

// TestToolSetGenerationTombstones reserves historical generation numbers and
// hides deleted generations and logical parents from every storage operation.
func TestToolSetGenerationTombstones(t *testing.T) {
	ctx, db, parent, first := toolSetGenerationForDatabaseTest(t)
	s := db.(*service)
	_, err := s.sq.Update(ToolSetGenerationsTable).
		Set("deleted_at", apctx.GetClock(ctx).Now()).
		Where(sq.Eq{"tool_set_id": parent.Id, "generation": 1}).
		RunWith(s.db).
		ExecContext(ctx)
	require.NoError(t, err)

	_, err = db.GetToolSetGeneration(ctx, parent.Id, 1)
	require.ErrorIs(t, err, ErrNotFound)

	_, err = db.UpdateToolSetGenerationDraft(ctx, parent.Id, 1, 1, first.EncryptedDefinition)
	require.ErrorIs(t, err, ErrNotFound)

	_, err = db.PublishToolSetGeneration(ctx, parent.Id, 1, 1)
	require.ErrorIs(t, err, ErrNotFound)

	second, err := db.CreateToolSetGeneration(ctx, parent.Id, first.EncryptedDefinition)
	require.NoError(t, err)
	require.Equal(t, uint64(2), second.Generation)

	duplicate := *first
	duplicate.State = toolsetschema.ToolSetReleaseStateArchived
	_, err = s.sq.Insert(ToolSetGenerationsTable).
		Columns(duplicate.cols()...).
		Values(duplicate.values()...).
		RunWith(s.db).
		ExecContext(ctx)
	require.Error(t, err, "soft deletion does not release the composite identity")

	require.NoError(t, db.DeleteToolSet(ctx, parent.Id))
	for _, id := range []apid.ID{
		parent.Id,
		apid.New(apid.PrefixToolSet),
	} {
		_, err := db.CreateToolSetGeneration(ctx, id, first.EncryptedDefinition)
		require.ErrorIs(t, err, ErrNotFound)

		_, err = db.GetToolSetGeneration(ctx, id, 2)
		require.ErrorIs(t, err, ErrNotFound)

		_, err = db.GetToolSetGenerationByState(ctx, id, toolsetschema.ToolSetReleaseStateDraft)
		require.ErrorIs(t, err, ErrNotFound)

		_, err = db.UpdateToolSetGenerationDraft(ctx, id, 2, 1, first.EncryptedDefinition)
		require.ErrorIs(t, err, ErrNotFound)

		_, err = db.PublishToolSetGeneration(ctx, id, 2, 1)
		require.ErrorIs(t, err, ErrNotFound)
	}
}

// TestToolSetGenerationConstraints verifies provider-enforced identity, state,
// and primary/draft uniqueness even when a write bypasses model validation.
func TestToolSetGenerationConstraints(t *testing.T) {
	ctx, db, parent, first := toolSetGenerationForDatabaseTest(t)
	s := db.(*service)
	for _, change := range []func(*ToolSetGeneration){
		func(g *ToolSetGeneration) { g.State = toolsetschema.ToolSetReleaseStateDraft },
		func(g *ToolSetGeneration) { g.Generation = 0 },
		func(g *ToolSetGeneration) { g.Revision = 0 },
		func(g *ToolSetGeneration) { g.State = "unknown" },
		func(g *ToolSetGeneration) { g.EncryptedDefinition = encfield.EncryptedField{} },
		func(g *ToolSetGeneration) { g.ToolSetId = apid.New(apid.PrefixToolSet) },
	} {
		invalid := *first
		invalid.Generation = 2
		invalid.State = toolsetschema.ToolSetReleaseStateActive
		change(&invalid)
		_, err := s.sq.Insert(ToolSetGenerationsTable).
			Columns(invalid.cols()...).
			Values(invalid.values()...).
			RunWith(s.db).
			ExecContext(ctx)
		require.Error(t, err)
	}
	_, err := db.PublishToolSetGeneration(ctx, parent.Id, 1, 1)
	require.NoError(t, err)

	secondPrimary := *first
	secondPrimary.Generation = 2
	secondPrimary.State = toolsetschema.ToolSetReleaseStatePrimary
	_, err = s.sq.Insert(ToolSetGenerationsTable).
		Columns(secondPrimary.cols()...).
		Values(secondPrimary.values()...).
		RunWith(s.db).
		ExecContext(ctx)
	require.Error(t, err, "a second live primary is forbidden")
}

// TestToolSetGenerationConcurrentWrites serializes draft creation and fences
// competing editing/publication against the same observed definition revision.
func TestToolSetGenerationConcurrentWrites(t *testing.T) {
	ctx, db, parent, first := toolSetGenerationForDatabaseTest(t)

	_, err := db.PublishToolSetGeneration(ctx, parent.Id, 1, 1)
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := db.CreateToolSetGeneration(ctx, parent.Id, first.EncryptedDefinition)
			results <- err
		}()
	}
	close(start)
	winners := 0
	for range 2 {
		if err := <-results; err != nil {
			require.ErrorIs(t, err, ErrDuplicate)
		} else {
			winners++
		}
	}
	require.Equal(t, 1, winners)

	start = make(chan struct{})
	replacement := first.EncryptedDefinition
	replacement.Data = "cmFjaW5nLWVkaXQ="
	go func() {
		<-start
		_, err := db.UpdateToolSetGenerationDraft(ctx, parent.Id, 2, 1, replacement)
		results <- err
	}()
	go func() {
		<-start
		_, err := db.PublishToolSetGeneration(ctx, parent.Id, 2, 1)
		results <- err
	}()
	close(start)
	winners = 0
	for range 2 {
		if err := <-results; err != nil {
			require.ErrorIs(t, err, ErrToolSetGenerationRevisionConflict)
		} else {
			winners++
		}
	}
	require.Equal(t, 1, winners)

	second, err := db.GetToolSetGeneration(ctx, parent.Id, 2)
	require.NoError(t, err)

	primary, err := db.GetToolSetGenerationByState(ctx, parent.Id, toolsetschema.ToolSetReleaseStatePrimary)
	require.NoError(t, err)

	if second.State == toolsetschema.ToolSetReleaseStateDraft {
		require.Equal(t, uint64(2), second.Revision)
		require.Equal(t, replacement, second.EncryptedDefinition)
		require.Equal(t, uint64(1), primary.Generation)
	} else {
		require.Equal(t, toolsetschema.ToolSetReleaseStatePrimary, second.State)
		require.Equal(t, uint64(1), second.Revision)
		require.Equal(t, first.EncryptedDefinition, second.EncryptedDefinition)
		require.Equal(t, uint64(2), primary.Generation)
	}
}
