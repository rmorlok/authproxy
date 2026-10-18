package database

import (
	"math"
	"strings"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/stretchr/testify/require"
)

// installedInventoryForTest creates an applied inventory and captures its next
// writable epoch while retaining the original preparation data for replacements.
func installedInventoryForTest(t *testing.T, count int) *toolSetInventoryFixture {
	t.Helper()

	f := explicitInventoryForTest(t, count)
	installed, err := f.db.InstallExplicitToolSetInventory(f.ctx, f.request)
	require.NoError(t, err)

	f.binding = installed
	f.request.ExpectedEpoch = installed.Epoch

	return f
}

// publishReplacementGenerationForTest makes the current generation active and
// returns a new primary without automatically changing any installed binding.
func publishReplacementGenerationForTest(t *testing.T, f *toolSetInventoryFixture) uint64 {
	t.Helper()

	generation, err := f.db.CreateToolSetGeneration(
		f.ctx,
		f.toolSet.Id,
		f.request.Tools[0].EncryptedDefinition,
	)
	require.NoError(t, err)

	_, err = f.db.PublishToolSetGeneration(f.ctx, f.toolSet.Id, generation.Generation, generation.Revision)
	require.NoError(t, err)

	return generation.Generation
}

// TestReplaceExplicitToolSetInventoryLifecycle migrates and rolls back a binding
// while retaining exact source identity, complete history, and caller ownership.
func TestReplaceExplicitToolSetInventoryLifecycle(t *testing.T) {
	// Capture the three original Tools before preparing a replacement generation.
	f := installedInventoryForTest(t, 3)

	original := copyInventoryForTest(f.request)
	first := readToolRowForTest(t, f.ctx, f.db, original.Tools[0].Tool.Id)

	// Publish twice so the prepared target is active rather than the latest primary.
	request := copyInventoryForTest(original)
	request.Generation = publishReplacementGenerationForTest(t, f)
	publishReplacementGenerationForTest(t, f) // The prepared target is now active, still eligible.

	// Keep the first source key, but change its name, metadata, and definition.
	request.Tools = request.Tools[:2]
	request.Tools[0].Tool.Id, request.Tools[0].Tool.Name = apid.New(apid.PrefixTool), "renamed"
	request.Tools[0].Tool.Description = "Updated operation"
	request.Tools[0].Tool.Verbs = []string{"write new", "read new"}
	request.Tools[0].Tool.Labels = Labels{"updated": "yes", "apxy/cxn/-/id": "forged"}
	request.Tools[0].Tool.Annotations = Annotations{"note": "updated"}
	request.Tools[0].EncryptedDefinition.Data = "bmV3"
	request.Tools[0].DefinitionDigest = strings.Repeat("a", 64)

	// Add a case-distinct source key and leave the other two original keys absent.
	request.Tools[1].Tool.Id, request.Tools[1].Tool.Name = apid.New(apid.PrefixTool), "new-source"
	request.Tools[1].Tool.SourceKey = "Operation-0" // Distinct from operation-0.

	// Commit the migration, advancing the binding without changing the request.
	before := copyInventoryForTest(request)
	changedAt := apctx.GetClock(f.ctx).Now().Add(time.Hour)
	migrated, err := f.db.ReplaceExplicitToolSetInventory(apctx.WithFixedClock(f.ctx, changedAt), request, 1)
	require.NoError(t, err)
	require.Equal(t, before, request)
	require.Equal(t, f.binding.Epoch+1, migrated.Epoch)
	require.Equal(t, uint64(2), migrated.InventoryRevision)
	require.Equal(t, request.Generation, *migrated.AppliedGeneration)
	require.True(t, changedAt.Equal(migrated.UpdatedAt))

	// The surviving key keeps its identity and creation time while its content changes.
	stored, err := f.db.GetTool(f.ctx, first.Id)
	require.NoError(t, err)
	require.Equal(t, common.ResourceName("renamed"), stored.Name)
	require.True(t, first.CreatedAt.Equal(stored.CreatedAt))
	require.True(t, changedAt.Equal(stored.UpdatedAt))
	require.Equal(t, uint64(2), stored.ActiveRevision)
	require.Equal(t, before.Tools[0].Tool.Description, stored.Description)
	require.Equal(t, []string{"read new", "write new"}, stored.Verbs)
	require.Equal(t, f.connection.Id.String(), stored.Labels["apxy/cxn/-/id"])
	require.Equal(t, "yes", stored.Labels["updated"])
	require.NotContains(t, stored.Labels, "local")
	require.Equal(t, before.Tools[0].Tool.Annotations, stored.Annotations)

	// Absent keys become tombstones without advancing their definition revisions.
	for _, entry := range original.Tools[1:] {
		removed := readToolRowForTest(t, f.ctx, f.db, entry.Tool.Id)
		require.NotNil(t, removed.DeletedAt)
		require.True(t, changedAt.Equal(*removed.DeletedAt))
		require.Equal(t, uint64(1), removed.ActiveRevision)
	}

	// The new source key receives its own identity at revision one.
	newTool, err := f.db.GetTool(f.ctx, request.Tools[1].Tool.Id)
	require.NoError(t, err)
	require.Equal(t, request.Tools[1].Tool.SourceKey, newTool.SourceKey)
	require.Equal(t, request.Tools[1].Tool.Name, newTool.Name)
	require.Equal(t, uint64(1), newTool.ActiveRevision)

	// Mutating the caller's maps and slices must not change the stored metadata.
	request.Tools[0].Tool.Labels["updated"] = "mutated"
	request.Tools[0].Tool.Annotations["note"] = "mutated"
	request.Tools[0].Tool.Verbs[0] = "mutated"
	unchanged, err := f.db.GetTool(f.ctx, first.Id)
	require.NoError(t, err)
	require.Equal(t, stored, unchanged)

	// Prepare a rollback with disposable IDs and omitted names for the original keys.
	rollback := copyInventoryForTest(original)
	rollback.ExpectedEpoch = migrated.Epoch

	for index := range rollback.Tools {
		rollback.Tools[index].Tool.Id = apid.New(apid.PrefixTool)
	}

	// Roll back to the earlier active generation using the migrated binding's epoch.
	beforeRollback := copyInventoryForTest(rollback)
	rolledBack, err := f.db.ReplaceExplicitToolSetInventory(apctx.WithFixedClock(f.ctx, changedAt.Add(time.Hour)), rollback, request.Generation)
	require.NoError(t, err, "the prior active generation remains a valid rollback target")
	require.Equal(t, beforeRollback, rollback)
	require.Equal(t, uint64(1), *rolledBack.AppliedGeneration)
	require.Equal(t, uint64(3), rolledBack.InventoryRevision)

	// Restore original IDs with new revisions; omitted names preserve the rename.
	for index, entry := range original.Tools {
		restored, err := f.db.GetTool(f.ctx, entry.Tool.Id)
		require.NoError(t, err)
		require.Nil(t, restored.DeletedAt)
		require.True(t, first.CreatedAt.Equal(restored.CreatedAt), "restoration preserves initial creation time")
		wantRevision := uint64(2)
		if index == 0 {
			wantRevision = 3
			require.Equal(t, common.ResourceName("renamed"), restored.Name)
		}
		require.Equal(t, wantRevision, restored.ActiveRevision)
		revision, err := f.db.GetToolRevision(f.ctx, restored.Id, wantRevision)
		require.NoError(t, err)
		require.Equal(t, uint64(1), *revision.SourceGeneration)
		require.Equal(t, entry.EncryptedDefinition, revision.EncryptedDefinition)
		require.Equal(t, entry.DefinitionDigest, revision.DefinitionDigest)
	}

	// Keep the migration's historical definition and retire its newly added Tool.
	prior, err := f.db.GetToolRevision(f.ctx, first.Id, 2)
	require.NoError(t, err)
	require.Equal(t, before.Tools[0].EncryptedDefinition, prior.EncryptedDefinition)
	require.Equal(t, request.Generation, *prior.SourceGeneration)
	require.NotNil(t, readToolRowForTest(t, f.ctx, f.db, newTool.Id).DeletedAt)

	// Inventory changes do not yet admit generated Tools to execution or discovery.
	_, err = f.db.GetCurrentToolRevision(f.ctx, first.Id)
	require.ErrorIs(t, err, ErrNotFound)
	page, err := f.db.ListToolCandidates(f.ctx, ToolCandidateQuery{NamespaceMatchers: []string{"root.**"}, Limit: 10})
	require.NoError(t, err)
	require.Empty(t, page.Items)
}

