package database

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/apctx"
	"github.com/rmorlok/authproxy/util"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	clock "k8s.io/utils/clock/testing"
	"testing"
	"time"
)

func TestOAuth2Token_IsAccessTokenExpired(t *testing.T) {
	t.Run("table-driven tests", func(t *testing.T) {
		now := time.Date(2023, time.November, 5, 6, 29, 0, 0, time.UTC)
		clock := clock.NewFakeClock(now)
		ctx := apctx.NewBuilderBackground().WithClock(clock).Build()

		testCases := []struct {
			name               string
			accessTokenExpires *time.Time
			expected           bool
		}{
			{
				name:               "no expiration time",
				accessTokenExpires: nil,
				expected:           false,
			},
			{
				name:               "expires in the past",
				accessTokenExpires: util.ToPtr(now.Add(-1 * time.Hour)),
				expected:           true,
			},
			{
				name:               "expires in the future",
				accessTokenExpires: util.ToPtr(now.Add(1 * time.Hour)),
				expected:           false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				token := &OAuth2Token{
					AccessTokenExpiresAt: tc.accessTokenExpires,
				}
				result := token.IsAccessTokenExpired(ctx)
				require.Equal(t, tc.expected, result)
			})
		}
	})
}

func TestOAuth2Tokens(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig("nonce_round_trip", nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		connectionId := uuid.New()

		tok, err := db.InsertOAuth2Token(
			ctx,
			connectionId,
			nil,
			"encryptedRefreshToken",
			"encryptedAccessToken",
			nil,
			"scope1 scope2",
		)
		require.NoError(t, err)
		require.NotNil(t, tok)
		require.Equal(t, connectionId, tok.ConnectionID)
		require.Nil(t, tok.RefreshedFromID)
		require.Equal(t, "encryptedRefreshToken", tok.EncryptedRefreshToken)
		require.Equal(t, "encryptedAccessToken", tok.EncryptedAccessToken)
		require.Equal(t, now, tok.CreatedAt)

		tok2, err := db.GetOAuth2Token(ctx, connectionId)
		require.NoError(t, err)
		require.NotNil(t, tok2)
		require.Equal(t, connectionId, tok2.ConnectionID)
		require.Nil(t, tok2.RefreshedFromID)
		require.Equal(t, "encryptedRefreshToken", tok2.EncryptedRefreshToken)
		require.Equal(t, "encryptedAccessToken", tok2.EncryptedAccessToken)
	})
	t.Run("no tokens", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig("nonce_round_trip", nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		connectionId1 := uuid.New()
		connectionId2 := uuid.New()

		_, err := db.InsertOAuth2Token(
			ctx,
			connectionId1,
			nil,
			"encryptedRefreshToken",
			"encryptedAccessToken",
			nil,
			"scope1 scope2",
		)
		require.NoError(t, err)

		tok, err := db.GetOAuth2Token(ctx, connectionId2)
		require.NoError(t, err)
		require.Nil(t, tok)
	})
	t.Run("replaces previous when tagging previous", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig("nonce_round_trip", nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		connectionId := uuid.New()

		tok1, err := db.InsertOAuth2Token(
			ctx,
			connectionId,
			nil,
			"encryptedRefreshToken",
			"encryptedAccessToken",
			nil,
			"scope1 scope2",
		)
		require.NoError(t, err)
		require.NotNil(t, tok1)
		require.Equal(t, connectionId, tok1.ConnectionID)
		require.Nil(t, tok1.RefreshedFromID)
		require.Equal(t, "encryptedRefreshToken", tok1.EncryptedRefreshToken)
		require.Equal(t, "encryptedAccessToken", tok1.EncryptedAccessToken)
		require.Equal(t, now, tok1.CreatedAt)

		tok2, err := db.InsertOAuth2Token(
			ctx,
			connectionId,
			&tok1.ID,
			"encryptedRefreshToken2",
			"encryptedAccessToken2",
			nil,
			"scope1 scope2",
		)
		require.NoError(t, err)
		require.NotNil(t, tok2)
		require.Equal(t, connectionId, tok2.ConnectionID)
		require.Equal(t, &tok1.ID, tok2.RefreshedFromID)
		require.Equal(t, "encryptedRefreshToken2", tok2.EncryptedRefreshToken)
		require.Equal(t, "encryptedAccessToken2", tok2.EncryptedAccessToken)
		require.Equal(t, now, tok2.CreatedAt)

		tok3, err := db.GetOAuth2Token(ctx, connectionId)
		require.NoError(t, err)
		require.NotNil(t, tok2)
		require.Equal(t, connectionId, tok3.ConnectionID)
		require.Equal(t, &tok1.ID, tok2.RefreshedFromID)
		require.Equal(t, "encryptedRefreshToken2", tok2.EncryptedRefreshToken)
		require.Equal(t, "encryptedAccessToken2", tok2.EncryptedAccessToken)
	})
	t.Run("replaces previous when not tagging previous", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig("nonce_round_trip", nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		connectionId := uuid.New()

		tok1, err := db.InsertOAuth2Token(
			ctx,
			connectionId,
			nil,
			"encryptedRefreshToken",
			"encryptedAccessToken",
			nil,
			"scope1 scope2",
		)
		require.NoError(t, err)
		require.NotNil(t, tok1)
		require.Equal(t, connectionId, tok1.ConnectionID)
		require.Nil(t, tok1.RefreshedFromID)
		require.Equal(t, "encryptedRefreshToken", tok1.EncryptedRefreshToken)
		require.Equal(t, "encryptedAccessToken", tok1.EncryptedAccessToken)
		require.Equal(t, now, tok1.CreatedAt)

		tok2, err := db.InsertOAuth2Token(
			ctx,
			connectionId,
			nil, // not tagging previous
			"encryptedRefreshToken2",
			"encryptedAccessToken2",
			nil,
			"scope1 scope2",
		)
		require.NoError(t, err)
		require.NotNil(t, tok2)
		require.Equal(t, connectionId, tok2.ConnectionID)
		require.Nil(t, tok2.RefreshedFromID)
		require.Equal(t, "encryptedRefreshToken2", tok2.EncryptedRefreshToken)
		require.Equal(t, "encryptedAccessToken2", tok2.EncryptedAccessToken)
		require.Equal(t, now, tok2.CreatedAt)

		tok3, err := db.GetOAuth2Token(ctx, connectionId)
		require.NoError(t, err)
		require.NotNil(t, tok2)
		require.Equal(t, connectionId, tok3.ConnectionID)
		require.Nil(t, tok2.RefreshedFromID)
		require.Equal(t, "encryptedRefreshToken2", tok2.EncryptedRefreshToken)
		require.Equal(t, "encryptedAccessToken2", tok2.EncryptedAccessToken)
	})
}

