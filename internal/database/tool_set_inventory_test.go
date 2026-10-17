package database

import (
	"context"
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

// toolSetInventoryFixture holds a published generation and a matching pending
// assignment, with caller-owned preparation data that has not been persisted.
type toolSetInventoryFixture struct {
	ctx        context.Context
	db         DB
	toolSet    *ToolSet
	connection *Connection
	binding    *ToolSetBinding
	request    ToolSetInventoryInstall
}

// explicitInventoryForTest prepares an inventory against real parent rows while
// keeping definition compilation and encryption outside these storage tests.
func explicitInventoryForTest(t *testing.T, count int) *toolSetInventoryFixture {
	t.Helper()

	ctx, db, toolSet, connection := toolSetBindingParentsForTest(t)
	require.NoError(t, db.SetConnectionState(ctx, connection.Id, ConnectionStateConfigured))

	definition := encfield.EncryptedField{
		ID:   apid.New(apid.PrefixDataEncryptionKey),
		Data: "ZGVmaW5pdGlvbg==",
	}
	generation, err := db.CreateToolSetGeneration(ctx, toolSet.Id, definition)
	require.NoError(t, err)

	_, err = db.PublishToolSetGeneration(ctx, toolSet.Id, generation.Generation, generation.Revision)
	require.NoError(t, err)

	binding, err := db.CreateToolSetBinding(ctx, toolSet.Id, connection.Id, toolSet.SelectorRevision)
	require.NoError(t, err)
	require.Nil(t, binding.AppliedGeneration)
	require.Zero(t, binding.InventoryRevision)

	request := ToolSetInventoryInstall{
		BindingId:                binding.Id,
		ExpectedEpoch:            binding.Epoch,
		ExpectedSelectorRevision: toolSet.SelectorRevision,
		Generation:               generation.Generation,
	}

	for index := range count {
		request.Tools = append(request.Tools, PreparedToolSetTool{
			Tool: Tool{
				Id:           apid.New(apid.PrefixTool),
				Namespace:    connection.Namespace,
				ConnectionId: connection.Id,
				BindingId:    binding.Id,
				SourceKey:    fmt.Sprintf("operation-%d", index),
				Description:  "An explicitly prepared operation",
				Verbs:        []string{"write", "read"},
				Labels: Labels{
					"local":         "value",
					"apxy/cxn/-/id": "forged",
				},
				Annotations: Annotations{"note": "own"},
			},
			EncryptedDefinition: definition,
			DefinitionDigest:    toolDefinitionDigestForTest,
		})
	}
	return &toolSetInventoryFixture{
		ctx,
		db,
		toolSet,
		connection,
		binding,
		request,
	}
}

// copyInventoryForTest detaches nested slices and maps so unchanged-input
// assertions can detect mutations instead of sharing the production input.
func copyInventoryForTest(request ToolSetInventoryInstall) ToolSetInventoryInstall {
	request.Tools = slices.Clone(request.Tools)
	for index := range request.Tools {
		request.Tools[index].Tool = copyToolMetadataForTest(&request.Tools[index].Tool)
	}
	return request
}

// readInventoryBindingForTest inspects physical state even after a parent or the
// binding is retired, so failed operations cannot hide partial state changes.
func readInventoryBindingForTest(t *testing.T, fixture *toolSetInventoryFixture) *ToolSetBinding {
	t.Helper()
	s := fixture.db.(*service)
	var binding ToolSetBinding
	err := s.sq.Select(binding.cols()...).From(ToolSetBindingsTable).
		Where(sq.Eq{"id": fixture.binding.Id}).RunWith(s.db).QueryRowContext(fixture.ctx).Scan(binding.fields()...)
	require.NoError(t, err)
	return &binding
}

// TestExplicitToolSetInventoryLifecycle installs all rows and provenance in one
// binding transition without modifying preparation data or activating execution.
func TestExplicitToolSetInventoryLifecycle(t *testing.T) {
	f := explicitInventoryForTest(t, 2)

	old := apctx.GetClock(f.ctx).Now().Add(-time.Hour)
	f.request.Tools[0].Tool.ActiveRevision = 99
	f.request.Tools[0].Tool.CreatedAt, f.request.Tools[0].Tool.UpdatedAt = old, old
	f.request.Tools[0].Tool.DeletedAt = &old
	f.request.Tools[0].Tool.SourceKey = " GET /v1/{id}?a='b'%_\t"
	before := copyInventoryForTest(f.request)
	installedAt := apctx.GetClock(f.ctx).Now().Add(time.Hour)

	installed, err := f.db.InstallExplicitToolSetInventory(apctx.WithFixedClock(f.ctx, installedAt), f.request)
	require.NoError(t, err)
	require.Equal(t, before, f.request)
	require.NoError(t, installed.Validate())
	require.Equal(t, f.binding.Epoch+1, installed.Epoch)
	require.Equal(t, uint64(1), installed.InventoryRevision)
	require.NotNil(t, installed.AppliedGeneration)
	require.Equal(t, f.request.Generation, *installed.AppliedGeneration)
	require.True(t, installedAt.Equal(installed.UpdatedAt))
	require.True(t, f.binding.CreatedAt.Equal(installed.CreatedAt))

	for _, prepared := range f.request.Tools {
		stored, err := f.db.GetTool(f.ctx, prepared.Tool.Id)
		require.NoError(t, err)
		require.NoError(t, stored.Validate())
		require.Equal(t, common.ResourceName(stored.Id.String()), stored.Name)
		require.Equal(t, prepared.Tool.SourceKey, stored.SourceKey)
		require.Equal(t, f.binding.Id, stored.BindingId)
		require.Equal(t, uint64(1), stored.ActiveRevision)
		require.Nil(t, stored.DeletedAt)
		require.True(t, installedAt.Equal(stored.CreatedAt))
		require.True(t, installedAt.Equal(stored.UpdatedAt))
		require.Equal(t, []string{"read", "write"}, stored.Verbs)
		require.Equal(t, "platform", stored.Labels["apxy/ns/team"])
		require.Equal(t, f.connection.Id.String(), stored.Labels["apxy/cxn/-/id"])
		require.Equal(t, prepared.Tool.Annotations, stored.Annotations)
		revision, err := f.db.GetToolRevision(f.ctx, stored.Id, 1)
		require.NoError(t, err)
		require.NotNil(t, revision.SourceGeneration)
		require.Equal(t, f.request.Generation, *revision.SourceGeneration)
		require.Equal(t, prepared.EncryptedDefinition, revision.EncryptedDefinition)
		require.Equal(t, prepared.DefinitionDigest, revision.DefinitionDigest)
		_, err = f.db.GetCurrentToolRevision(f.ctx, stored.Id)
		require.ErrorIs(t, err, ErrNotFound)
	}

	page, err := f.db.ListToolCandidates(f.ctx, ToolCandidateQuery{NamespaceMatchers: []string{"root.**"}, Limit: 10})
	require.NoError(t, err)
	require.Empty(t, page.Items)

	f.request.Tools[0].Tool.Labels["local"] = "changed"
	f.request.Tools[0].Tool.Annotations["note"] = "changed"
	f.request.Tools[0].Tool.Verbs[0] = "changed"
	stored, err := f.db.GetTool(f.ctx, before.Tools[0].Tool.Id)
	require.NoError(t, err)
	require.Equal(t, "value", stored.Labels["local"])
	require.Equal(t, "own", stored.Annotations["note"])
	require.Equal(t, []string{"read", "write"}, stored.Verbs)

	second, err := f.db.CreateToolSetGeneration(f.ctx, f.toolSet.Id, before.Tools[0].EncryptedDefinition)
	require.NoError(t, err)

	_, err = f.db.PublishToolSetGeneration(f.ctx, f.toolSet.Id, second.Generation, second.Revision)
	require.NoError(t, err)

	unchanged := readInventoryBindingForTest(t, f)
	require.Equal(t, installed.Epoch, unchanged.Epoch)
	require.Equal(t, installed.AppliedGeneration, unchanged.AppliedGeneration, "publication does not roll existing inventories forward")
}

// TestExplicitToolSetInventoryEmpty distinguishes a completed empty inventory
// from a pending binding, with the same fencing as a populated installation.
func TestExplicitToolSetInventoryEmpty(t *testing.T) {
	for _, tools := range [][]PreparedToolSetTool{nil, {}} {
		f := explicitInventoryForTest(t, 0)
		f.request.Tools = tools
		before := copyInventoryForTest(f.request)
		installed, err := f.db.InstallExplicitToolSetInventory(f.ctx, f.request)
		require.NoError(t, err)
		require.Equal(t, before, f.request)
		require.Equal(t, uint64(1), installed.InventoryRevision)
		require.Equal(t, uint64(1), *installed.AppliedGeneration)

		_, err = f.db.InstallExplicitToolSetInventory(f.ctx, f.request)
		require.ErrorIs(t, err, ErrToolSetBindingEpochConflict)

		f.request.ExpectedEpoch = installed.Epoch
		_, err = f.db.InstallExplicitToolSetInventory(f.ctx, f.request)
		require.ErrorIs(t, err, ErrToolSetBindingInventoryConflict)
	}
}

// TestExplicitToolSetInventoryRollback rejects invalid or conflicting members
// without installing earlier rows, moving binding state, or mutating inputs.
func TestExplicitToolSetInventoryRollback(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*ToolSetInventoryInstall)
	}{
		{"duplicate id", func(r *ToolSetInventoryInstall) { r.Tools[1].Tool.Id = r.Tools[0].Tool.Id }},
		{"duplicate name", func(r *ToolSetInventoryInstall) { r.Tools[0].Tool.Name, r.Tools[1].Tool.Name = "same", "same" }},
		{"duplicate source", func(r *ToolSetInventoryInstall) { r.Tools[1].Tool.SourceKey = r.Tools[0].Tool.SourceKey }},
		{"wrong namespace", func(r *ToolSetInventoryInstall) { r.Tools[1].Tool.Namespace = "root" }},
		{"wrong connection", func(r *ToolSetInventoryInstall) { r.Tools[1].Tool.ConnectionId = apid.New(apid.PrefixConnection) }},
		{"wrong binding", func(r *ToolSetInventoryInstall) { r.Tools[1].Tool.BindingId = apid.New(apid.PrefixToolSetBinding) }},
		{"blank source", func(r *ToolSetInventoryInstall) { r.Tools[1].Tool.SourceKey = " \t" }},
		{"labels", func(r *ToolSetInventoryInstall) { r.Tools[1].Tool.Labels = Labels{"invalid/key/name": "value"} }},
		{"annotations", func(r *ToolSetInventoryInstall) {
			r.Tools[1].Tool.Annotations = Annotations{"invalid/key/name": "value"}
		}},
		{"ciphertext", func(r *ToolSetInventoryInstall) { r.Tools[1].EncryptedDefinition = encfield.EncryptedField{} }},
		{"digest", func(r *ToolSetInventoryInstall) { r.Tools[1].DefinitionDigest = "invalid" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := explicitInventoryForTest(t, 2)
			test.edit(&f.request)
			before, binding := copyInventoryForTest(f.request), readInventoryBindingForTest(t, f)
			_, err := f.db.InstallExplicitToolSetInventory(f.ctx, f.request)
			require.Error(t, err)
			require.Equal(t, before, f.request)
			require.Equal(t, binding, readInventoryBindingForTest(t, f))
			for _, prepared := range f.request.Tools {
				_, err := f.db.GetTool(f.ctx, prepared.Tool.Id)
				require.ErrorIs(t, err, ErrNotFound)
				require.Zero(t, countToolRevisionsForTest(t, f.ctx, f.db, prepared.Tool.Id))
			}
		})
	}
	// These conflicts exist before the request and therefore must be handled
	// by database uniqueness, after any earlier inventory inserts are rolled back.
	for _, field := range []string{"id", "name"} {
		t.Run("existing "+field, func(t *testing.T) {
			f := explicitInventoryForTest(t, 2)
			occupied := copyToolMetadataForTest(&f.request.Tools[1].Tool)
			occupied.Id, occupied.Name = apid.New(apid.PrefixTool), "occupied"
			occupied.BindingId, occupied.SourceKey = apid.Nil, ""
			_, err := f.db.CreateStandaloneTool(f.ctx, &occupied, f.request.Tools[1].EncryptedDefinition, f.request.Tools[1].DefinitionDigest)
			require.NoError(t, err)
			if field == "id" {
				f.request.Tools[1].Tool.Id = occupied.Id
			} else {
				f.request.Tools[1].Tool.Name = occupied.Name
			}
			before, binding := copyInventoryForTest(f.request), readInventoryBindingForTest(t, f)
			original := readToolRowForTest(t, f.ctx, f.db, occupied.Id)
			_, err = f.db.InstallExplicitToolSetInventory(f.ctx, f.request)
			require.ErrorIs(t, err, ErrDuplicate)
			require.Equal(t, before, f.request)
			require.Equal(t, binding, readInventoryBindingForTest(t, f))
			require.Equal(t, original, readToolRowForTest(t, f.ctx, f.db, occupied.Id))
			require.Equal(t, 1, countToolRevisionsForTest(t, f.ctx, f.db, occupied.Id))
			_, err = f.db.GetTool(f.ctx, f.request.Tools[0].Tool.Id)
			require.ErrorIs(t, err, ErrNotFound)
			require.Zero(t, countToolRevisionsForTest(t, f.ctx, f.db, f.request.Tools[0].Tool.Id))
		})
	}
}

