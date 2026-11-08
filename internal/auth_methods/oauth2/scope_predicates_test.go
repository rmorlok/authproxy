package oauth2

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apjs"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
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

// scopePredicateConnection records the connection evaluator boundary without
// requiring JavaScript execution for scope ordering and error propagation tests.
type scopePredicateConnection struct {
	*mockCore.Connection
	evaluate func(context.Context, string) (bool, error)
}

// EvaluateJavascriptPredicate delegates to the test's assertion callback.
func (c *scopePredicateConnection) EvaluateJavascriptPredicate(ctx context.Context, source string) (bool, error) {
	return c.evaluate(ctx, source)
}

// GetJavascriptContext catches accidental use of the legacy unbounded path.
func (c *scopePredicateConnection) GetJavascriptContext(context.Context) (apjs.Context, error) {
	panic("scope predicates must use the bounded connection evaluator")
}

// TestEffectiveScopes_EvaluationOrder verifies lazy evaluation, caller context
// propagation, and preservation of declaration copies when requirements resolve.
func TestEffectiveScopes_EvaluationOrder(t *testing.T) {
	// Excluded scopes must never evaluate their required predicate.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sources []string
	connection := &scopePredicateConnection{
		Connection: &mockCore.Connection{},
		evaluate: func(gotCtx context.Context, source string) (bool, error) {
			require.Same(t, ctx, gotCtx)
			sources = append(sources, source)
			return source == "include", nil
		},
	}
	o := &oAuth2Connection{
		connection: connection,
		auth: &sconfig.AuthOAuth2{Scopes: []sconfig.Scope{
			{Id: "read"},
			{Id: "excluded", If: &common.Predicate{Javascript: "exclude"}, Required: sconfig.NewScopeRequiredPredicate(&common.Predicate{Javascript: "must not run"})},
			{Id: "activity", If: &common.Predicate{Javascript: "include"}, Required: sconfig.NewScopeRequiredPredicate(&common.Predicate{Javascript: "optional"})},
		}},
	}
	declared := o.auth.Scopes[2].Clone()

	got, err := o.effectiveScopes(ctx)
	require.NoError(t, err)

	// Resolve included requirements in order without rewriting the declarations.
	assert.Equal(t, []string{"exclude", "include", "optional"}, sources)
	require.Len(t, got, 2)
	assert.Equal(t, "read", got[0].Id)
	assert.Equal(t, "activity", got[1].Id)
	require.NotNil(t, got[1].Required.Bool)
	assert.False(t, *got[1].Required.Bool)
	assert.Nil(t, got[1].Required.Predicate)
	assert.Equal(t, declared, o.auth.Scopes[2])
	assert.NotSame(t, o.auth.Scopes[2].If, got[1].If)
	assert.NotSame(t, o.auth.Scopes[2].Required, got[1].Required)
}

// TestEffectiveScopes_StaticScopesSkipJavascript ensures literal requirements
// and omitted scopes do not depend on a usable JavaScript context or evaluator.
func TestEffectiveScopes_StaticScopesSkipJavascript(t *testing.T) {
	// Preserve the existing Boolean precedence for a declaration with both forms.
	both := sconfig.NewScopeRequiredBool(false)
	both.Predicate = &common.Predicate{Javascript: "must not run"}
	for name, auth := range map[string]*sconfig.AuthOAuth2{
		"no auth":   nil,
		"no scopes": {},
		"static scopes": {Scopes: []sconfig.Scope{
			{Id: "default"},
			{Id: "optional", Required: sconfig.NewScopeRequiredBool(false)},
			{Id: "boolean precedence", Required: both},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			o := &oAuth2Connection{
				connection: &scopePredicateConnection{
					Connection: &mockCore.Connection{},
					evaluate: func(context.Context, string) (bool, error) {
						t.Fatal("static scopes must not evaluate JavaScript")
						return false, nil
					},
				},
				auth: auth,
			}

			got, err := o.effectiveScopes(context.Background())
			require.NoError(t, err)
			if auth == nil || len(auth.Scopes) == 0 {
				assert.Nil(t, got)
				return
			}

			// Static output keeps defaults and literal values without aliases.
			require.Len(t, got, 3)
			assert.Nil(t, got[0].Required)
			assert.False(t, *got[1].Required.Bool)
			assert.NotSame(t, auth.Scopes[1].Required.Bool, got[1].Required.Bool)
			assert.False(t, *got[2].Required.Bool)
			assert.Nil(t, got[2].Required.Predicate)
			assert.NotNil(t, both.Predicate)
		})
	}
}

