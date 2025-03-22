package oauth2

import (
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/util"
	"gopkg.in/h2non/gentleman.v2"
	"strings"
	"time"
)

// tokenResponse is the OAuth response from the authorization token request and the refresh request
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    *int   `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// createDbTokenFromResponse deserializes an oauth token from a refresh or authorization code response. It deserializes
// the response, and then inserts a token into the databse. It returns the newly created token.
func (o *OAuth2) createDbTokenFromResponse(ctx context.Context, resp *gentleman.Response, refreshFrom *database.OAuth2Token) (*database.OAuth2Token, error) {
	jsonResp := tokenResponse{}
	err := resp.JSON(&jsonResp)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse token response")
	}

	if jsonResp.AccessToken == "" {
		return nil, errors.New("no access token in response")
	}

	encryptedAccessToken, err := o.encrypt.EncryptStringForConnection(ctx, o.connection, jsonResp.AccessToken)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to encrypt access token")
	}

	encryptedRefreshToken := ""

	// Not all OAuth has refresh tokens
	if jsonResp.RefreshToken != "" {
		encryptedRefreshToken, err = o.encrypt.EncryptStringForConnection(ctx, o.connection, jsonResp.RefreshToken)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to encrypt refresh token")
		}
	} else if refreshFrom != nil {
		encryptedRefreshToken = refreshFrom.EncryptedRefreshToken
	}

	scopes := strings.Join(util.Map(o.auth.Scopes, func(s config.Scope) string { return s.Id }), " ")
	if jsonResp.Scope != "" {
		scopes = jsonResp.Scope
	}

	var expiresAt *time.Time
	if jsonResp.ExpiresIn != nil {
		expiresAt = util.ToPtr(ctx.Clock().Now().Add(time.Duration(*jsonResp.ExpiresIn) * time.Second))
	}

	token, err := o.db.InsertOAuth2Token(
		ctx,
		o.connection.ID,
		util.ToPtr(refreshFrom.ID),
		encryptedRefreshToken,
		encryptedAccessToken,
		expiresAt,
		scopes,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to insert oauth2 token")
	}

	return token, nil
}