// TestExplicitToolSetInventoryFences rechecks publication, membership, readiness,
// and parent liveness instead of trusting the caller's earlier preparation.
func TestExplicitToolSetInventoryFences(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*testing.T, *toolSetInventoryFixture)
		want error
	}{
		{"epoch", func(_ *testing.T, f *toolSetInventoryFixture) { f.request.ExpectedEpoch++ }, ErrToolSetBindingEpochConflict},
		{"selector", func(t *testing.T, f *toolSetInventoryFixture) {
			_, err := f.db.UpdateToolSetConnectionSelector(f.ctx, f.toolSet.Id, 1, f.toolSet.ConnectionSelector)
			require.NoError(t, err)
		}, ErrToolSetSelectorRevisionConflict},
		{"draft", func(t *testing.T, f *toolSetInventoryFixture) {
			g, err := f.db.CreateToolSetGeneration(f.ctx, f.toolSet.Id, f.request.Tools[0].EncryptedDefinition)
			require.NoError(t, err)
			f.request.Generation = g.Generation
		}, ErrToolSetInventoryGenerationConflict},
		{"superseded primary", func(t *testing.T, f *toolSetInventoryFixture) {
			g, err := f.db.CreateToolSetGeneration(f.ctx, f.toolSet.Id, f.request.Tools[0].EncryptedDefinition)
			require.NoError(t, err)
			_, err = f.db.PublishToolSetGeneration(f.ctx, f.toolSet.Id, g.Generation, g.Revision)
			require.NoError(t, err)
		}, ErrToolSetInventoryGenerationConflict},
		{"membership", func(t *testing.T, f *toolSetInventoryFixture) {
			_, err := f.db.UpdateConnectionLabels(f.ctx, f.connection.Id, map[string]string{"env": "dev"})
			require.NoError(t, err)
		}, ErrToolSetBindingMembershipConflict},
		{"setup", func(t *testing.T, f *toolSetInventoryFixture) {
			require.NoError(t, f.db.SetConnectionState(f.ctx, f.connection.Id, ConnectionStateSetup))
		}, ErrToolSetInventoryConnectionNotReady},
		{"disabled", func(t *testing.T, f *toolSetInventoryFixture) {
			require.NoError(t, f.db.SetConnectionState(f.ctx, f.connection.Id, ConnectionStateDisabled))
		}, ErrToolSetInventoryConnectionNotReady},
		{"disconnecting", func(t *testing.T, f *toolSetInventoryFixture) {
			require.NoError(t, f.db.SetConnectionState(f.ctx, f.connection.Id, ConnectionStateDisconnecting))
		}, ErrToolSetInventoryConnectionNotReady},
		{"disconnected", func(t *testing.T, f *toolSetInventoryFixture) {
			require.NoError(t, f.db.SetConnectionState(f.ctx, f.connection.Id, ConnectionStateDisconnected))
		}, ErrToolSetInventoryConnectionNotReady},
		{"connection deleted", func(t *testing.T, f *toolSetInventoryFixture) {
			require.NoError(t, f.db.DeleteConnection(f.ctx, f.connection.Id))
		}, ErrNotFound},
		{"toolset deleted", func(t *testing.T, f *toolSetInventoryFixture) {
			require.NoError(t, f.db.DeleteToolSet(f.ctx, f.toolSet.Id))
		}, ErrNotFound},
		{"connection namespace deleted", func(t *testing.T, f *toolSetInventoryFixture) {
			require.NoError(t, f.db.DeleteNamespace(f.ctx, f.connection.Namespace))
		}, ErrNotFound},
		{"toolset namespace deleted", func(t *testing.T, f *toolSetInventoryFixture) {
			require.NoError(t, f.db.DeleteNamespace(f.ctx, f.toolSet.Namespace))
		}, ErrNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := explicitInventoryForTest(t, 1)
			test.edit(t, f)
			before := readInventoryBindingForTest(t, f)
			_, err := f.db.InstallExplicitToolSetInventory(f.ctx, f.request)
			require.ErrorIs(t, err, test.want)
			require.Equal(t, before, readInventoryBindingForTest(t, f))
			require.Zero(t, countToolRevisionsForTest(t, f.ctx, f.db, f.request.Tools[0].Tool.Id))
			f.request.Tools = nil
			_, err = f.db.InstallExplicitToolSetInventory(f.ctx, f.request)
			require.ErrorIs(t, err, test.want, "empty inventories require the same parent checks")
			require.Equal(t, before, readInventoryBindingForTest(t, f))
		})
	}
}

