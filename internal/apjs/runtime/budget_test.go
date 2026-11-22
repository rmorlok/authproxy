package runtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

// TestExecutionBudgetRequiresPositiveCaps rejects incomplete cumulative policy
// independently of the source, JSON-boundary, and timeout policy Run validates.
func TestExecutionBudgetRequiresPositiveCaps(t *testing.T) {
	for _, test := range []struct {
		name             string
		work, allocation int
	}{
		{"zero work", 0, 1},
		{"negative work", -1, 1},
		{"zero allocation", 1, 0},
		{"negative allocation", 1, -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			limits := outputTestLimits()
			limits.MaxWorkUnits, limits.MaxAllocationUnits = test.work, test.allocation
			// Neither a standalone ledger nor a full execution accepts an absent cap.
			ctx, err := WithExecutionBudget(context.Background(), limits)
			require.ErrorIs(t, err, ErrInvalidInput)
			require.Nil(t, ctx)
			require.ErrorIs(t, limits.Validate(), ErrInvalidInput)
		})
	}

	// Adapters can establish their outer ledger before selecting other boundaries.
	limits := Limits{MaxWorkUnits: 1, MaxAllocationUnits: 1}
	ctx, err := WithExecutionBudget(context.Background(), limits)
	require.NoError(t, err)
	require.NoError(t, ChargeWork(ctx, 1))
	require.NoError(t, ReserveAllocation(ctx, 1))
	ctx, err = WithExecutionBudget(nil, limits)
	require.ErrorIs(t, err, ErrInvalidInput)
	require.Nil(t, ctx)
}

// TestExecutionBudgetCumulativeCharges checks exact allowance boundaries and
// overflow-safe arithmetic without treating rejected negative input as spending.
func TestExecutionBudgetCumulativeCharges(t *testing.T) {
	for _, test := range []struct {
		name   string
		charge func(context.Context, int) error
	}{
		{"work", ChargeWork},
		{"allocation", ReserveAllocation},
	} {
		t.Run(test.name, func(t *testing.T) {
			maximum := int(^uint(0) >> 1)
			ctx, err := WithExecutionBudget(context.Background(), Limits{
				MaxWorkUnits: maximum, MaxAllocationUnits: maximum,
			})
			require.NoError(t, err)

			// Invalid input neither refunds usage nor poisons a valid execution.
			require.ErrorIs(t, test.charge(ctx, -1), ErrInvalidInput)
			require.NoError(t, test.charge(ctx, maximum-1))
			require.NoError(t, test.charge(ctx, 1))
			require.NoError(t, test.charge(ctx, 0))

			// A further unit fails without overflowing, then all phases see failure.
			require.ErrorIs(t, test.charge(ctx, 1), ErrLimitExceeded)
			require.ErrorIs(t, ChargeWork(ctx, 0), ErrLimitExceeded)
			require.ErrorIs(t, ReserveAllocation(ctx, 0), ErrLimitExceeded)
			select {
			case <-executionBudgetDone(ctx):
			default:
				t.Fatal("exhaustion must signal active executions")
			}
		})
	}
}

// TestExecutionBudgetNestedScopesCannotResetParent shares the parent's remaining
// allowance across enlarged children, grandchildren, and later sibling scopes.
func TestExecutionBudgetNestedScopesCannotResetParent(t *testing.T) {
	for _, test := range []struct {
		name   string
		charge func(context.Context, int) error
	}{
		{"work", ChargeWork},
		{"allocation", ReserveAllocation},
	} {
		t.Run(test.name, func(t *testing.T) {
			parent, err := WithExecutionBudget(context.Background(), Limits{MaxWorkUnits: 6, MaxAllocationUnits: 6})
			require.NoError(t, err)
			require.NoError(t, test.charge(parent, 3))

			// Larger local ceilings still debit the original execution.
			larger := Limits{MaxWorkUnits: 100, MaxAllocationUnits: 100}
			child, err := WithExecutionBudget(parent, larger)
			require.NoError(t, err)
			require.NoError(t, test.charge(child, 1))
			grandchild, err := WithExecutionBudget(child, larger)
			require.NoError(t, err)
			require.NoError(t, test.charge(grandchild, 1))
			sibling, err := WithExecutionBudget(parent, larger)
			require.NoError(t, err)
			require.NoError(t, test.charge(sibling, 1))

			// Opening another scope cannot replenish the six units already spent.
			require.ErrorIs(t, test.charge(grandchild, 1), ErrLimitExceeded)
			require.ErrorIs(t, test.charge(sibling, 0), ErrLimitExceeded)
			fresh, err := WithExecutionBudget(parent, larger)
			require.ErrorIs(t, err, ErrLimitExceeded)
			require.Nil(t, fresh)
		})
	}
}