// TestReplaceExplicitToolSetInventoryNames permits complete-inventory name swaps
// and reuses retired names without abandoning the original source identities.
func TestReplaceExplicitToolSetInventoryNames(t *testing.T) {
	// Swap two live names in one replacement while keeping their source identities.
	f := installedInventoryForTest(t, 2)
	first := readToolRowForTest(t, f.ctx, f.db, f.request.Tools[0].Tool.Id)
	second := readToolRowForTest(t, f.ctx, f.db, f.request.Tools[1].Tool.Id)
	request := copyInventoryForTest(f.request)
	request.Tools[0].Tool.Name, request.Tools[1].Tool.Name = second.Name, first.Name
	swapped, err := f.db.ReplaceExplicitToolSetInventory(f.ctx, request, 1)
	require.NoError(t, err)
	require.Equal(t, second.Name, readToolRowForTest(t, f.ctx, f.db, first.Id).Name)
	require.Equal(t, first.Name, readToolRowForTest(t, f.ctx, f.db, second.Id).Name)

	// Replace both old keys with a new key that reuses one of their names.
	request.ExpectedEpoch = swapped.Epoch
	request.Tools = request.Tools[:1]
	request.Tools[0].Tool.Id, request.Tools[0].Tool.SourceKey = apid.New(apid.PrefixTool), "replacement"
	created, err := f.db.ReplaceExplicitToolSetInventory(f.ctx, request, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(3), created.InventoryRevision)
	newTool := readToolRowForTest(t, f.ctx, f.db, request.Tools[0].Tool.Id)
	require.Equal(t, second.Name, newTool.Name)

	// Retire both original identities while preserving their revision histories.
	for _, id := range []apid.ID{first.Id, second.Id} {
		require.NotNil(t, readToolRowForTest(t, f.ctx, f.db, id).DeletedAt)
		require.Equal(t, 2, countToolRevisionsForTest(t, f.ctx, f.db, id))
	}
}

