package runtime

import (
	"net/http"

	"github.com/dop251/goja"
	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
)

// BindFetch returns a native fetch function permanently bound to dispatch. It
// installs no JavaScript global; an adapter chooses where to expose the function.
// All bindings share this host's initialization gate and pending-worker allowance.
// This ordinary fetch binding rejects the connection-specific labels option.
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

	direct := func(req *http.Request, _ map[string]string) (*aphttp.Response, error) {
		return dispatch(req)
	}

	return func(call goja.FunctionCall) goja.Value {
		return l.fetch(direct, nil, call)
	}, nil
}

// BindConnectionFetch binds a connection capability with optional per-request
// labels. Validation runs before worker admission; only detached requests/maps
// reach DoWithLabels. Unlike direct BindFetch, every connection binding supports
// labels without a separate enable flag. Initialization still refuses all I/O.
func (l *HTTPHost) BindConnectionFetch(dispatcher ConnectionDispatcher) (func(goja.FunctionCall) goja.Value, error) {
	if dispatcher == nil {
		return nil, ErrInvalidInput
	}

	return func(call goja.FunctionCall) goja.Value {
		return l.fetch(dispatcher.DoWithLabels, dispatcher.ValidateLabels, call)
	}, nil
}

// fetch validates and detaches authored request values on the VM goroutine.
// Even malformed input becomes a rejected Promise, with no private exception
// text. No worker is launched before initialization and admission complete.
func (l *HTTPHost) fetch(
	dispatch func(*http.Request, map[string]string) (*aphttp.Response, error),
	validateLabels func(map[string]string) error,
	call goja.FunctionCall,
) goja.Value {
	if !l.enabled {
		return l.rejected(ErrEvaluation)
	}
	if len(l.pending) >= l.options.MaxPendingOperations {
		return l.rejected(ErrLimitExceeded)
	}

	var req *http.Request
	var labels map[string]string
	var err error

	if exception := l.vm.Try(func() {
		req, labels, err = prepareFetchRequest(l.ctx, l.vm, call, l.options.MaxRequestBytes, validateLabels != nil)
	}); exception != nil {
		return l.rejected(ErrInvalidInput)
	}

	if err != nil {
		return l.rejected(err)
	}
	if validateLabels != nil {
		if err := validateLabels(labels); err != nil {
			if req.Body != nil {
				_ = req.Body.Close()
			}

			return l.rejected(ErrInvalidInput)
		}
	}

	// The worker captures only the dispatcher and detached request, never l or VM
	// values. It can finish safely even after evaluation has disposed its runtime.
	promise, admitted := l.start(func() fetchCompletion {
		response, err := dispatch(req, labels)
		return fetchCompletion{
			response: response,
			err:      err,
		}
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
