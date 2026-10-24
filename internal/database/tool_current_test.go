package database

import (
	"fmt"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apid"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/stretchr/testify/require"
)

// requireGeneratedToolCurrentForTest checks both current-definition and candidate
// reads without treating either internal result as permission to execute.
func requireGeneratedToolCurrentForTest(t *testing.T, f *toolSetInventoryFixture, visible bool) {
	t.Helper()
	id := f.request.Tools[0].Tool.Id
	revision, err := f.db.GetCurrentToolRevision(f.ctx, id)
	if visible {
		require.NoError(t, err)
		require.Equal(t, id, revision.ToolId)
		require.Equal(t, f.connection.Id, revision.ConnectionId)
		require.NotNil(t, revision.SourceGeneration)
		require.Equal(t, f.request.Generation, *revision.SourceGeneration)
	} else {
		require.ErrorIs(t, err, ErrNotFound)
	}

	page, err := f.db.ListToolCandidates(f.ctx, ToolCandidateQuery{
		NamespaceMatchers: []string{"root.**"}, Limit: MaxToolCandidateLimit,
	})
	require.NoError(t, err)
	if visible {
		require.Contains(t, toolCandidateIDsForTest(page), id)
	} else {
		require.NotContains(t, toolCandidateIDsForTest(page), id)
	}
}

// TestCurrentGeneratedToolOwnership rejects retired parents and inconsistent
// provenance even while the materialized Tool and its active pointer still exist.
func TestCurrentGeneratedToolOwnership(t *testing.T) {
	removedAt := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name    string
		table   string
		values  sq.Eq
		history bool
	}{
		{"deleted tool", ToolsTable, sq.Eq{"deleted_at": removedAt}, false},
		{"deleted revision", ToolRevisionsTable, sq.Eq{"deleted_at": removedAt}, false},
		{"deleted connection", ConnectionsTable, sq.Eq{"deleted_at": removedAt}, true},
		{"deleted connection namespace", "connection namespace", sq.Eq{"deleted_at": removedAt}, false},
		{"deleted binding", ToolSetBindingsTable, sq.Eq{"deleted_at": removedAt}, true},
		{"deleted toolset", ToolSetsTable, sq.Eq{"deleted_at": removedAt}, true},
		{"deleted toolset namespace", "toolset namespace", sq.Eq{"deleted_at": removedAt}, true},
		{"deleted generation", ToolSetGenerationsTable, sq.Eq{"deleted_at": removedAt}, true},
		{"draft generation", ToolSetGenerationsTable, sq.Eq{"state": toolsetschema.ToolSetReleaseStateDraft}, true},
		{"pending binding", ToolSetBindingsTable, sq.Eq{"applied_generation": nil, "inventory_revision": 0}, true},
		{"different applied generation", ToolSetBindingsTable, sq.Eq{"applied_generation": 2}, true},
		{"missing source generation", ToolRevisionsTable, sq.Eq{"source_generation": nil}, true},
		{"different source generation", ToolRevisionsTable, sq.Eq{"source_generation": 2}, true},
		{"different connection namespace", ToolsTable, sq.Eq{"namespace": "root"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Start with a valid installed inventory in a child of the ToolSet namespace.
			f := installedInventoryForTest(t, 1)
			requireGeneratedToolCurrentForTest(t, f, true)
			s := f.db.(*service)
			standalone := &Tool{
				Id: apid.New(apid.PrefixTool), Namespace: f.connection.Namespace,
				ConnectionId: f.connection.Id, Description: "Independent of the ToolSet",
				Verbs: []string{"read"},
			}
			_, err := f.db.CreateStandaloneTool(f.ctx, standalone, f.request.Tools[0].EncryptedDefinition, toolDefinitionDigestForTest)
			require.NoError(t, err)
			table, where := test.table, sq.Eq{}
			switch table {
			case ToolsTable:
				where["id"] = f.request.Tools[0].Tool.Id
			case ToolRevisionsTable:
				where["tool_id"], where["revision"] = f.request.Tools[0].Tool.Id, 1
			case ConnectionsTable:
				where["id"] = f.connection.Id
			case ToolSetBindingsTable:
				where["id"] = f.binding.Id
			case ToolSetsTable:
				where["id"] = f.toolSet.Id
			case ToolSetGenerationsTable:
				where["tool_set_id"], where["generation"] = f.toolSet.Id, 1
			case "connection namespace":
				table, where["path"] = NamespacesTable, f.connection.Namespace
			case "toolset namespace":
				table, where["path"] = NamespacesTable, f.toolSet.Namespace
			}

			// Alter one stored invariant without removing the Tool through cleanup.
			_, err = s.sq.Update(table).SetMap(test.values).Where(where).RunWith(s.db).ExecContext(f.ctx)
			require.NoError(t, err)
			requireGeneratedToolCurrentForTest(t, f, false)

			// ToolSet-only failures leave independently authored Tools discoverable.
			if test.table != ConnectionsTable && test.table != "connection namespace" {
				_, err = f.db.GetCurrentToolRevision(f.ctx, standalone.Id)
				require.NoError(t, err)
				page, err := f.db.ListToolCandidates(f.ctx, ToolCandidateQuery{NamespaceMatchers: []string{"root.**"}, Limit: 10})
				require.NoError(t, err)
				require.Equal(t, []apid.ID{standalone.Id}, toolCandidateIDsForTest(page))
			}

			// Parent and ownership changes do not erase inspectable retained history.
			if test.history {
				_, err = f.db.GetToolRevision(f.ctx, f.request.Tools[0].Tool.Id, 1)
				require.NoError(t, err)
			}
		})
	}
}

