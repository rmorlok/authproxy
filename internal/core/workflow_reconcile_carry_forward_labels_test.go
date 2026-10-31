package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cschleiden/go-workflows/backend"
	"github.com/cschleiden/go-workflows/client"
	"github.com/cschleiden/go-workflows/registry"
	"github.com/cschleiden/go-workflows/tester"
	wfworker "github.com/cschleiden/go-workflows/worker"
	wflib "github.com/cschleiden/go-workflows/workflow"
	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	mockLog "github.com/rmorlok/authproxy/internal/aplog/mock"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/migration"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// defaultRateLimiterMatcher matches a *rate.Limiter at the sweep's default rate.
type defaultRateLimiterMatcher struct{}

func (defaultRateLimiterMatcher) Matches(x any) bool {
	limiter, ok := x.(*rate.Limiter)
	return ok && limiter.Limit() == reconcileRecordsPerSecond
}

func (defaultRateLimiterMatcher) String() string { return "is *rate.Limiter at default rps" }

func TestReconcileCarryForwardLabelsWorkflowV1(t *testing.T) {
	sliceActivity := func(context.Context, reconcileCarryForwardLabelsPositionV1) (reconcileCarryForwardLabelsSliceResultV1, error) {
		return reconcileCarryForwardLabelsSliceResultV1{}, nil
	}
	continued := reconcileCarryForwardLabelsPositionV1{Step: "keys", AfterKey: "k_1"}

	t.Run("passes each slice's position to the next until done", func(t *testing.T) {
		workflowTester := tester.NewWorkflowTester[any](reconcileCarryForwardLabelsWorkflowV1)

		// The first slice stops partway; the second continues from there and finishes.
		first := workflowTester.
			OnActivityByName(ActivityNameReconcileCarryForwardLabelsSliceV1, sliceActivity, testifymock.Anything, reconcileCarryForwardLabelsPositionV1{}).
			Return(reconcileCarryForwardLabelsSliceResultV1{Next: continued}, nil).
			Once()
		workflowTester.
			OnActivityByName(ActivityNameReconcileCarryForwardLabelsSliceV1, sliceActivity, testifymock.Anything, continued).
			Return(reconcileCarryForwardLabelsSliceResultV1{Done: true}, nil).
			Once().
			NotBefore(first)

		workflowTester.Execute(context.Background(), reconcileCarryForwardLabelsPositionV1{})

		// The workflow finishes cleanly once a slice reports the sweep is done.
		require.True(t, workflowTester.WorkflowFinished())
		_, workflowErr := workflowTester.WorkflowResult()
		require.Nil(t, workflowErr)
		workflowTester.AssertExpectations(t)
	})

	t.Run("fails once retries run out", func(t *testing.T) {
		workflowTester := tester.NewWorkflowTester[any](reconcileCarryForwardLabelsWorkflowV1)

		// Every attempt fails, as during an outage outlasting the retry window.
		workflowTester.
			OnActivityByName(ActivityNameReconcileCarryForwardLabelsSliceV1, sliceActivity, testifymock.Anything, reconcileCarryForwardLabelsPositionV1{}).
			Return(reconcileCarryForwardLabelsSliceResultV1{}, errors.New("database unavailable"))

		workflowTester.Execute(context.Background(), reconcileCarryForwardLabelsPositionV1{})

		// The sweep gives up after the configured attempts rather than spinning forever.
		require.True(t, workflowTester.WorkflowFinished())
		_, workflowErr := workflowTester.WorkflowResult()
		require.NotNil(t, workflowErr)
		require.Contains(t, workflowErr.Error(), "database unavailable")
	})
}

func TestRegisterReconcileCarryForwardLabelsWorkflowV1DurableNames(t *testing.T) {
	reg := registry.New()
	svc := &service{}

	require.NoError(t, svc.registerReconcileCarryForwardLabelsWorkflow(reg))

	// These names are stored in workflow history, so they must never change.
	require.Equal(t, "core.labels.reconcile_carry_forward.v1", WorkflowNameReconcileCarryForwardLabelsV1)
	_, err := reg.GetWorkflow(WorkflowNameReconcileCarryForwardLabelsV1)
	require.NoError(t, err)

	require.Equal(t, "core.labels.reconcile_carry_forward.reconcile_slice.v1", ActivityNameReconcileCarryForwardLabelsSliceV1)
	_, err = reg.GetActivity(ActivityNameReconcileCarryForwardLabelsSliceV1)
	require.NoError(t, err)
}

