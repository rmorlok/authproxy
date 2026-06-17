package oauth2

import (
	"context"
	"net/url"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	mockEncrypt "github.com/rmorlok/authproxy/internal/encrypt/mock"
	mockH "github.com/rmorlok/authproxy/internal/httpf/mock"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	genmock "gopkg.in/h2non/gentleman-mock.v2"
	"gopkg.in/h2non/gock.v1"
)

func TestEffectiveScopes_UsesConnectionPredicateContext(t *testing.T) {
	o := &oAuth2Connection{
		connection: &mockCore.Connection{
			Configuration: map[string]any{
				"push_files":    true,
				"sync_activity": false,
			},
			Labels:      map[string]string{"env": "prod"},
			Annotations: map[string]string{"tier": "gold"},
		},
		auth: &sconfig.AuthOAuth2{
			Scopes: []sconfig.Scope{
				{Id: "read"},
				{
					Id: "write",
					If: &common.Predicate{Javascript: `cfg.push_files === true &&
						labels["env"] === "prod" &&
						annotations["tier"] === "gold"`},
				},
				{
					Id:       "activity",
					Required: sconfig.NewScopeRequiredPredicate(&common.Predicate{Javascript: `cfg.sync_activity === true`}),
				},
				{
					Id: "admin",
					If: &common.Predicate{Javascript: `labels["env"] === "dev"`},
				},
			},
		},
	}

	got, err := o.effectiveScopes(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"read", "write", "activity"}, []string{got[0].Id, got[1].Id, got[2].Id})
	assert.True(t, got[0].IsRequired())
	assert.True(t, got[1].IsRequired())
	assert.False(t, got[2].IsRequired())
}

func TestEffectiveScopes_PredicateErrorIncludesScopeContext(t *testing.T) {
	o := &oAuth2Connection{
		connection: &mockCore.Connection{},
		auth: &sconfig.AuthOAuth2{
			Scopes: []sconfig.Scope{
				{
					Id: "write",
					If: &common.Predicate{Javascript: `cfg.enabled ===`},
				},
			},
		},
	}

	_, err := o.effectiveScopes(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `scope "write" if.javascript`)
}

func TestGenerateAuthUrl_UsesEffectiveScopes(t *testing.T) {
	cfg := config.FromRoot(&sconfig.Root{
		Public: sconfig.ServicePublic{
			ServiceHttp: sconfig.ServiceHttp{
				PortVal: common.NewIntegerValueDirect(8080),
			},
		},
	})

	o := &oAuth2Connection{
		cfg: cfg,
		connection: &configuredConnection{
			Connection: &mockCore.Connection{
				Id:            apid.New(apid.PrefixConnection),
				Configuration: map[string]any{"push_files": false},
				Labels:        map[string]string{"env": "prod"},
			},
			connectorVersion: &mockCore.ConnectorVersion{Id: apid.New(apid.PrefixConnectorVersion)},
		},
		auth: &cschema.AuthOAuth2{
			Type:     cschema.AuthTypeOAuth2,
			ClientId: common.NewStringValueDirect("client-id"),
			Authorization: cschema.AuthOauth2Authorization{
				Endpoint: "https://example.com/oauth/authorize",
			},
			Scopes: []cschema.Scope{
				{Id: "read"},
				{Id: "write", If: &common.Predicate{Javascript: `cfg.push_files === true`}},
				{Id: "activity", If: &common.Predicate{Javascript: `labels["env"] === "prod"`}},
			},
		},
		state: &state{Id: apid.New(apid.PrefixOauth2State)},
	}

	authURL, err := o.GenerateAuthUrl(context.Background(), &mockActorData{id: apid.New(apid.PrefixActor)})
	require.NoError(t, err)

	parsed, err := url.Parse(authURL)
	require.NoError(t, err)
	assert.Equal(t, "read activity", parsed.Query().Get("scope"))
}

