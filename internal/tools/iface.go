// Package tools defines the shared internal contract between Tool admission and
// compiled executors. Executor implementations live in its child packages.
package tools

import (
	"context"
	"log/slog"
	"time"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/render"
)

// Invocation combines the Tool caller's arguments with the connection and Tool
// context captured by admission. The admission layer must associate the program,
// snapshot, and configured client with the same authorized Tool revision and
// connection. This is an internal input, not an API request type.
type Invocation struct {
	ID string // Captured tiv_ correlation identity.

	// Deadline carries the absolute policy deadline established during admission,
	// so preparation time consumes the same allowance as execution. A zero value
	// starts the timeout when Execute begins. The compiled timeout and any earlier
	// caller deadline still apply; this value cannot extend either of them.
	Deadline time.Time

	// Arguments is the JSON object supplied by the Tool caller for this call.
	// For a contact lookup, it might be {"id":"contact-42","limit":10}.
	// Execute decodes and validates it against the captured Tool's input schema
	// before rendering or I/O, then exposes the validated values as params.
	// Templates can use {{path.params.id}} or {{params.limit}}; response
	// projections use the same values. JavaScript receives params as the first
	// execute argument. Validation preserves exact JSON numbers and does not
	// insert defaults or coerce types; JavaScript then requires finite Number
	// values and uses JavaScript's numeric precision.
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
	// Execute copies these values and the validated arguments into an immutable
	// render context. JavaScript receives its own mutable native copy: cfg.tenant
	// becomes context.connections[0].cfg.tenant, while params.id is "contact-42".
	// Mutating that copy cannot change the configured client's connection.
	// Leave Params unset; Execute replaces it with validated Arguments.
	// Tool/connection identities and revision metadata also populate the result.
	// Authentication comes from the configured HTTP client; credentials must
	// never be placed in Cfg or other snapshot fields.
	Snapshot render.Input

	// Logger supplies the caller's logging configuration and captured attribution
	// for fixed JavaScript HTTP-worker diagnostics. Nil uses the adapter's default
	// logger; the declarative HTTP executor currently emits no such diagnostics.
	// This does not enable authored console logging or expose request data, panic
	// values, or credentials.
	Logger *slog.Logger
}

// Executor runs a compiled Tool against a matching admitted connection. The
// implementation owns its captured definition and effective policy; admission
// owns authorization, readiness, and the association between program and client.
type Executor interface {
	// Execute validates arguments before outbound work, applies the earlier
	// caller/policy deadline, and returns a bounded result with trusted send
	// evidence. Caller cancellation and internal failures may return a Go error
	// instead. Keep invocation data stable during the call; results are detached
	// from the input, and a compiled executor supports concurrent invocations.
	Execute(
		ctx context.Context,
		client iface.HTTPClient,
		invocation Invocation,
	) (*api.ToolResult, error)
}
