package runtime

import "github.com/dop251/goja"

// PromiseTracker records unhandled rejections in one fresh runtime. Install it
// before authored initialization so Goja's once-only rejection and first-handler
// notifications stay paired. Identity tracking allows result extensions to
// interpret precisely the root rejection without hiding unrelated failures.
// Every method and notification runs on the VM's owning goroutine. This is a
// low-level host utility: install exactly one tracker before authored code and
// close it before discarding the VM. No result may outlive its owning runtime.
type PromiseTracker struct {
	// vm owns the rejection hook, which is removed when evaluation ends.
	vm *goja.Runtime

	// unhandled holds rejected promises that have not acquired a handler. Their
	// reasons are never inspected by the tracker; Close releases all identities.
	unhandled map[*goja.Promise]struct{}
}

// NewPromiseTracker installs tracking before any authored code runs. Callers
// supply a non-nil fresh runtime and defer Close, including on initialization
// failure. It replaces the VM's existing rejection hook rather than chaining it.
func NewPromiseTracker(vm *goja.Runtime) *PromiseTracker {
	tracker := &PromiseTracker{vm: vm, unhandled: make(map[*goja.Promise]struct{})}
	vm.SetPromiseRejectionTracker(tracker.trackRejection)
	return tracker
}

// trackRejection records rejection state without inspecting private reasons.
func (t *PromiseTracker) trackRejection(
	promise *goja.Promise,
	operation goja.PromiseRejectionOperation,
) {
	switch operation {
	case goja.PromiseRejectionHandle:
		delete(t.unhandled, promise)
	case goja.PromiseRejectionReject:
		t.unhandled[promise] = struct{}{}
	}
}

// CheckRejections rejects failures left unhandled after a VM call has drained
// its microtasks. A catch installed before this check clears its rejection.
func (t *PromiseTracker) CheckRejections() error {
	if len(t.unhandled) != 0 {
		return ErrEvaluation
	}
	return nil
}

// checkRejectionsExcept permits only the exact root to reach a trusted result
// interpreter. A root may already have a handler; skipping its identity then
// exempts nothing, so an unrelated rejection cannot be mistaken for the root.
func (t *PromiseTracker) checkRejectionsExcept(root *goja.Promise) error {
	for promise := range t.unhandled {
		if promise != root {
			return ErrEvaluation
		}
	}
	return nil
}

// SettledResult returns a fulfilled native Promise's value after Goja drains
// its microtasks. The caller supplies a non-nil root from this VM after driving
// host work to settlement or deciding that no further progress is possible.
// Pending/rejected roots and unhandled detached failures are errors. The result
// belongs to this VM and must be consumed before its owner ends execution.
func (t *PromiseTracker) SettledResult(root *goja.Promise) (goja.Value, error) {
	if root.State() != goja.PromiseStateFulfilled {
		return nil, ErrEvaluation
	}

	if err := t.CheckRejections(); err != nil {
		return nil, err
	}

	return root.Result(), nil
}

// Close removes the hook before the owner discards the runtime.
func (t *PromiseTracker) Close() {
	t.vm.SetPromiseRejectionTracker(nil)
	t.unhandled = nil
}
