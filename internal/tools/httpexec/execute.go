package httpexec

import (
	"context"
	"errors"
	"io"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/rmorlok/authproxy/internal/tools/validation"
)

// ErrInvalidInvocation identifies invalid internal execution inputs, such as
// missing correlation identities or an unconfigured client. It carries no data.
var ErrInvalidInvocation = errors.New("invalid HTTP Tool invocation")

// Invocation combines the Tool caller's arguments with the connection and Tool
// context captured by admission. The admission layer must associate the program,
// snapshot, and configured client with the same authorized Tool revision and
// connection. This is an internal input, not an API request type.
type Invocation struct {
	ID string // Captured tiv_ correlation identity.

	// Arguments is the JSON object supplied by the Tool caller for this call.
	// For a contact lookup, it might be {"id":"contact-42","limit":10}.
	// Execute decodes and validates it against the captured Tool's input schema
	// before rendering or I/O, then exposes the validated values as params.
	// Templates can use {{path.params.id}} or {{params.limit}}; response
	// projections use the same values. Numbers retain their exact precision;
	// validation does not insert defaults or coerce types.
	Arguments common.RawJSON

	// Snapshot is the execution context supplied by admission: connection
	// configuration, labels/annotations, connection identity, and the captured
	// Tool revision and optional ToolSet/catalog provenance. For example:
	//
	//	render.Input{
	//	    Cfg: map[string]any{"tenant": "acme"},
	//	    ConnectionID: "cxn_01example0000001",
	//	    ToolID: "tol_01example0000001", Revision: 7,
	//	}
	//
	// With the Arguments above, /{{path.cfg.tenant}}/contacts/{{path.params.id}}
	// renders as /acme/contacts/contact-42: the tenant comes from the admitted
	// connection, while the contact ID comes from the Tool caller's arguments.
	// Execute copies these values and the validated arguments into one immutable
	// rendering context shared by the request and response projection. Leave
	// Params unset; Execute ignores it and replaces it with validated Arguments.
	// Tool/connection identities and revision metadata also populate the result.
	// Authentication comes from the configured HTTP client; credentials must
	// never be placed in Cfg or other snapshot fields.
	Snapshot render.Input
}

