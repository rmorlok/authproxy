package no_auth

import (
	"context"
	"net/http"

	"github.com/rmorlok/authproxy/core/iface"
	"github.com/rmorlok/authproxy/request_log"
)

func (n *noAuthConnection) ProxyRequest(ctx context.Context, reqType request_log.RequestType, req *iface.ProxyRequest) (*iface.ProxyResponse, error) {
	r := n.httpf.
		ForRequestType(reqType).
		ForConnection(&n.c).
		ForConnectorVersion(n.cv).
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

func (n *noAuthConnection) ProxyRequestRaw(ctx context.Context, reqType request_log.RequestType, req *iface.ProxyRequest, w http.ResponseWriter) error {
	return nil
}

var _ iface.Proxy = (*noAuthConnection)(nil)