func TestGenerateAuthUrl_ConditionalScopeError(t *testing.T) {
	cfg := config.FromRoot(&sconfig.Root{
		Public: sconfig.ServicePublic{
			ServiceHttp: sconfig.ServiceHttp{
				PortVal: common.NewIntegerValueDirect(8080),
			},
		},
	})

	o := &oAuth2Connection{
		cfg: cfg,
		connection: &configuredConnection{
			Connection:       &mockCore.Connection{Id: apid.New(apid.PrefixConnection)},
			connectorVersion: &mockCore.ConnectorVersion{Id: apid.New(apid.PrefixConnectorVersion)},
		},
		auth: &cschema.AuthOAuth2{
			Type:     cschema.AuthTypeOAuth2,
			ClientId: common.NewStringValueDirect("client-id"),
			Authorization: cschema.AuthOauth2Authorization{
				Endpoint: "https://example.com/oauth/authorize",
			},
			Scopes: []cschema.Scope{
				{Id: "write", If: &common.Predicate{Javascript: `cfg.enabled ===`}},
			},
		},
		state: &state{Id: apid.New(apid.PrefixOauth2State)},
	}

	_, err := o.GenerateAuthUrl(context.Background(), &mockActorData{id: apid.New(apid.PrefixActor)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve oauth2 scopes")
	assert.Contains(t, err.Error(), `scope "write" if.javascript`)
}

func TestExchangeClientCredentials_UsesEffectiveScopes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h := mockH.NewFactoryWithMockingClient(ctrl)
	db := mockDb.NewMockDB(ctrl)
	encrypt := mockEncrypt.NewMockE(ctrl)
	connectionId := apid.New(apid.PrefixConnection)
	grantType := sconfig.OAuth2GrantClientCredentials

	encryptedCredentials := encfield.EncryptedField{ID: "ekv_test", Data: "encrypted_credentials"}
	db.EXPECT().
		GetActiveApiKeyCredential(gomock.Any(), connectionId).
		Return(&database.ApiKeyCredential{
			Id:                   apid.New(apid.PrefixApiKeyCredential),
			ConnectionId:         connectionId,
			EncryptedCredentials: encryptedCredentials,
		}, nil)
	encrypt.EXPECT().
		DecryptString(gomock.Any(), encryptedCredentials).
		Return(`{"client_id":"client-id","client_secret":"client-secret"}`, nil)
	encrypt.EXPECT().
		EncryptStringForEntity(gomock.Any(), gomock.Any(), "access-token").
		Return(encfield.EncryptedField{ID: "ekv_test", Data: "encrypted_access_token"}, nil)
	db.EXPECT().
		InsertOAuth2Token(
			gomock.Any(),
			connectionId,
			nil,
			encfield.EncryptedField{},
			encfield.EncryptedField{ID: "ekv_test", Data: "encrypted_access_token"},
			gomock.Any(),
			"read activity",
			"read activity",
			gomock.Any(),
		).
		Return(&database.OAuth2Token{}, nil)

	o := &oAuth2Connection{
		db:      db,
		encrypt: encrypt,
		httpf:   h,
		connection: &mockCore.Connection{
			Id:            connectionId,
			Configuration: map[string]any{"push_files": false, "sync_activity": true},
		},
		auth: &cschema.AuthOAuth2{
			Type:         cschema.AuthTypeOAuth2,
			GrantType:    &grantType,
			ClientId:     common.NewStringValueDirect("client-id"),
			ClientSecret: common.NewStringValueDirect("client-secret"),
			Token: cschema.AuthOauth2Token{
				Endpoint: "https://example.com/oauth/token",
			},
			Scopes: []cschema.Scope{
				{Id: "read"},
				{Id: "write", If: &common.Predicate{Javascript: `cfg.push_files === true`}},
				{Id: "activity", If: &common.Predicate{Javascript: `cfg.sync_activity === true`}},
			},
		},
	}

	var capturedBody string
	genmock.
		New("https://example.com").
		Post("/oauth/token").
		MatchType("application/x-www-form-urlencoded").
		AddMatcher(captureBody(&capturedBody)).
		Reply(200).
		AddHeader("Content-Type", "application/json").
		BodyString(`{"access_token":"access-token","expires_in":3600}`)

	err := o.ExchangeClientCredentials(context.Background())
	require.NoError(t, err)

	form, err := url.ParseQuery(capturedBody)
	require.NoError(t, err)
	assert.Equal(t, "client_credentials", form.Get("grant_type"))
	assert.Equal(t, "read activity", form.Get("scope"))
}

func TestRefreshAccessToken_ClientCredentialsUsesEffectiveScopes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	_, r := apredis.MustApplyTestConfig(nil)
	h := mockH.NewFactoryWithMockingClient(ctrl)
	db := mockDb.NewMockDB(ctrl)
	encrypt := mockEncrypt.NewMockE(ctrl)
	connectionId := apid.New(apid.PrefixConnection)
	tokenId := apid.New(apid.PrefixOAuth2Token)
	grantType := sconfig.OAuth2GrantClientCredentials

	existing := &database.OAuth2Token{
		Id:           tokenId,
		ConnectionId: connectionId,
	}
	db.EXPECT().
		GetOAuth2Token(gomock.Any(), connectionId).
		Return(existing, nil)

	encryptedCredentials := encfield.EncryptedField{ID: "ekv_test", Data: "encrypted_credentials"}
	db.EXPECT().
		GetActiveApiKeyCredential(gomock.Any(), connectionId).
		Return(&database.ApiKeyCredential{
			Id:                   apid.New(apid.PrefixApiKeyCredential),
			ConnectionId:         connectionId,
			EncryptedCredentials: encryptedCredentials,
		}, nil)
	encrypt.EXPECT().
		DecryptString(gomock.Any(), encryptedCredentials).
		Return(`{"client_id":"client-id","client_secret":"client-secret"}`, nil)
	encrypt.EXPECT().
		EncryptStringForEntity(gomock.Any(), gomock.Any(), "new-access-token").
		Return(encfield.EncryptedField{ID: "ekv_test", Data: "encrypted_access_token"}, nil)
	db.EXPECT().
		InsertOAuth2Token(
			gomock.Any(),
			connectionId,
			&tokenId,
			encfield.EncryptedField{},
			encfield.EncryptedField{ID: "ekv_test", Data: "encrypted_access_token"},
			gomock.Any(),
			"read activity",
			"read activity",
			gomock.Any(),
		).
		Return(&database.OAuth2Token{}, nil)

	o := &oAuth2Connection{
		db:      db,
		encrypt: encrypt,
		httpf:   h,
		r:       r,
		connection: &mockCore.Connection{
			Id:            connectionId,
			Configuration: map[string]any{"push_files": false, "sync_activity": true},
		},
		auth: &cschema.AuthOAuth2{
			Type:         cschema.AuthTypeOAuth2,
			GrantType:    &grantType,
			ClientId:     common.NewStringValueDirect("client-id"),
			ClientSecret: common.NewStringValueDirect("client-secret"),
			Token: cschema.AuthOauth2Token{
				Endpoint: "https://example.com/oauth/token",
			},
			Scopes: []cschema.Scope{
				{Id: "read"},
				{Id: "write", If: &common.Predicate{Javascript: `cfg.push_files === true`}},
				{Id: "activity", If: &common.Predicate{Javascript: `cfg.sync_activity === true`}},
			},
		},
	}

	var capturedBody string
	genmock.
		New("https://example.com").
		Post("/oauth/token").
		MatchType("application/x-www-form-urlencoded").
		AddMatcher(captureBody(&capturedBody)).
		Reply(200).
		AddHeader("Content-Type", "application/json").
		BodyString(`{"access_token":"new-access-token","expires_in":3600}`)

	_, err := o.refreshAccessToken(context.Background(), existing, refreshModeAlways)
	require.NoError(t, err)

	form, err := url.ParseQuery(capturedBody)
	require.NoError(t, err)
	assert.Equal(t, "client_credentials", form.Get("grant_type"))
	assert.Equal(t, "read activity", form.Get("scope"))
}

