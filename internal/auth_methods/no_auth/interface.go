package no_auth

import (
	"context"
	"net/http"

	connIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/request_log"
)

//go:generate mockgen -source=./interface.go -destination=./mock/noauth.go -package=mock
type NoAuthConnection interface {
	ProxyRequest(ctx context.Context, reqType request_log.RequestType, req *connIface.ProxyRequest) (*connIface.ProxyResponse, error)
	ProxyRequestRaw(ctx context.Context, reqType request_log.RequestType, req *connIface.ProxyRequest, w http.ResponseWriter) error
}
