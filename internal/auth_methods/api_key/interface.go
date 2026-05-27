package api_key

import (
	"context"

	"github.com/rmorlok/authproxy/internal/auth_methods"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

//go:generate mockgen -source=./interface.go -destination=./mock/api_key.go -package=mock

// Factory builds the Authenticator for an api-key connection and owns the
// api-key-specific credential lifecycle (persistence of credentials submitted
// during setup; rotation is handled by InsertApiKeyCredential's
// soft-delete + insert behavior). One factory per core service, shared
// across all api-key connections.
type Factory interface {
	NewAuthenticator(connection coreIface.Connection) auth_methods.Authenticator

	// PersistCredentials extracts the api-key credential field values from
	// credData (the validated form payload from a credentials-phase submit),
	// encrypts them as a single JSON blob, and inserts the row into
	// api_key_credentials. The actor on the request context is recorded as
	// the credential's creator. Validation of which fields are present is
	// placement-specific (e.g. basic placement requires the username field).
	//
	// Returns an *httperr.Err on user-visible errors (e.g. missing required
	// field), which is the contract the HTTP route relies on.
	PersistCredentials(
		ctx context.Context,
		connection coreIface.Connection,
		placement *cschema.ApiKeyPlacement,
		credData map[string]any,
	) error
}
