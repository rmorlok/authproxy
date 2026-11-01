package core

import (
	"context"
	"errors"
	"fmt"

	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/httpexec"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/rmorlok/authproxy/internal/util"
)

// invokeHTTPTool authorizes, prepares, and executes the current captured HTTP
// Tool revision under explicit operator policy. It is an internal composition
// point, not a public API: policy configuration and bounded schema evaluation
// must be in place before a route can expose it. No policy defaults are supplied.
//
// Authorization and expectedRevision errors remain Go errors. After authorization,
// known readiness/capability failures return bounded notStarted ToolResults;
// unexpected storage, decryption, or compilation failures remain internal errors.
// The executor preserves rich output and cumulative upstream send evidence.
//
// The effective timeout begins after authorization and covers preparation,
// compilation, and execution. Synchronous schema compilation/evaluation can only
// observe cancellation afterward for now; no work is detached on timeout. The
// caller must not mutate arguments or request-auth data while this call runs.
func (s *service) invokeHTTPTool(
	ctx context.Context,
	ra *authcore.RequestAuth,
	id apid.ID,
	expectedRevision *uint64,
	arguments common.RawJSON,
	policy httpexec.Limits,
) (*api.ToolResult, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: invocation context is required", ErrInvalidArgument)
	}

	authorized, err := s.authorizeToolInvocation(ctx, ra, id, expectedRevision)
	if err != nil {
		return nil, err
	}

	limits, err := httpexec.ResolveLimits(policy, authorized.definition.Limits)
	if err != nil {
		return nil, err
	}

	// These two allowances also govern failures that never compile an HTTP
	// program. Remaining component policy is validated by the compiler below.
	if limits.HTTP.Timeout <= 0 || limits.Response.MaxEncodedResultBytes <= 0 {
		return nil, errors.New("HTTP Tool invocation requires positive timeout and result byte limits")
	}

	workCtx, stop := context.WithTimeoutCause(ctx, limits.HTTP.Timeout, httphost.ErrTimeout)
	defer stop()

	deadline, _ := workCtx.Deadline()
	captured := authorized.snapshot

	invocation := httpexec.Invocation{
		ID:        apid.New(apid.PrefixToolInvocation).String(),
		Arguments: arguments,
		Deadline:  deadline,

		// A readiness failure still identifies the authorized revision and source,
		// without needing to decrypt configuration or create a connection client.
		Snapshot: render.Input{
			ToolID:            captured.Revision.ToolId.String(),
			Revision:          captured.Revision.Revision,
			ConnectionID:      captured.Connection.Id.String(),
			ToolSetGeneration: util.CloneValue(captured.Revision.SourceGeneration),
		},
	}

	// No target request can have started on these paths. Only fixed messages
	// enter ToolResults; private dependency/compiler diagnostics stay Go errors.
	failBeforeExecution := func(err error, code, message string) (*api.ToolResult, error) {
		if callerErr := ctx.Err(); callerErr != nil {
			return nil, callerErr
		}

		if errors.Is(context.Cause(workCtx), httphost.ErrTimeout) {
			code = "EXECUTION_LIMIT_EXCEEDED"
			message = "The Tool invocation exceeded its time limit."
		}

		if code == "" {
			return nil, err
		}

		return httpexec.ErrorResult(
			ctx,
			invocation,
			limits.Response.MaxEncodedResultBytes,
			api.ToolExecutionStateNotStarted,
			code,
			message,
		)
	}

	if authorized.definition.ProxyHTTP == nil || authorized.definition.Javascript != nil {
		return failBeforeExecution(nil, "UNSUPPORTED_CAPABILITY", "The Tool executor is not supported.")
	}

	prepared, err := s.prepareToolInvocation(workCtx, authorized)
	if err != nil {
		switch {
		case errors.Is(err, errToolConnectionUnavailable):
			return failBeforeExecution(
				err,
				"CONNECTION_UNAVAILABLE",
				"The Tool connection is not configured.",
			)
		case errors.Is(err, ErrProxyNotImplemented):
			return failBeforeExecution(
				err,
				"UNSUPPORTED_CAPABILITY",
				"The Tool connection does not support HTTP requests.",
			)
		default:
			return failBeforeExecution(
				err,
				"", // code
				"", // message
			)
		}
	}

	program, err := httpexec.Compile(authorized.definition, limits)
	if err != nil {
		return failBeforeExecution(
			fmt.Errorf("failed to compile tool %s revision %d: %w", id, captured.Revision.Revision, err),
			"", // code
			"", // message
		)
	}

	if err := workCtx.Err(); err != nil {
		return failBeforeExecution(err, "", "")
	}

	invocation.Snapshot = prepared.snapshot
	
	// Keep caller cancellation separate from the policy deadline. Passing workCtx
	// as the parent would make expiry look like caller cancellation and discard
	// the executor's evidence that a target request may already have been sent.
	return program.Execute(ctx, prepared.client, invocation)
}