// TestReplaceExplicitToolSetInventoryEmpty retains tombstones through repeated
// empty replacements, then restores the same identity with a fresh revision.
func TestReplaceExplicitToolSetInventoryEmpty(t *testing.T) {
	f := installedInventoryForTest(t, 1)
	request := copyInventoryForTest(f.request)

	// Both empty forms retire the Tool while retaining its generation and history.
	for _, entries := range [][]PreparedToolSetTool{nil, {}} {
		request.Tools = entries
		updated, err := f.db.ReplaceExplicitToolSetInventory(f.ctx, request, 1)
		require.NoError(t, err)
		request.ExpectedEpoch = updated.Epoch
		require.Equal(t, uint64(1), *updated.AppliedGeneration)
		require.NotNil(t, readToolRowForTest(t, f.ctx, f.db, f.request.Tools[0].Tool.Id).DeletedAt)
		require.Equal(t, 1, countToolRevisionsForTest(t, f.ctx, f.db, f.request.Tools[0].Tool.Id))
	}

	// Reintroduce the same definition with a disposable ID to restore the tombstone.
	request.Tools = copyInventoryForTest(f.request).Tools
	request.Tools[0].Tool.Id = apid.New(apid.PrefixTool)
	restored, err := f.db.ReplaceExplicitToolSetInventory(f.ctx, request, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(4), restored.InventoryRevision)

	// Restoration advances the original Tool's revision instead of creating a new ID.
	tool, err := f.db.GetTool(f.ctx, f.request.Tools[0].Tool.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(2), tool.ActiveRevision, "equal definition digests still append a revision")
	_, err = f.db.GetTool(f.ctx, request.Tools[0].Tool.Id)
	require.ErrorIs(t, err, ErrNotFound, "restoration must ignore the provisional identity")
}

