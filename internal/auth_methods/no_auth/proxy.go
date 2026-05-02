package no_auth

import (
	"context"
	"net/http"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
)

func (n *noAuthConnection) ProxyRequest(ctx context.Context, reqType httpf.RequestType, req *iface.ProxyRequest) (*iface.ProxyResponse, error) {
	r := n.httpf.
		ForRequestType(reqType).
		ForConnection(n.c).
		ForActor(actorFromContext(ctx)).
		ForLabels(req.Labels).
		New().
		UseContext(ctx).
		Request()

	req.Apply(r)

	resp, err := r.Do()
	if err != nil {
		return nil, err
	}

	return iface.ProxyResponseFromGentlemen(resp)
}

func (n *noAuthConnection) ProxyRequestRaw(ctx context.Context, reqType httpf.RequestType, req *iface.ProxyRequest, w http.ResponseWriter) error {
	return nil
}

// actorFromContext returns the initiating actor from request context as a
// httpf.Actor, or nil if the request is unauthenticated. The explicit
// authentication check avoids passing a typed-nil *apauthcore.Actor through
// the httpf.Actor interface (which would defeat ForActor's nil-receiver
// short-circuit).
func actorFromContext(ctx context.Context) httpf.Actor {
	auth := apauthcore.GetAuthFromContext(ctx)
	if !auth.IsAuthenticated() {
		return nil
	}
	return auth.GetActor()
}

var _ iface.Proxy = (*noAuthConnection)(nil)