// TestExecutionBudgetStrictChildPoisonsRun prevents an adapter from ignoring a
// smaller child's failure and publishing a result through its parent or sibling.
func TestExecutionBudgetStrictChildPoisonsRun(t *testing.T) {
	limits := outputTestLimits()
	err := Run(context.Background(), limits, func(ctx context.Context, _ *goja.Runtime) error {
		child, err := WithExecutionBudget(ctx, Limits{MaxWorkUnits: 1, MaxAllocationUnits: 1})
		if err != nil {
			return err
		}
		sibling, err := WithExecutionBudget(ctx, limits)
		if err != nil {
			return err
		}

		// Deliberately discard the refusal as a fallible trusted adapter might.
		_ = ReserveAllocation(child, 2)
		require.ErrorIs(t, ChargeWork(sibling, 0), ErrLimitExceeded)
		return nil
	})
	// Run's terminal check must override the callback's apparent success.
	require.ErrorIs(t, err, ErrLimitExceeded)
}

// TestExecutionBudgetCancellationTakesPrecedence keeps caller cancellation
// authoritative even after a native operation has exhausted the allowance.
func TestExecutionBudgetCancellationTakesPrecedence(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, err := WithExecutionBudget(parent, Limits{MaxWorkUnits: 1, MaxAllocationUnits: 1})
	require.NoError(t, err)
	require.ErrorIs(t, ChargeWork(ctx, 2), ErrLimitExceeded)
	cancel()

	// Every public entry point preserves the canceled caller's classification.
	require.ErrorIs(t, ChargeWork(ctx, 0), context.Canceled)
	require.ErrorIs(t, ReserveAllocation(ctx, 0), context.Canceled)
	child, err := WithExecutionBudget(ctx, Limits{MaxWorkUnits: 1, MaxAllocationUnits: 1})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, child)

	// The runtime applies the same precedence during its final cleanup.
	parent, cancel = context.WithCancel(context.Background())
	defer cancel()
	limits := outputTestLimits()
	err = Run(parent, limits, func(ctx context.Context, _ *goja.Runtime) error {
		_ = ChargeWork(ctx, limits.MaxWorkUnits+1)
		cancel()
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)

	// An already-expired caller deadline is also preserved before VM creation.
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancelExpired()
	_, err = WithExecutionBudget(expired, limits)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, ChargeWork(expired, 0), context.DeadlineExceeded)
}

// TestExecutionBudgetConcurrentConsumption admits exactly the shared remaining
// allowance when independent worker contexts race to spend their parent budget.
func TestExecutionBudgetConcurrentConsumption(t *testing.T) {
	for _, test := range []struct {
		name   string
		charge func(context.Context, int) error
	}{
		{"work", ChargeWork},
		{"allocation", ReserveAllocation},
	} {
		t.Run(test.name, func(t *testing.T) {
			const capacity, workers, attempts = 127, 16, 32
			parent, err := WithExecutionBudget(context.Background(), Limits{MaxWorkUnits: capacity, MaxAllocationUnits: capacity})
			require.NoError(t, err)
			start := make(chan struct{})
			var joined sync.WaitGroup
			var accepted, unexpected atomic.Int64
			children := make([]*executionBudget, 0, workers)

			// Each worker has a distinct local ceiling but shares the same parent.
			for range workers {
				child, err := WithExecutionBudget(parent, Limits{MaxWorkUnits: capacity, MaxAllocationUnits: capacity})
				require.NoError(t, err)
				children = append(children, executionBudgetFromContext(child))
				joined.Add(1)
				go func() {
					defer joined.Done()
					<-start
					for range attempts {
						err := test.charge(child, 1)
						if err == nil {
							accepted.Add(1)
						} else if !errors.Is(err, ErrLimitExceeded) {
							unexpected.Add(1)
						}
					}
				}()
			}
			close(start)
			joined.Wait()

			// No failed charge partially spends an ancestor or permits overspending.
			require.Equal(t, int64(capacity), accepted.Load())
			require.Zero(t, unexpected.Load())
			require.ErrorIs(t, test.charge(parent, 0), ErrLimitExceeded)
			ledger := executionBudgetFromContext(parent)
			ledger.state.mu.Lock()
			parentUsage, childUsage := ledger.workUnits+ledger.allocationUnits, 0
			for _, child := range children {
				childUsage += child.workUnits + child.allocationUnits
			}
			ledger.state.mu.Unlock()
			require.Equal(t, capacity, parentUsage)
			require.Equal(t, parentUsage, childUsage)
		})
	}
}

