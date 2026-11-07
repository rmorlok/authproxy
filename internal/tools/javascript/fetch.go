package javascript

import (
	"context"
	"log/slog"

	"github.com/dop251/goja"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
)

// FetchOptions supplies the already authorized connection and host-side policy.
// JavaScript receives only its copied metadata and a bound fetch capability,
// never a live Go connection or credentials.
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

// EvaluateWithHTTP evaluates a Tool with exactly one entry in context.connections
// and bounded, single-use response readers. The caller must authorize and validate
// the Tool before transferring a session dedicated to this invocation. Validate
// and ordinary Evaluate remain free of I/O. Connection fetch is unavailable during
// initialization, and global fetch remains undefined throughout execution.
//
// All Goja access stays on the calling goroutine. HTTP workers exchange detached
// Go data; evaluation cancels its requests and schedules unread-body cleanup on
// every exit without waiting for an uncooperative client or body. Inspect the
// session's ExecutionState even on failure: cancellation cannot undo sends.
//
// When execute settles, outstanding operations are canceled and late results
// are discarded; they do not turn a successful race or caught failure into an
// evaluation error. Already-observed unhandled rejections still fail. A pending
// root with no host work also fails. HTTP status errors resolve normally;
// transport/body errors reject with fixed messages. Caller cancellation takes
// precedence over session failure, which takes precedence over runtime errors.
func (p *ToolProgram) EvaluateWithHTTP(
	ctx context.Context,
	input Input,
	options FetchOptions,
) (common.RawJSON, error) {
	// Take ownership before checking any other input so rejected options do not
	// leave an initialized session's timer running. A zero-value session has no
	// context or timer and cannot be closed; reject it below without calling Close.
	if options.Session != nil && options.Session.Context() != nil {
		// Closing synchronously gates late admission before the caller can sample
		// send evidence; request cancellation alone has an asynchronous handoff.
		defer options.Session.Close()
	}

	if p == nil ||
		p.program == nil ||
		ctx == nil ||
		options.Session == nil ||
		options.Session.Context() == nil ||
		options.MaxRequestBytes <= 0 ||
		options.MaxPendingOperations <= 0 {
		return nil, apjsruntime.ErrInvalidInput
	}

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

	result, err := p.program.Evaluate(
		parent,
		func(runCtx context.Context, vm *goja.Runtime, invocation *apjsruntime.AsyncInvocation) error {
			host, err := apjsruntime.NewHTTPHost(runCtx, vm, apjsruntime.HTTPOptions{
				MaxRequestBytes:      options.MaxRequestBytes,
				MaxPendingOperations: options.MaxPendingOperations,
				Logger:               options.Logger,
			}, p.limits)
			if err != nil {
				return err
			}
			// Register cleanup before touching the VM again: preparation can be
			// interrupted after creating the host but before returning it.
			invocation.Close = host.Close
			invocation.Start = host.Start
			invocation.Await = host.Await

			return p.prepareInvocation(runCtx, vm, invocation, input, host, options.Session)
		},
	)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if sessionErr := options.Session.Err(); sessionErr != nil {
		return nil, sessionErr
	}

	return common.RawJSON(result), err
}
