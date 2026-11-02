package javascript

import "github.com/dop251/goja"

// promiseTracker counts unhandled rejections in one fresh runtime. Install it
// before authored initialization so Goja's once-only rejection and first-handler
// notifications stay paired. Counting avoids retaining private rejection values.
// Every method and notification runs on the VM's owning goroutine.
type promiseTracker struct {
	// vm owns the rejection hook, which is removed when evaluation ends.
	vm *goja.Runtime

	// unhandled counts rejected promises that have not acquired a handler.
	unhandled int
}

// newPromiseTracker installs tracking before any authored code runs. Callers
// supply a fresh runtime and defer close, including on initialization failure.
func newPromiseTracker(vm *goja.Runtime) *promiseTracker {
	tracker := &promiseTracker{vm: vm}
	vm.SetPromiseRejectionTracker(tracker.trackRejection)
	return tracker
}

// trackRejection records rejection state without inspecting private reasons.
func (t *promiseTracker) trackRejection(_ *goja.Promise, operation goja.PromiseRejectionOperation) {
	switch operation {
	case goja.PromiseRejectionHandle:
		t.unhandled--
	case goja.PromiseRejectionReject:
		t.unhandled++
	}
}

// checkRejections rejects failures left unhandled after a VM call has drained
// its microtasks. A catch installed before this check clears its rejection.
func (t *promiseTracker) checkRejections() error {
	if t.unhandled != 0 {
		return ErrEvaluation
	}
	return nil
}

// settledResult returns a fulfilled native Promise's value after Goja drains
// its microtasks. This host supplies no external work, so a still-pending root
// cannot progress. Rejected roots and unhandled detached failures are errors.
func (t *promiseTracker) settledResult(root *goja.Promise) (goja.Value, error) {
	if root.State() != goja.PromiseStateFulfilled {
		return nil, ErrEvaluation
	}
	if err := t.checkRejections(); err != nil {
		return nil, err
	}
	return root.Result(), nil
}

// close removes the hook before the owner discards the runtime.
func (t *promiseTracker) close() {
	t.vm.SetPromiseRejectionTracker(nil)
}
