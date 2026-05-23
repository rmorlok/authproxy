package oauth2

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// resolveClientCredentials reads the connector's client_id and (when present)
// client_secret. For token_endpoint_auth_method == "none" the secret is
// optional — public clients have no secret to send — so its absence is not
// an error.
func (o *oAuth2Connection) resolveClientCredentials(ctx context.Context) (string, string, error) {
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

// applyTokenEndpointClientAuth attaches client credentials to the
// token-endpoint request per the connector's configured
// token_endpoint_auth_method (RFC 6749 §2.3.1, RFC 7591 §2).
//
// Returns the (possibly augmented) form values that should be sent in the
// request body, and the value of the Authorization header to set on the
// request (empty string when no header is required).
//
// Method semantics:
//   - client_secret_post (default): client_id + client_secret go in the form
//     body, no Authorization header.
//   - client_secret_basic: Authorization: Basic base64(qs(id):qs(secret)) is
//     set; neither client_id nor client_secret goes in the form body.
//     RFC 6749 §2.3.1 forbids sending the credentials in two places.
//   - none: client_id only in the form body, no client_secret, no header.
//     The token endpoint relies on PKCE for proof-of-possession.
//
// The values map is treated as input-only and not mutated; the returned
// url.Values is a copy with the appropriate fields set.
func applyTokenEndpointClientAuth(
	method sconfig.TokenEndpointAuthMethod,
	clientId, clientSecret string,
	values url.Values,
) (url.Values, string, error) {
	// Effective default — empty method behaves as client_secret_post for
	// backwards compatibility.
	if method == "" {
		method = sconfig.TokenEndpointAuthClientSecretPost
	}

	out := make(url.Values, len(values)+2)
	for k, v := range values {
		out[k] = v
	}

	switch method {
	case sconfig.TokenEndpointAuthClientSecretPost:
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
		out.Set("client_id", clientId)
		return out, "", nil
	default:
		return nil, "", fmt.Errorf("unsupported token_endpoint_auth_method %q", method)
	}
}
