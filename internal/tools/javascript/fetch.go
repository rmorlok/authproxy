package javascript

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/dop251/goja"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
)

// FetchOptions supplies the already authorized connection and host-side policy.
// No connection or credential object is exposed to JavaScript.
type FetchOptions struct {
	// Session belongs to this invocation. Its configured client fixes connection
	// identity and attribution; it also owns cumulative HTTP budgets and send
	// evidence. Evaluation takes ownership and closes it before returning; the
	// caller can still read ExecutionState or call the idempotent Close afterward.
	Session *httphost.Session

	// MaxRequestBytes bounds the combined URL, method, headers, and body of each
	// request before handing a detached copy to an HTTP worker.
	MaxRequestBytes int

	// MaxPendingOperations bounds concurrent fetch and body-read workers before
	// goroutines are started. Session's logical request cap is independent.
	MaxPendingOperations int

	// Logger receives fixed diagnostics and Go call locations for abnormal
	// worker exits, never panic values or request/response data. Nil uses slog's
	// default logger; callers may supply their existing instrumented logger.
	Logger *slog.Logger
}

// EvaluateWithHTTP evaluates a Tool with connection-bound fetch and bounded,
// single-use response readers. The caller must authorize and validate the Tool
// before transferring a session dedicated to this invocation. Validate and ordinary
// Evaluate remain free of I/O. Fetch is unavailable during initialization.
//
// All Goja access stays on the calling goroutine. HTTP workers exchange detached
// Go data; evaluation cancels its requests and schedules unread-body cleanup on
// every exit without waiting for an uncooperative client or body. Inspect the
// session's ExecutionState even on failure: cancellation cannot undo sends.
//
// A Tool must await its HTTP work. Finishing execute with outstanding operations
// fails rather than reporting success while detached work may still run. A
// pending root with no host work also fails. HTTP status errors resolve normally;
// transport/body errors reject with fixed messages. Caller cancellation takes
// precedence over session failure, which takes precedence over runtime errors.
func (p *ToolProgram) EvaluateWithHTTP(
	ctx context.Context,
	input Input,
	options FetchOptions,
) (common.RawJSON, error) {
	if p == nil ||
		p.program == nil ||
		ctx == nil ||
		options.Session == nil ||
		options.Session.Context() == nil ||
		options.MaxRequestBytes <= 0 ||
		options.MaxPendingOperations <= 0 {
		return nil, ErrInvalidInput
	}

	// Closing the session synchronously gates late worker admission before the
	// caller can sample send evidence. Canceling only a request context would
	// leave a window while Session.Do's asynchronous cancellation hook runs.
	defer options.Session.Close()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := options.Session.Err(); err != nil {
		return nil, err
	}

	// Session exhaustion must interrupt busy JavaScript as well as pending I/O.
	// Keep caller cancellation separate so it retains its original classification.
	parent, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(options.Session.Context(), cancel)
	defer func() { stop(); cancel() }()

	result, err := evaluateJSON(
		parent,
		p.limits,
		input,
		func(runCtx context.Context, vm *goja.Runtime) (goja.Value, error) {
			loop := newFetchLoop(runCtx, vm, options, p.limits)
			defer loop.close()

			if err := vm.Set("fetch", loop.fetch); err != nil {
				return nil, ErrInvalidInput
			}

			tracker := newPromiseTracker(vm)
			defer tracker.close()

			execute, err := p.Initialize(vm)
			if err != nil {
				return nil, err
			}

			if err := tracker.checkRejections(); err != nil {
				return nil, err
			}

			loop.enabled = true
			root, err := callTool(vm, execute)

			if err != nil {
				return nil, err
			}
			return loop.settle(root, tracker)
		},
	)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if sessionErr := options.Session.Err(); sessionErr != nil {
		return nil, sessionErr
	}

	return result, err
}

// fetch validates and detaches authored request values on the VM goroutine.
// Even malformed input becomes a rejected Promise, with no private exception
// text. No worker is launched before initialization and admission complete.
func (l *fetchLoop) fetch(call goja.FunctionCall) goja.Value {
	if !l.enabled {
		return l.rejected(ErrEvaluation)
	}
	if len(l.pending) >= l.options.MaxPendingOperations {
		return l.rejected(ErrLimitExceeded)
	}
	var req *http.Request
	var err error
	if exception := l.vm.Try(func() {
		req, err = prepareFetchRequest(l.ctx, l.vm, call, l.options.MaxRequestBytes)
	}); exception != nil {
		return l.rejected(ErrInvalidInput)
	}
	if err != nil {
		return l.rejected(err)
	}

	// The worker captures only the session and detached request, never l or VM
	// values. It can finish safely even after evaluation has disposed its runtime.
	session := l.options.Session
	return l.start(func() fetchCompletion {
		response, err := session.Do(req)
		return fetchCompletion{response: response, err: err}
	}, func(done fetchCompletion) (goja.Value, error) {
		if done.err != nil {
			return nil, done.err
		}
		return l.responseValue(done.response)
	})
}
