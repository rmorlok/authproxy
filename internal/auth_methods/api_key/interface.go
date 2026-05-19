package api_key

import (
	"github.com/rmorlok/authproxy/internal/auth_methods"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
)

//go:generate mockgen -source=./interface.go -destination=./mock/api_key.go -package=mock

// Factory builds the Authenticator for an api-key connection. One factory
// per core service, shared across all api-key connections.
type Factory interface {
	NewAuthenticator(connection coreIface.Connection) auth_methods.Authenticator
}
