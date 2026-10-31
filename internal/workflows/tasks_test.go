package workflows

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"testing"
	"time"

	wfbackend "github.com/cschleiden/go-workflows/backend"
	"github.com/cschleiden/go-workflows/client"
	wfworker "github.com/cschleiden/go-workflows/worker"
	wflib "github.com/cschleiden/go-workflows/workflow"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/migration"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

// recordingRemover captures the removal options the task passes to the backend.
type recordingRemover struct {
	calls   []wfbackend.RemovalOptions
	failure error
}

func (r *recordingRemover) RemoveWorkflowInstances(_ context.Context, options ...wfbackend.RemovalOption) error {
	removal := wfbackend.DefaultRemovalOptions
	for _, option := range options {
		option(&removal)
	}
	r.calls = append(r.calls, removal)
	return r.failure
}

func TestTaskHandlerGetCronTasks(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	t.Run("schedules daily cleanup by default", func(t *testing.T) {
		// An absent tasks block still gets the default retention and schedule.
		tasks := NewTaskHandler(nil, nil, logger).GetCronTasks()
		require.Len(t, tasks, 1)
		assert.Equal(t, "@daily", tasks[0].Cronspec)
		assert.Equal(t, taskTypeRemoveFinishedInstances, tasks[0].Task.Type())
	})

	t.Run("omits cleanup when retention is disabled", func(t *testing.T) {
		cfg := &config.Tasks{WorkflowRetention: common.HumanDurationFor("0s")}
		assert.Empty(t, NewTaskHandler(nil, cfg, logger).GetCronTasks())
	})
}

func TestRemoveFinishedInstancesCutoff(t *testing.T) {
	now := time.Date(2026, time.October, 31, 12, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilder(context.Background()).WithClock(clock.NewFakeClock(now)).Build()
	task := asynq.NewTask(taskTypeRemoveFinishedInstances, nil)

	newHandler := func(remover instanceRemover, cfg *config.Tasks) *TaskHandler {
		return &TaskHandler{remover: remover, cfg: cfg, logger: slog.New(slog.DiscardHandler)}
	}

	t.Run("uses the default retention", func(t *testing.T) {
		remover := &recordingRemover{}
		require.NoError(t, newHandler(remover, nil).removeFinishedInstances(ctx, task))

		// The cutoff is 30 days before the clock's current time.
		require.Len(t, remover.calls, 1)
		assert.True(t, remover.calls[0].FinishedBefore.Equal(now.Add(-30*24*time.Hour)))
	})

	t.Run("uses the configured retention", func(t *testing.T) {
		remover := &recordingRemover{}
		cfg := &config.Tasks{WorkflowRetention: common.HumanDurationFor("7d")}
		require.NoError(t, newHandler(remover, cfg).removeFinishedInstances(ctx, task))

		// A shorter configured retention moves the cutoff forward.
		require.Len(t, remover.calls, 1)
		assert.True(t, remover.calls[0].FinishedBefore.Equal(now.Add(-7*24*time.Hour)))
	})

	t.Run("does nothing when retention is disabled", func(t *testing.T) {
		remover := &recordingRemover{}
		cfg := &config.Tasks{WorkflowRetention: common.HumanDurationFor("0s")}
		require.NoError(t, newHandler(remover, cfg).removeFinishedInstances(ctx, task))

		// A task queued before cleanup was disabled must not delete anything.
		assert.Empty(t, remover.calls)
	})

	t.Run("returns backend failures so Asynq retries", func(t *testing.T) {
		failure := errors.New("database unavailable")
		err := newHandler(&recordingRemover{failure: failure}, nil).removeFinishedInstances(ctx, task)
		require.ErrorIs(t, err, failure)
	})
}

// TestRemoveFinishedInstancesAgainstBackend runs real workflows on the configured test database to prove the
// cutoff works with how each backend stores completion times and that running workflows survive.
func TestRemoveFinishedInstancesAgainstBackend(t *testing.T) {
	cfg, _, rawDB := database.MustApplyBlankTestDbConfigRaw(t, nil)
	root := cfg.GetRoot()
	logger := slog.New(slog.DiscardHandler)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	require.NoError(t, RunMigrations(ctx, root, logger, migration.DirectionUp, nil, WithPostgresMigrationDB(rawDB)))

	// The migrator keeps rawDB's only connection checked out, so Postgres needs a separate runtime handle.
	var runtimeOptions []RuntimeOption
	if postgresCfg, ok := root.Database.InnerVal.(*config.DatabasePostgres); ok {
		runtimeDB, err := sql.Open("pgx", postgresCfg.GetDsn())
		require.NoError(t, err)
		defer runtimeDB.Close()
		runtimeOptions = append(runtimeOptions, WithPostgresDB(runtimeDB))
	}

	runtime, err := NewRuntime(root, nil, logger, runtimeOptions...)
	require.NoError(t, err)
	defer runtime.Close()

	// Register one workflow that finishes at once and one that stays in progress for the whole test.
	worker, err := NewWorker(runtime, &wfworker.DefaultOptions)
	require.NoError(t, err)
	finishing := func(wflib.Context) error { return nil }
	running := func(ctx wflib.Context) error { return wflib.Sleep(ctx, time.Hour) }
	require.NoError(t, worker.RegisterWorkflow(finishing))
	require.NoError(t, worker.RegisterWorkflow(running))

	workerCtx, stopWorker := context.WithCancel(ctx)
	defer func() {
		stopWorker()
		require.NoError(t, worker.WaitForCompletion())
	}()
	require.NoError(t, worker.Start(workerCtx))

	startWorkflow := func(instanceID string, workflow wflib.Workflow) *wflib.Instance {
		instance, err := runtime.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
			InstanceID: instanceID,
			Queue:      DefaultQueue,
		}, workflow)
		require.NoError(t, err)
		return instance
	}
	finished := startWorkflow("finished", finishing)
	inProgress := startWorkflow("in-progress", running)
	require.NoError(t, runtime.client.WaitForWorkflowInstance(ctx, finished, 10*time.Second))

	handler := NewTaskHandler(runtime, nil, logger)
	runAt := func(at time.Time) {
		clockCtx := apctx.NewBuilder(ctx).WithClock(clock.NewFakeClock(at)).Build()
		require.NoError(t, handler.removeFinishedInstances(clockCtx, asynq.NewTask(taskTypeRemoveFinishedInstances, nil)))
	}
	exists := func(instance *wflib.Instance) bool {
		_, err := runtime.GetWorkflowInstanceState(ctx, instance)
		if errors.Is(err, wfbackend.ErrInstanceNotFound) {
			return false
		}
		require.NoError(t, err)
		return true
	}

	// Inside the retention window the finished instance is kept.
	runAt(time.Now().Add(29 * 24 * time.Hour))
	require.True(t, exists(finished))

	// Past the window it is removed along with its history, but the running workflow is untouched.
	runAt(time.Now().Add(31 * 24 * time.Hour))
	require.False(t, exists(finished))
	require.True(t, exists(inProgress))
}
