package jsexec

import (
	"context"
	"errors"
	"time"

	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/api"
	toolresult "github.com/rmorlok/authproxy/internal/tools/result"
	"github.com/rmorlok/authproxy/internal/tools/validation"
)

// executionContext owns the full invocation deadline independently of the VM
// and HTTP session. Earlier admission and caller deadlines cannot be extended.
func (p *Program) executionContext(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	timed, stop := context.WithTimeoutCause(ctx, min(p.limits.Runtime.Timeout, p.limits.HTTP.Timeout), apjsruntime.ErrLimitExceeded)
	current, _ := timed.Deadline()
	if !deadline.IsZero() && deadline.Before(current) {
		withAdmission, stopAdmission := context.WithDeadlineCause(timed, deadline, apjsruntime.ErrLimitExceeded)
		return withAdmission, func() { stopAdmission(); stop() }
	}
	return timed, stop
}

// failure maps fixed host/runtime categories into public outcomes. It uses the
// original caller context to report policy failures after work has expired;
// caller cancellation still wins. Exceptions and dependency text never escape.
func (p *Program) failure(
	caller, work context.Context,
	base *api.ToolResult,
	state api.ToolExecutionState,
	err error,
	code, message string,
) (*api.ToolResult, error) {
	if callerErr := caller.Err(); callerErr != nil {
		return nil, callerErr
	}
	if workErr := apjsruntime.ChargeWork(work, 0); workErr != nil {
		err = workErr
	}
	if errors.Is(context.Cause(work), apjsruntime.ErrLimitExceeded) {
		err = apjsruntime.ErrLimitExceeded
	}
	switch {
	case errors.Is(err, apjsruntime.ErrLimitExceeded), errors.Is(err, validation.ErrLimit):
		code, message = "EXECUTION_LIMIT_EXCEEDED", "The Tool invocation exceeded an execution limit."
	case errors.Is(err, aphttp.ErrResponseLimit):
		code, message = "OUTPUT_TOO_LARGE", "The upstream response exceeds the response size limits."
	case errors.Is(err, apjsruntime.ErrInvalidInput):
		code, message = "INPUT_INVALID", "The Tool invocation context cannot be prepared."
	}

	toolresult.SetFailure(base, &api.ToolResultError{Code: code, Message: message}, state)
	return toolresult.Finish(caller, base, state, p.limits.MaxEncodedResultBytes)
}
