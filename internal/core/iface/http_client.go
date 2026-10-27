package iface

import (
	"net/http"

	"github.com/rmorlok/authproxy/internal/httpf"
)

// HTTPClient sends requests through one captured connection using the standard
// HTTP client method signature. Connection.HTTPClient configures attribution;
// each request's context supplies its actor, cancellation, and deadline.
// Implementations retain the connection proxy's credentials, recovery, rate
// limiting, and instrumentation. They do not check caller permissions.
type HTTPClient interface {
	// Do returns headers and a streaming body without consuming the response.
	// It takes ownership of the outgoing body, including on errors before send.
	// The caller must close a returned response body. Requests with no body or
	// GetBody may be retried once after successful credential recovery from 401;
	// other failures do not gain additional retries. See ProxyRequestStream.
	Do(*http.Request) (*http.Response, error)
}

// HTTPClientOptions configures one reusable connection HTTP client. Options are
// captured at construction, so separate clients can attribute traffic differently
// without changing their shared connection. The zero value selects proxy traffic.
type HTTPClientOptions struct {
	// RequestType classifies traffic for middleware and probe acceleration.
	// Empty defaults to httpf.RequestTypeProxy.
	RequestType httpf.RequestType

	// Labels adds request-level labels for rate limits, events, and telemetry.
	// The client copies this map; later caller changes do not affect requests.
	Labels map[string]string
}