// TestCurrentGeneratedToolConnectionAndGenerationIdentity requires the complete
// binding, Tool, and revision chain to agree on one connection and generation.
func TestCurrentGeneratedToolConnectionAndGenerationIdentity(t *testing.T) {
	for _, table := range []string{ToolSetBindingsTable, ToolRevisionsTable} {
		t.Run(table, func(t *testing.T) {
			// A second live connection must not satisfy this Tool's installed binding.
			f := installedInventoryForTest(t, 1)
			other := standaloneToolConnectionForTest(t, f.ctx, f.db, f.connection.Namespace)
			s := f.db.(*service)
			where := sq.Eq{"id": f.binding.Id}
			if table == ToolRevisionsTable {
				where = sq.Eq{"tool_id": f.request.Tools[0].Tool.Id, "revision": 1}
			}
			_, err := s.sq.Update(table).Set("connection_id", other.Id).Where(where).RunWith(s.db).ExecContext(f.ctx)
			require.NoError(t, err)
			requireGeneratedToolCurrentForTest(t, f, false)
		})
	}

	// Matching counters alone cannot replace the actual published generation row.
	f := installedInventoryForTest(t, 1)
	s := f.db.(*service)
	_, err := s.sq.Delete(ToolSetGenerationsTable).
		Where(sq.Eq{"tool_set_id": f.toolSet.Id, "generation": 1}).RunWith(s.db).ExecContext(f.ctx)
	require.NoError(t, err)
	requireGeneratedToolCurrentForTest(t, f, false)
}

// TestCurrentGeneratedToolPinnedGeneration preserves installed content when a
// newer primary is published or the installed generation is archived.
func TestCurrentGeneratedToolPinnedGeneration(t *testing.T) {
	// Publication makes the installed generation active without migrating it.
	f := installedInventoryForTest(t, 1)
	publishReplacementGenerationForTest(t, f)
	requireGeneratedToolCurrentForTest(t, f, true)

	// Archival blocks new installations, but an existing pin remains readable.
	s := f.db.(*service)
	_, err := s.sq.Update(ToolSetGenerationsTable).
		Set("state", toolsetschema.ToolSetReleaseStateArchived).
		Where(sq.Eq{"tool_set_id": f.toolSet.Id, "generation": 1}).RunWith(s.db).ExecContext(f.ctx)
	require.NoError(t, err)
	requireGeneratedToolCurrentForTest(t, f, true)
}

// TestCurrentGeneratedToolDeferredPolicy keeps storage reads separate from
// eventual membership removal and the future authorization/readiness checks.
func TestCurrentGeneratedToolDeferredPolicy(t *testing.T) {
	// A temporarily unavailable connection still has an inspectable installed Tool.
	f := installedInventoryForTest(t, 1)
	for _, state := range []ConnectionState{
		ConnectionStateSetup, ConnectionStateDisabled, ConnectionStateDisconnecting,
		ConnectionStateDisconnected, ConnectionStateConfigured,
	} {
		require.NoError(t, f.db.SetConnectionState(f.ctx, f.connection.Id, state))
		requireGeneratedToolCurrentForTest(t, f, true)
	}

	// Updated labels do not instantly revoke membership before reconciliation.
	_, err := f.db.UpdateConnectionLabels(f.ctx, f.connection.Id, map[string]string{"env": "dev"})
	require.NoError(t, err)
	requireGeneratedToolCurrentForTest(t, f, true)

	// A selector edit also leaves the installed inventory visible until removal.
	selector := toolsetschema.ConnectionSelector{MatchLabels: map[string]string{"env": "staging"}}
	updated, err := f.db.UpdateToolSetConnectionSelector(f.ctx, f.toolSet.Id, f.toolSet.SelectorRevision, selector)
	require.NoError(t, err)
	requireGeneratedToolCurrentForTest(t, f, true)

	// Reconciliation retires the binding and both reads stop returning its Tool.
	require.NoError(t, f.db.DeleteToolSetBinding(f.ctx, f.binding.Id, f.binding.Epoch, updated.SelectorRevision))
	requireGeneratedToolCurrentForTest(t, f, false)
}

