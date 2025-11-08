package oauth2

import (
	"context"
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	mockE "github.com/rmorlok/authproxy/internal/encrypt/mock"
)

// MockOAuthTokenForConnection sets up mocks to return the specified token. The values `EncryptedRefreshToken` and
// `EncryptedAccessToken` should actually be the unencrypted values you want to get back. This method will use fake
// encrypted equivalents and mock the calls to enencrypt them.
func MockOAuthTokenForConnection(ctx context.Context, dbMock *mockDb.MockDB, e *mockE.MockE, token database.OAuth2Token) {
	clock := apctx.GetClock(ctx)

	unencryptedRefreshToken := token.EncryptedRefreshToken
	unencryptedAccessToken := token.EncryptedAccessToken

	encryptedRefreshToken := fmt.Sprintf("%s-encrypted-refresh-token", token.ConnectionID.String())
	encryptedAccessToken := fmt.Sprintf("%s-encrypted-access-token", token.ConnectionID.String())
	token.EncryptedRefreshToken = encryptedRefreshToken
	token.EncryptedAccessToken = encryptedAccessToken
	if token.CreatedAt.IsZero() {
		token.CreatedAt = clock.Now()
	}

	dbMock.
		EXPECT().
		GetOAuth2Token(gomock.Any(), token.ConnectionID).
		Return(&token, nil).
		AnyTimes()

	e.
		EXPECT().
		DecryptStringForConnection(
			gomock.Any(),
			mockDb.ConnectionMatcher{
				ExpectedId: token.ConnectionID,
			},
			encryptedRefreshToken).
		Return(unencryptedRefreshToken, nil).
		AnyTimes()

	e.
		EXPECT().
		DecryptStringForConnection(
			gomock.Any(),
			mockDb.ConnectionMatcher{
				ExpectedId: token.ConnectionID,
			},
			encryptedAccessToken).
		Return(unencryptedAccessToken, nil).
		AnyTimes()
}