// TestEffectiveScopes_PredicateErrorCause retains evaluator errors while adding
// the scope identity and which predicate failed.
func TestEffectiveScopes_PredicateErrorCause(t *testing.T) {
	want := errors.New("connection configuration unavailable")
	for _, required := range []bool{false, true} {
		field := "if"
		scope := sconfig.Scope{Id: "activity", If: &common.Predicate{Javascript: "source"}}
		if required {
			field = "required"
			scope.If = nil
			scope.Required = sconfig.NewScopeRequiredPredicate(&common.Predicate{Javascript: "source"})
		}
		t.Run(field, func(t *testing.T) {
			// A connection preparation failure must remain distinguishable to callers.
			o := &oAuth2Connection{
				connection: &scopePredicateConnection{
					Connection: &mockCore.Connection{},
					evaluate:   func(context.Context, string) (bool, error) { return false, want },
				},
				auth: &sconfig.AuthOAuth2{Scopes: []sconfig.Scope{scope}},
			}
			got, err := o.effectiveScopes(context.Background())
			assert.Nil(t, got)
			require.ErrorIs(t, err, want)
			assert.Contains(t, err.Error(), `scope "activity" `+field+".javascript")
		})
	}
}

// TestEffectiveScopes_BoundedPredicates exercises both dynamic scope fields
// through the real bounded mock evaluator, including cancellation and privacy.
func TestEffectiveScopes_BoundedPredicates(t *testing.T) {
	for _, required := range []bool{false, true} {
		field := "if"
		if required {
			field = "required"
		}
		t.Run(field, func(t *testing.T) {
			for _, tc := range []struct {
				name          string
				source        string
				configuration map[string]any
				cancel        bool
				deadline      bool
				want          error
			}{
				{name: "execution budget", source: "for (;;) {}", want: jsruntime.ErrLimitExceeded},
				{name: "source budget", source: strings.Repeat(" ", apjs.ConnectorPredicateLimits().MaxSourceBytes) + "true", want: jsruntime.ErrLimitExceeded},
				{name: "input budget", source: "true", configuration: map[string]any{"padding": strings.Repeat("x", apjs.ConnectorPredicateLimits().MaxInputBytes)}, want: jsruntime.ErrLimitExceeded},
				{name: "private throw", source: `throw new Error(cfg.secret)`, configuration: map[string]any{"secret": "privateScopeDetail"}, want: jsruntime.ErrEvaluation},
				{name: "private syntax", source: "privateScopeDetail = ;", want: jsruntime.ErrInvalidSource},
				{name: "canceled", source: "true", cancel: true, want: context.Canceled},
				{name: "deadline during execution", source: "for (;;) {}", deadline: true, want: context.DeadlineExceeded},
			} {
				t.Run(tc.name, func(t *testing.T) {
					// Apply the same failure scenario independently to inclusion and required status.
					scope := sconfig.Scope{Id: "activity"}
					predicate := &common.Predicate{Javascript: tc.source}
					if required {
						scope.Required = sconfig.NewScopeRequiredPredicate(predicate)
					} else {
						scope.If = predicate
					}
					o := &oAuth2Connection{
						connection: &mockCore.Connection{Configuration: tc.configuration},
						auth:       &sconfig.AuthOAuth2{Scopes: []sconfig.Scope{scope}},
					}
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					if tc.cancel {
						cancel()
					}
					if tc.deadline {
						var stop context.CancelFunc
						ctx, stop = context.WithTimeout(ctx, 50*time.Millisecond)
						defer stop()
					}

					got, err := o.effectiveScopes(ctx)

					// Fail closed with a classified error and no authored or private details.
					assert.Nil(t, got)
					require.ErrorIs(t, err, tc.want)
					assert.Contains(t, err.Error(), `scope "activity" `+field+".javascript")
					assert.NotContains(t, err.Error(), "privateScopeDetail")
				})
			}
		})
	}
}