func TestStartReconcileCarryForwardLabels(t *testing.T) {
	setup := func(t *testing.T, startErr error) (*service, *fakeDisconnectWorkflowClient) {
		logger, _ := mockLog.NewTestLogger(t)
		wc := &fakeDisconnectWorkflowClient{instance: &wflib.Instance{InstanceID: "instance"}, err: startErr}
		return &service{wc: wc, logger: logger}, wc
	}
	task := newReconcileCarryForwardLabelsTask()

	t.Run("starts the singleton workflow from the beginning", func(t *testing.T) {
		svc, wc := setup(t, nil)

		require.NoError(t, svc.startReconcileCarryForwardLabels(context.Background(), task))

		// A fixed instance ID is what keeps two sweeps from running together.
		require.Equal(t, reconcileCarryForwardLabelsInstanceID, wc.options.InstanceID)
		require.Equal(t, apworkflows.DefaultQueue, wc.options.Queue)
		require.Equal(t, WorkflowNameReconcileCarryForwardLabelsV1, wc.workflow)
		require.Equal(t, []any{reconcileCarryForwardLabelsPositionV1{}}, wc.args)
	})

	t.Run("a sweep already running is not an error", func(t *testing.T) {
		// Fleets too large for one day hit this on every daily start.
		svc, _ := setup(t, fmt.Errorf("creating instance: %w", backend.ErrInstanceAlreadyExists))
		require.NoError(t, svc.startReconcileCarryForwardLabels(context.Background(), task))
	})

	t.Run("other start failures are returned so asynq retries", func(t *testing.T) {
		startErr := errors.New("workflow backend unavailable")
		svc, _ := setup(t, startErr)
		require.ErrorIs(t, svc.startReconcileCarryForwardLabels(context.Background(), task), startErr)
	})
}

func TestReconcileCarryForwardLabelsSliceActivity(t *testing.T) {
	position := reconcileCarryForwardLabelsPositionV1{Step: "keys", AfterKey: "k_1"}

	t.Run("passes the position through the rate limited slice", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		db := mockDb.NewMockDB(ctrl)
		logger, _ := mockLog.NewTestLogger(t)
		svc := &service{db: db, logger: logger}

		// The activity translates the durable position both ways.
		db.EXPECT().
			ReconcileCarryForwardLabelsSlice(
				gomock.Any(),
				database.ReconcilePosition{Step: "keys", AfterKey: "k_1"},
				reconcileSliceRows,
				reconcileBatchSize,
				defaultRateLimiterMatcher{},
			).
			Return(database.ReconcileSlice{Next: database.ReconcilePosition{Step: "tools"}}, nil)

		result, err := svc.reconcileCarryForwardLabelsSliceV1(context.Background(), position)
		require.NoError(t, err)
		require.Equal(t, reconcileCarryForwardLabelsPositionV1{Step: "tools"}, result.Next)
		require.False(t, result.Done)
	})

	t.Run("returns database errors so the workflow retries the slice", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		db := mockDb.NewMockDB(ctrl)
		logger, _ := mockLog.NewTestLogger(t)
		svc := &service{db: db, logger: logger}

		dbErr := errors.New("transient")
		db.EXPECT().
			ReconcileCarryForwardLabelsSlice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(database.ReconcileSlice{}, dbErr)

		_, err := svc.reconcileCarryForwardLabelsSliceV1(context.Background(), position)
		require.ErrorIs(t, err, dbErr)
	})
}

func TestNewReconcileCarryForwardLabelsTask(t *testing.T) {
	// The task keeps its old type so that one queued across an upgrade is handled.
	require.Equal(t, "database:reconcile_carry_forward_labels", newReconcileCarryForwardLabelsTask().Type())
}

