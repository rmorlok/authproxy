package database

import (
	"context"
	"strings"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// toolExecutionSnapshotFixture owns a real connector, connection, and standalone
// Tool; its re-read parents include the exact stored metadata and timestamps.
type toolExecutionSnapshotFixture struct {
	ctx        context.Context
	db         DB
	tool       *Tool
	revision   *ToolRevision
	connection *Connection
	connector  *ConnectorWithDefinition
}

// newToolExecutionSnapshotFixture uses a sub-microsecond clock so write/read
// comparisons exercise timestamp precision on both database providers.
func newToolExecutionSnapshotFixture(t *testing.T) *toolExecutionSnapshotFixture {
	t.Helper()
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.WithFixedClock(context.Background(), time.Date(2026, 11, 1, 12, 0, 0, 123456789, time.UTC))
	for _, namespace := range []string{"root.provider", "root.owner"} {
		require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: namespace, Labels: Labels{"team": "platform"}}))
	}
	connector := testConnectorWithDefinition(apid.New(apid.PrefixConnector), "root.provider", "provider", 1)
	connector.State = ConnectorGenerationStatePrimary
	connector.Labels, connector.Annotations = Labels{"version": "one"}, Annotations{"note": "connector"}
	require.NoError(t, db.UpsertConnectorGeneration(ctx, connector))
	connection := &Connection{
		Id: apid.New(apid.PrefixConnection), Namespace: "root.owner", Name: "account",
		ConnectorId: connector.Id, ConnectorGeneration: 1, State: ConnectionStateConfigured,
		Labels: Labels{"tenant": "first"}, Annotations: Annotations{"note": "connection"},
		EncryptedConfiguration: &encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "Y29uZmlnLW9uZQ=="},
	}
	require.NoError(t, db.CreateConnection(ctx, connection))
	tool := &Tool{
		Id: apid.New(apid.PrefixTool), Namespace: connection.Namespace, Name: "lookup",
		ConnectionId: connection.Id, Description: "Read a contact", Verbs: []string{"read"},
	}
	revision, err := db.CreateStandaloneTool(ctx, tool, encfield.EncryptedField{
		ID: apid.New(apid.PrefixDataEncryptionKey), Data: "ZGVmaW5pdGlvbg==",
	}, toolDefinitionDigestForTest)
	require.NoError(t, err)
	connection, err = db.GetConnection(ctx, connection.Id)
	require.NoError(t, err)
	connector, err = db.GetConnectorGeneration(ctx, connector.Id, 1)
	require.NoError(t, err)
	return &toolExecutionSnapshotFixture{ctx, db, tool, revision, connection, connector}
}