// TestExplicitToolSetInventoryExistingRows treats both live and retired owned
// identities as inventory conflicts, rather than replacing their history.
func TestExplicitToolSetInventoryExistingRows(t *testing.T) {
	for _, retired := range []bool{false, true} {
		f := explicitInventoryForTest(t, 1)
		base := copyToolMetadataForTest(&f.request.Tools[0].Tool)
		base.Id, base.BindingId, base.SourceKey = apid.New(apid.PrefixTool), apid.Nil, ""
		_, err := f.db.CreateStandaloneTool(f.ctx, &base, f.request.Tools[0].EncryptedDefinition, f.request.Tools[0].DefinitionDigest)
		require.NoError(t, err)
		s := f.db.(*service)
		update := s.sq.Update(ToolsTable).Set("binding_id", f.binding.Id).Set("source_key", "existing").Where(sq.Eq{"id": base.Id})
		if retired {
			update = update.Set("deleted_at", apctx.GetClock(f.ctx).Now())
		}
		_, err = update.RunWith(s.db).ExecContext(f.ctx)
		require.NoError(t, err)
		before := readInventoryBindingForTest(t, f)
		_, err = f.db.InstallExplicitToolSetInventory(f.ctx, f.request)
		require.ErrorIs(t, err, ErrToolSetBindingInventoryConflict)
		require.Equal(t, before, readInventoryBindingForTest(t, f))
		require.Equal(t, 1, countToolRevisionsForTest(t, f.ctx, f.db, base.Id))
		require.Zero(t, countToolRevisionsForTest(t, f.ctx, f.db, f.request.Tools[0].Tool.Id))
	}
}

