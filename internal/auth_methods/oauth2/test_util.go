package oauth2

import (
	"context"
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	mockE "github.com/rmorlok/authproxy/internal/encrypt/mock"
)

// MockOAuthTokenForConnection sets up mocks to return the specified token. The values `EncryptedRefreshToken` and
// `EncryptedAccessToken` should contain the unencrypted values in the Data field. This method will use fake
// encrypted equivalents and mock the calls to decrypt them.
func MockOAuthTokenForConnection(ctx context.Context, dbMock *mockDb.MockDB, e *mockE.MockE, token database.OAuth2Token) {
	clock := apctx.GetClock(ctx)

	unencryptedRefreshToken := token.EncryptedRefreshToken.Data
	unencryptedAccessToken := token.EncryptedAccessToken.Data

	encryptedRefreshToken := encfield.EncryptedField{
		ID:   "ekv_mock",
		Data: fmt.Sprintf("%s-encrypted-refresh-token", token.ConnectionId.String()),
	}
	encryptedAccessToken := encfield.EncryptedField{
		ID:   "ekv_mock",
		Data: fmt.Sprintf("%s-encrypted-access-token", token.ConnectionId.String()),
	}
	token.EncryptedRefreshToken = encryptedRefreshToken
	token.EncryptedAccessToken = encryptedAccessToken
	if token.CreatedAt.IsZero() {
		token.CreatedAt = clock.Now()
	}

	dbMock.
		EXPECT().
		GetOAuth2Token(gomock.Any(), token.ConnectionId).
		Return(&token, nil).
		AnyTimes()

	e.
		EXPECT().
		DecryptString(
			gomock.Any(),
			encryptedRefreshToken).
		Return(unencryptedRefreshToken, nil).
		AnyTimes()

	e.
		EXPECT().
		DecryptString(
			gomock.Any(),
			encryptedAccessToken).
		Return(unencryptedAccessToken, nil).
		AnyTimes()
}