// TestToolExecutionSnapshotCapturesConnectionAndConnector returns all captured
// execution inputs together and follows the connection's pin rather than primary.
func TestToolExecutionSnapshotCapturesConnectionAndConnector(t *testing.T) {
	f := newToolExecutionSnapshotFixture(t)
	first, err := f.db.GetToolExecutionSnapshot(f.ctx, f.tool.Id)
	require.NoError(t, err)
	storedRevision, err := f.db.GetCurrentToolRevision(f.ctx, f.tool.Id)
	require.NoError(t, err)
	require.Equal(t, *storedRevision, first.Revision)
	require.True(t, f.revision.CreatedAt.Equal(first.Revision.CreatedAt))
	require.True(t, f.revision.EncryptedAt.Equal(*first.Revision.EncryptedAt))
	require.Equal(t, *f.connection, first.Connection)
	require.Equal(t, *f.connector, first.Connector)
	require.Equal(t, apid.Nil, first.ToolSetId)

	// Publishing another primary cannot silently migrate this connection's pin.
	next := testConnectorWithDefinition(f.connector.Id, f.connector.Namespace, f.connector.Name, 2)
	next.State = ConnectorGenerationStatePrimary
	require.NoError(t, f.db.UpsertConnectorGeneration(f.ctx, next))
	pinned, err := f.db.GetToolExecutionSnapshot(f.ctx, f.tool.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(1), pinned.Connector.Generation)
	require.Equal(t, ConnectorGenerationStateActive, pinned.Connector.State)
	require.Equal(t, first.Connector.EncryptedDefinition, pinned.Connector.EncryptedDefinition)

	// A migration atomically changes the pin, configuration, metadata, and setup state.
	step := connectorschema.MustNewSetupStep("configure-tenant")
	_, err = f.db.UpdateConnectionForGenerationMigration(f.ctx, ConnectionGenerationMigrationUpdate{
		Id: f.connection.Id, ConnectorId: f.connector.Id, ConnectorGeneration: 2,
		EncryptedConfiguration: &encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "Y29uZmlnLXR3bw=="},
		UserLabels:             map[string]string{"tenant": "second"}, Annotations: map[string]string{"note": "migrated"},
		SetupStep: &step, SetupError: util.ToPtr("setup pending"), HealthState: util.ToPtr(ConnectionHealthStateUnhealthy),
	})
	require.NoError(t, err)
	current, err := f.db.GetToolExecutionSnapshot(f.ctx, f.tool.Id)
	require.NoError(t, err)
	storedConnection, err := f.db.GetConnection(f.ctx, f.connection.Id)
	require.NoError(t, err)
	storedConnector, err := f.db.GetConnectorGeneration(f.ctx, f.connector.Id, 2)
	require.NoError(t, err)
	require.Equal(t, *storedConnection, current.Connection)
	require.Equal(t, *storedConnector, current.Connector)
	require.Equal(t, first.Revision, current.Revision)
	require.Equal(t, "first", first.Connection.Labels["tenant"])
	require.Equal(t, uint64(1), first.Connector.Generation)
	require.Equal(t, "Y29uZmlnLW9uZQ==", first.Connection.EncryptedConfiguration.Data)

	// Mutating returned maps and encrypted-field pointers cannot alter another read.
	current.Connection.Labels["tenant"] = "mutated"
	current.Connection.Annotations["note"] = "mutated"
	current.Connection.EncryptedConfiguration.Data = "mutated"
	current.Connector.Labels["version"] = "mutated"
	again, err := f.db.GetToolExecutionSnapshot(f.ctx, f.tool.Id)
	require.NoError(t, err)
	require.Equal(t, *storedConnection, again.Connection)
	require.Equal(t, *storedConnector, again.Connector)
}

// TestToolExecutionSnapshotStandaloneRetarget changes executable revision and
// connection together while a previously captured snapshot remains unchanged.
func TestToolExecutionSnapshotStandaloneRetarget(t *testing.T) {
	f := newToolExecutionSnapshotFixture(t)
	first, err := f.db.GetToolExecutionSnapshot(f.ctx, f.tool.Id)
	require.NoError(t, err)
	connector := testConnectorWithDefinition(apid.New(apid.PrefixConnector), f.connector.Namespace, "other", 1)
	connector.State = ConnectorGenerationStatePrimary
	require.NoError(t, f.db.UpsertConnectorGeneration(f.ctx, connector))
	target := *f.connection
	target.Id, target.Name, target.ConnectorId = apid.New(apid.PrefixConnection), "other-account", connector.Id
	require.NoError(t, f.db.CreateConnection(f.ctx, &target))

	// Standalone edits immediately replace the active revision and connection.
	f.tool.ConnectionId = target.Id
	definition := f.revision.EncryptedDefinition
	definition.Data = "bmV3LWRlZmluaXRpb24="
	written, err := f.db.UpdateStandaloneTool(f.ctx, f.tool, 1, definition, strings.Repeat("a", 64))
	require.NoError(t, err)
	current, err := f.db.GetToolExecutionSnapshot(f.ctx, f.tool.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(2), current.Revision.Revision)
	require.Equal(t, written.ConnectionId, current.Connection.Id)
	require.Equal(t, definition, current.Revision.EncryptedDefinition)
	require.Equal(t, written.DefinitionDigest, current.Revision.DefinitionDigest)
	require.True(t, written.CreatedAt.Equal(current.Revision.CreatedAt))
	require.Equal(t, connector.Id, current.Connector.Id)
	require.Equal(t, connector.EncryptedDefinition, current.Connector.EncryptedDefinition)
	require.Equal(t, apid.Nil, current.ToolSetId)
	require.Equal(t, uint64(1), first.Revision.Revision)
	require.Equal(t, f.connection.Id, first.Connection.Id)
	require.Equal(t, f.connector.Id, first.Connector.Id)
}