// TestExplicitToolSetInventoryRemoval retires one complete inventory and retains
// source history without affecting standalone Tools or another binding.
func TestExplicitToolSetInventoryRemoval(t *testing.T) {
	f := explicitInventoryForTest(t, 2)
	installed, err := f.db.InstallExplicitToolSetInventory(f.ctx, f.request)
	require.NoError(t, err)
	base := copyToolMetadataForTest(&f.request.Tools[0].Tool)
	base.Id, base.BindingId, base.SourceKey = apid.New(apid.PrefixTool), apid.Nil, ""
	definition, err := f.db.CreateStandaloneTool(f.ctx, &base, f.request.Tools[0].EncryptedDefinition, f.request.Tools[0].DefinitionDigest)
	require.NoError(t, err)
	other := toolOwnershipBindingForTest(t, f.ctx, f.db, &base, "other-tools")
	sibling := generatedToolForTest(t, f.ctx, f.db, &base, definition, other, "same-source")
	require.ErrorIs(t, f.db.DeleteToolSetBinding(f.ctx, f.binding.Id, installed.Epoch, 1), ErrToolSetBindingMembershipConflict)
	_, err = f.db.UpdateConnectionLabels(f.ctx, f.connection.Id, map[string]string{"env": "dev"})
	require.NoError(t, err)
	require.ErrorIs(t, f.db.DeleteToolSetBinding(f.ctx, f.binding.Id, 1, 1), ErrToolSetBindingEpochConflict)
	removedAt := apctx.GetClock(f.ctx).Now().Add(time.Hour)
	require.NoError(t, f.db.DeleteToolSetBinding(apctx.WithFixedClock(f.ctx, removedAt), f.binding.Id, installed.Epoch, 1))
	tombstone := readInventoryBindingForTest(t, f)
	require.Equal(t, installed.Epoch+1, tombstone.Epoch)
	require.Equal(t, installed.AppliedGeneration, tombstone.AppliedGeneration)
	require.Equal(t, installed.InventoryRevision, tombstone.InventoryRevision)
	require.NotNil(t, tombstone.DeletedAt)
	require.True(t, removedAt.Equal(*tombstone.DeletedAt))
	for _, prepared := range f.request.Tools {
		tool := readToolRowForTest(t, f.ctx, f.db, prepared.Tool.Id)
		require.NotNil(t, tool.DeletedAt)
		require.True(t, removedAt.Equal(*tool.DeletedAt))
		require.Equal(t, prepared.Tool.SourceKey, tool.SourceKey)
		require.Equal(t, 1, countToolRevisionsForTest(t, f.ctx, f.db, tool.Id))
		s := f.db.(*service)
		var revision ToolRevision
		err := s.sq.Select(revision.cols()...).From(ToolRevisionsTable).
			Where(sq.Eq{"tool_id": tool.Id, "revision": 1}).RunWith(s.db).QueryRowContext(f.ctx).Scan(revision.fields()...)
		require.NoError(t, err)
		require.Nil(t, revision.DeletedAt)
		require.Equal(t, prepared.EncryptedDefinition, revision.EncryptedDefinition)
		require.Equal(t, installed.AppliedGeneration, revision.SourceGeneration)
	}
	for _, id := range []apid.ID{base.Id, sibling.Id} {
		_, err := f.db.GetTool(f.ctx, id)
		require.NoError(t, err)
	}
	_, err = f.db.UpdateConnectionLabels(f.ctx, f.connection.Id, map[string]string{"env": "prod"})
	require.NoError(t, err)
	replacement, err := f.db.CreateToolSetBinding(f.ctx, f.toolSet.Id, f.connection.Id, 1)
	require.NoError(t, err)
	f.request.BindingId, f.request.ExpectedEpoch = replacement.Id, replacement.Epoch
	for index := range f.request.Tools {
		f.request.Tools[index].Tool.Id = apid.New(apid.PrefixTool)
		f.request.Tools[index].Tool.BindingId = replacement.Id
	}
	_, err = f.db.InstallExplicitToolSetInventory(f.ctx, f.request)
	require.NoError(t, err, "a new binding lifetime may install new IDs for the same source keys")
	require.ErrorIs(t, f.db.DeleteToolSetBinding(f.ctx, f.binding.Id, installed.Epoch, 1), ErrNotFound)
}

