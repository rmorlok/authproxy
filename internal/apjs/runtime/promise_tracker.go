package runtime

import "github.com/dop251/goja"

// PromiseTracker counts unhandled rejections in one fresh runtime. Install it
// before authored initialization so Goja's once-only rejection and first-handler
// notifications stay paired. Counting avoids retaining private rejection values.
// Every method and notification runs on the VM's owning goroutine. This is a
// low-level host utility: install exactly one tracker before authored code and
// close it before discarding the VM. No result may outlive its owning runtime.
type PromiseTracker struct {
	// vm owns the rejection hook, which is removed when evaluation ends.
	vm *goja.Runtime

	// unhandled counts rejected promises that have not acquired a handler.
	unhandled int
}

// NewPromiseTracker installs tracking before any authored code runs. Callers
// supply a non-nil fresh runtime and defer Close, including on initialization
// failure. It replaces the VM's existing rejection hook rather than chaining it.
func NewPromiseTracker(vm *goja.Runtime) *PromiseTracker {
	tracker := &PromiseTracker{vm: vm}
	vm.SetPromiseRejectionTracker(tracker.trackRejection)
	return tracker
}

// trackRejection records rejection state without inspecting private reasons.
func (t *PromiseTracker) trackRejection(
	_ *goja.Promise,
	operation goja.PromiseRejectionOperation,
) {
	switch operation {
	case goja.PromiseRejectionHandle:
		t.unhandled--
	case goja.PromiseRejectionReject:
		t.unhandled++
	}
}

// CheckRejections rejects failures left unhandled after a VM call has drained
// its microtasks. A catch installed before this check clears its rejection.
func (t *PromiseTracker) CheckRejections() error {
	if t.unhandled != 0 {
		return ErrEvaluation
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
}