// TestReplaceExplicitToolSetInventoryRollback keeps removals and restorations
// atomic when a later new member collides with an unrelated retained identity.
func TestReplaceExplicitToolSetInventoryRollback(t *testing.T) {
	for _, conflict := range []string{"id", "name", "other binding"} {
		t.Run(conflict, func(t *testing.T) {
			// Leave one original key live and two retired before attempting replacement.
			f := installedInventoryForTest(t, 3)
			request := copyInventoryForTest(f.request)
			request.Tools = request.Tools[:1]
			updated, err := f.db.ReplaceExplicitToolSetInventory(f.ctx, request, 1)
			require.NoError(t, err)

			// Create a conflicting record outside this binding's inventory.
			occupied := copyToolMetadataForTest(&f.request.Tools[0].Tool)
			occupied.Id, occupied.Name, occupied.BindingId, occupied.SourceKey = apid.New(apid.PrefixTool), "occupied", apid.Nil, ""
			definition, err := f.db.CreateStandaloneTool(f.ctx, &occupied, request.Tools[0].EncryptedDefinition, request.Tools[0].DefinitionDigest)
			require.NoError(t, err)
			if conflict == "other binding" {
				other := toolOwnershipBindingForTest(t, f.ctx, f.db, &occupied, "other-tools")
				occupied = *generatedToolForTest(t, f.ctx, f.db, &occupied, definition, other, "foreign")
			}

			// Restore one retired key, then make the later new entry collide.
			request = copyInventoryForTest(f.request)
			request.ExpectedEpoch = updated.Epoch
			request.Tools = request.Tools[1:]
			request.Tools[0].Tool.Id = apid.New(apid.PrefixTool) // Restore operation-1.
			request.Tools[1].Tool.Id, request.Tools[1].Tool.SourceKey = apid.New(apid.PrefixTool), "new"
			if conflict == "name" {
				request.Tools[1].Tool.Name = occupied.Name
			} else {
				request.Tools[1].Tool.Id = occupied.Id
			}

			// Snapshot the caller, binding, and all affected identities before the write.
			before, binding := copyInventoryForTest(request), readInventoryBindingForTest(t, f)
			var originals []*Tool
			for _, entry := range f.request.Tools {
				originals = append(originals, readToolRowForTest(t, f.ctx, f.db, entry.Tool.Id))
			}
			foreign := readToolRowForTest(t, f.ctx, f.db, occupied.Id)

			// The late failure must undo retirement, restoration, and appended revisions.
			_, err = f.db.ReplaceExplicitToolSetInventory(f.ctx, request, 1)
			require.Error(t, err)
			require.Equal(t, before, request)
			require.Equal(t, binding, readInventoryBindingForTest(t, f))
			require.Equal(t, foreign, readToolRowForTest(t, f.ctx, f.db, occupied.Id))
			for _, tool := range originals {
				require.Equal(t, tool, readToolRowForTest(t, f.ctx, f.db, tool.Id))
				require.Equal(t, int(tool.ActiveRevision), countToolRevisionsForTest(t, f.ctx, f.db, tool.Id))
			}
		})
	}
}

