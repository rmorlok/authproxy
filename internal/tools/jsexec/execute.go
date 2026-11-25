package jsexec

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"time"

	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	toolresult "github.com/rmorlok/authproxy/internal/tools/result"
	"github.com/rmorlok/authproxy/internal/tools/validation"
)

// ErrInvalidInvocation identifies invalid internal inputs, such as a missing
// program, configured client, or captured identity. It contains no invocation data.
var ErrInvalidInvocation = errors.New("invalid JavaScript Tool invocation")

// Invocation combines caller arguments with the connection and Tool revision
// captured by admission. The caller must associate this snapshot, the compiled
// program, and the configured HTTP client with the same authorized connection.
// Invocation is an internal execution input, not a serialized API request.
type Invocation struct {
	ID string // Captured tiv_ correlation identity.

	// Deadline includes admission/preparation time in the execution allowance.
	// Zero starts the timeout in Execute. A supplied deadline can only shorten
	// the compiled runtime/HTTP timeouts and any earlier caller deadline.
	Deadline time.Time

	// Arguments is the caller's JSON object, for example {"id":"customer-42"}.
	// Execute validates it against the captured input schema before running
	// authored code, then exposes its values as execute's params argument.
	Arguments common.RawJSON

	// Snapshot supplies captured connection configuration, metadata, identities,
	// and Tool provenance. For example Cfg["tenant"] = "acme" becomes
	// context.connections[0].cfg.tenant while Arguments supplies params.id.
	// Execute copies this data and replaces Params with validated Arguments;
	// leave Snapshot.Params unset. Credentials belong only to the HTTP client.
	Snapshot render.Input

	// Logger receives fixed HTTP-worker diagnostics with caller attribution.
	// Nil uses the adapter's default logger. It does not enable authored console
	// logging or expose request data, panic values, or credentials.
	Logger *slog.Logger
}