func TestCreateDbTokenFromResponse_UsesEffectiveScopesForOptionalDynamicRequired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := mockDb.NewMockDB(ctrl)
	encrypt := mockEncrypt.NewMockE(ctrl)
	connectionId := apid.New(apid.PrefixConnection)

	encrypt.EXPECT().
		EncryptStringForEntity(gomock.Any(), gomock.Any(), "access-token").
		Return(encfield.EncryptedField{ID: "ekv_test", Data: "encrypted_access_token"}, nil)
	db.EXPECT().
		InsertOAuth2Token(
			gomock.Any(),
			connectionId,
			nil,
			encfield.EncryptedField{},
			encfield.EncryptedField{ID: "ekv_test", Data: "encrypted_access_token"},
			gomock.Any(),
			"read",
			"read activity",
			gomock.Any(),
		).
		Return(&database.OAuth2Token{}, nil)

	o := &oAuth2Connection{
		db:      db,
		encrypt: encrypt,
		connection: &mockCore.Connection{
			Id: connectionId,
			Configuration: map[string]any{
				"push_files":    false,
				"sync_activity": false,
			},
		},
		auth: &sconfig.AuthOAuth2{
			Scopes: []sconfig.Scope{
				{Id: "read"},
				{Id: "write", If: &common.Predicate{Javascript: `cfg.push_files === true`}},
				{
					Id:       "activity",
					Required: sconfig.NewScopeRequiredPredicate(&common.Predicate{Javascript: `cfg.sync_activity === true`}),
				},
			},
		},
	}

	resp := test_utils.MockGentlemenGetResponse("https://example.com", "example", func(m *gock.Request) {
		m.
			Reply(200).
			AddHeader("Content-Type", "application/json").
			BodyString(`{"access_token":"access-token","scope":"read","expires_in":3600}`)
	})

	_, err := o.createDbTokenFromResponse(context.Background(), resp, nil)
	require.NoError(t, err)
}

