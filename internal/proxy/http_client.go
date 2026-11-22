package proxy

import (
	"errors"
	"net/http"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// proxyHTTPClient is a connection HTTP client whose factory captures attribution.
// Its middleware stack is built lazily and reused for calls without extra labels.
// Labeled calls derive a private stack from that same snapshot. Neither path
// rereads actor or connection metadata, so concurrent calls cannot change one
// another's attribution. Clients are intended to be short-lived.
type proxyHTTPClient struct {
	// proxy supplies credentials, 401 recovery, probe acceleration, and logging.
	proxy *proxy

	// requestType classifies every request for middleware and probe acceleration.
	requestType httpf.RequestType

	// actor is the validated attribution choice used to check request contexts.
	actor iface.HTTPClientActor

	// factory retains the resolved connection, actor, policy, and base labels.
	// Per-call labels derive another factory rather than mutating this snapshot
	// or resolving attribution again from live connection/actor objects.
	factory httpf.F
}

// HTTPClient validates options and captures attribution in a factory. Middleware
// is built on demand; calls with extra labels derive their own stack from that
// snapshot. Construction performs no I/O: credentials are resolved per request.
// The returned client is safe for concurrent use.
func (p *proxy) HTTPClient(options iface.HTTPClientOptions) (iface.HTTPClient, error) {
	// Normalize captures the actor's id here, immediately before ForActor
	// snapshots the same actor, so Do's agreement check compares against the
	// identity the middleware attributes even if the actor object later changes.
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

	factory := p.httpf.
		ForRequestType(options.RequestType).
		ForConnection(p.conn).
		ForActor(actor).
		ForLabels(options.Labels)

	return &proxyHTTPClient{
		proxy:       p,
		requestType: options.RequestType,
		actor:       options.Actor,
		factory:     factory,
	}, nil
}

// Do sends req through the factory's cached middleware stack with the request's own
// context for cancellation and deadlines. It owns req.Body on every path,
// including validation and actor-mismatch failures that happen before send.
func (c *proxyHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.DoWithLabels(req, nil)
}

// DoWithLabels validates per-call user labels before resolving credentials and
// sends through a private middleware stack derived from captured attribution.
// The same client handles every retry/redirect of this request. Empty labels reuse
// the factory's cached stack; labeled stacks are not shared with other calls.
func (c *proxyHTTPClient) DoWithLabels(req *http.Request, labels map[string]string) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("connection HTTP client requires a request")
	}
	if req.URL == nil {
		closeRawRequestBody(req)
		httpf.ReportSendEvidence(req.Context(), false)
		return nil, errors.New("connection HTTP client requires a request URL")
	}

	// The client's actor is authoritative for attribution. A request context
	// naming a different actor signals a caller mixing identities, so refuse it
	// rather than silently attributing the request to someone else.
	if !c.actor.Agrees(apauthcore.ActorFromContext(req.Context())) {
		closeRawRequestBody(req)
		httpf.ReportSendEvidence(req.Context(), false)
		return nil, iface.ErrHTTPClientActorMismatch
	}

	if err := meta.ValidateUserLabels(labels); err != nil {
		closeRawRequestBody(req)
		httpf.ReportSendEvidence(req.Context(), false)
		return nil, iface.ErrHTTPClientInvalidLabels
	}

	// ForLabels copies and merges nonempty labels, or returns the same factory
	// for empty labels. NewHTTPClient reuses that factory's cached transport, so
	// only the lightweight client wrapper is new on each unlabeled call. Keep
	// one client for the entire logical request, including recovery/redirects.
	client := c.factory.ForLabels(labels).NewHTTPClient()

	return c.proxy.streamWithClient(req.Context(), c.requestType, client, req)
}