// TestExplicitToolSetInventoryConcurrentInstall permits one complete winner and
// rejects the competing captured epoch without orphaning its proposed rows.
func TestExplicitToolSetInventoryConcurrentInstall(t *testing.T) {
	f := explicitInventoryForTest(t, 1)
	f.db.(*service).db.SetMaxOpenConns(2)
	start := make(chan struct{})
	type outcome struct {
		request ToolSetInventoryInstall
		binding *ToolSetBinding
		err     error
	}
	results := make(chan outcome, 2)
	for range 2 {
		request := copyInventoryForTest(f.request)
		request.Tools[0].Tool.Id = apid.New(apid.PrefixTool)
		go func() {
			<-start
			binding, err := f.db.InstallExplicitToolSetInventory(f.ctx, request)
			results <- outcome{request, binding, err}
		}()
	}
	close(start)
	wins := 0
	for range 2 {
		result := <-results
		count := countToolRevisionsForTest(t, f.ctx, f.db, result.request.Tools[0].Tool.Id)
		if result.err == nil {
			wins++
			require.Equal(t, 1, count)
			require.Equal(t, uint64(2), result.binding.Epoch)
		} else {
			require.ErrorIs(t, result.err, ErrToolSetBindingEpochConflict)
			require.Zero(t, count)
		}
	}
	require.Equal(t, 1, wins)
	require.Equal(t, uint64(1), readInventoryBindingForTest(t, f).InventoryRevision)
}

