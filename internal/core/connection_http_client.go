package core

import (
	"errors"
	"net/http"
	"sync"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// connectionHTTPClient defers resolving the connection's proxy until the first
// request, then delegates every call to one proxy-level client whose factory
// captures attribution once and builds middleware on demand. Per-request labels
// derive from that snapshot. No context or response state is shared between calls.
type connectionHTTPClient struct {
	// connection is the only destination capability available to this client.
	connection *connection

	// options are validated and detached at construction; they never change.
	options iface.HTTPClientOptions

	// once guards resolving the connection proxy and building its client.
	once sync.Once
	// client is the proxy-level client, set by once when initialization succeeds.
	client iface.HTTPClient
	// err records a failed initialization; every later Do returns it.
	err error
}

// HTTPClient validates and copies options without resolving credentials or
// sending requests. It fails only when options lack an explicit actor choice.
// The returned client snapshots actor and connection labels and rate-limit
// configuration on first use, so it is intended for one invocation or one
// inbound request rather than long-lived caching.
func (c *connection) HTTPClient(options iface.HTTPClientOptions) (iface.HTTPClient, error) {
	normalized, err := options.Normalize()
	if err != nil {
		return nil, err
	}
	return &connectionHTTPClient{connection: c, options: normalized}, nil
}

// Do resolves the connection proxy on first use, then sends through its
// captured client. It owns req.Body on every path, including initialization
// failures before send. Nil requests fail before any initialization.
func (c *connectionHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.DoWithLabels(req, nil)
}

// DoWithLabels validates user labels before resolving the proxy, then delegates
// to its captured client. Nil or empty labels reuse the ordinary Do path. Invalid
// labels release the outgoing body and report no send without initializing or
// changing the client; valid labels affect only this logical request.
func (c *connectionHTTPClient) DoWithLabels(req *http.Request, labels map[string]string) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("connection HTTP client requires a request")
	}
	if err := meta.ValidateUserLabels(labels); err != nil {
		httpf.ReportSendEvidence(req.Context(), false)
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, iface.ErrHTTPClientInvalidLabels
	}

	c.once.Do(func() {
		proxy, err := c.connection.getProxyImpl()
		if err != nil {
			c.err = err
			return
		}
		c.client, c.err = proxy.HTTPClient(c.options)
	})
	if c.err != nil {
		// Initialization failed before any target request could reach the proxy
		// client, including when this error was cached by an earlier call.
		httpf.ReportSendEvidence(req.Context(), false)
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, c.err
	}

	if len(labels) == 0 {
		return c.client.Do(req)
	}
	return c.client.DoWithLabels(req, labels)
}