// TestReplaceExplicitToolSetInventoryFences validates the captured application
// and current policy even for empty replacements, leaving the old inventory intact.
func TestReplaceExplicitToolSetInventoryFences(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*testing.T, *toolSetInventoryFixture, *uint64)
		want error
	}{
		{"expected generation", func(_ *testing.T, _ *toolSetInventoryFixture, expected *uint64) { *expected = 2 }, ErrToolSetBindingGenerationConflict},
		{"epoch", func(_ *testing.T, f *toolSetInventoryFixture, _ *uint64) { f.request.ExpectedEpoch-- }, ErrToolSetBindingEpochConflict},
		{"selector", func(t *testing.T, f *toolSetInventoryFixture, _ *uint64) {
			_, err := f.db.UpdateToolSetConnectionSelector(f.ctx, f.toolSet.Id, 1, f.toolSet.ConnectionSelector)
			require.NoError(t, err)
		}, ErrToolSetSelectorRevisionConflict},
		{"membership", func(t *testing.T, f *toolSetInventoryFixture, _ *uint64) {
			_, err := f.db.UpdateConnectionLabels(f.ctx, f.connection.Id, map[string]string{"env": "dev"})
			require.NoError(t, err)
		}, ErrToolSetBindingMembershipConflict},
		{"connection not ready", func(t *testing.T, f *toolSetInventoryFixture, _ *uint64) {
			require.NoError(t, f.db.SetConnectionState(f.ctx, f.connection.Id, ConnectionStateDisconnected))
		}, ErrToolSetInventoryConnectionNotReady},
		{"connection deleted", func(t *testing.T, f *toolSetInventoryFixture, _ *uint64) {
			require.NoError(t, f.db.DeleteConnection(f.ctx, f.connection.Id))
		}, ErrNotFound},
		{"namespace deleted", func(t *testing.T, f *toolSetInventoryFixture, _ *uint64) {
			require.NoError(t, f.db.DeleteNamespace(f.ctx, f.connection.Namespace))
		}, ErrNotFound},
		{"draft target", func(t *testing.T, f *toolSetInventoryFixture, _ *uint64) {
			g, err := f.db.CreateToolSetGeneration(f.ctx, f.toolSet.Id, f.request.Tools[0].EncryptedDefinition)
			require.NoError(t, err)
			f.request.Generation = g.Generation
		}, ErrToolSetInventoryGenerationConflict},
		{"archived target", func(t *testing.T, f *toolSetInventoryFixture, _ *uint64) {
			s := f.db.(*service)
			_, err := s.sq.Update(ToolSetGenerationsTable).Set("state", toolsetschema.ToolSetReleaseStateArchived).Where(sq.Eq{"tool_set_id": f.toolSet.Id, "generation": 1}).RunWith(s.db).ExecContext(f.ctx)
			require.NoError(t, err)
		}, ErrToolSetInventoryGenerationConflict},
		{"inventory exhausted", func(t *testing.T, f *toolSetInventoryFixture, _ *uint64) {
			s := f.db.(*service)
			_, err := s.sq.Update(ToolSetBindingsTable).Set("inventory_revision", int64(math.MaxInt64)).Where(sq.Eq{"id": f.binding.Id}).RunWith(s.db).ExecContext(f.ctx)
			require.NoError(t, err)
		}, ErrToolSetBindingInventoryConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Invalidate one replacement precondition after installing the original inventory.
			f := installedInventoryForTest(t, 1)
			expected := uint64(1)
			test.edit(t, f, &expected)
			binding, tool := readInventoryBindingForTest(t, f), readToolRowForTest(t, f.ctx, f.db, f.request.Tools[0].Tool.Id)

			// Populated and empty requests must both fail without altering stored state.
			for _, empty := range []bool{false, true} {
				request := copyInventoryForTest(f.request)
				if empty {
					request.Tools = nil
				}
				_, err := f.db.ReplaceExplicitToolSetInventory(f.ctx, request, expected)
				require.ErrorIs(t, err, test.want)
				require.Equal(t, binding, readInventoryBindingForTest(t, f))
				require.Equal(t, tool, readToolRowForTest(t, f.ctx, f.db, tool.Id))
				require.Equal(t, 1, countToolRevisionsForTest(t, f.ctx, f.db, tool.Id))
			}
		})
	}

	// A pending binding must use first installation rather than replacement.
	pending := explicitInventoryForTest(t, 1)
	_, err := pending.db.ReplaceExplicitToolSetInventory(pending.ctx, pending.request, 1)
	require.ErrorIs(t, err, ErrToolSetBindingGenerationConflict)
}

