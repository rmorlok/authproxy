package core

import (
	"context"
	"errors"
	"net/http"

	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/proxy"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

var ErrProxyNotImplemented = errors.New("auth type for connection does not implement proxy")

// getProxyImpl resolves the connection's auth type to an Authenticator and
// constructs the internal/proxy orchestrator that runs proxied requests
// through it. The per-auth-method packages describe credential application
// (Authenticator); orchestration — build, send, retry-on-401 — lives in
// internal/proxy.
func (c *connection) getProxyImpl() (iface.Proxy, error) {
	c.proxyImplOnce.Do(func() {
		def, err := c.connector.getDefinition()
		if err != nil {
			c.proxyImplErr = err
			return
		}

		auth, err := c.resolveAuthenticator(def)
		if err != nil {
			c.proxyImplErr = err
			return
		}

		c.proxyImpl = proxy.New(c.s.httpf, c, auth, c.s)
	})

	return c.proxyImpl, c.proxyImplErr
}

func (c *connection) resolveAuthenticator(
	connector *cschema.ConnectorDefinition,
) (auth_methods.Authenticator, error) {
	factory := c.s.getAuthMethodFactory(connector)
	if factory == nil {
		return nil, ErrProxyNotImplemented
	}
	return factory.NewAuthenticator(c), nil
}

// ProxyRequest initializes the connection proxy and delegates a buffered target
// request. Initialization errors report no target send, including cached errors;
// evidence from an earlier attempt in the same observation is preserved.
func (c *connection) ProxyRequest(
	ctx context.Context,
	reqType httpf.RequestType,
	req *iface.ProxyRequest,
) (*iface.ProxyResponse, error) {
	p, err := c.getProxyImpl()
	if err != nil {
		httpf.ReportSendEvidence(ctx, false)
		return nil, err
	}

	return p.ProxyRequest(ctx, reqType, req)
}

// ProxyRequestRaw initializes the connection proxy and streams the target
// response into w. Initialization errors report no target send and close the
// outgoing body without reading it or writing a downstream response.
func (c *connection) ProxyRequestRaw(
	ctx context.Context,
	reqType httpf.RequestType,
	req *iface.RawProxyRequest,
	w http.ResponseWriter,
) error {
	p, err := c.getProxyImpl()
	if err != nil {
		httpf.ReportSendEvidence(ctx, false)
		closeRawProxyRequestBody(req)
		return err
	}

	return p.ProxyRequestRaw(ctx, reqType, req, w)
}

// ProxyRequestStream initializes the connection proxy and returns a target
// response whose body the caller owns. Initialization errors report no target
// send and close the outgoing body without reading it.
func (c *connection) ProxyRequestStream(
	ctx context.Context,
	reqType httpf.RequestType,
	req *iface.RawProxyRequest,
) (*http.Response, error) {
	p, err := c.getProxyImpl()
	if err != nil {
		httpf.ReportSendEvidence(ctx, false)
		closeRawProxyRequestBody(req)
		return nil, err
	}

	return p.ProxyRequestStream(ctx, reqType, req)
}

func closeRawProxyRequestBody(req *iface.RawProxyRequest) {
	if req != nil && req.Outbound != nil && req.Outbound.Body != nil {
		_ = req.Outbound.Body.Close()
	}
}