func TestEnumerateOAuth2TokensExpiringWithin(t *testing.T) {
	t.Run("table-driven tests", func(t *testing.T) {
		now := time.Date(2023, time.November, 5, 6, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		createdConnection := Connection{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			State:       ConnectionStateCreated,
			ConnectorId: "some-connector",
			CreatedAt:   apctx.GetClock(ctx).Now().Add(-1 * time.Hour),
			UpdatedAt:   apctx.GetClock(ctx).Now().Add(-1 * time.Hour),
		}

		readyConnection1 := Connection{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			State:       ConnectionStateReady,
			ConnectorId: "some-connector",
			CreatedAt:   apctx.GetClock(ctx).Now().Add(-1 * time.Hour),
			UpdatedAt:   apctx.GetClock(ctx).Now().Add(-1 * time.Hour),
		}

		readyConnection2 := Connection{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000003"),
			State:       ConnectionStateReady,
			ConnectorId: "some-connector",
			CreatedAt:   apctx.GetClock(ctx).Now().Add(-1 * time.Hour),
			UpdatedAt:   apctx.GetClock(ctx).Now().Add(-1 * time.Hour),
		}

		disabledConnection := Connection{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000004"),
			State:       ConnectionStateDisabled,
			ConnectorId: "some-connector",
			CreatedAt:   apctx.GetClock(ctx).Now().Add(-1 * time.Hour),
			UpdatedAt:   apctx.GetClock(ctx).Now().Add(-1 * time.Hour),
		}

		deletedConnection := Connection{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000005"),
			State:       ConnectionStateReady,
			ConnectorId: "some-connector",
			CreatedAt:   apctx.GetClock(ctx).Now().Add(-1 * time.Hour),
			UpdatedAt:   apctx.GetClock(ctx).Now().Add(-1 * time.Hour),
			DeletedAt: gorm.DeletedAt{
				Time:  apctx.GetClock(ctx).Now().Add(-30 * time.Minute),
				Valid: true,
			},
		}

		manyReadyConnections := make([]Connection, 0)
		for i := 0; i < 200; i++ {
			manyReadyConnections = append(manyReadyConnections, Connection{
				ID:          uuid.New(),
				State:       ConnectionStateReady,
				ConnectorId: "some-connector",
				CreatedAt:   apctx.GetClock(ctx).Now().Add(-1 * time.Hour),
				UpdatedAt:   apctx.GetClock(ctx).Now().Add(-1 * time.Hour),
			})
		}

		tokens150AllExpiring := make([]*OAuth2Token, 0)
		for i := 0; i < 150; i++ {
			tokens150AllExpiring = append(tokens150AllExpiring, &OAuth2Token{
				ConnectionID:         manyReadyConnections[i].ID,
				AccessTokenExpiresAt: util.ToPtr(now.Add(15 * time.Minute)),
			})
		}

		connections := []Connection{
			createdConnection,
			readyConnection1,
			readyConnection2,
			disabledConnection,
			deletedConnection,
		}

		connections = append(connections, manyReadyConnections...)

		testCases := []struct {
			name           string
			tokens         []*OAuth2Token
			duration       time.Duration
			callbackError  bool
			expectedTokens int
		}{
			{
				name:           "no tokens in database",
				tokens:         nil,
				duration:       time.Hour,
				callbackError:  false,
				expectedTokens: 0,
			},
			{
				name: "one token expiring within duration",
				tokens: []*OAuth2Token{
					{
						ConnectionID:         readyConnection1.ID,
						AccessTokenExpiresAt: util.ToPtr(now.Add(30 * time.Minute)),
					},
				},
				duration:       time.Hour,
				callbackError:  false,
				expectedTokens: 1,
			},
			{
				name: "one token already expired",
				tokens: []*OAuth2Token{
					{
						ConnectionID:         readyConnection1.ID,
						AccessTokenExpiresAt: util.ToPtr(now.Add(-30 * time.Minute)),
					},
				},
				duration:       time.Hour,
				callbackError:  false,
				expectedTokens: 1,
			},
			{
				name: "multiple tokens expiring within duration",
				tokens: []*OAuth2Token{
					{
						ConnectionID:         readyConnection1.ID,
						AccessTokenExpiresAt: util.ToPtr(now.Add(15 * time.Minute)),
					},
					{
						ConnectionID:         readyConnection2.ID,
						AccessTokenExpiresAt: util.ToPtr(now.Add(45 * time.Minute)),
					},
				},
				duration:       time.Hour,
				callbackError:  false,
				expectedTokens: 2,
			},
			{
				name: "tokens expiring beyond provided duration",
				tokens: []*OAuth2Token{
					{
						ConnectionID:         readyConnection1.ID,
						AccessTokenExpiresAt: util.ToPtr(now.Add(2 * time.Hour)),
					},
				},
				duration:       time.Hour,
				callbackError:  false,
				expectedTokens: 0,
			},
			{
				name: "ignores tokens for disabled connections",
				tokens: []*OAuth2Token{
					{
						ConnectionID:         disabledConnection.ID,
						AccessTokenExpiresAt: util.ToPtr(now.Add(30 * time.Minute)),
					},
				},
				duration:       time.Hour,
				callbackError:  false,
				expectedTokens: 0,
			},
			{
				name: "ignores tokens for deleted connections",
				tokens: []*OAuth2Token{
					{
						ConnectionID:         deletedConnection.ID,
						AccessTokenExpiresAt: util.ToPtr(now.Add(30 * time.Minute)),
					},
				},
				duration:       time.Hour,
				callbackError:  false,
				expectedTokens: 0,
			},
			{
				name:           "multiple pages of tokens",
				tokens:         tokens150AllExpiring,
				duration:       time.Hour,
				callbackError:  false,
				expectedTokens: 150,
			},
			{
				name: "callback returns error",
				tokens: []*OAuth2Token{
					{
						ConnectionID:         readyConnection1.ID,
						AccessTokenExpiresAt: util.ToPtr(now.Add(15 * time.Minute)),
					},
				},
				duration:       time.Hour,
				callbackError:  true,
				expectedTokens: 0,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				_, db := MustApplyBlankTestDbConfig("enumerate_tokens_test", nil)
				err := db.Migrate(ctx)
				require.NoError(t, err)

				dbRaw := db.(*gormDB)

				for _, connection := range connections {
					result := dbRaw.gorm.Create(&connection)
					require.NoError(t, result.Error)
				}

				for _, token := range tc.tokens {
					_, err := db.InsertOAuth2Token(
						ctx,
						token.ConnectionID,
						nil,
						"refreshToken",
						"accessToken",
						token.AccessTokenExpiresAt,
						"scope1 scope2",
					)
					require.NoError(t, err)
				}

				count := 0
				err = db.EnumerateOAuth2TokensExpiringWithin(ctx, tc.duration, func(tokens []*OAuth2TokenWithConnection, lastPage bool) (bool, error) {
					if tc.callbackError {
						return true, fmt.Errorf("callback error")
					}
					count += len(tokens)
					return false, nil
				})

				if tc.callbackError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					require.Equal(t, tc.expectedTokens, count)
				}
			})
		}
	})
}