func TestCreateDbTokenFromResponse_UsesEffectiveScopesForRequiredDynamicRequired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := mockDb.NewMockDB(ctrl)
	encrypt := mockEncrypt.NewMockE(ctrl)
	connectionId := apid.New(apid.PrefixConnection)

	encrypt.EXPECT().
		EncryptStringForEntity(gomock.Any(), gomock.Any(), "access-token").
		Return(encfield.EncryptedField{ID: "ekv_test", Data: "encrypted_access_token"}, nil)
	db.EXPECT().
		InsertOAuth2Token(
			gomock.Any(),
			connectionId,
			nil,
			encfield.EncryptedField{},
			encfield.EncryptedField{ID: "ekv_test", Data: "encrypted_access_token"},
			gomock.Any(),
			"read",
			"read activity",
			gomock.Any(),
		).
		Return(&database.OAuth2Token{}, nil)

	o := &oAuth2Connection{
		db:      db,
		encrypt: encrypt,
		connection: &mockCore.Connection{
			Id:            connectionId,
			Configuration: map[string]any{"sync_activity": true},
		},
		auth: &sconfig.AuthOAuth2{
			Scopes: []sconfig.Scope{
				{Id: "read"},
				{
					Id:       "activity",
					Required: sconfig.NewScopeRequiredPredicate(&common.Predicate{Javascript: `cfg.sync_activity === true`}),
				},
			},
		},
	}

	resp := test_utils.MockGentlemenGetResponse("https://example.com", "example", func(m *gock.Request) {
		m.
			Reply(200).
			AddHeader("Content-Type", "application/json").
			BodyString(`{"access_token":"access-token","scope":"read","expires_in":3600}`)
	})

	_, err := o.createDbTokenFromResponse(context.Background(), resp, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required oauth2 scopes were not granted")
	assert.Contains(t, err.Error(), "activity")
}

func TestCreateDbTokenFromResponse_ScopePredicateError(t *testing.T) {
	o := &oAuth2Connection{
		connection: &mockCore.Connection{},
		auth: &sconfig.AuthOAuth2{
			Scopes: []sconfig.Scope{
				{
					Id:       "activity",
					Required: sconfig.NewScopeRequiredPredicate(&common.Predicate{Javascript: `cfg.sync_activity ===`}),
				},
			},
		},
	}

	resp := test_utils.MockGentlemenGetResponse("https://example.com", "example", func(m *gock.Request) {
		m.
			Reply(200).
			AddHeader("Content-Type", "application/json").
			BodyString(`{"access_token":"access-token","scope":"read","expires_in":3600}`)
	})

	_, err := o.createDbTokenFromResponse(context.Background(), resp, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve oauth2 scopes")
	assert.Contains(t, err.Error(), `scope "activity" required.javascript`)
}
