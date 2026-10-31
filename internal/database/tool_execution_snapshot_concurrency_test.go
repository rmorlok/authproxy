package database

import (
	"context"
	"runtime"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/stretchr/testify/require"
)

// writeToolSnapshotConnectionForTest changes configuration and the connector
// pin in separate statements of one transaction. Readers must never observe the
// temporary mismatch between those statements, even while reads overlap commits.
func writeToolSnapshotConnectionForTest(
	ctx context.Context,
	s *service,
	id apid.ID,
	generation uint64,
	configuration encfield.EncryptedField,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := s.sq.Update(ConnectionsTable).
		Set("encrypted_configuration", configuration).
		Where(sq.Eq{"id": id}).RunWith(tx).ExecContext(ctx); err != nil {
		return err
	}
	// Give the reader an opportunity to run while the transaction is incomplete.
	runtime.Gosched()
	if _, err := s.sq.Update(ConnectionsTable).
		Set("connector_generation", generation).
		Where(sq.Eq{"id": id}).RunWith(tx).ExecContext(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

// TestToolExecutionSnapshotConcurrentConnectionMigration keeps the connection's
// configuration, generation pin, and connector definition from one committed
// state while another database session repeatedly migrates the connection.
func TestToolExecutionSnapshotConcurrentConnectionMigration(t *testing.T) {
	f := newToolExecutionSnapshotFixture(t)
	s := f.db.(*service)
	// The shared PostgreSQL fixture may allow only one database connection.
	// Give the reader and writer separate connections to exercise SQL snapshots.
	s.db.SetMaxOpenConns(2)

	second := testConnectorWithDefinition(f.connector.Id, f.connector.Namespace, f.connector.Name, 2)
	second.State = ConnectorGenerationStatePrimary
	require.NoError(t, f.db.UpsertConnectorGeneration(f.ctx, second))
	require.NotNil(t, f.connection.EncryptedConfiguration)
	configurations := map[uint64]encfield.EncryptedField{
		1: *f.connection.EncryptedConfiguration,
		2: {ID: f.connection.EncryptedConfiguration.ID, Data: "Y29uZmlnLXR3bw=="},
	}
	definitions := map[uint64]encfield.EncryptedField{
		1: f.connector.EncryptedDefinition,
		2: second.EncryptedDefinition,
	}
	first, err := f.db.GetToolExecutionSnapshot(f.ctx, f.tool.Id)
	require.NoError(t, err)

	// The writer returns errors to this test goroutine and is always joined,
	// including if a snapshot assertion fails or the bounded context expires.
	ctx, cancel := context.WithTimeout(f.ctx, 15*time.Second)
	start, done := make(chan struct{}), make(chan struct{})
	var writeErr error
	go func() {
		defer close(done)
		<-start
		for i := 0; i < 64; i++ {
			generation := uint64(1 + i%2)
			if writeErr = writeToolSnapshotConnectionForTest(ctx, s, f.connection.Id, generation, configurations[generation]); writeErr != nil {
				return
			}
		}
	}()
	defer func() {
		cancel()
		<-done
	}()
	close(start)

	// Every read may see either committed generation, but never combine that
	// generation's definition with the other generation's configuration.
	for i := 0; i < 128; i++ {
		snapshot, err := f.db.GetToolExecutionSnapshot(ctx, f.tool.Id)
		require.NoError(t, err)
		generation := snapshot.Connection.ConnectorGeneration
		require.Contains(t, configurations, generation)
		require.NotNil(t, snapshot.Connection.EncryptedConfiguration)
		require.Equal(t, configurations[generation], *snapshot.Connection.EncryptedConfiguration)
		require.Equal(t, generation, snapshot.Connector.Generation)
		require.Equal(t, snapshot.Connection.ConnectorId, snapshot.Connector.Id)
		require.Equal(t, definitions[generation], snapshot.Connector.EncryptedDefinition)
		require.Equal(t, snapshot.Revision.ConnectionId, snapshot.Connection.Id)
		require.Equal(t, first.Revision, snapshot.Revision)
	}
	<-done
	require.NoError(t, writeErr)

	// The final migration is visible to new reads without rewriting the snapshot
	// captured before any migration started.
	last, err := f.db.GetToolExecutionSnapshot(ctx, f.tool.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(2), last.Connection.ConnectorGeneration)
	require.Equal(t, configurations[2], *last.Connection.EncryptedConfiguration)
	require.Equal(t, definitions[2], last.Connector.EncryptedDefinition)
	require.Equal(t, uint64(1), first.Connection.ConnectorGeneration)
	require.Equal(t, configurations[1], *first.Connection.EncryptedConfiguration)
	require.Equal(t, definitions[1], first.Connector.EncryptedDefinition)
}