// TestEffectiveScopes_PredicateInputsAreIsolated ensures one predicate cannot
// mutate connection metadata observed by the next predicate or its Go owner.
func TestEffectiveScopes_PredicateInputsAreIsolated(t *testing.T) {
	// Use statement-list completion and Boolean wrappers while mutating every input map.
	connection := &mockCore.Connection{
		Configuration: map[string]any{"nested": map[string]any{"enabled": true}},
		Labels:        map[string]string{"env": "prod"},
		Annotations:   map[string]string{"tier": "gold"},
	}
	o := &oAuth2Connection{
		connection: connection,
		auth: &sconfig.AuthOAuth2{Scopes: []sconfig.Scope{
			{
				Id:       "mutates",
				If:       &common.Predicate{Javascript: `cfg.nested.enabled = false; labels.env = "dev"; annotations.tier = "silver"; new Boolean(true);`},
				Required: sconfig.NewScopeRequiredPredicate(&common.Predicate{Javascript: `new Boolean(!cfg.nested.enabled);`}),
			},
			{Id: "fresh", If: &common.Predicate{Javascript: `cfg.nested.enabled && labels.env === "prod" && annotations.tier === "gold";`}},
		}},
	}

	got, err := o.effectiveScopes(context.Background())
	require.NoError(t, err)

	// Each later evaluation sees the original metadata, and host maps remain intact.
	require.Len(t, got, 2)
	assert.False(t, *got[0].Required.Bool)
	assert.Equal(t, "fresh", got[1].Id)
	assert.Equal(t, map[string]any{"nested": map[string]any{"enabled": true}}, connection.Configuration)
	assert.Equal(t, map[string]string{"env": "prod"}, connection.Labels)
	assert.Equal(t, map[string]string{"tier": "gold"}, connection.Annotations)
}

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
	required, err := got[0].IsRequired(apjs.Context{})
	require.NoError(t, err)
	assert.True(t, required)
	required, err = got[1].IsRequired(apjs.Context{})
	require.NoError(t, err)
	assert.True(t, required)
	required, err = got[2].IsRequired(apjs.Context{})
	require.NoError(t, err)
	assert.False(t, required)
}

func TestEffectiveScopes_UsesConnectionJavascriptLibrary(t *testing.T) {
	library, err := apjs.CompileAndValidateLibrary(`
		function includeWriteScope() {
			return cfg.push_files === true && labels["env"] === "prod";
		}

		function requireActivityScope() {
			return annotations["tier"] === "gold";
		}
	`)
	require.NoError(t, err)

	o := &oAuth2Connection{
		connection: &mockCore.Connection{
			Configuration:     map[string]any{"push_files": true},
			Labels:            map[string]string{"env": "prod"},
			Annotations:       map[string]string{"tier": "gold"},
			JavascriptLibrary: library,
		},
		auth: &sconfig.AuthOAuth2{
			Scopes: []sconfig.Scope{
				{Id: "read"},
				{Id: "write", If: &common.Predicate{Javascript: `includeWriteScope()`}},
				{
					Id:       "activity",
					Required: sconfig.NewScopeRequiredPredicate(&common.Predicate{Javascript: `requireActivityScope()`}),
				},
			},
		},
	}

	got, err := o.effectiveScopes(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"read", "write", "activity"}, []string{got[0].Id, got[1].Id, got[2].Id})
	required, err := got[2].IsRequired(apjs.Context{})
	require.NoError(t, err)
	assert.True(t, required)
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
			connectorGeneration: &mockCore.Connector{Id: apid.New(apid.PrefixConnector)},
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
			Connection:          &mockCore.Connection{Id: apid.New(apid.PrefixConnection)},
			connectorGeneration: &mockCore.Connector{Id: apid.New(apid.PrefixConnector)},
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
		Return(`{"clientId":"client-id","clientSecret":"client-secret"}`, nil)
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
		Return(`{"clientId":"client-id","clientSecret":"client-secret"}`, nil)
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