// TestCurrentStandaloneToolOwnership rejects generated provenance or a moved
// namespace without requiring standalone Tools to have a ToolSet parent.
func TestCurrentStandaloneToolOwnership(t *testing.T) {
	for _, provenance := range []bool{false, true} {
		t.Run(fmt.Sprintf("generated provenance %t", provenance), func(t *testing.T) {
			// The normal standalone revision remains readable with no binding.
			ctx, db, tool, _ := standaloneToolForTest(t)
			_, err := db.GetCurrentToolRevision(ctx, tool.Id)
			require.NoError(t, err)
			s := db.(*service)

			// A malformed namespace or generated source must fail both current reads.
			query := s.sq.Update(ToolsTable).Set("namespace", "root").Where(sq.Eq{"id": tool.Id})
			if provenance {
				query = s.sq.Update(ToolRevisionsTable).Set("source_generation", 1).
					Where(sq.Eq{"tool_id": tool.Id, "revision": 1})
			}
			_, err = query.RunWith(s.db).ExecContext(ctx)
			require.NoError(t, err)
			_, err = db.GetCurrentToolRevision(ctx, tool.Id)
			require.ErrorIs(t, err, ErrNotFound)
			page, err := db.ListToolCandidates(ctx, ToolCandidateQuery{NamespaceMatchers: []string{"root.**"}, Limit: 10})
			require.NoError(t, err)
			require.Empty(t, page.Items)

			// History is inspection data and retains its original read contract.
			_, err = db.GetToolRevision(ctx, tool.Id, 1)
			require.NoError(t, err)
		})
	}
}

// TestCurrentToolCandidatesMixedPagination applies common filters and one cursor
// order across standalone Tools and installed generated Tools without duplicates.
func TestCurrentToolCandidatesMixedPagination(t *testing.T) {
	// Interleave filtered generated rows with matching standalone/generated rows.
	f := explicitInventoryForTest(t, 3)
	ids := make([]apid.ID, 6)
	for index := range ids {
		ids[index] = apid.ID(fmt.Sprintf("tol_%016d", index))
	}
	for index := range f.request.Tools {
		entry := &f.request.Tools[index]
		entry.Tool.Id = ids[index*2]
		entry.Tool.Labels = Labels{"eligible": "yes"}
		if index == 0 {
			entry.Tool.Labels["eligible"] = "no"
		} else if index == 1 {
			entry.Tool.Verbs = []string{"write"}
		}
	}
	_, err := f.db.InstallExplicitToolSetInventory(f.ctx, f.request)
	require.NoError(t, err)
	for _, index := range []int{1, 3, 5} {
		tool := &Tool{
			Id: ids[index], Namespace: f.connection.Namespace, ConnectionId: f.connection.Id,
			Description: "An independently authored operation", Verbs: []string{"read"},
			Labels: Labels{"eligible": "yes"},
		}
		_, err = f.db.CreateStandaloneTool(f.ctx, tool, f.request.Tools[0].EncryptedDefinition, toolDefinitionDigestForTest)
		require.NoError(t, err)
	}

	// Namespace, label, and verb filters apply before the common page boundary.
	first, err := f.db.ListToolCandidates(f.ctx, ToolCandidateQuery{
		NamespaceMatchers: []string{f.connection.Namespace},
		LabelSelector:     "eligible=yes,apxy/cxn/-/id=" + f.connection.Id.String(),
		AnyVerbs:          []string{"read"}, Limit: 2,
	})
	require.NoError(t, err)
	require.Equal(t, []apid.ID{ids[1], ids[3]}, toolCandidateIDsForTest(first))
	require.NotEmpty(t, first.Cursor)

	// The saved query continues through both ownership kinds and terminates once.
	last, err := f.db.ListToolCandidatesFromCursor(f.ctx, first.Cursor)
	require.NoError(t, err)
	require.Equal(t, []apid.ID{ids[4], ids[5]}, toolCandidateIDsForTest(last))
	require.Empty(t, last.Cursor)
	for _, candidate := range append(first.Items, last.Items...) {
		require.Equal(t, f.connection.Id, candidate.ConnectionId)
		require.Equal(t, uint64(1), candidate.ActiveRevision)
		require.Equal(t, f.connection.Id.String(), candidate.Labels["apxy/cxn/-/id"])
	}
}