// TestReconcileCarryForwardLabelsOnRealBackend runs the sweep on the configured
// provider's real go-workflows backend. The tester stands in for the backend's
// instance handling, so only this test covers ContinueAsNew and singleton starts.
func TestReconcileCarryForwardLabelsOnRealBackend(t *testing.T) {
	cfg, db, rawDB := database.MustApplyBlankTestDbConfigRaw(t, nil)
	root := cfg.GetRoot()
	logger := slog.New(slog.DiscardHandler)

	// Workflow pollers, activities and the database layer all share this pool, so
	// the test harness's single connection would deadlock them.
	rawDB.SetMaxOpenConns(10)

	require.NoError(t, apworkflows.RunMigrations(
		context.Background(),
		root,
		logger,
		migration.DirectionUp,
		nil,
		apworkflows.WithPostgresMigrationDB(rawDB),
	))
	runtime, err := apworkflows.NewRuntime(root, nil, logger, apworkflows.WithPostgresDB(rawDB))
	require.NoError(t, err)
	defer runtime.Close()

	// Seed enough drifted rows that the sweep spans several executions.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for i := 0; i < 12; i++ {
		require.NoError(t, db.CreateKey(ctx, &database.Key{
			Id: apid.New(apid.PrefixKey), Namespace: "root", State: database.KeyStateActive,
		}))
	}
	_, err = rawDB.Exec(`UPDATE keys SET labels = '{"apxy/stale":"1"}'`)
	require.NoError(t, err)

	svc := &service{db: db, wc: runtime, logger: logger}

	// Each execution runs one small slice, so every slice crosses a ContinueAsNew.
	// The second slice waits, to hold the sweep open in a continued execution.
	var slices atomic.Int32
	secondSlice := make(chan struct{})
	release := make(chan struct{})
	worker, err := apworkflows.NewWorker(runtime, &wfworker.DefaultOptions)
	require.NoError(t, err)
	require.NoError(t, worker.RegisterWorkflow(
		func(ctx wflib.Context, position reconcileCarryForwardLabelsPositionV1) error {
			return runReconcileCarryForwardLabels(ctx, position, 1)
		},
		registry.WithName(WorkflowNameReconcileCarryForwardLabelsV1),
	))
	require.NoError(t, worker.RegisterActivity(
		func(ctx context.Context, position reconcileCarryForwardLabelsPositionV1) (reconcileCarryForwardLabelsSliceResultV1, error) {
			if slices.Add(1) == 2 {
				close(secondSlice)
				<-release
			}
			slice, err := db.ReconcileCarryForwardLabelsSlice(
				ctx,
				database.ReconcilePosition{Step: position.Step, AfterKey: position.AfterKey},
				5, // maxRows
				5, // batchSize
				nil,
			)
			if err != nil {
				return reconcileCarryForwardLabelsSliceResultV1{}, err
			}
			return reconcileCarryForwardLabelsSliceResultV1{
				Next: reconcileCarryForwardLabelsPositionV1{Step: slice.Next.Step, AfterKey: slice.Next.AfterKey},
				Done: slice.Done,
			}, nil
		},
		registry.WithName(ActivityNameReconcileCarryForwardLabelsSliceV1),
	))
	workerCtx, stopWorker := context.WithCancel(ctx)
	defer stopWorker()
	require.NoError(t, worker.Start(workerCtx))
	defer func() {
		stopWorker()
		require.NoError(t, worker.WaitForCompletion())
	}()

	task := newReconcileCarryForwardLabelsTask()
	require.NoError(t, svc.startReconcileCarryForwardLabels(ctx, task))

	// startDirectly surfaces what the daily task would otherwise treat as a no-op.
	startDirectly := func() error {
		_, err := runtime.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
			InstanceID: reconcileCarryForwardLabelsInstanceID,
			Queue:      apworkflows.DefaultQueue,
		}, WorkflowNameReconcileCarryForwardLabelsV1, reconcileCarryForwardLabelsPositionV1{})
		return err
	}

	// Having reached slice two, the sweep is in a continued execution, and the
	// singleton instance must still refuse a second sweep.
	select {
	case <-secondSlice:
	case <-ctx.Done():
		t.Fatal("sweep never reached its second slice")
	}
	require.ErrorIs(t, startDirectly(), backend.ErrInstanceAlreadyExists)
	require.NoError(t, svc.startReconcileCarryForwardLabels(ctx, task))

	// Once released, the sweep runs through every remaining execution and ends,
	// which frees the instance ID for the next day's start.
	close(release)
	require.Eventually(t, func() bool {
		err := startDirectly()
		return err == nil
	}, 30*time.Second, 50*time.Millisecond)

	// Every drifted row was repaired across the slices.
	var stale int
	require.NoError(t, rawDB.QueryRow(`SELECT COUNT(*) FROM keys WHERE CAST(labels AS text) LIKE '%apxy/stale%'`).Scan(&stale))
	require.Zero(t, stale)
	require.GreaterOrEqual(t, slices.Load(), int32(3))
}
