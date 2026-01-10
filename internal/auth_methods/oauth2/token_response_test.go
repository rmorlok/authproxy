package oauth2

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	database_mock "github.com/rmorlok/authproxy/internal/database/mock"
	encrypt_mock "github.com/rmorlok/authproxy/internal/encrypt/mock"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"gopkg.in/h2non/gock.v1"
)

func TestCreateDbTokenFromResponse(t *testing.T) {
	tests := []struct {
		name           string
		responseStatus int
		responseBody   string
		responseError  error
		wantErr        string
		setupMocks     func(mdb *database_mock.MockDB, mencrypt *encrypt_mock.MockE)
	}{
		{
			name:         "valid response",
			responseBody: `{"access_token": "valid_access_token", "refresh_token": "valid_refresh_token", "scope": "read write", "expires_in": 3600}`,
			wantErr:      "",
			setupMocks: func(mdb *database_mock.MockDB, mencrypt *encrypt_mock.MockE) {
				mencrypt.EXPECT().EncryptStringForConnection(gomock.Any(), gomock.Any(), "valid_access_token").Return("encrypted_access_token", nil)
				mencrypt.EXPECT().EncryptStringForConnection(gomock.Any(), gomock.Any(), "valid_refresh_token").Return("encrypted_refresh_token", nil)
				mdb.EXPECT().InsertOAuth2Token(gomock.Any(), gomock.Any(), nil, "encrypted_refresh_token", "encrypted_access_token", gomock.Any(), "read write").Return(&database.OAuth2Token{}, nil)
			},
		},
		{
			name:         "error deserializing response",
			responseBody: `invalid_json`,
			wantErr:      "failed to parse token response",
			setupMocks:   func(mdb *database_mock.MockDB, mencrypt *encrypt_mock.MockE) {},
		},
		{
			name:         "missing access token",
			responseBody: `{"refresh_token": "valid_refresh_token"}`,
			wantErr:      "no access token in response",
			setupMocks:   func(mdb *database_mock.MockDB, mencrypt *encrypt_mock.MockE) {},
		},
		{
			name:         "encryption failure for access token",
			responseBody: `{"access_token": "valid_access_token"}`,
			wantErr:      "failed to encrypt access token",
			setupMocks: func(mdb *database_mock.MockDB, mencrypt *encrypt_mock.MockE) {
				mencrypt.EXPECT().EncryptStringForConnection(gomock.Any(), gomock.Any(), "valid_access_token").Return("", errors.New("encryption failed"))
			},
		},
		{
			name:         "encryption failure for refresh token",
			responseBody: `{"access_token": "valid_access_token", "refresh_token": "valid_refresh_token"}`,
			wantErr:      "failed to encrypt refresh token",
			setupMocks: func(mdb *database_mock.MockDB, mencrypt *encrypt_mock.MockE) {
				mencrypt.EXPECT().EncryptStringForConnection(gomock.Any(), gomock.Any(), "valid_access_token").Return("encrypted_access_token", nil)
				mencrypt.EXPECT().EncryptStringForConnection(gomock.Any(), gomock.Any(), "valid_refresh_token").Return("", errors.New("encryption failed"))
			},
		},
		{
			name:         "database insert failure",
			responseBody: `{"access_token": "valid_access_token"}`,
			wantErr:      "failed to insert oauth2 token",
			setupMocks: func(mdb *database_mock.MockDB, mencrypt *encrypt_mock.MockE) {
				mencrypt.EXPECT().EncryptStringForConnection(gomock.Any(), gomock.Any(), "valid_access_token").Return("encrypted_access_token", nil)
				mdb.EXPECT().InsertOAuth2Token(gomock.Any(), gomock.Any(), nil, "", "encrypted_access_token", gomock.Any(), gomock.Any()).Return(nil, errors.New("insert failed"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockDB := database_mock.NewMockDB(ctrl)
			mockEncrypt := encrypt_mock.NewMockE(ctrl)

			if tt.setupMocks != nil {
				tt.setupMocks(mockDB, mockEncrypt)
			}

			oauth := &oAuth2Connection{
				db:      mockDB,
				encrypt: mockEncrypt,
				auth: &sconfig.AuthOAuth2{
					Scopes: []sconfig.Scope{{Id: "read"}, {Id: "write"}},
				},
				connection: &mockCore.Connection{
					Id: uuid.MustParse("12345678-1234-1234-1234-123456789abc"),
				},
			}

			responseStatus := tt.responseStatus
			if responseStatus == 0 {
				responseStatus = 200
			}

			resp := test_utils.MockGentlemenGetResponse("https://example.com", "example", func(m *gock.Request) {
				m.
					Reply(responseStatus).
					AddHeader("Content-Type", "application/json").
					BodyString(tt.responseBody)

				if tt.responseError != nil {
					m.ReplyError(tt.responseError)
				}
			})

			ctx := context.Background()

			_, err := oauth.createDbTokenFromResponse(ctx, resp, nil)
			if tt.wantErr == "" && err != nil {
				t.Errorf("unexpected error: %v", err)
			} else if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("expected error: %v, got: %v", tt.wantErr, err)
			}
		})
	}
}
