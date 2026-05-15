package no_auth

import (
	"context"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
)

func (n *noAuthConnection) ProxyRequest(ctx context.Context, reqType httpf.RequestType, req *iface.ProxyRequest) (*iface.ProxyResponse, error) {
	r := n.httpf.
		ForRequestType(reqType).
		ForConnection(n.c).
		ForActor(apauthcore.ActorFromContext(ctx)).
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

var _ iface.Proxy = (*noAuthConnection)(nil)
