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

func TestRunMigrationsUpStopsAfterCurrentMigrationWhenContextCancelled(t *testing.T) {
	sourceDriver, err := sourcestub.WithInstance(struct{}{}, &sourcestub.Config{})
	require.NoError(t, err)
	sourceBase := sourceDriver.(*sourcestub.Stub)
	require.True(t, sourceBase.Migrations.Append(&source.Migration{
		Version:    1,
		Direction:  source.Up,
		Identifier: "CREATE 1",
	}))
	require.True(t, sourceBase.Migrations.Append(&source.Migration{
		Version:    2,
		Direction:  source.Up,
		Identifier: "CREATE 2",
	}))

	databaseDriver, err := dbstub.WithInstance(struct{}{}, &dbstub.Config{})
	require.NoError(t, err)
	databaseBase := databaseDriver.(*dbstub.Stub)

	continueSource := make(chan struct{})
	continueMigration := make(chan struct{})
	var continueSourceOnce sync.Once
	var continueMigrationOnce sync.Once
	t.Cleanup(func() {
		continueSourceOnce.Do(func() { close(continueSource) })
		continueMigrationOnce.Do(func() { close(continueMigration) })
	})

	blockingSource := &blockingMigrationSource{
		Stub:         sourceBase,
		nextStarted:  make(chan struct{}),
		continueNext: continueSource,
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

	requireSignal(t, blockingDatabase.firstRunStarted, "first migration to start")
	requireSignal(t, blockingSource.nextStarted, "second migration to be discovered")
	cancel()
	require.Eventually(t, func() bool {
		return len(migrator.GracefulStop) == 1
	}, time.Second, time.Millisecond, "context cancellation was not bridged to GracefulStop")

	continueSourceOnce.Do(func() { close(continueSource) })
	continueMigrationOnce.Do(func() { close(continueMigration) })

	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("migration did not stop after context cancellation")
	}
	require.Equal(t, []string{"CREATE 1"}, databaseBase.MigrationSequence)
}

type blockingMigrationSource struct {
	*sourcestub.Stub
	nextStarted  chan struct{}
	continueNext chan struct{}
	blockOnce    sync.Once
}

func (s *blockingMigrationSource) Next(version uint) (uint, error) {
	s.blockOnce.Do(func() {
		close(s.nextStarted)
		<-s.continueNext
	})
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