// TestExecutionBudgetWorkerFailureInterruptsVM checks the budget watcher rather
// than waiting for the policy deadline to stop JavaScript that makes no helpers.
func TestExecutionBudgetWorkerFailureInterruptsVM(t *testing.T) {
	limits := outputTestLimits()
	limits.Timeout = 2 * time.Second
	workerResult := make(chan error, 1)
	var contextAfterInterrupt error
	err := Run(context.Background(), limits, func(ctx context.Context, vm *goja.Runtime) error {
		started := make(chan struct{})
		if err := vm.Set("started", func() { close(started) }); err != nil {
			return err
		}
		// The worker waits until authored execution starts, then exhausts its Go-only ledger.
		go func() {
			select {
			case <-started:
				workerResult <- ChargeWork(ctx, limits.MaxWorkUnits+1)
			case <-ctx.Done():
				workerResult <- ctx.Err()
			}
		}()
		_, err := vm.RunString(`started(); for (;;) {}`)
		contextAfterInterrupt = ctx.Err()
		return EvaluationError(err)
	})

	// An exhausted budget ends the busy VM while its execution deadline is live.
	require.ErrorIs(t, err, ErrLimitExceeded)
	require.ErrorIs(t, <-workerResult, ErrLimitExceeded)
	require.NoError(t, contextAfterInterrupt)
}

// TestExecutionBudgetIndependentRunsStartFresh keeps usage and terminal failure
// local to one execution when callers reuse an ordinary background context.
func TestExecutionBudgetIndependentRunsStartFresh(t *testing.T) {
	limits := outputTestLimits()
	limits.MaxWorkUnits, limits.MaxAllocationUnits = 1, 1
	for _, exhaust := range []bool{false, true, false} {
		// Reusing the caller context never reuses a completed Run's private ledger.
		err := Run(context.Background(), limits, func(ctx context.Context, _ *goja.Runtime) error {
			if err := ChargeWork(ctx, 1); err != nil {
				return err
			}
			if err := ReserveAllocation(ctx, 1); err != nil {
				return err
			}
			if exhaust {
				_ = ChargeWork(ctx, 1)
			}
			return nil
		})
		if exhaust {
			require.ErrorIs(t, err, ErrLimitExceeded)
		} else {
			require.NoError(t, err)
		}
	}
}

// TestExecutionBudgetStandaloneChargesAreNoops preserves standalone converters'
// explicit boundary limits without silently introducing a cumulative allowance.
func TestExecutionBudgetStandaloneChargesAreNoops(t *testing.T) {
	ctx := context.Background()
	maximum := int(^uint(0) >> 1)
	for range 2 {
		// Even repeated maximal admissions remain unlimited without a ledger.
		require.NoError(t, ChargeWork(ctx, maximum))
		require.NoError(t, ReserveAllocation(ctx, maximum))
	}
	require.Nil(t, executionBudgetDone(ctx))

	// Absence of a ledger does not suppress malformed arguments or cancellation.
	require.ErrorIs(t, ChargeWork(nil, 0), ErrInvalidInput)
	require.ErrorIs(t, ReserveAllocation(nil, 0), ErrInvalidInput)
	require.ErrorIs(t, ChargeWork(ctx, -1), ErrInvalidInput)
	require.ErrorIs(t, ReserveAllocation(ctx, -1), ErrInvalidInput)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, ChargeWork(canceled, 0), context.Canceled)
	require.ErrorIs(t, ReserveAllocation(canceled, 0), context.Canceled)
}
