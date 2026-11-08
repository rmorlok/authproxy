package runtime

import (
	"reflect"

	"github.com/dop251/goja"
)

// PromiseTracker counts unhandled rejections in one fresh runtime. Install it
// before authored initialization so Goja's once-only rejection and first-handler
// notifications stay paired. During the initial execute call it also records
// identities until the returned root is known; afterward it retains only that
// root, allowing result interpretation without hiding unrelated failures.
// Every method and notification runs on the VM's owning goroutine. This is a
// low-level host utility: install exactly one tracker before authored code and
// close it before discarding the VM. No result may outlive its owning runtime.
type PromiseTracker struct {
	// vm owns the rejection hook, which is removed when evaluation ends.
	vm *goja.Runtime

	// unhandled counts all rejected promises that have not acquired a handler.
	// Counting does not keep detached promises or their private reasons alive.
	unhandled int

	// initialUnhandled temporarily records identities while callAsync executes
	// the entry point and drains its microtasks. The root may reject and acquire
	// a handler before that call returns, so its state alone cannot identify
	// whether it contributes to unhandled. The map is released on every exit.
	initialUnhandled map[*goja.Promise]struct{}

	// root is the native Promise returned by the entry point. Once known, it is
	// the only identity the tracker retains, until Close releases it.
	root *goja.Promise

	// rootUnhandled records whether the exact root currently contributes to
	// unhandled. A handled rejected root does not exempt any other rejection.
	rootUnhandled bool
}

// NewPromiseTracker installs tracking before any authored code runs. Callers
// supply a non-nil fresh runtime and defer Close, including on initialization
// failure. It replaces the VM's existing rejection hook rather than chaining it.
func NewPromiseTracker(vm *goja.Runtime) *PromiseTracker {
	tracker := &PromiseTracker{vm: vm}
	vm.SetPromiseRejectionTracker(tracker.trackRejection)
	return tracker
}

// trackRejection counts transitions without inspecting private reasons. Only
// events during the initial call need temporary identities; later events update
// the known root's state and the aggregate count without retaining other promises.
func (t *PromiseTracker) trackRejection(
	promise *goja.Promise,
	operation goja.PromiseRejectionOperation,
) {
	switch operation {
	case goja.PromiseRejectionHandle:
		t.unhandled--
		delete(t.initialUnhandled, promise)
		if promise == t.root {
			t.rootUnhandled = false
		}
	case goja.PromiseRejectionReject:
		t.unhandled++
		if t.initialUnhandled != nil {
			t.initialUnhandled[promise] = struct{}{}
		}
		if promise == t.root {
			t.rootUnhandled = true
		}
	}
}

// callAsync invokes the entry point once, requires a native Promise, and captures
// the root's rejection status before dropping all temporary identities. The
// caller checks initialization rejections first, so any already-rejected root
// returned here must have rejected during this call or already have a handler.
// Checking ExportType avoids traversing arbitrary getters or exporting an
// unchecked object graph when authored initialization replaces the entry point.
func (t *PromiseTracker) callAsync(
	execute goja.Callable,
	params, contextValue goja.Value,
) (*goja.Promise, error) {
	t.initialUnhandled = make(map[*goja.Promise]struct{})
	defer func() { t.initialUnhandled = nil }()

	value, err := execute(goja.Undefined(), params, contextValue)
	if err != nil {
		return nil, EvaluationError(err)
	}

	if value == nil || value.ExportType() != reflect.TypeFor[*goja.Promise]() {
		return nil, ErrEvaluation
	}

	t.root = value.Export().(*goja.Promise)

	_, t.rootUnhandled = t.initialUnhandled[t.root]

	return t.root, nil
}

// CheckRejections rejects failures left unhandled after a VM call has drained
// its microtasks. A catch installed before this check clears its rejection.
func (t *PromiseTracker) CheckRejections() error {
	if t.unhandled != 0 {
		return ErrEvaluation
	}
	return nil
}

// checkRootRejections permits only the exact root captured by callAsync to
// reach a trusted interpreter. If it has acquired a handler, it no longer
// contributes to the count and exempts nothing. Later detached rejections and
// handlers still update the count without retaining those promises.
func (t *PromiseTracker) checkRootRejections() error {
	allowed := 0

	if t.rootUnhandled {
		allowed = 1
	}

	if t.unhandled != allowed {
		return ErrEvaluation
	}

	return nil
}

// Close removes the hook and releases retained identities before VM disposal.
func (t *PromiseTracker) Close() {
	t.vm.SetPromiseRejectionTracker(nil)
	t.initialUnhandled = nil
	t.root = nil
}
