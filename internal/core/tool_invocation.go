package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools"
	"github.com/rmorlok/authproxy/internal/tools/httpexec"
	"github.com/rmorlok/authproxy/internal/tools/jsexec"
	toolresult "github.com/rmorlok/authproxy/internal/tools/result"
)

// invokeTool authorizes, prepares, and executes the current captured HTTP or JavaScript
// Tool revision under the server's tools.invocation policy and finite defaults.
// It is an internal composition point, not a public API: bounded schema
// evaluation must be in place before a route can expose it.
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
func (s *service) invokeTool(
	ctx context.Context,
	ra *authcore.RequestAuth,
	id apid.ID,
	expectedRevision *uint64,
	arguments common.RawJSON,
) (*api.ToolResult, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: invocation context is required", ErrInvalidArgument)
	}

	authorized, err := s.authorizeToolInvocation(ctx, ra, id, expectedRevision)
	if err != nil {
		return nil, err
	}

	// Choose only the captured executor's policy. In particular, an independent
	// response-transform allowance must not govern JavaScript Tool execution.
	javascriptTool := authorized.definition.Javascript != nil && authorized.definition.ProxyHTTP == nil

	var httpLimits httpexec.Limits
	var javascriptLimits jsexec.Limits
	var timeout time.Duration
	var maxResultBytes int
	var program tools.Executor

	if javascriptTool {
		policy, err := s.toolJavaScriptInvocationPolicy()
		if err != nil {
			return nil, err
		}

		javascriptLimits, err = jsexec.ResolveLimits(policy, authorized.definition.Limits)
		if err != nil {
			return nil, err
		}

		timeout = min(javascriptLimits.HTTP.Timeout, javascriptLimits.Runtime.Timeout)
		maxResultBytes = javascriptLimits.MaxEncodedResultBytes
	} else {
		policy, err := s.toolHTTPInvocationPolicy()
		if err != nil {
			return nil, err
		}

		httpLimits, err = httpexec.ResolveLimits(policy, authorized.definition.Limits)
		if err != nil {
			return nil, err
		}

		timeout, maxResultBytes = httpLimits.HTTP.Timeout, httpLimits.Response.MaxEncodedResultBytes
	}

	workCtx, stop := context.WithTimeoutCause(ctx, timeout, aphttp.ErrTimeout)
	defer stop()

	deadline, _ := workCtx.Deadline()
	captured := authorized.snapshot
	invocationID := apid.New(apid.PrefixToolInvocation).String()

	// No target request can have started on these paths. Only fixed messages
	// enter ToolResults; private dependency/compiler diagnostics stay Go errors.
	failBeforeExecution := func(err error, code, message string) (*api.ToolResult, error) {
		if callerErr := ctx.Err(); callerErr != nil {
			return nil, callerErr
		}

		if errors.Is(context.Cause(workCtx), aphttp.ErrTimeout) {
			code = "EXECUTION_LIMIT_EXCEEDED"
			message = "The Tool invocation exceeded its time limit."
		} else if errors.Is(err, apjsruntime.ErrLimitExceeded) {
			code = "EXECUTION_LIMIT_EXCEEDED"
			message = "The Tool invocation exceeded an execution limit."
		}

		if code == "" {
			return nil, err
		}

		// Identity-only failures need no decrypted configuration, compiled source,
		// or executor-specific invocation wrapper. New detaches source metadata.
		base, buildErr := toolresult.New(
			captured.Revision.ToolId.String(),
			captured.Revision.Revision,
			invocationID,
			api.ToolResultMetadata{
				ConnectionID:      captured.Connection.Id.String(),
				ToolSetGeneration: captured.Revision.SourceGeneration,
			},
			maxResultBytes,
		)
		if callerErr := ctx.Err(); callerErr != nil {
			return nil, callerErr
		}
		if buildErr != nil {
			return nil, buildErr
		}

		toolresult.SetFailure(
			base,
			&api.ToolResultError{
				Code:    code,
				Message: message,
			},
			api.ToolExecutionStateNotStarted,
		)

		return toolresult.Finish(
			ctx,
			base,
			api.ToolExecutionStateNotStarted,
			maxResultBytes,
		)
	}

	// Future proof ourselves
	if !javascriptTool &&
		(authorized.definition.ProxyHTTP == nil ||
			authorized.definition.Javascript != nil) {
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

	if javascriptTool {
		// Retain the prepared connector instead of rereading its mutable current
		// generation. Raw combined source is admitted before library compilation,
		// including whitespace-only libraries whose compiled source is empty.
		library, err := prepared.connector.getBoundedJavascriptLibrary(
			workCtx,
			*authorized.definition.Javascript,
			javascriptLimits.Runtime,
		)
		if err != nil {
			return failBeforeExecution(err, "", "")
		}

		program, err = jsexec.Compile(
			authorized.definition,
			library,
			javascriptLimits,
		)
		if err != nil {
			return failBeforeExecution(
				fmt.Errorf("failed to compile tool %s revision %d: %w", id, captured.Revision.Revision, err),
				"", // code
				"", // message
			)
		}
	} else {
		program, err = httpexec.Compile(authorized.definition, httpLimits)
		if err != nil {
			return failBeforeExecution(
				fmt.Errorf("failed to compile tool %s revision %d: %w", id, captured.Revision.Revision, err),
				"", // code
				"", // message
			)
		}
	}

	if err := workCtx.Err(); err != nil {
		return failBeforeExecution(err, "", "")
	}

	// Keep caller cancellation separate from the policy deadline. Passing workCtx
	// as the parent would make expiry look like caller cancellation and discard
	// the executor's evidence that a target request may already have been sent.
	return program.Execute(
		ctx,
		prepared.client,
		tools.Invocation{
			ID:        invocationID,
			Arguments: arguments,
			Deadline:  deadline,
			Snapshot:  prepared.snapshot,

			// Worker diagnostics must use the configured service sink and captured
			// identities. Keep arguments/configuration out of logs; supplying this
			// logger does not enable JavaScript's separate authored console API.
			Logger: s.logger.With(
				"tool_id", prepared.snapshot.ToolID,
				"tool_revision", prepared.snapshot.Revision,
				"invocation_id", invocationID,
				"connection_id", prepared.snapshot.ConnectionID,
				"namespace", prepared.snapshot.ConnectionNamespace,
				"connector_id", prepared.connector.Id.String(),
				"connector_generation", prepared.connector.Generation,
				"actor_id", authorized.actor.Id.String(),
			),
		},
	)
}
