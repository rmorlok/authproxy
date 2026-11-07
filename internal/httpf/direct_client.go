package httpf

import (
	"net/http"

	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
)

// DirectClientOptions captures attribution for HTTP that does not use a
// connection. The caller chooses every field before exposing the capability;
// request headers and editable JavaScript metadata cannot replace this snapshot.
type DirectClientOptions struct {
	// Namespace owns the request events and must be explicitly nonempty.
	Namespace string

	// RequestType is an explicit, valid traffic classification for middleware.
	RequestType RequestType

	// Actor contributes captured actor identity and labels. Nil explicitly means
	// unattributed background traffic; the request context supplies no fallback.
	Actor Actor

	// Labels supplies copied user labels. As with ForLabels, protected apxy/
	// entries are ignored and cannot replace captured actor attribution.
	Labels map[string]string
}

// DirectClient bridges the instrumented factory to the shared HTTP budget
// contract. It performs ordinary HTTP with no connection credentials, lookup,
// or authentication recovery. Create it with NewDirectClient; the zero value
// rejects requests. It may be shared by executions, each with its own Session.
type DirectClient struct {
	// client owns this capability's immutable client configuration. Its factory
	// transport retains the caller's middleware and is never closed or mutated.
	client *http.Client
}

// NewDirectClient captures an explicitly supplied factory and attribution
// without performing I/O or creating a default factory. It clears inherited
// connection, connector, actor, and per-connection policy before constructing
// the middleware stack. Client creation snapshots actor/user labels and keeps
// the original factory reusable. Invalid options return a fixed safe error.
func NewDirectClient(factory F, options DirectClientOptions) (*DirectClient, error) {
	if factory == nil || options.Namespace == "" || options.RequestType.Validate() != nil {
		return nil, aphttp.ErrInvalidInput
	}
	client := factory.ForRequestInfo(RequestInfo{
		Namespace: options.Namespace,
		Type:      options.RequestType,
	}).ForActor(options.Actor).ForLabels(options.Labels).NewHTTPClient()
	if client == nil {
		return nil, aphttp.ErrInvalidInput
	}
	return &DirectClient{client: client}, nil
}

// Do takes request-body ownership and preserves stock client response/body
// semantics, including a response returned together with an error. The shared
// HTTP Session owns every returned body. Redirects share one evidence tracker;
// a middleware rejection can prove notSent while a target response proves sent.
// Private transport diagnostics are discarded. No connection-auth retry runs.
func (c *DirectClient) Do(req *http.Request) (*http.Response, aphttp.SendState, error) {
	if c == nil || c.client == nil || req == nil || req.URL == nil {
		if req != nil && req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, aphttp.SendStateNotSent, aphttp.ErrInvalidInput
	}
	response, observed, err := DoWithSendState(c.client, req)
	state := aphttp.SendStateMayHaveSent
	switch observed {
	case SendStateNotSent:
		state = aphttp.SendStateNotSent
	case SendStateSent:
		state = aphttp.SendStateSent
	}
	if err != nil {
		err = aphttp.ErrTransport
	}
	return response, state, err
}
