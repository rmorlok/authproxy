package runtime

import (
	"context"
	"fmt"
	"log/slog"
	goruntime "runtime"
	"strings"

	"github.com/dop251/goja"
	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
)

// fetchCompletion crosses the worker boundary using only owned Go data. A
// response body must either transfer to the owner or be closed by the worker.
type fetchCompletion struct {
	id       uint64           // Owner's lookup key; no VM callback crosses threads.
	response *aphttp.Response // Successful fetch; metadata and body transfer together.
	body     []byte           // Successful bounded read; worker never touches it again.
	err      error            // Fixed host classification; no upstream diagnostics.
}

// HTTPOptions supplies resolved limits and diagnostics for one invocation's HTTP
// host. The caller separately supplies bounded dispatchers through BindFetch.
type HTTPOptions struct {
	// MaxRequestBytes bounds the combined URL, method, headers, and body of each
	// request before handing a detached copy to an HTTP worker.
	MaxRequestBytes int

	// MaxPendingOperations bounds concurrent fetch and body-read workers across
	// every binding on the host, before goroutines are started. HTTP session
	// request and response allowances are independent of this worker cap.
	MaxPendingOperations int

	// Logger receives fixed diagnostics and Go call locations for abnormal
	// worker exits, never panic values or request/response data. Nil uses slog's
	// default logger.
	Logger *slog.Logger
}

// HTTPHost owns Promise settlement and response lifetimes on one VM goroutine.
// Workers receive no pointer to this object, so a stalled transport cannot retain
// the VM through its completion callback. Only the unbuffered channel is shared.
// Create one host per AsyncInvocation so all bound fetch functions share its
// pending-worker allowance. Its methods and bindings belong to that VM goroutine;
// they must not be retained after evaluation. The zero value is not usable.
type HTTPHost struct {
	ctx       context.Context                        // Invocation runtime deadline plus host cleanup cancellation.
	cancel    context.CancelFunc                     // Stops requests and body reads on every exit.
	vm        *goja.Runtime                          // Accessed exclusively by the owner.
	options   HTTPOptions                            // Shared admission limits and diagnostics logger.
	limits    Limits                                 // Bounds response JSON parsing as well as final output.
	decoder   *JSONDecoder                           // Native JSON decoder captured before authored initialization.
	enabled   bool                                   // I/O is admitted only after initialization succeeds.
	nextID    uint64                                 // Monotonic operation identity within this invocation.
	pending   map[uint64]func(fetchCompletion) error // VM-only settlement callbacks.
	completed chan fetchCompletion                   // Unbuffered ownership handoff from workers.
}

// NewHTTPHost captures pristine parsing and starts no work. Call it during
// AsyncProgram.Evaluate preparation, before any authored code runs, with the
// evaluation's context and limits. Register Close on AsyncInvocation immediately
// after construction, then Start and Await, before touching the VM again.
// Invalid policy or missing parser bindings return ErrInvalidInput without
// acquiring resources. The caller still owns the HTTP session and its cleanup.
// It must link session cancellation to the evaluation context when session
// exhaustion or expiry should also interrupt JavaScript that is not awaiting I/O.
func NewHTTPHost(
	ctx context.Context,
	vm *goja.Runtime,
	options HTTPOptions,
	limits Limits,
) (*HTTPHost, error) {
	if ctx == nil || vm == nil || options.MaxRequestBytes <= 0 ||
		options.MaxPendingOperations <= 0 || limits.Validate() != nil {
		return nil, ErrInvalidInput
	}
	decoder, err := NewJSONDecoder(vm)
	if err != nil {
		return nil, err
	}
	run, cancel := context.WithCancel(ctx)

	if options.Logger == nil {
		options.Logger = slog.Default()
	}

	return &HTTPHost{
		ctx:       run,
		cancel:    cancel,
		vm:        vm,
		options:   options,
		limits:    limits,
		decoder:   decoder,
		pending:   make(map[uint64]func(fetchCompletion) error),
		completed: make(chan fetchCompletion),
	}, nil
}

// Start enables I/O after initialization and its Promise jobs succeed. Use it as
// AsyncInvocation.Start; calling it earlier would allow initialization side effects.
func (l *HTTPHost) Start() {
	l.enabled = true
}

// rejected creates a native rejected Promise with a fixed, JavaScript-native
// error. NewGoError is deliberately avoided because it reflects a Go error.
func (l *HTTPHost) rejected(err error) goja.Value {
	promise, _, reject := l.vm.NewPromise()
	_ = reject(l.vm.NewTypeError("%s", fetchErrorMessage(err)))
	return l.vm.ToValue(promise)
}

// start reserves a worker slot before launching it. Work must capture Go data
// only; convert and the Promise resolvers remain in the owner-only pending map.
// The boolean reports worker admission, so a refused body read can stay unused.
func (l *HTTPHost) start(
	work func() fetchCompletion,
	convert func(fetchCompletion) (goja.Value, error),
) (goja.Value, bool) {
	if len(l.pending) >= l.options.MaxPendingOperations || l.ctx.Err() != nil {
		return l.rejected(ErrLimitExceeded), false
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
			return EvaluationError(err)
		}
		return nil
	}

	go runFetchWorker(l.ctx, l.completed, l.options.Logger, id, work)

	return l.vm.ToValue(promise), true
}

// Await processes one host completion on the VM goroutine. Resolving its Promise
// drains Goja jobs, which may admit more work. The shared runtime decides whether
// the root still needs another completion; a winning race never drains siblings.
// With no pending host work, an unresolved root cannot make progress.
func (l *HTTPHost) Await() error {
	if len(l.pending) == 0 {
		return ErrEvaluation
	}

	select {
	case <-l.ctx.Done():
		return l.ctx.Err()
	case done := <-l.completed:
		callback := l.pending[done.id]
		delete(l.pending, done.id)
		return callback(done)
	}
}

// Close cancels requests; the dispatcher's session cancellation callbacks close received
// bodies. Never wait for Body.Close on the VM owner: even a close can block in
// an uncooperative transport. Workers blocked on the unbuffered completion channel
// close their own untransferred bodies. Late workers cannot settle VM Promises.
// Close is idempotent and does not close the caller-owned HTTP session or client.
func (l *HTTPHost) Close() {
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
		err: aphttp.ErrTransport,
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
				"JavaScript fetch worker exited abnormally",
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
	n := goruntime.Callers(3, pcs)
	frames := goruntime.CallersFrames(pcs[:n])
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
		aphttp.ErrRequestLimit,
		aphttp.ErrResponseLimit,
		aphttp.ErrStreamLimit,
		aphttp.ErrHeaderLimit,
		aphttp.ErrEncoding,
		aphttp.ErrCredentialsUnavailable:
		return err.Error()
	default:
		return "JavaScript fetch failed"
	}
}
