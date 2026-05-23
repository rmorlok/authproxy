package core

import (
	"context"
	"errors"
	"net/http"

	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/auth_methods/no_auth"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/proxy"
	"github.com/rmorlok/authproxy/internal/schema/config"
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

		auth, err := c.resolveAuthenticator(def.Auth.Inner())
		if err != nil {
			c.proxyImplErr = err
			return
		}

		c.proxyImpl = proxy.New(c.s.httpf, c, auth, c.s)
	})

	return c.proxyImpl, c.proxyImplErr
}

func (c *connection) resolveAuthenticator(authConfig any) (auth_methods.Authenticator, error) {
	switch authConfig.(type) {
	case *config.AuthOAuth2:
		return c.s.getOAuth2Factory().NewAuthenticator(c), nil
	case *config.AuthApiKey:
		return c.s.getApiKeyFactory().NewAuthenticator(c), nil
	case *config.AuthNoAuth:
		return no_auth.NewAuthenticator(), nil
	}
	return nil, ErrProxyNotImplemented
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
