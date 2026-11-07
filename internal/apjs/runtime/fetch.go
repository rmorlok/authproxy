package runtime

import (
	"net/http"

	"github.com/dop251/goja"
	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
)

// BindFetch returns a native fetch function permanently bound to dispatch. It
// installs no JavaScript global; an adapter chooses where to expose the function.
// All bindings share this host's initialization gate and pending-worker allowance.
// A nil dispatcher returns ErrInvalidInput.
//
// Dispatch is trusted Go integration code running off the VM goroutine. It must
// capture only detached Go state and use an already authorized, bounded HTTP
// capability (for example, an apjs/http.ClientHandle). It takes request-body
// ownership on entry and returns an owned response whose body honors request
// cancellation. Success requires a nonnil response and body; failure requires a
// nil response, with dispatch cleaning up any partial response. Unknown errors
// are sanitized before reaching JavaScript. Bind all capabilities to the same HTTP
// session so their request, response, deadline, and send-evidence accounting agree.
func (l *HTTPHost) BindFetch(
	dispatch func(*http.Request) (*aphttp.Response, error),
) (func(goja.FunctionCall) goja.Value, error) {
	if dispatch == nil {
		return nil, ErrInvalidInput
	}
	return func(call goja.FunctionCall) goja.Value {
		return l.fetch(dispatch, call)
	}, nil
}

// fetch validates and detaches authored request values on the VM goroutine.
// Even malformed input becomes a rejected Promise, with no private exception
// text. No worker is launched before initialization and admission complete.
func (l *HTTPHost) fetch(dispatch func(*http.Request) (*aphttp.Response, error), call goja.FunctionCall) goja.Value {
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

	// The worker captures only the dispatcher and detached request, never l or VM
	// values. It can finish safely even after evaluation has disposed its runtime.
	promise, admitted := l.start(func() fetchCompletion {
		response, err := dispatch(req)
		return fetchCompletion{response: response, err: err}
	}, func(done fetchCompletion) (goja.Value, error) {
		if done.err != nil {
			return nil, done.err
		}
		return l.responseValue(done.response)
	})

	if !admitted && req.Body != nil {
		_ = req.Body.Close()
	}

	return promise
}
