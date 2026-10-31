package core

import (
	"context"
	"errors"
	"time"

	"github.com/cschleiden/go-workflows/backend"
	"github.com/cschleiden/go-workflows/client"
	"github.com/cschleiden/go-workflows/registry"
	wflib "github.com/cschleiden/go-workflows/workflow"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/database"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
	"golang.org/x/time/rate"
)

const (
	WorkflowNameReconcileCarryForwardLabelsV1 = "core.labels.reconcile_carry_forward.v1"

	ActivityNameReconcileCarryForwardLabelsSliceV1 = "core.labels.reconcile_carry_forward.reconcile_slice.v1"

	// taskTypeReconcileCarryForwardLabels keeps the name this task had when it ran
	// the sweep itself, so a task queued across an upgrade still has a handler.
	taskTypeReconcileCarryForwardLabels = "database:reconcile_carry_forward_labels"

	// reconcileCarryForwardLabelsInstanceID is fixed so that only one sweep runs at
	// a time: a start fails while the instance is active and succeeds once it ends.
	reconcileCarryForwardLabelsInstanceID = WorkflowNameReconcileCarryForwardLabelsV1
)

// Defaults for the sweep. Drift is rare under normal operation, so the rate limit
// is set conservatively to keep DB load negligible. At 100 records/sec a million
// rows take about 2.8 hours and a day covers about 8.6 million rows. Larger fleets
// simply take longer: the workflow runs until it has visited every row, and the
// daily start is a no-op until then.
const (
	reconcileBatchSize        int32      = 100
	reconcileRecordsPerSecond rate.Limit = 100
	reconcileBurst            int        = 100

	// reconcileSliceRows is the size of one activity, about 5 minutes of work. It
	// sets what a crash can repeat and how much workflow history a sweep writes.
	// AuthProxy does not purge history and a slice writes about 6 events, so 8
	// million rows write about 1,600 events at this size, against about 480,000
	// with one activity per page. Repeating a slice is cheap because rows without
	// drift are not rewritten.
	reconcileSliceRows = 30_000

	// reconcileSlicesPerExecution bounds one execution's history, and with it the
	// cost of replaying the workflow. Changing it changes the command sequence, so
	// it needs a new workflow version.
	reconcileSlicesPerExecution = 100
)

// reconcileSliceRetryOptions ride out an outage of about six hours. A sweep that
// exhausts them fails and the next daily start begins again from the first table,
// so these are deliberately more patient than go-workflows' default of 3 attempts.
var reconcileSliceRetryOptions = wflib.RetryOptions{
	MaxAttempts:        12,
	FirstRetryInterval: time.Minute,
	BackoffCoefficient: 2,
	MaxRetryInterval:   time.Hour,
}

// reconcileCarryForwardLabelsPositionV1 is where a sweep continues. It is the
// workflow input and activity input, so it is persisted in workflow history. The
// zero value starts a sweep.
type reconcileCarryForwardLabelsPositionV1 struct {
	Step     string `json:"step,omitempty"`     // Step names the sweep step to continue.
	AfterKey string `json:"afterKey,omitempty"` // AfterKey is the last key finished within Step.
}

// reconcileCarryForwardLabelsSliceResultV1 is the persisted result of one slice.
type reconcileCarryForwardLabelsSliceResultV1 struct {
	Next reconcileCarryForwardLabelsPositionV1 `json:"next"` // Next is where the following slice continues.
	Done bool                                  `json:"done"` // Done is true when the sweep has visited every table.
}

// newReconcileCarryForwardLabelsTask creates the daily task that starts a sweep.
func newReconcileCarryForwardLabelsTask() *asynq.Task {
	return asynq.NewTask(taskTypeReconcileCarryForwardLabels, nil, asynq.MaxRetry(3))
}

