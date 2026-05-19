package auth_methods

import (
	"context"
	"errors"
)

// ErrCannotRecover is returned by Authenticator.RecoverFrom401 when the auth
// method has no mechanism to obtain new credentials without user
// interaction. The proxy orchestrator treats it as terminal: the 401 is
// returned to the caller unchanged.
var ErrCannotRecover = errors.New("authenticator cannot recover from 401")

// AuthApplication describes how the resolved credential should be applied
// to a single outgoing request. The orchestrator merges these on top of
// the caller-supplied headers/query so the auth credential always wins.
type AuthApplication struct {
	// Headers to set on the outgoing request. Set, not added — the auth
	// credential always replaces any caller-supplied value for these
	// header names.
	Headers map[string]string
	// QueryParams to set on the outgoing request URL.
	QueryParams map[string]string
}

// Authenticator is the auth-method-shaped contract used by the proxy
// orchestrator. Each auth method (oauth2, api_key, no_auth) exposes one
// implementation per connection.
//
// The split between Resolve and RecoverFrom401 mirrors the retry-once-
// after-refresh semantics already exercised by OAuth2: build the request
// with the freshly resolved credential, send it, and if the upstream
// rejects it with 401, recover (refresh) and retry exactly once.
type Authenticator interface {
	// Resolve loads the active credential (refreshing automatically if
	// it is locally known to be expired) and returns the application to
	// apply to the outgoing request. Called once per attempt — including
	// the retry-after-recover attempt — so post-recovery the next
	// Resolve picks up the new credential.
	Resolve(ctx context.Context) (AuthApplication, error)

	// RecoverFrom401 attempts to obtain a new credential after the
	// upstream returned 401. For OAuth2 this is a forced refresh; for
	// auth methods with no recovery mechanism (api_key, no_auth) it
	// returns ErrCannotRecover and the orchestrator surfaces the
	// upstream 401 unchanged.
	RecoverFrom401(ctx context.Context) error
}