// snapshotInventoryForTest adds the real connector chain required for execution
// to the existing generated-inventory fixture, then installs at precise time.
func snapshotInventoryForTest(t *testing.T) *toolSetInventoryFixture {
	t.Helper()
	f := explicitInventoryForTest(t, 1)
	f.ctx = apctx.WithFixedClock(f.ctx, time.Date(2026, 11, 1, 12, 0, 0, 123456789, time.UTC))
	require.NoError(t, f.db.CreateNamespace(f.ctx, &Namespace{Path: "root.provider"}))
	connector := testConnectorWithDefinition(f.connection.ConnectorId, "root.provider", "provider", 1)
	connector.State = ConnectorGenerationStatePrimary
	require.NoError(t, f.db.UpsertConnectorGeneration(f.ctx, connector))
	installed, err := f.db.InstallExplicitToolSetInventory(f.ctx, f.request)
	require.NoError(t, err)
	f.binding, f.request.ExpectedEpoch = installed, installed.Epoch
	return f
}

// TestToolExecutionSnapshotGeneratedPin retains the installed ToolSet provenance
// until migration or reconciliation changes that binding, independently of readiness.
func TestToolExecutionSnapshotGeneratedPin(t *testing.T) {
	f := snapshotInventoryForTest(t)
	id := f.request.Tools[0].Tool.Id
	first, err := f.db.GetToolExecutionSnapshot(f.ctx, id)
	require.NoError(t, err)
	require.Equal(t, f.toolSet.Id, first.ToolSetId)
	require.Equal(t, f.request.Generation, *first.Revision.SourceGeneration)
	require.Equal(t, f.connection.Id, first.Connection.Id)
	require.Nil(t, first.Connection.EncryptedConfiguration)
	publishReplacementGenerationForTest(t, f)
	s := f.db.(*service)
	_, err = s.sq.Update(ToolSetGenerationsTable).Set("state", toolsetschema.ToolSetReleaseStateArchived).
		Where(sq.Eq{"tool_set_id": f.toolSet.Id, "generation": 1}).RunWith(s.db).ExecContext(f.ctx)
	require.NoError(t, err)

	// An archived installed source and an unavailable connection remain readable.
	for _, state := range []ConnectionState{ConnectionStateSetup, ConnectionStateDisabled, ConnectionStateDisconnecting, ConnectionStateDisconnected, ConnectionStateConfigured} {
		require.NoError(t, f.db.SetConnectionState(f.ctx, f.connection.Id, state))
		snapshot, err := f.db.GetToolExecutionSnapshot(f.ctx, id)
		require.NoError(t, err)
		require.Equal(t, state, snapshot.Connection.State)
		require.Equal(t, f.toolSet.Id, snapshot.ToolSetId)
		require.Equal(t, uint64(1), *snapshot.Revision.SourceGeneration)
	}

	// Live selector drift waits for reconciliation; retiring the binding removes it.
	_, err = f.db.UpdateConnectionLabels(f.ctx, f.connection.Id, map[string]string{"env": "dev"})
	require.NoError(t, err)
	_, err = f.db.GetToolExecutionSnapshot(f.ctx, id)
	require.NoError(t, err)
	require.NoError(t, f.db.DeleteToolSetBinding(f.ctx, f.binding.Id, f.binding.Epoch, f.toolSet.SelectorRevision))
	snapshot, err := f.db.GetToolExecutionSnapshot(f.ctx, id)
	require.ErrorIs(t, err, ErrNotFound)
	require.Nil(t, snapshot)
}