// TestToolSetInventoryModelValidation keeps pending/installed counters paired and
// prevents optional generation provenance from overflowing either SQL provider.
func TestToolSetInventoryModelValidation(t *testing.T) {
	f := explicitInventoryForTest(t, 1)
	zero, one, maximum, overflow := uint64(0), uint64(1), uint64(math.MaxInt64), uint64(math.MaxInt64)+1
	for _, test := range []struct {
		generation *uint64
		revision   uint64
		valid      bool
	}{
		{nil, 0, true}, {nil, 1, false}, {&zero, 1, false},
		{&one, 0, false}, {&one, 1, true}, {&maximum, maximum, true},
		{&overflow, 1, false}, {&one, overflow, false},
	} {
		binding := *f.binding
		binding.AppliedGeneration, binding.InventoryRevision = test.generation, test.revision
		err := binding.Validate()
		if test.valid {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	tool := f.request.Tools[0].Tool
	tool.ActiveRevision = 1
	revision := newToolRevision(f.ctx, &tool, f.request.Tools[0].EncryptedDefinition, f.request.Tools[0].DefinitionDigest)
	for _, generation := range []*uint64{nil, &zero, &one, &maximum, &overflow} {
		revision.SourceGeneration = generation
		err := revision.Validate()
		if generation == nil || *generation == one || *generation == maximum {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}

// TestExplicitToolSetInventoryNamespaceLocks keeps both namespace parents live
// until an installation commits, including installations with no Tool rows.
func TestExplicitToolSetInventoryNamespaceLocks(t *testing.T) {
	for _, parent := range []string{"toolset", "connection"} {
		t.Run(parent, func(t *testing.T) {
			f := explicitInventoryForTest(t, 0)
			s := f.db.(*service)
			if s.cfg.GetProvider() != config.DatabaseProviderPostgres {
				t.Skip("PostgreSQL share-lock contention; SQLite serializes writers")
			}
			s.db.SetMaxOpenConns(2)
			ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
			defer cancel()
			installTx, err := s.db.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer installTx.Rollback()
			_, _, _, err = s.lockToolSetBindingMembership(ctx, installTx, f.toolSet.Id, f.connection.Id)
			require.NoError(t, err)
			_, err = s.toolSetInventoryNamespace(ctx, installTx, f.binding)
			require.NoError(t, err)
			deleteTx, err := s.db.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer deleteTx.Rollback()
			_, err = deleteTx.ExecContext(ctx, "SET LOCAL lock_timeout = '150ms'")
			require.NoError(t, err)
			path := f.toolSet.Namespace
			if parent == "connection" {
				path = f.connection.Namespace
			}
			_, err = s.sq.Update(NamespacesTable).Set("deleted_at", apctx.GetClock(ctx).Now()).
				Where(sq.Eq{"path": path}).RunWith(deleteTx).ExecContext(ctx)
			var postgresError interface{ SQLState() string }
			require.ErrorAs(t, err, &postgresError)
			require.Equal(t, "55P03", postgresError.SQLState(), "namespace deletion must wait for the inventory transaction")
			require.NoError(t, deleteTx.Rollback())
			require.NoError(t, installTx.Rollback())
			require.NoError(t, f.db.DeleteNamespace(ctx, path))
			_, err = f.db.InstallExplicitToolSetInventory(ctx, f.request)
			require.ErrorIs(t, err, ErrNotFound)
		})
	}
}