// startReconcileCarryForwardLabels starts the sweep workflow. A sweep that is still
// running keeps going and this start is skipped, which is expected on fleets too
// large to sweep in a day.
func (s *service) startReconcileCarryForwardLabels(ctx context.Context, _ *asynq.Task) error {
	_, err := s.wc.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: reconcileCarryForwardLabelsInstanceID,
		Queue:      apworkflows.DefaultQueue,
	}, WorkflowNameReconcileCarryForwardLabelsV1, reconcileCarryForwardLabelsPositionV1{})
	if errors.Is(err, backend.ErrInstanceAlreadyExists) {
		s.logger.Info("carry-forward labels reconciliation is already running")
		return nil
	}
	return err
}

// reconcileCarryForwardLabelsWorkflowV1 walks the whole sweep one slice at a time.
// Workflow history records each slice's position, so a restarted worker continues
// from the last finished slice.
func reconcileCarryForwardLabelsWorkflowV1(
	ctx wflib.Context,
	position reconcileCarryForwardLabelsPositionV1,
) error {
	return runReconcileCarryForwardLabels(ctx, position, reconcileSlicesPerExecution)
}

// runReconcileCarryForwardLabels runs up to slicesPerExecution slices, then starts
// a fresh execution of the same instance from the current position so that a long
// sweep's history stays small enough to replay.
func runReconcileCarryForwardLabels(
	ctx wflib.Context,
	position reconcileCarryForwardLabelsPositionV1,
	slicesPerExecution int,
) error {
	options := wflib.DefaultActivityOptions
	options.RetryOptions = reconcileSliceRetryOptions

	for i := 0; i < slicesPerExecution; i++ {
		result, err := wflib.ExecuteActivity[reconcileCarryForwardLabelsSliceResultV1](
			ctx,
			options,
			ActivityNameReconcileCarryForwardLabelsSliceV1,
			position,
		).Get(ctx)
		if err != nil {
			return err
		}
		if result.Done {
			return nil
		}
		position = result.Next
	}

	return wflib.ContinueAsNew(ctx, position)
}

// reconcileCarryForwardLabelsSliceV1 advances the sweep by one slice.
func (s *service) reconcileCarryForwardLabelsSliceV1(
	ctx context.Context,
	position reconcileCarryForwardLabelsPositionV1,
) (reconcileCarryForwardLabelsSliceResultV1, error) {
	return s.runReconcileCarryForwardLabelsSlice(ctx, position, reconcileSliceRows)
}

// runReconcileCarryForwardLabelsSlice advances the sweep by about maxRows rows. The
// limiter is per slice because the fixed instance ID allows one slice at a time.
func (s *service) runReconcileCarryForwardLabelsSlice(
	ctx context.Context,
	position reconcileCarryForwardLabelsPositionV1,
	maxRows int,
) (reconcileCarryForwardLabelsSliceResultV1, error) {
	limiter := rate.NewLimiter(reconcileRecordsPerSecond, reconcileBurst)
	slice, err := s.db.ReconcileCarryForwardLabelsSlice(
		ctx,
		database.ReconcilePosition{Step: position.Step, AfterKey: position.AfterKey},
		maxRows,
		reconcileBatchSize,
		limiter,
	)
	if err != nil {
		return reconcileCarryForwardLabelsSliceResultV1{}, err
	}

	s.logger.Info(
		"reconciled slice of carry-forward labels",
		"corrected", slice.Corrected,
		"next_step", slice.Next.Step,
		"done", slice.Done,
	)
	return reconcileCarryForwardLabelsSliceResultV1{
		Next: reconcileCarryForwardLabelsPositionV1{Step: slice.Next.Step, AfterKey: slice.Next.AfterKey},
		Done: slice.Done,
	}, nil
}

// registerReconcileCarryForwardLabelsWorkflow registers the sweep's durable names.
func (s *service) registerReconcileCarryForwardLabelsWorkflow(worker workflowRegistrar) error {
	if err := worker.RegisterWorkflow(
		reconcileCarryForwardLabelsWorkflowV1,
		registry.WithName(WorkflowNameReconcileCarryForwardLabelsV1),
	); err != nil {
		return err
	}
	return worker.RegisterActivity(
		s.reconcileCarryForwardLabelsSliceV1,
		registry.WithName(ActivityNameReconcileCarryForwardLabelsSliceV1),
	)
}
