package oauth2

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// resolveClientCredentials reads the connector's client_id and (when present)
// client_secret. For token_endpoint_auth_method == "none" the secret is
// optional — public clients have no secret to send — so its absence is not
// an error.
func (o *oAuth2Connection) resolveClientCredentials(ctx context.Context) (string, string, error) {
	if o.auth.GetGrantTypeOrDefault() == sconfig.OAuth2GrantClientCredentials {
		return o.resolveStoredClientCredentials(ctx)
	}

	clientId, err := o.auth.ClientId.GetValue(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to get client id for connector: %w", err)
	}

	if o.auth.GetTokenEndpointAuthMethodOrDefault() == sconfig.TokenEndpointAuthNone {
		return clientId, "", nil
	}

	clientSecret, err := o.auth.ClientSecret.GetValue(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to get client secret for connector: %w", err)
	}
	return clientId, clientSecret, nil
}

func (o *oAuth2Connection) resolveStoredClientCredentials(ctx context.Context) (string, string, error) {
	cred, err := o.db.GetActiveApiKeyCredential(ctx, o.connection.GetId())
	if err != nil {
		return "", "", fmt.Errorf("failed to load OAuth2 client credentials: %w", err)
	}

	plaintextJSON, err := o.encrypt.DecryptString(ctx, cred.EncryptedCredentials)
	if err != nil {
		return "", "", fmt.Errorf("failed to decrypt OAuth2 client credentials: %w", err)
	}

	var plaintext database.OAuth2ClientCredentialsPlaintext
	if err := json.Unmarshal([]byte(plaintextJSON), &plaintext); err != nil {
		return "", "", fmt.Errorf("failed to parse OAuth2 client credentials: %w", err)
	}
	if plaintext.ClientId == "" {
		return "", "", fmt.Errorf("OAuth2 client credentials missing client_id")
	}
	if o.auth.GetTokenEndpointAuthMethodOrDefault() != sconfig.TokenEndpointAuthNone &&
		plaintext.ClientSecret == "" {
		return "", "", fmt.Errorf("OAuth2 client credentials missing client_secret")
	}

	return plaintext.ClientId, plaintext.ClientSecret, nil
}

// applyTokenEndpointClientAuth attaches client credentials to the
// token-endpoint request per the connector's configured
// token_endpoint_auth_method (RFC 6749 §2.3.1, RFC 7591 §2).
//
// Returns the (possibly augmented) form values that should be sent in the
// request body, and the value of the Authorization header to set on the
// request (empty string when no header is required).
//
// Callers MUST pass a resolved (non-empty) method — typically by routing
// through AuthOAuth2.GetTokenEndpointAuthMethodOrDefault, which is the
// single place where the nil → client_secret_post default is applied.
// Empty-string is treated as invalid here, not silently defaulted.
//
// The values map is treated as input-only and not mutated; the returned
// url.Values is a copy with the appropriate fields set.
func applyTokenEndpointClientAuth(
	method sconfig.TokenEndpointAuthMethod,
	clientId, clientSecret string,
	values url.Values,
) (url.Values, string, error) {
	out := make(url.Values, len(values)+2)
	for k, v := range values {
		out[k] = v
	}

	switch method {
	case sconfig.TokenEndpointAuthClientSecretPost:
		// RFC 6749 §2.3.1 / RFC 7591 §2 — credentials in the form body.
		// RFC 6749 §2.3.1 marks this NOT RECOMMENDED relative to Basic
		// but it is widely supported and is AuthProxy's historical
		// default, so it stays as the nil → default fallback.
		out.Set("client_id", clientId)
		out.Set("client_secret", clientSecret)
		return out, "", nil
	case sconfig.TokenEndpointAuthClientSecretBasic:
		// RFC 6749 §2.3.1: client_id and client_secret are
		// application/x-www-form-urlencoded before being colon-joined and
		// base64-encoded. The gentleman/std basic-auth helpers do not do
		// this form-encoding step, so we build the header by hand.
		userPass := url.QueryEscape(clientId) + ":" + url.QueryEscape(clientSecret)
		encoded := base64.StdEncoding.EncodeToString([]byte(userPass))
		return out, "Basic " + encoded, nil
	case sconfig.TokenEndpointAuthNone:
		// RFC 7591 §2 — public-client method. client_id identifies the
		// client; no client_secret exists. Proof-of-possession comes from
		// PKCE (RFC 7636), which the connector validator requires when
		// this method is selected.
		out.Set("client_id", clientId)
		return out, "", nil
	default:
		return nil, "", fmt.Errorf("unsupported token_endpoint_auth_method %q", method)
	}
}
