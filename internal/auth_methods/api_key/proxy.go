package api_key

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

// ProxyRequest authenticates an outbound proxied request with the connection's
// active api-key credential and returns the upstream response.
//
// Each call loads the active credential fresh from the database so that a
// rotation (which inserts a new row and soft-deletes the prior) takes effect
// immediately on the next request — no in-memory cache to invalidate.
func (a *apiKeyConnection) ProxyRequest(ctx context.Context, reqType httpf.RequestType, req *iface.ProxyRequest) (*iface.ProxyResponse, error) {
	app, err := a.resolveAuth(ctx)
	if err != nil {
		return nil, err
	}

	r := a.httpf.
		ForRequestType(reqType).
		ForConnection(a.connection).
		ForActor(apauthcore.ActorFromContext(ctx)).
		ForLabels(req.Labels).
		New().
		UseContext(ctx).
		Request()

	req.Apply(r)
	if app.HeaderName != "" {
		r.SetHeader(app.HeaderName, app.HeaderValue)
	}
	if app.QueryName != "" {
		r.SetQuery(app.QueryName, app.QueryValue)
	}

	resp, err := r.Do()
	if err != nil {
		return nil, err
	}
	return iface.ProxyResponseFromGentlemen(resp)
}

// ProxyRequestRaw is not yet implemented for api-key. Mirrors the current
// state of no_auth — when the streaming-proxy path is added, both auth methods
// will adopt the same pattern.
func (a *apiKeyConnection) ProxyRequestRaw(ctx context.Context, reqType httpf.RequestType, req *iface.ProxyRequest, w http.ResponseWriter) error {
	return nil
}

// resolveAuth fetches the active credential row, decrypts the plaintext, and
// computes the authentication application for the placement that was in effect
// when the credential was submitted. The placement_snapshot stored on the
// credential row is used in preference to the live connector definition so an
// in-flight YAML change can't break previously-submitted credentials.
func (a *apiKeyConnection) resolveAuth(ctx context.Context) (authApplication, error) {
	cred, err := a.db.GetActiveApiKeyCredential(ctx, a.connection.GetId())
	if err != nil {
		return authApplication{}, fmt.Errorf("failed to load api-key credential: %w", err)
	}

	placement := cred.PlacementSnapshot
	if placement == nil {
		// Fall back to the connector definition's placement when the credential
		// row didn't snapshot one (rows inserted before snapshot wiring shipped).
		// In current code Connector.Normalize materializes placement and
		// InsertApiKeyCredential captures it, so this fallback should rarely
		// trigger — but it keeps the proxy resilient if the credential row
		// predates snapshot capture.
		cv := a.connection.GetConnectorVersionEntity()
		if cv == nil {
			return authApplication{}, errors.New("connector version missing for api-key proxy request")
		}
		def := cv.GetDefinition()
		if def == nil || def.Auth == nil {
			return authApplication{}, errors.New("connector definition missing for api-key proxy request")
		}
		ak, ok := def.Auth.Inner().(*cschema.AuthApiKey)
		if !ok {
			return authApplication{}, fmt.Errorf("expected AuthApiKey, got %T", def.Auth.Inner())
		}
		placement = ak.Placement
	}

	decrypted, err := a.encrypt.DecryptString(ctx, cred.EncryptedCredentials)
	if err != nil {
		return authApplication{}, fmt.Errorf("failed to decrypt api-key credential: %w", err)
	}
	var plaintext database.ApiKeyCredentialPlaintext
	if err := json.Unmarshal([]byte(decrypted), &plaintext); err != nil {
		return authApplication{}, fmt.Errorf("failed to unmarshal api-key plaintext: %w", err)
	}

	return computeAuthApplication(placement, plaintext)
}