// Execute evaluates one admitted JavaScript Tool in a fresh VM with exactly one
// connection capability. The configured client owns authentication and request
// attribution; admission, readiness, actor/JWT permissions, expectedRevision,
// and connector-library selection belong to the caller. Global fetch is absent.
//
// One outer deadline spans argument validation, snapshot preparation, JavaScript,
// HTTP/body reads, successful-output validation, and final envelope assembly.
// Managed JavaScript work and admitted argument/output data share a native budget;
// snapshot copying and envelope encoding retain their independent data limits.
// Schema evaluation still needs its separately tracked work bound before public
// invocation is enabled. Synchronous work is checked for cancellation afterward,
// never abandoned in a detached validation goroutine.
//
// Runtime failures become safe ToolResults with captured send evidence. Caller
// cancellation/deadline, invalid internal inputs, and an allowance too small for
// a safe error return a Go error with no result. No automatic retries occur here.
// Callers must keep invocation data stable during the call; returned data is owned
// by this invocation and compiled programs support concurrent execution.
func (p *Program) Execute(
	ctx context.Context,
	client iface.HTTPClient,
	invocation Invocation,
) (*api.ToolResult, error) {
	if p == nil || p.script == nil || p.schemas == nil || ctx == nil || client == nil {
		return nil, ErrInvalidInvocation
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Keep this owner alive after the JavaScript adapter closes its HTTP session.
	// Output schemas and envelope assembly must consume the original deadline.
	timed, stop := p.executionContext(ctx, invocation.Deadline)
	defer stop()

	input := invocation.Snapshot
	base, err := toolresult.New(
		input.ToolID,
		input.Revision,
		invocation.ID,
		api.ToolResultMetadata{
			ConnectionID:      input.ConnectionID,
			ToolSetGeneration: input.ToolSetGeneration,
			CatalogRevision:   input.CatalogRevision,
		},
		p.limits.MaxEncodedResultBytes,
	)
	if callerErr := ctx.Err(); callerErr != nil {
		return nil, callerErr
	}
	if err != nil {
		if errors.Is(err, toolresult.ErrInvalidResult) {
			return nil, ErrInvalidInvocation
		}
		return nil, err
	}

	state := api.ToolExecutionStateNotStarted
	workCtx, err := apjsruntime.WithExecutionBudget(timed, p.limits.Runtime)
	if err != nil {
		return p.failure(ctx, timed, base, state, err, "EXECUTION_LIMIT_EXCEEDED", "The Tool invocation exceeded an execution limit.")
	}

	fail := func(err error, code, message string) (*api.ToolResult, error) {
		return p.failure(ctx, workCtx, base, state, err, code, message)
	}

	// Charge the known JSON pass before decoding. Native schema evaluation has
	// separate data limits; this charge does not claim to bound schema expansion.
	if err := chargeDataPass(workCtx, len(invocation.Arguments)); err != nil {
		return fail(err, "INPUT_INVALID", "The Tool arguments cannot be prepared.")
	}

	params, err := p.schemas.ValidateInput(workCtx, invocation.Arguments)
	if err != nil {
		return fail(err, "INPUT_INVALID", "The Tool arguments do not match the input schema.")
	}

	// Schema numbers retain exact JSON spelling, but JavaScript requires finite
	// Number values. Attribute this incompatibility to arguments before combining
	// them with the captured connection data.
	if err := validateArgumentNumbers(workCtx, params); err != nil {
		return fail(err, "INPUT_INVALID", "The Tool arguments contain a number unsupported by JavaScript.")
	}

	input.Params = params
	snapshot, err := render.NewContext(input, p.limits.Render)
	if err != nil {
		return fail(err, "CONNECTION_UNAVAILABLE", "The captured Tool connection context cannot be prepared.")
	}

	if err := apjsruntime.ChargeWork(workCtx, 0); err != nil {
		return fail(err, "EXECUTION_LIMIT_EXCEEDED", "The Tool invocation exceeded an execution limit.")
	}

	session, err := httphost.New(workCtx, client, p.limits.HTTP)
	if err != nil {
		return fail(err, "EXECUTION_LIMIT_EXCEEDED", "The Tool invocation could not start.")
	}
	defer session.Close()

	output, err := p.script.EvaluateWithHTTP(
		workCtx,
		javascript.Input{Context: snapshot},
		javascript.FetchOptions{
			Session:              session,
			MaxRequestBytes:      p.limits.MaxRequestBytes,
			MaxPendingOperations: p.limits.MaxPendingOperations,
			Logger:               invocation.Logger,
		},
	)

	// Evaluation closes the dedicated session. Snapshot evidence once; later
	// worker completions cannot revise the uncertainty reported by this call.
	state = session.ExecutionState()
	if err != nil {
		return fail(err, "OUTPUT_INVALID", "The Tool script could not produce a valid result.")
	}
	if err := apjsruntime.ChargeWork(workCtx, 0); err != nil {
		return fail(err, "EXECUTION_LIMIT_EXCEEDED", "The Tool invocation exceeded an execution limit.")
	}

	if output.Error != nil {
		// Authored errors already own validated public fields. Execution state
		// comes from HTTP evidence; authored code cannot supply retry advice.
		toolresult.SetFailure(base, output.Error, state)
	} else {
		if err := chargeDataPass(workCtx, len(output.StructuredContent)); err != nil {
			return fail(err, "EXECUTION_LIMIT_EXCEEDED", "The Tool invocation exceeded an execution limit.")
		}
		if err := p.schemas.ValidateOutput(workCtx, output.StructuredContent); err != nil {
			if errors.Is(err, validation.ErrLimit) {
				return fail(nil, "OUTPUT_TOO_LARGE", "The structured Tool result exceeds the validation data limits.")
			}
			return fail(err, "OUTPUT_INVALID", "The Tool result does not match the output schema.")
		}
		base.StructuredContent, base.Content, base.Provider = output.StructuredContent, output.Content, output.Provider
	}

	result, err := toolresult.Finish(workCtx, base, state, p.limits.MaxEncodedResultBytes)

	// The HTTP session is closed; only the still-live outer context/budget can
	// decide whether post-VM work completed inside this invocation's allowance.
	if workErr := apjsruntime.ChargeWork(workCtx, 0); workErr != nil {
		return fail(workErr, "EXECUTION_LIMIT_EXCEEDED", "The Tool invocation exceeded an execution limit.")
	}
	if errors.Is(err, toolresult.ErrEncodedResultLimit) {
		return nil, err
	}
	if err != nil {
		return fail(err, "OUTPUT_INVALID", "The Tool result could not be encoded.")
	}

	return result, nil
}

// chargeDataPass admits processing and possible copying of known JSON bytes,
// including structured output without a configured schema. Independent validator
// depth/value/number bounds still apply when validation decodes the data.
func chargeDataPass(ctx context.Context, size int) error {
	if err := apjsruntime.ChargeWork(ctx, size); err != nil {
		return err
	}
	return apjsruntime.ReserveAllocation(ctx, size)
}

// validateArgumentNumbers checks the already bounded, schema-validated argument
// tree for numbers the JavaScript decoder cannot represent as finite values. It
// consumes shared traversal work, retains exact json.Number values, and never
// coerces or normalizes caller data. The validator's tree cannot contain cycles.
func validateArgumentNumbers(ctx context.Context, value any) error {
	if err := apjsruntime.ChargeWork(ctx, 1); err != nil {
		return err
	}
	switch value := value.(type) {
	case json.Number:
		number, err := value.Float64()
		if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
			return validation.ErrInvalidInput
		}
	case map[string]any:
		for _, child := range value {
			if err := validateArgumentNumbers(ctx, child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range value {
			if err := validateArgumentNumbers(ctx, child); err != nil {
				return err
			}
		}
	}
	return apjsruntime.ChargeWork(ctx, 0)
}
