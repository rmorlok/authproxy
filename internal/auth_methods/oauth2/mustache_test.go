package oauth2

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/internal/apid"
	mockLog "github.com/rmorlok/authproxy/internal/aplog/mock"
	mockRedis "github.com/rmorlok/authproxy/internal/apredis/mock"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core/iface"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	mockEncrypt "github.com/rmorlok/authproxy/internal/encrypt/mock"
	mockH "github.com/rmorlok/authproxy/internal/httpf/mock"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	genmock "gopkg.in/h2non/gentleman-mock.v2"
)

func TestRenderMustache(t *testing.T) {
	ctx := context.Background()
	connectionId := apid.New(apid.PrefixConnection)

	newO2WithConfig := func(cfg map[string]any) *oAuth2Connection {
		return &oAuth2Connection{
			connection: &mockCore.Connection{
				Id:            connectionId,
				Configuration: cfg,
			},
		}
	}

	t.Run("renders cfg values in endpoint URL", func(t *testing.T) {
		o := newO2WithConfig(map[string]any{"tenant": "acme"})

		result, err := o.renderMustache(ctx, "https://{{cfg.tenant}}.example.com/oauth/authorize")
		require.NoError(t, err)
		assert.Equal(t, "https://acme.example.com/oauth/authorize", result)
	})

	t.Run("renders multiple cfg variables", func(t *testing.T) {
		o := newO2WithConfig(map[string]any{"tenant": "acme", "region": "eu"})

		result, err := o.renderMustache(ctx, "https://{{cfg.tenant}}.{{cfg.region}}.example.com/api")
		require.NoError(t, err)
		assert.Equal(t, "https://acme.eu.example.com/api", result)
	})

	t.Run("renders labels in template", func(t *testing.T) {
		o := &oAuth2Connection{
			connection: &mockCore.Connection{
				Id:     connectionId,
				Labels: map[string]string{"env": "production"},
			},
		}

		result, err := o.renderMustache(ctx, "https://{{labels.env}}.example.com/api")
		require.NoError(t, err)
		assert.Equal(t, "https://production.example.com/api", result)
	})

	t.Run("renders annotations in template", func(t *testing.T) {
		o := &oAuth2Connection{
			connection: &mockCore.Connection{
				Id:          connectionId,
				Annotations: map[string]string{"base_url": "custom.example.com"},
			},
		}

		result, err := o.renderMustache(ctx, "https://{{annotations.base_url}}/api")
		require.NoError(t, err)
		assert.Equal(t, "https://custom.example.com/api", result)
	})

	t.Run("renders cfg, labels, and annotations together", func(t *testing.T) {
		o := &oAuth2Connection{
			connection: &mockCore.Connection{
				Id:            connectionId,
				Configuration: map[string]any{"tenant": "acme"},
				Labels:        map[string]string{"env": "prod"},
				Annotations:   map[string]string{"region": "us"},
			},
		}

		result, err := o.renderMustache(ctx, "https://{{cfg.tenant}}.{{labels.env}}.{{annotations.region}}.example.com")
		require.NoError(t, err)
		assert.Equal(t, "https://acme.prod.us.example.com", result)
	})

	t.Run("returns plain string unchanged without fetching context", func(t *testing.T) {
		o := &oAuth2Connection{
			connection: &errorConnection{
				Connection: &mockCore.Connection{Id: connectionId},
				configErr:  fmt.Errorf("should not be called"),
			},
		}

		result, err := o.renderMustache(ctx, "https://example.com/oauth/token")
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/oauth/token", result)
	})

	t.Run("missing variable renders empty", func(t *testing.T) {
		o := newO2WithConfig(map[string]any{})

		result, err := o.renderMustache(ctx, "https://{{cfg.tenant}}.example.com")
		require.NoError(t, err)
		assert.Equal(t, "https://.example.com", result)
	})

	t.Run("nil configuration renders variables empty", func(t *testing.T) {
		o := newO2WithConfig(nil)

		result, err := o.renderMustache(ctx, "https://{{cfg.tenant}}.example.com")
		require.NoError(t, err)
		assert.Equal(t, "https://.example.com", result)
	})

	t.Run("propagates configuration error", func(t *testing.T) {
		o := &oAuth2Connection{
			connection: &errorConnection{
				Connection: &mockCore.Connection{Id: connectionId},
				configErr:  fmt.Errorf("encryption key not found"),
			},
		}

		_, err := o.renderMustache(ctx, "https://{{cfg.tenant}}.example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "encryption key not found")
	})
}

// errorConnection wraps a mock Connection but returns an error from GetMustacheContext.
type errorConnection struct {
	*mockCore.Connection
	configErr error
}

func (e *errorConnection) GetMustacheContext(ctx context.Context) (map[string]any, error) {
	return nil, e.configErr
}

// configuredConnection wraps a mock Connection to also return a ConnectorVersion.
type configuredConnection struct {
	*mockCore.Connection
	connectorVersion iface.ConnectorVersion
}

func (c *configuredConnection) GetConnectorVersionEntity() iface.ConnectorVersion {
	return c.connectorVersion
}

func TestGenerateAuthUrl_TemplatedEndpoint(t *testing.T) {
	ctx := context.Background()
	connectionId := apid.New(apid.PrefixConnection)
	connectorVersionId := apid.New(apid.PrefixConnectorVersion)
	stateId := apid.New(apid.PrefixOauth2State)

	cfg := config.FromRoot(&sconfig.Root{
		Public: sconfig.ServicePublic{
			ServiceHttp: sconfig.ServiceHttp{
				PortVal: common.NewIntegerValueDirect(8080),
			},
		},
	})

	t.Run("renders mustache template in authorization endpoint", func(t *testing.T) {
		o := &oAuth2Connection{
			cfg: cfg,
			connection: &configuredConnection{
				Connection: &mockCore.Connection{
					Id: connectionId,
					Configuration: map[string]any{
						"tenant": "acme-corp",
					},
				},
				connectorVersion: &mockCore.ConnectorVersion{
					Id: connectorVersionId,
				},
			},
			auth: &cschema.AuthOAuth2{
				Type:     cschema.AuthTypeOAuth2,
				ClientId: common.NewStringValueDirect("my-client-id"),
				Authorization: cschema.AuthOauth2Authorization{
					Endpoint: "https://{{cfg.tenant}}.example.com/oauth/authorize",
				},
			},
			state: &state{
				Id: stateId,
			},
		}

		authUrl, err := o.GenerateAuthUrl(ctx, &mockActorData{id: apid.New(apid.PrefixActor)})
		require.NoError(t, err)

		parsed, err := url.Parse(authUrl)
		require.NoError(t, err)
		assert.Equal(t, "acme-corp.example.com", parsed.Host)
		assert.Equal(t, "/oauth/authorize", parsed.Path)
		assert.Equal(t, "my-client-id", parsed.Query().Get("client_id"))
	})

	t.Run("renders mustache templates in query overrides", func(t *testing.T) {
		o := &oAuth2Connection{
			cfg: cfg,
			connection: &configuredConnection{
				Connection: &mockCore.Connection{
					Id: connectionId,
					Configuration: map[string]any{
						"tenant": "acme-corp",
					},
				},
				connectorVersion: &mockCore.ConnectorVersion{
					Id: connectorVersionId,
				},
			},
			auth: &cschema.AuthOAuth2{
				Type:     cschema.AuthTypeOAuth2,
				ClientId: common.NewStringValueDirect("my-client-id"),
				Authorization: cschema.AuthOauth2Authorization{
					Endpoint: "https://example.com/oauth/authorize",
					QueryOverrides: map[string]string{
						"resource": "https://{{cfg.tenant}}.example.com/api",
					},
				},
			},
			state: &state{
				Id: stateId,
			},
		}

		authUrl, err := o.GenerateAuthUrl(ctx, &mockActorData{id: apid.New(apid.PrefixActor)})
		require.NoError(t, err)

		parsed, err := url.Parse(authUrl)
		require.NoError(t, err)
		assert.Equal(t, "https://acme-corp.example.com/api", parsed.Query().Get("resource"))
	})

	t.Run("renders labels in authorization endpoint", func(t *testing.T) {
		o := &oAuth2Connection{
			cfg: cfg,
			connection: &configuredConnection{
				Connection: &mockCore.Connection{
					Id:     connectionId,
					Labels: map[string]string{"tenant": "label-tenant"},
				},
				connectorVersion: &mockCore.ConnectorVersion{
					Id: connectorVersionId,
				},
			},
			auth: &cschema.AuthOAuth2{
				Type:     cschema.AuthTypeOAuth2,
				ClientId: common.NewStringValueDirect("my-client-id"),
				Authorization: cschema.AuthOauth2Authorization{
					Endpoint: "https://{{labels.tenant}}.example.com/oauth/authorize",
				},
			},
			state: &state{
				Id: stateId,
			},
		}

		authUrl, err := o.GenerateAuthUrl(ctx, &mockActorData{id: apid.New(apid.PrefixActor)})
		require.NoError(t, err)

		parsed, err := url.Parse(authUrl)
		require.NoError(t, err)
		assert.Equal(t, "label-tenant.example.com", parsed.Host)
	})

	t.Run("static endpoint works without configuration", func(t *testing.T) {
		o := &oAuth2Connection{
			cfg: cfg,
			connection: &configuredConnection{
				Connection: &mockCore.Connection{
					Id:            connectionId,
					Configuration: nil,
				},
				connectorVersion: &mockCore.ConnectorVersion{
					Id: connectorVersionId,
				},
			},
			auth: &cschema.AuthOAuth2{
				Type:     cschema.AuthTypeOAuth2,
				ClientId: common.NewStringValueDirect("my-client-id"),
				Authorization: cschema.AuthOauth2Authorization{
					Endpoint: "https://example.com/oauth/authorize",
				},
			},
			state: &state{
				Id: stateId,
			},
		}

		authUrl, err := o.GenerateAuthUrl(ctx, &mockActorData{id: apid.New(apid.PrefixActor)})
		require.NoError(t, err)

		parsed, err := url.Parse(authUrl)
		require.NoError(t, err)
		assert.Equal(t, "example.com", parsed.Host)
		assert.Equal(t, "/oauth/authorize", parsed.Path)
	})
}

func TestCallbackFrom3rdParty_TemplatedEndpoint(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	tokenId := apid.New(apid.PrefixOAuth2Token)

	cfg := config.FromRoot(&sconfig.Root{
		Public: sconfig.ServicePublic{
			ServiceHttp: sconfig.ServiceHttp{
				PortVal: common.NewIntegerValueDirect(8080),
			},
		},
	})

	setupWithMocks := func(t *testing.T, tenantEndpoint string, connectionConfig map[string]any) (*oAuth2Connection, *mockDb.MockDB, *mockEncrypt.MockE, *gomock.Controller) {
		ctrl := gomock.NewController(t)
		h := mockH.NewFactoryWithMockingClient(ctrl)
		db := mockDb.NewMockDB(ctrl)
		encrypt := mockEncrypt.NewMockE(ctrl)
		r := mockRedis.NewMockClient(ctrl)
		logger, _ := mockLog.NewTestLogger(t)

		// CallbackFrom3rdParty deletes the OAuth state from Redis after consuming it
		r.EXPECT().Del(gomock.Any(), gomock.Any()).Return(redis.NewIntCmd(context.Background())).AnyTimes()

		return &oAuth2Connection{
			cfg:     cfg,
			db:      db,
			httpf:   h,
			r:       r,
			encrypt: encrypt,
			logger:  logger,
			connection: &mockCore.Connection{
				Id:            connectionId,
				Configuration: connectionConfig,
			},
			auth: &cschema.AuthOAuth2{
				Type:         cschema.AuthTypeOAuth2,
				ClientId:     common.NewStringValueDirect("test-client-id"),
				ClientSecret: common.NewStringValueDirect("test-client-secret"),
				Token: cschema.AuthOauth2Token{
					Endpoint: tenantEndpoint,
				},
				Scopes: []cschema.Scope{{Id: "read"}},
			},
			state: &state{
				Id:          apid.New(apid.PrefixOauth2State),
				ReturnToUrl: "https://app.example.com/callback",
			},
		}, db, encrypt, ctrl
	}

	t.Run("uses rendered mustache endpoint for token exchange", func(t *testing.T) {
		o2, db, encrypt, ctrl := setupWithMocks(
			t,
			"https://{{cfg.tenant}}.example.com/oauth/token",
			map[string]any{"tenant": "acme-corp"},
		)
		defer ctrl.Finish()

		genmock.
			New("https://acme-corp.example.com").
			Post("/oauth/token").
			MatchType("application/x-www-form-urlencoded").
			Reply(200).
			AddHeader("Content-Type", "application/json").
			BodyString(`{"access_token": "new-access-token", "refresh_token": "new-refresh-token", "scope": "read", "expires_in": 3600}`)

		encrypt.EXPECT().EncryptStringForEntity(gomock.Any(), gomock.Any(), "new-access-token").Return(encfield.EncryptedField{ID: "ekv_test", Data: "encrypted-access"}, nil)
		encrypt.EXPECT().EncryptStringForEntity(gomock.Any(), gomock.Any(), "new-refresh-token").Return(encfield.EncryptedField{ID: "ekv_test", Data: "encrypted-refresh"}, nil)
		db.EXPECT().InsertOAuth2Token(gomock.Any(), connectionId, nil, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&database.OAuth2Token{Id: tokenId}, nil)

		query := url.Values{"code": {"auth-code-123"}}
		returnUrl, err := o2.CallbackFrom3rdParty(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, "https://app.example.com/callback", returnUrl)
	})

	t.Run("static token endpoint works without configuration", func(t *testing.T) {
		o2, db, encrypt, ctrl := setupWithMocks(
			t,
			"https://example.com/oauth/token",
			nil,
		)
		defer ctrl.Finish()

		genmock.
			New("https://example.com").
			Post("/oauth/token").
			MatchType("application/x-www-form-urlencoded").
			Reply(200).
			AddHeader("Content-Type", "application/json").
			BodyString(`{"access_token": "access-tok", "scope": "read", "expires_in": 3600}`)

		encrypt.EXPECT().EncryptStringForEntity(gomock.Any(), gomock.Any(), "access-tok").Return(encfield.EncryptedField{ID: "ekv_test", Data: "encrypted-access"}, nil)
		db.EXPECT().InsertOAuth2Token(gomock.Any(), connectionId, nil, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&database.OAuth2Token{Id: tokenId}, nil)

		query := url.Values{"code": {"auth-code-456"}}
		returnUrl, err := o2.CallbackFrom3rdParty(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, "https://app.example.com/callback", returnUrl)
	})
}

func TestRevokeTokens_TemplatedEndpoint(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	tokenId := apid.New(apid.PrefixOAuth2Token)

	setupWithMocks := func(t *testing.T, revocationEndpoint string, connectionConfig map[string]any) (*oAuth2Connection, *mockDb.MockDB, *mockEncrypt.MockE, *gomock.Controller) {
		ctrl := gomock.NewController(t)
		h := mockH.NewFactoryWithMockingClient(ctrl)
		db := mockDb.NewMockDB(ctrl)
		encrypt := mockEncrypt.NewMockE(ctrl)
		logger, _ := mockLog.NewTestLogger(t)

		return &oAuth2Connection{
			cfg:     nil,
			db:      db,
			httpf:   h,
			r:       nil,
			encrypt: encrypt,
			logger:  logger,
			connection: &mockCore.Connection{
				Id:            connectionId,
				Configuration: connectionConfig,
			},
			auth: &cschema.AuthOAuth2{
				Type: cschema.AuthTypeOAuth2,
				Revocation: &cschema.AuthOauth2Revocation{
					Endpoint: revocationEndpoint,
				},
			},
		}, db, encrypt, ctrl
	}

	t.Run("uses rendered mustache endpoint for revocation", func(t *testing.T) {
		o2, db, encrypt, ctrl := setupWithMocks(
			t,
			"https://{{cfg.tenant}}.example.com/oauth/revoke",
			map[string]any{"tenant": "acme-corp"},
		)
		defer ctrl.Finish()

		MockOAuthTokenForConnection(context.Background(), db, encrypt, database.OAuth2Token{
			Id:                    tokenId,
			ConnectionId:          connectionId,
			EncryptedAccessToken:  encfield.EncryptedField{ID: "ekv_test", Data: "some-access-token"},
			EncryptedRefreshToken: encfield.EncryptedField{ID: "ekv_test", Data: "some-refresh-token"},
		})

		db.EXPECT().DeleteOAuth2Token(gomock.Any(), tokenId).Return(nil)

		genmock.
			New("https://acme-corp.example.com").
			Post("/oauth/revoke").
			MatchType("application/x-www-form-urlencoded").
			Reply(200)

		genmock.
			New("https://acme-corp.example.com").
			Post("/oauth/revoke").
			MatchType("application/x-www-form-urlencoded").
			Reply(200)

		err := o2.RevokeTokens(context.Background())
		require.NoError(t, err)
	})

	t.Run("static revocation endpoint works without configuration", func(t *testing.T) {
		o2, db, encrypt, ctrl := setupWithMocks(
			t,
			"https://example.com/oauth/revoke",
			nil,
		)
		defer ctrl.Finish()

		MockOAuthTokenForConnection(context.Background(), db, encrypt, database.OAuth2Token{
			Id:                    tokenId,
			ConnectionId:          connectionId,
			EncryptedAccessToken:  encfield.EncryptedField{ID: "ekv_test", Data: "some-access-token"},
			EncryptedRefreshToken: encfield.EncryptedField{ID: "ekv_test", Data: "some-refresh-token"},
		})

		db.EXPECT().DeleteOAuth2Token(gomock.Any(), tokenId).Return(nil)

		genmock.
			New("https://example.com").
			Post("/oauth/revoke").
			MatchType("application/x-www-form-urlencoded").
			Reply(200)

		genmock.
			New("https://example.com").
			Post("/oauth/revoke").
			MatchType("application/x-www-form-urlencoded").
			Reply(200)

		err := o2.RevokeTokens(context.Background())
		require.NoError(t, err)
	})

	t.Run("renders different tenants to different endpoints", func(t *testing.T) {
		o2a, dbA, encryptA, ctrlA := setupWithMocks(
			t,
			"https://{{cfg.tenant}}.example.com/oauth/revoke",
			map[string]any{"tenant": "alpha"},
		)
		defer ctrlA.Finish()

		o2b, dbB, encryptB, ctrlB := setupWithMocks(
			t,
			"https://{{cfg.tenant}}.example.com/oauth/revoke",
			map[string]any{"tenant": "beta"},
		)
		defer ctrlB.Finish()

		tokenIdA := apid.New(apid.PrefixOAuth2Token)
		tokenIdB := apid.New(apid.PrefixOAuth2Token)

		MockOAuthTokenForConnection(context.Background(), dbA, encryptA, database.OAuth2Token{
			Id:                    tokenIdA,
			ConnectionId:          connectionId,
			EncryptedAccessToken:  encfield.EncryptedField{ID: "ekv_test", Data: "access-a"},
			EncryptedRefreshToken: encfield.EncryptedField{ID: "ekv_test", Data: "refresh-a"},
		})
		dbA.EXPECT().DeleteOAuth2Token(gomock.Any(), tokenIdA).Return(nil)

		MockOAuthTokenForConnection(context.Background(), dbB, encryptB, database.OAuth2Token{
			Id:                    tokenIdB,
			ConnectionId:          connectionId,
			EncryptedAccessToken:  encfield.EncryptedField{ID: "ekv_test", Data: "access-b"},
			EncryptedRefreshToken: encfield.EncryptedField{ID: "ekv_test", Data: "refresh-b"},
		})
		dbB.EXPECT().DeleteOAuth2Token(gomock.Any(), tokenIdB).Return(nil)

		genmock.
			New("https://alpha.example.com").
			Post("/oauth/revoke").
			MatchType("application/x-www-form-urlencoded").
			Reply(200)
		genmock.
			New("https://alpha.example.com").
			Post("/oauth/revoke").
			MatchType("application/x-www-form-urlencoded").
			Reply(200)

		genmock.
			New("https://beta.example.com").
			Post("/oauth/revoke").
			MatchType("application/x-www-form-urlencoded").
			Reply(200)
		genmock.
			New("https://beta.example.com").
			Post("/oauth/revoke").
			MatchType("application/x-www-form-urlencoded").
			Reply(200)

		err := o2a.RevokeTokens(context.Background())
		require.NoError(t, err)

		err = o2b.RevokeTokens(context.Background())
		require.NoError(t, err)
	})
}

// mockActorData implements IActorData for testing GenerateAuthUrl.
type mockActorData struct {
	id apid.ID
}

func (m *mockActorData) GetId() apid.ID                       { return m.id }
func (m *mockActorData) GetExternalId() string                 { return "ext-123" }
func (m *mockActorData) GetLabels() map[string]string          { return nil }
func (m *mockActorData) GetPermissions() []aschema.Permission  { return nil }
func (m *mockActorData) GetNamespace() string                  { return "/" }
