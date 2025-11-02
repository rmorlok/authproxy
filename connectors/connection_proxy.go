package connectors

import (
	"context"
	"net/http"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/auth_methods/no_auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/connectors/iface"
	"github.com/rmorlok/authproxy/request_log"
)

var ErrProxyNotImplemented = errors.New("auth type for connection does not implement proxy")

// getProxyImpl gets the object that implements proxy for this connection's connector version. This will depend on the
// definition's auth type to delegate to an appropriate implementation.
func (c *connection) getProxyImpl() (iface.Proxy, error) {
	c.proxyImplOnce.Do(func() {
		def, err := c.cv.getDefinition()
		if err != nil {
			c.proxyImplErr = err
			return
		}

		auth := def.Auth
		if _, ok := auth.(*config.AuthOAuth2); ok {
			o2f := c.s.getOAuth2Factory()
			c.proxyImpl = o2f.NewOAuth2(c.Connection, c.cv)
			c.proxyImplErr = nil
			return
		}

		if auth, ok := auth.(*config.AuthNoAuth); ok {
			c.proxyImpl = no_auth.NewNoAuth(c.s.logger, c.s.httpf, auth, c.Connection, c.cv)
			c.proxyImplErr = nil
			return
		}

		c.proxyImplErr = ErrProxyNotImplemented
	})

	return c.proxyImpl, c.proxyImplErr
}

func (c *connection) ProxyRequest(
	ctx context.Context,
	reqType request_log.RequestType,
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
	reqType request_log.RequestType,
	req *iface.ProxyRequest,
	w http.ResponseWriter,
) error {
	p, err := c.getProxyImpl()
	if err != nil {
		return err
	}

	return p.ProxyRequestRaw(ctx, reqType, req, w)
}