// Execute runs one logical HTTP request through the supplied connection client.
// Admission, connection readiness, actor/JWT permissions, expectedRevision, and
// request attribution belong to the caller. No proxy permission is checked here.
// Client must use the admitted connection and explicit actor configuration.
//
// The invocation timeout spans input validation, rendering, I/O, projection, and
// output validation. Schema evaluation currently checks cancellation only around
// its synchronous library call; it still needs an evaluation budget before public
// invocation is enabled. Snapshot copying is bounded synchronous work and observes
// cancellation afterward. No work is detached or automatically retried here.
//
// Runtime failures produce safe bounded ToolResults with cumulative send evidence.
// Caller cancellation/deadline, invalid internal inputs, or a result allowance
// too small for an error return a Go error and no result. Policy timeout remains
// a ToolResult when the caller context is live. Callers must not modify invocation
// data during the call; results own their bytes, slices, and metadata pointers.
func (p *Program) Execute(
	ctx context.Context,
	client iface.HTTPClient,
	invocation Invocation,
) (*api.ToolResult, error) {
	if p == nil ||
		p.request == nil ||
		p.response == nil ||
		p.schemas == nil ||
		ctx == nil ||
		client == nil {
		return nil, ErrInvalidInvocation
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Transform the invocation data in the core shape for the response with
	// ids/etc that will be added to as the invocation proceeds.
	base, err := p.resultIdentity(invocation)
	if err != nil {
		return nil, err
	}

	// Create the HTTP session that acts as an HTTP client. This can be used for
	// multiple HTTP requests through the connection each automatically applying
	// the credentials to the request. Each request will have the appropriate
	// labels, etc. It tracks whether any requests made to the 3rd party to
	// allow us to understand if the tool might have applied to the 3rd party.
	session, err := httphost.New(ctx, client, p.limits.HTTP)
	if err != nil {
		return nil, err
	}

	defer session.Close()

	// Failure normalization uses the still-live parent context so a policy
	// timeout can be reported after the shared work context has expired.
	fail := func(err error, code, message string) (*api.ToolResult, error) {
		if sessionErr := session.Err(); sessionErr != nil {
			err = sessionErr
		}
		return p.failure(ctx, base, session.ExecutionState(), err, code, message)
	}

	workCtx := session.Context()

	// Validate the data being passed against the schemas defined for this tool
	params, err := p.schemas.ValidateInput(workCtx, invocation.Arguments)
	if err != nil {
		return fail(err, "INPUT_INVALID", "The Tool arguments do not match the input schema.")
	}

	// All templates and the response projection share one copied snapshot. The
	// validated raw argument tree is the only source of params, even if a caller
	// accidentally populated Snapshot.Params with another value.
	input := invocation.Snapshot
	input.Params = params

	snapshot, err := render.NewContext(input, p.limits.Render)
	if err != nil {
		return fail(err, "INPUT_INVALID", "The Tool invocation context cannot be prepared.")
	}
	req, err := p.request.Render(workCtx, snapshot)
	if err != nil {
		return fail(err, "INPUT_INVALID", "The Tool request cannot be prepared from the supplied input.")
	}
	response, err := session.Do(req)
	if err != nil {
		return fail(err, "UPSTREAM_FAILED", "The upstream request failed.")
	}
	defer func() { _ = response.Body.Close() }()

	// Error statuses (including authored 2xx error rules) do not need a body.
	// Avoid reading or initializing gzip for discarded provider error content.
	var body []byte
	if p.response.NeedsBody(response.StatusCode) {
		body, err = io.ReadAll(response.Body)
		if err != nil {
			return fail(err, "UPSTREAM_FAILED", "The upstream response could not be read.")
		}
	}
	// Release the provider stream before projection/schema work. Close is
	// idempotent; the defer still covers failures while reading the body.
	_ = response.Body.Close()
	result, err := p.response.Map(
		workCtx,
		httpcodec.ResponseInput{
			StatusCode:     response.StatusCode,
			ContentType:    response.ContentType,
			Body:           body,
			Headers:        response.Header,
			Context:        snapshot,
			ToolID:         base.ToolID,
			Revision:       base.Revision,
			InvocationID:   base.InvocationID,
			Metadata:       base.Metadata,
			ExecutionState: session.ExecutionState(),
		},
	)
	if err != nil {
		if errors.Is(err, httpcodec.ErrEncodedResultLimit) && session.Err() == nil {
			return nil, err
		}
		return fail(err, "OUTPUT_INVALID", "The Tool response could not be mapped.")
	}
	if !result.IsError {
		if err := p.schemas.ValidateOutput(workCtx, result.StructuredContent); err != nil {
			if errors.Is(err, validation.ErrLimit) {
				return fail(nil, "OUTPUT_TOO_LARGE", "The structured Tool result exceeds the validation data limits.")
			}
			return fail(err, "OUTPUT_INVALID", "The Tool result does not match the output schema.")
		}
	}
	// A timeout after mapping/validation must not turn into a successful result.
	if err := session.Err(); err != nil {
		return fail(err, "EXECUTION_LIMIT_EXCEEDED", "The Tool invocation exceeded its time limit.")
	}
	return result, nil
}

// failure converts host and validation categories into fixed public errors.
// Arbitrary compiler/transport/validation messages never cross this boundary.
func (p *Program) failure(
	ctx context.Context,
	base *api.ToolResult,
	state api.ToolExecutionState,
	err error,
	code, message string) (*api.ToolResult, error) {
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}

	switch {
	case errors.Is(err, httphost.ErrTimeout),
		errors.Is(err, httphost.ErrRequestLimit),
		errors.Is(err, validation.ErrLimit):
		code, message = "EXECUTION_LIMIT_EXCEEDED", "The Tool invocation exceeded an execution limit."
	case errors.Is(err, httphost.ErrResponseLimit),
		errors.Is(err, httphost.ErrStreamLimit),
		errors.Is(err, httphost.ErrHeaderLimit):
		code, message = "OUTPUT_TOO_LARGE", "The upstream response exceeds the response size limits."
	case errors.Is(err, httphost.ErrCredentialsUnavailable):
		code, message = "CONNECTION_UNAVAILABLE", "Credentials for the Tool connection are unavailable."
	case errors.Is(err, httphost.ErrEncoding),
		errors.Is(err, httphost.ErrInvalidResponse):
		code, message = "UPSTREAM_PROTOCOL_ERROR", "The upstream response is invalid or uses an unsupported encoding."
	}

	return p.errorResult(ctx, base, state, code, message)
}
