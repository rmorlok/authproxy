package api_key

import (
	"context"
	"net/http"

	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
)

//go:generate mockgen -source=./interface.go -destination=./mock/api_key.go -package=mock

// ApiKeyConnection is the auth-method-specific view of a connection during
// proxied request handling. Mirrors NoAuthConnection in shape — the only
// outward-facing operations are the two ProxyRequest variants.
type ApiKeyConnection interface {
	ProxyRequest(ctx context.Context, reqType httpf.RequestType, req *coreIface.ProxyRequest) (*coreIface.ProxyResponse, error)
	ProxyRequestRaw(ctx context.Context, reqType httpf.RequestType, req *coreIface.ProxyRequest, w http.ResponseWriter) error
}

// Factory builds ApiKeyConnection instances for individual connections. One
// factory per core service, shared across all api-key connections.
type Factory interface {
	NewApiKey(connection coreIface.Connection) ApiKeyConnection
}
