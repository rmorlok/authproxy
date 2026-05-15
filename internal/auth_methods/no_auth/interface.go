package no_auth

import (
	"context"

	connIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
)

//go:generate mockgen -source=./interface.go -destination=./mock/noauth.go -package=mock
type NoAuthConnection interface {
	ProxyRequest(ctx context.Context, reqType httpf.RequestType, req *connIface.ProxyRequest) (*connIface.ProxyResponse, error)
}
