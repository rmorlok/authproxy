package iface

import (
	"errors"
	"maps"
	"net/http"
	"reflect"

	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/httpf"
)

var (
	// ErrHTTPClientActorRequired rejects options whose Actor is the zero value
	// or an actor without an identity. Callers must choose ActingAs or
	// WithoutActor explicitly so unattributed traffic is never accidental.
	ErrHTTPClientActorRequired = errors.New("connection HTTP client requires an explicit actor choice")

	// ErrHTTPClientActorMismatch rejects a request whose context carries an
	// actor that disagrees with the client's configured actor. The outgoing
	// body is closed and nothing is sent.
	ErrHTTPClientActorMismatch = errors.New("request context actor does not match the connection HTTP client actor")
)

// HTTPClient sends requests through one captured connection using the standard
// HTTP client method signature. Connection.HTTPClient fixes the request type,
// labels, and actor attribution for every request; each request's context only
// supplies cancellation and deadlines. Implementations retain the connection
// proxy's credentials, recovery, rate limiting, and instrumentation. They do
// not check caller permissions.
//
// A client snapshots the actor's labels and the connection's labels and
// rate-limit configuration when its middleware stack is built. It is meant to
// be short-lived (one invocation or one inbound request); create a new client
// to observe later label or configuration changes.
type HTTPClient interface {
	// Do returns headers and a streaming body without consuming the response.
	// It takes ownership of the outgoing body, including on errors before send.
	// The caller must close a returned response body. Requests with no body or
	// GetBody may be retried once after successful credential recovery from 401;
	// other failures do not gain additional retries. See ProxyRequestStream.
	//
	// A request whose context carries an actor different from the client's
	// actor, or any actor when the client was built WithoutActor, fails with
	// ErrHTTPClientActorMismatch. A context without an actor is accepted.
	Do(*http.Request) (*http.Response, error)
}

// HTTPClientActor is the explicit attribution choice for a connection HTTP
// client: either a specific actor (ActingAs) or a deliberate opt-out
// (WithoutActor) for background work such as probes. The zero value is
// neither and is rejected, so forgetting to choose cannot silently produce
// unattributed traffic. WithoutActor is not a system actor: it has no
// identity, permissions, or actor-scoped rate limits; requests simply carry
// no actor attribution.
type HTTPClientActor struct {
	// kind records which branch of the union was chosen; zero means neither.
	kind httpClientActorKind
	// actor is the attributed actor for ActingAs; always nil for WithoutActor.
	actor authcore.IActorData
}

// httpClientActorKind tags the HTTPClientActor union. The zero value is unset.
type httpClientActorKind uint8

const (
	httpClientActorUnset httpClientActorKind = iota // Zero value: no choice made.
	httpClientActorAs                               // ActingAs: attribute to one actor.
	httpClientActorNone                             // WithoutActor: explicit opt-out.
)

// ActingAs attributes every request from the client to actor. The actor must
// be non-nil (including not a typed-nil pointer) with a non-nil id; otherwise
// client construction fails with ErrHTTPClientActorRequired. ActingAs(nil) is
// never treated as WithoutActor.
func ActingAs(actor authcore.IActorData) HTTPClientActor {
	return HTTPClientActor{kind: httpClientActorAs, actor: actor}
}

// WithoutActor explicitly opts out of actor attribution, for background work
// that no actor initiated. Requests are rejected if their context carries an
// actor, since that would indicate actor-initiated work using the opt-out.
func WithoutActor() HTTPClientActor {
	return HTTPClientActor{kind: httpClientActorNone}
}

// Actor returns the attributed actor, or nil for WithoutActor and the zero value.
func (a HTTPClientActor) Actor() authcore.IActorData {
	if a.kind != httpClientActorAs {
		return nil
	}
	return a.actor
}

// IsWithoutActor reports whether the choice is the explicit WithoutActor opt-out.
func (a HTTPClientActor) IsWithoutActor() bool {
	return a.kind == httpClientActorNone
}

// Validate returns ErrHTTPClientActorRequired for the zero value and for
// ActingAs with a nil actor (including a typed-nil pointer) or a nil id.
func (a HTTPClientActor) Validate() error {
	switch a.kind {
	case httpClientActorNone:
		return nil
	case httpClientActorAs:
		if a.actor == nil {
			return ErrHTTPClientActorRequired
		}
		// A typed-nil pointer wrapped in the interface would panic on GetId.
		if value := reflect.ValueOf(a.actor); value.Kind() == reflect.Pointer && value.IsNil() {
			return ErrHTTPClientActorRequired
		}
		if a.actor.GetId().IsNil() {
			return ErrHTTPClientActorRequired
		}
		return nil
	default:
		return ErrHTTPClientActorRequired
	}
}

// Agrees reports whether a request context's actor is consistent with this
// choice. A context without an actor always agrees. Otherwise ActingAs
// requires matching ids, and WithoutActor never agrees with a context actor.
// The caller must have validated the choice first.
func (a HTTPClientActor) Agrees(contextActor authcore.IActorData) bool {
	if contextActor == nil {
		return true
	}
	if a.kind != httpClientActorAs {
		return false
	}
	return a.actor.GetId() == contextActor.GetId()
}

// HTTPClientOptions configures one reusable connection HTTP client. Options are
// captured at construction, so separate clients can attribute traffic differently
// without changing their shared connection. Actor is required; the remaining
// zero values select proxy traffic with no extra labels.
type HTTPClientOptions struct {
	// RequestType classifies traffic for middleware and probe acceleration.
	// Empty defaults to httpf.RequestTypeProxy.
	RequestType httpf.RequestType

	// Labels adds request-level labels for rate limits, events, and telemetry.
	// The client copies this map; later caller changes do not affect requests.
	Labels map[string]string

	// Actor attributes every request. Use ActingAs or WithoutActor; the zero
	// value fails construction with ErrHTTPClientActorRequired.
	Actor HTTPClientActor
}

// Normalize validates the options and returns a detached copy with the
// default request type applied and labels cloned, so later caller edits
// cannot reconfigure a client built from them.
func (o HTTPClientOptions) Normalize() (HTTPClientOptions, error) {
	if err := o.Actor.Validate(); err != nil {
		return HTTPClientOptions{}, err
	}
	if o.RequestType == "" {
		o.RequestType = httpf.RequestTypeProxy
	}
	o.Labels = maps.Clone(o.Labels)
	return o, nil
}
