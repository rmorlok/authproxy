package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/migration"
	"github.com/stretchr/testify/require"
	clocktesting "k8s.io/utils/clock/testing"
)

func TestParseMigrateArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		target    migration.Target
		direction migration.Direction
		version   *uint
		wantErr   bool
	}{
		{name: "defaults up", args: []string{"all"}, target: migration.TargetAll, direction: migration.DirectionUp},
		{name: "down one", args: []string{"workflows", "down"}, target: migration.TargetWorkflows, direction: migration.DirectionDown},
		{name: "exact version", args: []string{"main-database", "up", "12"}, target: migration.TargetMainDatabase, direction: migration.DirectionUp, version: uintPtr(12)},
		{name: "bad target", args: []string{"main"}, wantErr: true},
		{name: "bad direction", args: []string{"workflows", "sideways"}, wantErr: true},
		{name: "bad version", args: []string{"app-metrics", "up", "latest"}, wantErr: true},
		{name: "zero version", args: []string{"main-database", "down", "0"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, direction, version, err := parseMigrateArgs(tt.args)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.target, target)
			require.Equal(t, tt.direction, direction)
			require.Equal(t, tt.version, version)
		})
	}
}

func TestPrepareServeRunsDevelopmentPlanOnce(t *testing.T) {
	fake := &fakeMigrationManager{}
	restoreMigrationManager(t, fake)
	var warnings bytes.Buffer

	require.NoError(t, prepareServe(context.Background(), true, &warnings))
	require.Equal(t, 1, fake.developmentCalls)
	require.Zero(t, fake.verifyCalls)
	require.Equal(t, 1, fake.shutdownCalls)
	require.Contains(t, warnings.String(), "WARNING")
}

func TestPrepareServeWithoutFlagOnlyVerifies(t *testing.T) {
	fake := &fakeMigrationManager{}
	restoreMigrationManager(t, fake)

	require.NoError(t, prepareServe(context.Background(), false, &bytes.Buffer{}))
	require.Zero(t, fake.developmentCalls)
	require.Equal(t, 1, fake.verifyCalls)
	require.Equal(t, 1, fake.shutdownCalls)
}

func TestResolveServicesAll(t *testing.T) {
	options, err := (serveFlags{}).resolve("all")
	require.NoError(t, err)
	require.Len(t, options.services, 4)
}

func TestServeDoesNotStartServicesWhenVerificationFails(t *testing.T) {
	for _, failure := range []string{"stale schema", "dirty schema"} {
		t.Run(failure, func(t *testing.T) {
			fake := &fakeMigrationManager{verifyErr: errors.New(failure)}
			restoreMigrationManager(t, fake)
			originalStartServices := startServices
			started := 0
			startServices = func(serveOptions) error {
				started++
				return nil
			}
			t.Cleanup(func() { startServices = originalStartServices })

			// Execute would supply a background context; RunE is called directly here.
			cmd := cmdServe()
			cmd.SetContext(context.Background())
			cmd.SetErr(&bytes.Buffer{})
			err := cmd.RunE(cmd, []string{"all"})
			require.ErrorContains(t, err, failure)
			require.Zero(t, started)
		})
	}
}

func TestPrepareServeRetriesUnavailableSchemas(t *testing.T) {
	unavailable := &migration.UnavailableError{Target: migration.TargetAppMetrics, Err: errors.New("memory limit exceeded")}

	// The app-metrics store rejects the first two checks, then recovers.
	fake := &fakeMigrationManager{verifyErrs: []error{unavailable, unavailable}}
	restoreMigrationManager(t, fake)
	var warnings bytes.Buffer

	// Serve preparation waits it out instead of failing startup, retrying once
	// per interval on the fake clock.
	ctx, fakeClock := fakeClockContext()
	err := runStepping(t, fakeClock, func() error { return prepareServe(ctx, false, &warnings) })
	require.NoError(t, err)
	require.Equal(t, 3, fake.verifyCalls)
	require.Equal(t, 2, strings.Count(warnings.String(), "retrying"))
}

func TestPrepareServeGivesUpOnPersistentlyUnavailableSchemas(t *testing.T) {
	unavailable := &migration.UnavailableError{Target: migration.TargetMainDatabase, Err: errors.New("connection refused")}

	// A store that never comes back is checked once per interval until the
	// wait timeout elapses on the fake clock, then startup fails.
	fake := &fakeMigrationManager{verifyErr: unavailable}
	restoreMigrationManager(t, fake)

	ctx, fakeClock := fakeClockContext()
	err := runStepping(t, fakeClock, func() error { return prepareServe(ctx, false, &bytes.Buffer{}) })
	require.ErrorIs(t, err, unavailable)
	require.Equal(t, int(startupSchemaWaitTimeout/startupSchemaRetryInterval)+1, fake.verifyCalls)
}

func TestPrepareServeDoesNotRetryIncompatibleSchemas(t *testing.T) {
	// A stale schema needs a migration, so it fails on the first check without
	// waiting on the clock.
	fake := &fakeMigrationManager{verifyErr: errors.New("app-metrics schema is behind")}
	restoreMigrationManager(t, fake)

	ctx, _ := fakeClockContext()
	require.Error(t, prepareServe(ctx, false, &bytes.Buffer{}))
	require.Equal(t, 1, fake.verifyCalls)
}

// fakeClockContext returns a context carrying a fake clock for apctx.GetClock.
func fakeClockContext() (context.Context, *clocktesting.FakeClock) {
	fakeClock := clocktesting.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	return apctx.WithClock(context.Background(), fakeClock), fakeClock
}

// runStepping runs fn in a goroutine and advances fakeClock by one retry
// interval whenever fn is blocked waiting on it, returning fn's result.
func runStepping(t *testing.T, fakeClock *clocktesting.FakeClock, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()

	timeout := time.After(10 * time.Second)
	for {
		select {
		case err := <-done:
			return err
		case <-timeout:
			t.Fatal("timed out waiting for startup schema verification")
		default:
		}
		if fakeClock.HasWaiters() {
			fakeClock.Step(startupSchemaRetryInterval)
		} else {
			time.Sleep(time.Millisecond)
		}
	}
}

func restoreMigrationManager(t *testing.T, fake migrationManager) {
	t.Helper()
	original := newMigrationManager
	newMigrationManager = func(string, config.C) migrationManager { return fake }
	t.Cleanup(func() { newMigrationManager = original })
}

func uintPtr(value uint) *uint { return &value }

type fakeMigrationManager struct {
	developmentCalls int
	verifyCalls      int
	shutdownCalls    int
	verifyErr        error
	// verifyErrs, when set, is returned one entry per VerifyMigrations call;
	// once exhausted, verifyErr is returned.
	verifyErrs []error
}

func (f *fakeMigrationManager) RunDevelopmentMigration(context.Context) error {
	f.developmentCalls++
	return nil
}

func (f *fakeMigrationManager) VerifyMigrations(context.Context) error {
	f.verifyCalls++
	if len(f.verifyErrs) > 0 {
		err := f.verifyErrs[0]
		f.verifyErrs = f.verifyErrs[1:]
		return err
	}
	return f.verifyErr
}

func (f *fakeMigrationManager) RunProductionMigration(context.Context, migration.Target, migration.Direction, *uint) error {
	return nil
}

func (f *fakeMigrationManager) MigrationStatuses(context.Context, migration.Target) []migration.Status {
	return nil
}

func (f *fakeMigrationManager) ShutdownMigrationResources() { f.shutdownCalls++ }
