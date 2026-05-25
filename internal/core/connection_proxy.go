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
		def, err := c.cv.getDefinition()
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

func (c *connection) resolveAuthenticator(connector *cschema.Connector) (auth_methods.Authenticator, error) {
	factory := c.s.getAuthMethodFactory(connector)
	if factory == nil {
		return nil, ErrProxyNotImplemented
	}
	return factory.NewAuthenticator(c), nil
}

func (c *connection) ProxyRequest(
	ctx context.Context,
	reqType httpf.RequestType,
	req *iface.ProxyRequest,
) (*iface.ProxyResponse, error) {
	p, err := c.getProxyImpl()
	if err != nil {
		return nil, err
	}

	return p.ProxyRequest(ctx, reqType, req)
}

func (c *connection) ProxyRequestRaw(
	ctx context.Context,
	reqType httpf.RequestType,
	req *iface.RawProxyRequest,
	w http.ResponseWriter,
) error {
	p, err := c.getProxyImpl()
	if err != nil {
		return err
	}

	return p.ProxyRequestRaw(ctx, reqType, req, w)
}
