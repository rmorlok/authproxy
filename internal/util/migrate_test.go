package util

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	dbstub "github.com/golang-migrate/migrate/v4/database/stub"
	"github.com/golang-migrate/migrate/v4/source"
	sourcestub "github.com/golang-migrate/migrate/v4/source/stub"
	"github.com/stretchr/testify/require"
)

// TestRunMigrationsUpStopsAfterCurrentMigrationWhenContextCancelled verifies that
// cancellation lets the running migration finish but skips already queued work.
func TestRunMigrationsUpStopsAfterCurrentMigrationWhenContextCancelled(t *testing.T) {
	// Provide two runnable migrations so stopping cannot be mistaken for completion.
	sourceDriver, err := sourcestub.WithInstance(struct{}{}, &sourcestub.Config{})
	require.NoError(t, err)
	sourceBase := sourceDriver.(*sourcestub.Stub)
	require.True(t, sourceBase.Migrations.Append(&source.Migration{
		Version:    1,
		Direction:  source.Up,
		Identifier: "CREATE 1",
	}))
	// The stub uses Identifier as the body; an empty body lets prefetch finish even
	// when execution is skipped. Running it would still record a second migration.
	require.True(t, sourceBase.Migrations.Append(&source.Migration{
		Version:    2,
		Direction:  source.Up,
		Identifier: "",
	}))

	databaseDriver, err := dbstub.WithInstance(struct{}{}, &dbstub.Config{})
	require.NoError(t, err)
	databaseBase := databaseDriver.(*dbstub.Stub)

	// Hold the first execution and pause discovery after the second migration is queued.
	continueSource := make(chan struct{})
	continueMigration := make(chan struct{})
	var continueSourceOnce sync.Once
	var continueMigrationOnce sync.Once
	t.Cleanup(func() {
		continueSourceOnce.Do(func() { close(continueSource) })
		continueMigrationOnce.Do(func() { close(continueMigration) })
	})

	blockingSource := &blockingMigrationSource{
		Stub:              sourceBase,
		blockAfterVersion: 2,
		nextStarted:       make(chan struct{}),
		continueNext:      continueSource,
	}
	blockingDatabase := &blockingMigrationDatabase{
		Stub:             databaseBase,
		firstRunStarted:  make(chan struct{}),
		continueFirstRun: continueMigration,
	}

	migrator, err := migrate.NewWithInstance("test-source", blockingSource, "test-database", blockingDatabase)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = migrator.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- RunMigrationsUp(ctx, migrator)
	}()

	// Cancel only after both barriers are reached, then verify the stop signal is sent.
	requireSignal(t, blockingDatabase.firstRunStarted, "first migration to start")
	requireSignal(t, blockingSource.nextStarted, "second migration to be queued")
	cancel()
	require.Eventually(t, func() bool {
		return len(migrator.GracefulStop) == 1
	}, time.Second, time.Millisecond, "context cancellation was not bridged to GracefulStop")

	// Keep the reader paused so only the executor can consume the graceful-stop signal.
	continueMigrationOnce.Do(func() { close(continueMigration) })

	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("migration did not stop after context cancellation")
	}

	// Let discovery finish after execution stops, and check that the queued work was skipped.
	continueSourceOnce.Do(func() { close(continueSource) })
	require.Equal(t, []string{"CREATE 1"}, databaseBase.MigrationSequence)
	require.Equal(t, 1, databaseBase.CurrentVersion)
	require.False(t, databaseBase.IsDirty)
}

// blockingMigrationSource pauses discovery after a chosen migration is queued.
// Keeping it paused through cancellation prevents golang-migrate's reader and
// executor from racing over the dependency's unsynchronized graceful-stop flag.
type blockingMigrationSource struct {
	*sourcestub.Stub
	blockAfterVersion uint
	nextStarted       chan struct{}
	continueNext      chan struct{}
	blockOnce         sync.Once
}

// Next blocks only after the chosen version has been sent to the execution queue.
func (s *blockingMigrationSource) Next(version uint) (uint, error) {
	if version == s.blockAfterVersion {
		s.blockOnce.Do(func() {
			close(s.nextStarted)
			<-s.continueNext
		})
	}
	return s.Stub.Next(version)
}

type blockingMigrationDatabase struct {
	*dbstub.Stub
	firstRunStarted  chan struct{}
	continueFirstRun chan struct{}
	blockOnce        sync.Once
}

func (d *blockingMigrationDatabase) Run(migration io.Reader) error {
	d.blockOnce.Do(func() {
		close(d.firstRunStarted)
		<-d.continueFirstRun
	})
	return d.Stub.Run(migration)
}

func requireSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}
