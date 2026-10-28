package proxy

import (
	"errors"
	"net/http"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
)

// proxyHTTPClient is a connection HTTP client whose middleware stack is built
// once from fixed attribution (request type, connection, actor, labels) and
// reused for every Do. Because the actor is part of the client rather than
// read from each request's context, the stack does not depend on per-request
// state. The stack snapshots actor and connection labels and rate-limit
// configuration at construction, so clients are intended to be short-lived.
type proxyHTTPClient struct {
	// proxy supplies credentials, 401 recovery, probe acceleration, and logging.
	proxy *proxy
	// requestType classifies every request for middleware and probe acceleration.
	requestType httpf.RequestType
	// actor is the validated attribution choice used to check request contexts.
	actor iface.HTTPClientActor
	// client wraps the connection's middleware stack, built once at construction.
	client *http.Client
}

// HTTPClient validates options and builds the middleware stack once for the
// client's lifetime. It performs no I/O: credentials are resolved per Do. The
// returned client is safe for concurrent use.
func (p *proxy) HTTPClient(options iface.HTTPClientOptions) (iface.HTTPClient, error) {
	options, err := options.Normalize()
	if err != nil {
		return nil, err
	}

	// Convert explicitly so WithoutActor reaches ForActor as an untyped nil,
	// which httpf treats as "no actor attribution".
	var actor httpf.Actor
	if a := options.Actor.Actor(); a != nil {
		actor = a
	}

	client := p.httpf.
		ForRequestType(options.RequestType).
		ForConnection(p.conn).
		ForActor(actor).
		ForLabels(options.Labels).
		NewHTTPClient()

	return &proxyHTTPClient{
		proxy:       p,
		requestType: options.RequestType,
		actor:       options.Actor,
		client:      client,
	}, nil
}

// Do sends req through the prebuilt middleware stack with the request's own
// context for cancellation and deadlines. It owns req.Body on every path,
// including validation and actor-mismatch failures that happen before send.
func (c *proxyHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("connection HTTP client requires a request")
	}
	if req.URL == nil {
		closeRawRequestBody(req)
		return nil, errors.New("connection HTTP client requires a request URL")
	}

	// The client's actor is authoritative for attribution. A request context
	// naming a different actor signals a caller mixing identities, so refuse it
	// rather than silently attributing the request to someone else.
	if !c.actor.Agrees(apauthcore.ActorFromContext(req.Context())) {
		closeRawRequestBody(req)
		return nil, iface.ErrHTTPClientActorMismatch
	}

	return c.proxy.streamWithClient(req.Context(), c.requestType, c.client, req)
}
