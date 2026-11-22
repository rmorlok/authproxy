package runtime

import (
	"context"
	"sync"
)

// executionBudgetKey identifies the managed-work ledger inherited by execution
// callbacks and their descendants. Its value contains no VM, callable, or input.
type executionBudgetKey struct{}

// executionBudgetState coordinates a budget tree, including concurrent HTTP
// readers. Exhaustion is terminal for the whole tree. The signal wakes active
// runtimes without retaining their VMs or interrupt callbacks in a context.
type executionBudgetState struct {
	mu        sync.Mutex    // Protects all counters and exhaustion in this tree.
	exhausted bool          // Remains true after any cumulative allowance fails.
	done      chan struct{} // Closed exactly once, when exhausted becomes true.
}

// executionBudget adds a local ceiling to an execution's inherited allowance.
// A charge consumes every ancestor as well as this scope; entering a nested
// execution therefore cannot reset, replenish, or enlarge its parent's budget.
type executionBudget struct {
	state              *executionBudgetState // Shared synchronization and failure signal.
	parent             *executionBudget      // Optional enclosing execution's ledger.
	maxWorkUnits       int                   // Local cumulative managed-work ceiling.
	maxAllocationUnits int                   // Local cumulative managed-storage ceiling.
	workUnits          int                   // Work admitted in this scope and its descendants.
	allocationUnits    int                   // Storage admitted; releasing data does not refund it.
}

// WithExecutionBudget establishes managed-work accounting for an adapter that
// performs conversions before or after Run. It validates the two cumulative
// caps; Run also validates the execution's boundary, stack, and timeout limits.
// Reusing the returned context shares counters. Calling this again creates a
// child ceiling whose charges also consume the original remaining allowance.
// The context retains only Go counters and a failure signal, never a VM.
func WithExecutionBudget(ctx context.Context, limits Limits) (context.Context, error) {
	if ctx == nil || limits.MaxWorkUnits <= 0 || limits.MaxAllocationUnits <= 0 {
		return nil, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	parent := executionBudgetFromContext(ctx)
	var state *executionBudgetState
	if parent != nil {
		if err := parent.err(); err != nil {
			return nil, err
		}
		state = parent.state
	} else {
		state = &executionBudgetState{done: make(chan struct{})}
	}

	budget := &executionBudget{
		state:              state,
		parent:             parent,
		maxWorkUnits:       limits.MaxWorkUnits,
		maxAllocationUnits: limits.MaxAllocationUnits,
	}

	return context.WithValue(ctx, executionBudgetKey{}, budget), nil
}

// executionBudgetFromContext finds the enclosing ledger without creating an
// allowance. Standalone conversion APIs may have only their local boundary caps.
func executionBudgetFromContext(ctx context.Context) *executionBudget {
	if ctx == nil {
		return nil
	}

	budget, _ := ctx.Value(executionBudgetKey{}).(*executionBudget)

	return budget
}

// executionBudgetDone returns the shared failure signal, or a nil channel for
// standalone bounded operations. It carries no VM state into worker waits.
func executionBudgetDone(ctx context.Context) <-chan struct{} {
	if budget := executionBudgetFromContext(ctx); budget != nil {
		return budget.state.done
	}

	return nil
}

// ChargeWork admits managed native work before processing. A unit is a native
// operation entry, inspected byte/UTF-16 code unit, or visited value/metadata
// slot. It does not count JavaScript instructions. Execution callers must pass
// runCtx so all phases share the ledger; outside an execution this leaves the
// standalone converter's explicit boundary caps in control.
func ChargeWork(ctx context.Context, units int) error {
	return chargeExecution(ctx, units, 0)
}

// ReserveAllocation admits managed storage before allocation where its size is
// known: one unit per buffer byte, string UTF-16 code unit, or native value/tuple
// slot. These logical units bound storage by implementation-specific multiples;
// they are not heap bytes or a complete Goja allocation meter. Reservations are
// cumulative, including intermediate copies; discarded values do not refund it.
func ReserveAllocation(ctx context.Context, units int) error {
	return chargeExecution(ctx, 0, units)
}

// chargeExecution applies cancellation before consulting the inherited ledger.
func chargeExecution(ctx context.Context, work, allocation int) error {
	if ctx == nil || work < 0 || allocation < 0 {
		return ErrInvalidInput
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if budget := executionBudgetFromContext(ctx); budget != nil {
		return budget.charge(work, allocation)
	}

	return nil
}

// charge checks every ceiling before atomically consuming all ancestor ledgers.
// Subtraction prevents overflow. A refused charge poisons the entire execution,
// even if a caller ignores the returned error or JavaScript catches a rejection.
func (b *executionBudget) charge(work, allocation int) error {
	b.state.mu.Lock()
	defer b.state.mu.Unlock()

	if b.state.exhausted {
		return ErrLimitExceeded
	}

	for scope := b; scope != nil; scope = scope.parent {
		if work > scope.maxWorkUnits-scope.workUnits ||
			allocation > scope.maxAllocationUnits-scope.allocationUnits {
			b.exhaustLocked()
			return ErrLimitExceeded
		}
	}

	for scope := b; scope != nil; scope = scope.parent {
		scope.workUnits += work
		scope.allocationUnits += allocation
	}

	return nil
}

// exhaust records an arithmetic or native limit failure using the same terminal
// state as refused cumulative charges. Only runtime-owned callers use it.
func (b *executionBudget) exhaust() {
	b.state.mu.Lock()
	defer b.state.mu.Unlock()
	b.exhaustLocked()
}

// exhaustLocked closes the failure signal once while the tree mutex is held.
func (b *executionBudget) exhaustLocked() {
	if !b.state.exhausted {
		b.state.exhausted = true
		close(b.state.done)
	}
}

// err reports terminal exhaustion without retaining or revealing private data.
func (b *executionBudget) err() error {
	b.state.mu.Lock()
	defer b.state.mu.Unlock()
	if b.state.exhausted {
		return ErrLimitExceeded
	}
	return nil
}