// TestReplaceExplicitToolSetInventoryValidation rejects malformed requests and
// exhausted captured counters before altering existing definitions or metadata.
func TestReplaceExplicitToolSetInventoryValidation(t *testing.T) {
	f := installedInventoryForTest(t, 2)

	// Try malformed requests independently, preserving each input and binding snapshot.
	for _, change := range []func(*ToolSetInventoryInstall){
		func(r *ToolSetInventoryInstall) { r.ExpectedEpoch = math.MaxInt64 },
		func(r *ToolSetInventoryInstall) { r.Generation = 0 },
		func(r *ToolSetInventoryInstall) { r.Tools[1].Tool.SourceKey = r.Tools[0].Tool.SourceKey },
		func(r *ToolSetInventoryInstall) { r.Tools[1].Tool.Labels = Labels{"invalid/key/name": "value"} },
		func(r *ToolSetInventoryInstall) { r.Tools[1].Tool.ConnectionId = apid.New(apid.PrefixConnection) },
	} {
		request := copyInventoryForTest(f.request)
		change(&request)
		before := copyInventoryForTest(request)
		binding := readInventoryBindingForTest(t, f)
		_, err := f.db.ReplaceExplicitToolSetInventory(f.ctx, request, 1)
		require.Error(t, err)
		require.Equal(t, before, request)
		require.Equal(t, binding, readInventoryBindingForTest(t, f))
	}

	// Reject missing or unrepresentable expected generations as well.
	for _, expected := range []uint64{0, math.MaxInt64 + 1} {
		_, err := f.db.ReplaceExplicitToolSetInventory(f.ctx, f.request, expected)
		require.Error(t, err)
	}

	// None of the rejected requests may append a definition revision.
	for _, entry := range f.request.Tools {
		require.Equal(t, 1, countToolRevisionsForTest(t, f.ctx, f.db, entry.Tool.Id))
	}
}

// TestReplaceExplicitToolSetInventoryConcurrent allows one whole replacement to
// win a captured epoch without mixing the losers' definitions or new identities.
func TestReplaceExplicitToolSetInventoryConcurrent(t *testing.T) {
	f := installedInventoryForTest(t, 1)
	f.db.(*service).db.SetMaxOpenConns(2)
	start := make(chan struct{})
	type outcome struct {
		request ToolSetInventoryInstall
		err     error
	}
	results := make(chan outcome, 2)

	// Prepare competing batches from the same epoch with distinguishable content and IDs.
	for _, description := range []string{"first", "second"} {
		request := copyInventoryForTest(f.request)
		request.Tools[0].Tool.Description = description
		newEntry := request.Tools[0]
		newEntry.Tool = copyToolMetadataForTest(&newEntry.Tool)
		newEntry.Tool.Id, newEntry.Tool.SourceKey = apid.New(apid.PrefixTool), "new"
		request.Tools = append(request.Tools, newEntry)
		go func() {
			<-start
			_, err := f.db.ReplaceExplicitToolSetInventory(f.ctx, request, 1)
			results <- outcome{request, err}
		}()
	}

	// Release both writers and require one winner with no revisions from the loser.
	close(start)
	var winner ToolSetInventoryInstall
	for range 2 {
		result := <-results
		if result.err == nil {
			require.Empty(t, winner.Tools)
			winner = result.request
		} else {
			require.ErrorIs(t, result.err, ErrToolSetBindingEpochConflict)
			require.Zero(t, countToolRevisionsForTest(t, f.ctx, f.db, result.request.Tools[1].Tool.Id))
		}
	}

	// Both the retained and newly added Tool must come from the winning batch.
	require.NotEmpty(t, winner.Tools)
	retained, err := f.db.GetTool(f.ctx, f.request.Tools[0].Tool.Id)
	require.NoError(t, err)
	require.Equal(t, winner.Tools[0].Tool.Description, retained.Description)
	require.Equal(t, uint64(2), retained.ActiveRevision)
	added, err := f.db.GetTool(f.ctx, winner.Tools[1].Tool.Id)
	require.NoError(t, err)
	require.Equal(t, retained.Description, added.Description)
	require.Equal(t, uint64(2), readInventoryBindingForTest(t, f).InventoryRevision)
}

