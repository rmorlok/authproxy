package javascript

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"

	"github.com/dop251/goja"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
)

// fetchCompletion crosses the worker boundary using only owned Go data. A
// response body must either transfer to the owner or be closed by the worker.
type fetchCompletion struct {
	id       uint64             // Owner's lookup key; no VM callback crosses threads.
	response *httphost.Response // Successful fetch; metadata and body transfer together.
	body     []byte             // Successful bounded read; worker never touches it again.
	err      error              // Fixed host classification; no upstream diagnostics.
}

// fetchLoop owns Promise settlement and response lifetimes on one VM goroutine.
// Workers receive no pointer to this object, so a stalled transport cannot retain
// the VM through its completion callback. Only the unbuffered channel is shared.
type fetchLoop struct {
	ctx       context.Context                        // Invocation runtime deadline plus host cleanup cancellation.
	cancel    context.CancelFunc                     // Stops requests and body reads on every exit.
	vm        *goja.Runtime                          // Accessed exclusively by the owner.
	options   FetchOptions                           // Captured session, admission limits, and diagnostics logger.
	limits    Limits                                 // Bounds response JSON parsing as well as final output.
	parseJSON goja.Callable                          // Pristine JSON.parse captured before authored initialization.
	enabled   bool                                   // I/O is admitted only after initialization succeeds.
	nextID    uint64                                 // Monotonic operation identity within this invocation.
	pending   map[uint64]func(fetchCompletion) error // VM-only settlement callbacks.
	completed chan fetchCompletion                   // Unbuffered ownership handoff from workers.
}

// newFetchLoop captures pristine parsing and starts no work. Its caller closes
// it before disposing the VM, including after an initialization failure.
func newFetchLoop(
	ctx context.Context,
	vm *goja.Runtime,
	options FetchOptions,
	limits Limits,
) *fetchLoop {
	run, cancel := context.WithCancel(ctx)

	parse, _ := goja.AssertFunction(vm.Get("JSON").ToObject(vm).Get("parse"))

	if options.Logger == nil {
		options.Logger = slog.Default()
	}

	return &fetchLoop{
		ctx:       run,
		cancel:    cancel,
		vm:        vm,
		options:   options,
		limits:    limits,
		parseJSON: parse,
		pending:   make(map[uint64]func(fetchCompletion) error),
		completed: make(chan fetchCompletion),
	}
}

// rejected creates a native rejected Promise with a fixed, JavaScript-native
// error. NewGoError is deliberately avoided because it reflects a Go error.
func (l *fetchLoop) rejected(err error) goja.Value {
	promise, _, reject := l.vm.NewPromise()
	_ = reject(l.vm.NewTypeError("%s", fetchErrorMessage(err)))
	return l.vm.ToValue(promise)
}

// start reserves a worker slot before launching it. Work must capture Go data
// only; convert and the Promise resolvers remain in the owner-only pending map.
func (l *fetchLoop) start(
	work func() fetchCompletion,
	convert func(fetchCompletion) (goja.Value, error),
) goja.Value {
	if len(l.pending) >= l.options.MaxPendingOperations || l.ctx.Err() != nil {
		return l.rejected(ErrLimitExceeded)
	}

	promise, resolve, reject := l.vm.NewPromise()

	l.nextID++
	id := l.nextID

	l.pending[id] = func(done fetchCompletion) error {
		value, err := convert(done)
		if err != nil {
			err = reject(l.vm.NewTypeError("%s", fetchErrorMessage(err)))
		} else {
			err = resolve(value)
		}
		if err != nil {
			return runtimeEvaluationError(err)
		}
		return nil
	}

	go runFetchWorker(l.ctx, l.completed, l.options.Logger, id, work)

	return l.vm.ToValue(promise)
}

// settle drives host completions while execute is pending. Promise callbacks
// may admit additional work; each settlement drains its Goja microtask queue.
// We wait to check detached rejections until execution finishes, allowing a
// handler to be installed after an awaited operation, as normal Promise code can.
func (l *fetchLoop) settle(root *goja.Promise, tracker *promiseTracker) (goja.Value, error) {
	for root.State() == goja.PromiseStatePending {
		if len(l.pending) == 0 {
			return nil, ErrEvaluation
		}

		select {
		case <-l.ctx.Done():
			return nil, l.ctx.Err()
		case done := <-l.completed:
			callback := l.pending[done.id]
			delete(l.pending, done.id)
			if err := callback(done); err != nil {
				return nil, err
			}
		}
	}

	if len(l.pending) != 0 {
		return nil, ErrEvaluation
	}

	return tracker.settledResult(root)
}

// close cancels requests; the session's cancellation callbacks close received
// bodies. Never wait for Body.Close on the VM owner: even a close can block in
// an uncooperative transport. Workers blocked on the unbuffered completion channel
// close their own untransferred bodies. Late workers cannot settle VM Promises.
func (l *fetchLoop) close() {
	l.cancel()
}

// runFetchWorker always reports a completion, including after panic or Goexit.
// The unbuffered send transfers response ownership only to an active receiver;
// a canceled/disposed owner cannot leave a body parked in an abandoned queue.
func runFetchWorker(
	ctx context.Context,
	completed chan<- fetchCompletion,
	logger *slog.Logger,
	id uint64,
	work func() fetchCompletion,
) {
	done := fetchCompletion{
		id:  id,
		err: httphost.ErrTransport,
	}

	returned := false

	defer func() {
		recovered := recover()
		if !returned {
			kind := "goexit"
			if recovered != nil {
				kind = "panic"
			}
			logger.ErrorContext(
				ctx,
				"Tool fetch worker exited abnormally",
				"kind", kind,
				"panic_type", fmt.Sprintf("%T", recovered),
				"stack", fetchWorkerStack(),
			)
		}

		select {
		case completed <- done:
		case <-ctx.Done():
			if done.response != nil {
				_ = done.response.Body.Close()
			}
		}
	}()

	done = work()
	done.id = id
	returned = true
}

// fetchWorkerStack records Go function names and source locations, omitting
// stack argument values and the panic payload, which can contain credentials.
func fetchWorkerStack() string {
	pcs := make([]uintptr, 24)
	n := runtime.Callers(3, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	var stack strings.Builder
	for {
		frame, more := frames.Next()
		fmt.Fprintf(&stack, "%s %s:%d\n", frame.Function, frame.File, frame.Line)
		if !more {
			return stack.String()
		}
	}
}

// fetchErrorMessage restricts Promise rejection text to known classifications.
// Unexpected host errors never disclose their original text to authored code.
func fetchErrorMessage(err error) string {
	switch err {
	case ErrInvalidInput,
		ErrLimitExceeded,
		httphost.ErrRequestLimit,
		httphost.ErrResponseLimit,
		httphost.ErrStreamLimit,
		httphost.ErrHeaderLimit,
		httphost.ErrEncoding,
		httphost.ErrCredentialsUnavailable:
		return err.Error()
	default:
		return "Tool fetch failed"
	}
}