// TestToolExecutionSnapshotRejectsBrokenParents checks the live ownership chain
// directly, including connector conditions absent from the historical read API.
func TestToolExecutionSnapshotRejectsBrokenParents(t *testing.T) {
	removedAt := time.Date(2026, 11, 1, 13, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name, table    string
		values         sq.Eq
		legacyReadable bool
	}{
		{"tool", ToolsTable, sq.Eq{"deleted_at": removedAt}, false},
		{"revision", ToolRevisionsTable, sq.Eq{"deleted_at": removedAt}, false},
		{"connection", ConnectionsTable, sq.Eq{"deleted_at": removedAt}, false},
		{"connection namespace", "connection namespace", sq.Eq{"deleted_at": removedAt}, false},
		{"connector", ConnectorsTable, sq.Eq{"deleted_at": removedAt}, true},
		{"connector generation", ConnectorGenerationsTable, sq.Eq{"deleted_at": removedAt}, true},
		{"connector namespace", "connector namespace", sq.Eq{"deleted_at": removedAt}, true},
		{"missing pinned generation", ConnectionsTable, sq.Eq{"connector_generation": 99}, true},
		{"mismatched revision connection", ToolRevisionsTable, sq.Eq{"connection_id": apid.New(apid.PrefixConnection)}, false},
		{"binding", ToolSetBindingsTable, sq.Eq{"deleted_at": removedAt}, false},
		{"toolset", ToolSetsTable, sq.Eq{"deleted_at": removedAt}, false},
		{"toolset namespace", "toolset namespace", sq.Eq{"deleted_at": removedAt}, false},
		{"toolset generation", ToolSetGenerationsTable, sq.Eq{"deleted_at": removedAt}, false},
		{"draft source", ToolSetGenerationsTable, sq.Eq{"state": toolsetschema.ToolSetReleaseStateDraft}, false},
		{"mismatched source pin", ToolSetBindingsTable, sq.Eq{"applied_generation": 2}, false},
		{"pending inventory", ToolSetBindingsTable, sq.Eq{"applied_generation": nil, "inventory_revision": 0}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := snapshotInventoryForTest(t)
			id := f.request.Tools[0].Tool.Id
			_, err := f.db.GetToolExecutionSnapshot(f.ctx, id)
			require.NoError(t, err)
			s := f.db.(*service)
			table, where := test.table, sq.Eq{}
			switch table {
			case ToolsTable:
				where["id"] = id
			case ToolRevisionsTable:
				where["tool_id"], where["revision"] = id, 1
			case ConnectionsTable:
				where["id"] = f.connection.Id
			case ConnectorsTable:
				where["id"] = f.connection.ConnectorId
			case ConnectorGenerationsTable:
				where["connector_id"], where["generation"] = f.connection.ConnectorId, 1
			case ToolSetBindingsTable:
				where["id"] = f.binding.Id
			case ToolSetsTable:
				where["id"] = f.toolSet.Id
			case ToolSetGenerationsTable:
				where["tool_set_id"], where["generation"] = f.toolSet.Id, 1
			case "connection namespace":
				table, where["path"] = NamespacesTable, f.connection.Namespace
			case "connector namespace":
				table, where["path"] = NamespacesTable, "root.provider"
			case "toolset namespace":
				table, where["path"] = NamespacesTable, f.toolSet.Namespace
			}
			// Break one parent invariant without cascading deletion through the Tool.
			_, err = s.sq.Update(table).SetMap(test.values).Where(where).RunWith(s.db).ExecContext(f.ctx)
			require.NoError(t, err)
			snapshot, err := f.db.GetToolExecutionSnapshot(f.ctx, id)
			require.ErrorIs(t, err, ErrNotFound)
			require.Nil(t, snapshot)
			if test.legacyReadable {
				// The existing revision-only read deliberately keeps its older contract.
				_, err = f.db.GetCurrentToolRevision(f.ctx, id)
				require.NoError(t, err)
			}
		})
	}
}

// TestToolExecutionSnapshotReadErrors returns no partial execution data for
// invalid identities, absent Tools, or a caller cancellation before the query.
func TestToolExecutionSnapshotReadErrors(t *testing.T) {
	f := newToolExecutionSnapshotFixture(t)
	for _, id := range []apid.ID{apid.Nil, f.connection.Id} {
		// Malformed or wrong-resource identifiers fail validation before selection.
		snapshot, err := f.db.GetToolExecutionSnapshot(f.ctx, id)
		require.Error(t, err)
		require.Nil(t, snapshot)
	}
	snapshot, err := f.db.GetToolExecutionSnapshot(f.ctx, apid.New(apid.PrefixTool))
	require.ErrorIs(t, err, ErrNotFound)
	require.Nil(t, snapshot)

	// A canceled valid lookup preserves cancellation rather than fabricating absence.
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	snapshot, err = f.db.GetToolExecutionSnapshot(ctx, f.tool.Id)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, snapshot)
}
