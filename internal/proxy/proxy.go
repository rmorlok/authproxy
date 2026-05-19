// Package proxy orchestrates a single proxied request through a connection:
// resolve credentials via the auth method's Authenticator, send the request
// through the httpf client (which carries rate-limit / telemetry /
// request-log middleware), and on a 401 from the upstream attempt the
// retry-once-after-recover dance. Owns ProxyRequest and (in #330)
// ProxyRequestRaw so the per-auth-method packages only have to describe
// "how to apply this credential to a request" — not how to drive a proxy
// call.
package proxy

import (
	"context"
	"errors"
	"net/http"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
	gentleman "gopkg.in/h2non/gentleman.v2"
)

type proxy struct {
	httpf httpf.F
	conn  iface.Connection
	auth  auth_methods.Authenticator
}

// New constructs an iface.Proxy that orchestrates calls for a single
// connection using the supplied Authenticator. One instance per
// connection — held inside the connection's lazy proxy-impl cache.
func New(h httpf.F, conn iface.Connection, auth auth_methods.Authenticator) iface.Proxy {
	return &proxy{httpf: h, conn: conn, auth: auth}
}

// ProxyRequest resolves credentials, sends the request, and on a 401
// from the upstream attempts to recover (e.g. refresh an OAuth2 token)
// and replay the request exactly once.
//
// If RecoverFrom401 returns auth_methods.ErrCannotRecover the upstream
// 401 is returned to the caller unchanged. If recovery fails for any
// other reason the original 401 is also returned unchanged — the
// customer's app sees the same auth failure it would have without this
// retry path, and the recovery failure was already classified inside
// the authenticator.
func (p *proxy) ProxyRequest(ctx context.Context, reqType httpf.RequestType, req *iface.ProxyRequest) (*iface.ProxyResponse, error) {
	resp, err := p.send(ctx, reqType, req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		recoverErr := p.auth.RecoverFrom401(ctx)
		if recoverErr == nil {
			retried, retryErr := p.send(ctx, reqType, req)
			if retryErr == nil {
				resp = retried
			}
		} else if !errors.Is(recoverErr, auth_methods.ErrCannotRecover) {
			// Refresh failed for a recoverable auth method. Surface the
			// original 401; the recovery failure already self-reported.
			_ = recoverErr
		}
	}

	return iface.ProxyResponseFromGentlemen(resp)
}

// ProxyRequestRaw is the streaming proxy path. Implemented in #330; for
// now this preserves the prior stub behavior so callers that touch the
// path during the refactor see no change.
func (p *proxy) ProxyRequestRaw(ctx context.Context, reqType httpf.RequestType, req *iface.ProxyRequest, w http.ResponseWriter) error {
	return nil
}

// send builds a fresh gentleman request with the resolved credential
// applied and sends it. Split out so the retry-once-after-recover path
// can construct a new request rather than mutate the existing one —
// gentleman requests are single-use (Send panics on the second call).
//
// Order is deliberate: caller-supplied headers go on first (via
// ProxyRequest.Apply), then the authenticator's headers via SetHeader
// so the credential always wins. Same for query params.
func (p *proxy) send(ctx context.Context, reqType httpf.RequestType, req *iface.ProxyRequest) (*gentleman.Response, error) {
	app, err := p.auth.Resolve(ctx)
	if err != nil {
		return nil, err
	}

	r := p.httpf.
		ForRequestType(reqType).
		ForConnection(p.conn).
		ForActor(apauthcore.ActorFromContext(ctx)).
		ForLabels(req.Labels).
		New().
		UseContext(ctx).
		Request()

	req.Apply(r)
	for h, v := range app.Headers {
		r.SetHeader(h, v)
	}
	for k, v := range app.QueryParams {
		r.SetQuery(k, v)
	}
	return r.Do()
}
