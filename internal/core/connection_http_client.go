package core

import (
	"errors"
	"maps"
	"net/http"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
)

// connectionHTTPClient binds immutable attribution to one connection. Each Do
// delegates to its streaming proxy, leaving authentication and recovery in their
// existing implementation. No context or response state is shared between calls.
type connectionHTTPClient struct {
	// connection is the only destination capability available to this client.
	connection *connection

	// requestType and labels are the captured attribution for every logical call.
	requestType httpf.RequestType
	labels      map[string]string
}

// HTTPClient creates a reusable client without resolving credentials or sending
// requests. Labels are copied so changing options cannot reconfigure the client.
func (c *connection) HTTPClient(options iface.HTTPClientOptions) iface.HTTPClient {
	requestType := options.RequestType
	if requestType == "" {
		requestType = httpf.RequestTypeProxy
	}

	return &connectionHTTPClient{
		connection:  c,
		requestType: requestType,
		labels:      maps.Clone(options.Labels),
	}
}

// Do sends through the captured connection with the request's own context.
// ProxyRequestStream owns the outgoing body on every delegation path and returns
// a response whose body belongs to the caller. Nil requests fail before handoff.
func (c *connectionHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("connection HTTP client requires a request")
	}

	return c.connection.ProxyRequestStream(req.Context(), c.requestType, &iface.RawProxyRequest{
		Outbound: req,
		// Each handoff owns its map, including when calls execute concurrently.
		Labels: maps.Clone(c.labels),
	})
}