// TestReplaceExplicitToolSetInventoryExhaustedRevision never wraps a retained
// Tool revision or partially advances the surrounding binding inventory.
func TestReplaceExplicitToolSetInventoryExhaustedRevision(t *testing.T) {
	// Seed the largest valid revision directly to exercise overflow without a long loop.
	f := installedInventoryForTest(t, 1)
	id := f.request.Tools[0].Tool.Id
	revision, err := f.db.GetToolRevision(f.ctx, id, 1)
	require.NoError(t, err)
	revision.Revision = math.MaxInt64
	s := f.db.(*service)
	_, err = s.sq.Insert(ToolRevisionsTable).Columns(revision.cols()...).Values(revision.values()...).RunWith(s.db).ExecContext(f.ctx)
	require.NoError(t, err)
	_, err = s.sq.Update(ToolsTable).Set("active_revision", int64(math.MaxInt64)).Where(sq.Eq{"id": id}).RunWith(s.db).ExecContext(f.ctx)
	require.NoError(t, err)

	// Reject the next revision without changing the Tool, binding, or retained history.
	before, binding := readToolRowForTest(t, f.ctx, f.db, id), readInventoryBindingForTest(t, f)
	_, err = f.db.ReplaceExplicitToolSetInventory(f.ctx, f.request, 1)
	require.ErrorIs(t, err, ErrToolRevisionConflict)
	require.Equal(t, before, readToolRowForTest(t, f.ctx, f.db, id))
	require.Equal(t, binding, readInventoryBindingForTest(t, f))
	require.Equal(t, 2, countToolRevisionsForTest(t, f.ctx, f.db, id))
}

// TestReplaceExplicitToolSetInventoryProvisionalNames checks uniqueness after
// adopting retained identity, not against names defaulted from disposable IDs.
func TestReplaceExplicitToolSetInventoryProvisionalNames(t *testing.T) {
	f := installedInventoryForTest(t, 1)
	original := readToolRowForTest(t, f.ctx, f.db, f.request.Tools[0].Tool.Id)

	// Give the retained key a disposable ID and omit its name to preserve the stored one.
	request := copyInventoryForTest(f.request)
	request.Tools[0].Tool.Id = apid.New(apid.PrefixTool)
	request.Tools[0].Tool.Name = ""

	// Give a new key the name that would have been defaulted from that disposable ID.
	added := request.Tools[0]
	added.Tool = copyToolMetadataForTest(&added.Tool)
	added.Tool.Id = apid.New(apid.PrefixTool)
	added.Tool.SourceKey = "new"
	added.Tool.Name = common.ResourceName(request.Tools[0].Tool.Id.String())
	request.Tools = append(request.Tools, added)

	// Resolve the retained identity first, making both final names valid and distinct.
	_, err := f.db.ReplaceExplicitToolSetInventory(f.ctx, request, 1)
	require.NoError(t, err)
	retained, err := f.db.GetTool(f.ctx, original.Id)
	require.NoError(t, err)
	require.Equal(t, original.Name, retained.Name)
	created, err := f.db.GetTool(f.ctx, added.Tool.Id)
	require.NoError(t, err)
	require.Equal(t, added.Tool.Name, created.Name)
}
