package ratelimit

import (
	"net/url"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// RequestContext is the per-request input to rate-limit evaluation. The
// proxy enforcement layer (#223) populates this from httpf.RequestInfo plus
// the resolved upstream URL and the actor identified by the auth layer.
//
// Construction is intentionally explicit (rather than wrapping
// httpf.RequestInfo) so the evaluator stays low in the import graph and is
// trivially constructible in tests.
type RequestContext struct {
	// Type identifies what kind of traffic this request represents. Compared
	// against the rule's Selector.RequestTypes (default [proxy, probe] when
	// the rule omits the field).
	Type common.RequestType

	// Method is the HTTP verb (canonical upper-case form, e.g. "GET").
	Method string

	// UpstreamURL is the *final* URL being sent to the 3rd party, after any
	// connector-level templating/rewriting. PathMatch evaluates against
	// UpstreamURL.Path. May be nil if the rule's PathMatch clause is empty.
	UpstreamURL *url.URL

	// Namespace is the namespace path the request is being made in.
	Namespace string

	// ActorID is the actor that owns the connection / initiated the call.
	// Empty for system / unauthenticated traffic — bucket dimensions
	// referencing "actor" resolve to "" in that case.
	ActorID apid.ID

	// ConnectionID, ConnectorID, ConnectorVersion identify the connection
	// being proxied through (where applicable). Zero values for traffic
	// that doesn't traverse a connection.
	ConnectionID     apid.ID
	ConnectorID      apid.ID
	ConnectorVersion uint64

	// Labels is the per-request label snapshot the labelSelector clause is
	// evaluated against. Carry-forward / system labels are expected to
	// already be merged in by the caller.
	Labels map[string]string
}
