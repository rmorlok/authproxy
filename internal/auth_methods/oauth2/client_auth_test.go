package oauth2

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/internal/apid"
	mockLog "github.com/rmorlok/authproxy/internal/aplog/mock"
	mockRedis "github.com/rmorlok/authproxy/internal/apredis/mock"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	genmock "gopkg.in/h2non/gentleman-mock.v2"
	"gopkg.in/h2non/gock.v1"
)

func TestApplyTokenEndpointClientAuth_RejectsEmptyMethod(t *testing.T) {
	// Empty string is no longer silently defaulted — callers MUST pass a
	// resolved method (typically via AuthOAuth2.GetTokenEndpointAuthMethodOrDefault).
	// This guards against the helper drifting away from that contract.
	_, _, err := applyTokenEndpointClientAuth("", "client-id", "client-secret", url.Values{})
	require.Error(t, err)
}

func TestApplyTokenEndpointClientAuth_PostExplicit(t *testing.T) {
	out, header, err := applyTokenEndpointClientAuth(
		sconfig.TokenEndpointAuthClientSecretPost,
		"client-id", "client-secret",
		url.Values{},
	)
	require.NoError(t, err)
	assert.Empty(t, header)
	assert.Equal(t, "client-id", out.Get("client_id"))
	assert.Equal(t, "client-secret", out.Get("client_secret"))
}

func TestApplyTokenEndpointClientAuth_Basic(t *testing.T) {
	out, header, err := applyTokenEndpointClientAuth(
		sconfig.TokenEndpointAuthClientSecretBasic,
		"client-id", "client-secret",
		url.Values{"grant_type": {"refresh_token"}},
	)
	require.NoError(t, err)
	// RFC 6749 §2.3.1: client_id / client_secret URL-form-encoded then
	// colon-joined then base64-encoded.
	wantEnc := base64.StdEncoding.EncodeToString([]byte(
		url.QueryEscape("client-id") + ":" + url.QueryEscape("client-secret"),
	))
	assert.Equal(t, "Basic "+wantEnc, header)
	assert.Empty(t, out.Get("client_id"), "basic must not duplicate creds in the form body")
	assert.Empty(t, out.Get("client_secret"))
	assert.Equal(t, "refresh_token", out.Get("grant_type"))
}

func TestApplyTokenEndpointClientAuth_BasicEncodesSpecialChars(t *testing.T) {
	// Per RFC 6749 §2.3.1, the credentials are application/x-www-form-urlencoded
	// before base64. A secret containing ':' (which would otherwise corrupt the
	// userinfo split on the server) must come through url-encoded.
	clientId := "client id with space"
	clientSecret := "secret:with:colons+and&amps"

	_, header, err := applyTokenEndpointClientAuth(
		sconfig.TokenEndpointAuthClientSecretBasic,
		clientId, clientSecret,
		url.Values{},
	)
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(header, "Basic "))
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(header, "Basic "))
	require.NoError(t, err)

	// The decoded value should be qs(clientId) + ":" + qs(clientSecret) —
	// the colons inside the secret are escaped, so there is exactly one
	// unescaped colon (the user:pass separator).
	parts := strings.SplitN(string(decoded), ":", 2)
	require.Len(t, parts, 2)
	gotId, err := url.QueryUnescape(parts[0])
	require.NoError(t, err)
	gotSecret, err := url.QueryUnescape(parts[1])
	require.NoError(t, err)
	assert.Equal(t, clientId, gotId)
	assert.Equal(t, clientSecret, gotSecret)
}

func TestApplyTokenEndpointClientAuth_None(t *testing.T) {
	out, header, err := applyTokenEndpointClientAuth(
		sconfig.TokenEndpointAuthNone,
		"client-id", "",
		url.Values{"grant_type": {"authorization_code"}},
	)
	require.NoError(t, err)
	assert.Empty(t, header, "none must not set Authorization header")
	assert.Equal(t, "client-id", out.Get("client_id"))
	assert.Empty(t, out.Get("client_secret"))
}

func TestApplyTokenEndpointClientAuth_UnknownMethod(t *testing.T) {
	_, _, err := applyTokenEndpointClientAuth(
		sconfig.TokenEndpointAuthMethod("bogus"),
		"client-id", "client-secret",
		url.Values{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
}

// captureHeader is a gock matcher that records the value of a specific
// header from the incoming request. Pairs with captureBody (defined in
// pkce_test.go) when a test needs both.
func captureHeader(name string, dst *string) func(*http.Request, *gock.Request) (bool, error) {
	return func(req *http.Request, _ *gock.Request) (bool, error) {
		*dst = req.Header.Get(name)
		return true, nil
	}
}

// callbackConnFor builds a minimal oAuth2Connection wired against the
// gentleman mock client. Shared by the per-method callback tests below to
// avoid repeating 30-line fixture blocks.
func callbackConnFor(t *testing.T, ctrl *gomock.Controller, method sconfig.TokenEndpointAuthMethod, clientSecret string) (*oAuth2Connection, apid.ID) {
	cfg := config.FromRoot(&sconfig.Root{
		Public: sconfig.ServicePublic{
			ServiceHttp: sconfig.ServiceHttp{
				PortVal: common.NewIntegerValueDirect(8080),
			},
		},
	})

	h := mockH.NewFactoryWithMockingClient(ctrl)
	db := mockDb.NewMockDB(ctrl)
	encrypt := mockEncrypt.NewMockE(ctrl)
	r := mockRedis.NewMockClient(ctrl)
	logger, _ := mockLog.NewTestLogger(t)

	r.EXPECT().Del(gomock.Any(), gomock.Any()).Return(redis.NewIntCmd(context.Background())).AnyTimes()

	connectionId := apid.New(apid.PrefixConnection)
	tokenId := apid.New(apid.PrefixOAuth2Token)

	auth := &cschema.AuthOAuth2{
		Type:                    cschema.AuthTypeOAuth2,
		TokenEndpointAuthMethod: cschema.NewTokenEndpointAuthMethod(method),
		ClientId:                common.NewStringValueDirect("client-id"),
		Token: cschema.AuthOauth2Token{
			Endpoint: "https://example.com/oauth/token",
		},
	}
	if clientSecret != "" {
		auth.ClientSecret = common.NewStringValueDirect(clientSecret)
	}

	o := &oAuth2Connection{
		cfg:     cfg,
		db:      db,
		httpf:   h,
		r:       r,
		encrypt: encrypt,
		logger:  logger,
		connection: &mockCore.Connection{
			Id: connectionId,
		},
		auth: auth,
		state: &state{
			Id:          apid.New(apid.PrefixOauth2State),
			ReturnToUrl: "https://app.example.com/callback",
		},
	}

	encrypt.EXPECT().EncryptStringForEntity(gomock.Any(), gomock.Any(), "a").
		Return(encfield.EncryptedField{ID: "ekv_test", Data: "enc-a"}, nil)
	db.EXPECT().InsertOAuth2Token(gomock.Any(), connectionId, nil, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&database.OAuth2Token{Id: tokenId}, nil)

	return o, connectionId
}

// TestCallback_ClientSecretPost asserts the default behaviour: client_id and
// client_secret are sent in the form body, no Authorization header.
func TestCallback_ClientSecretPost(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	o, _ := callbackConnFor(t, ctrl, sconfig.TokenEndpointAuthClientSecretPost, "client-secret")

	var capturedBody, capturedAuth string
	genmock.
		New("https://example.com").
		Post("/oauth/token").
		MatchType("application/x-www-form-urlencoded").
		AddMatcher(captureBody(&capturedBody)).
		AddMatcher(captureHeader("Authorization", &capturedAuth)).
		Reply(200).
		AddHeader("Content-Type", "application/json").
		BodyString(`{"access_token":"a","expires_in":3600}`)

	_, err := o.CallbackFrom3rdParty(context.Background(), url.Values{"code": {"auth-code"}})
	require.NoError(t, err)

	form, err := url.ParseQuery(capturedBody)
	require.NoError(t, err)
	assert.Equal(t, "client-id", form.Get("client_id"))
	assert.Equal(t, "client-secret", form.Get("client_secret"))
	assert.Empty(t, capturedAuth, "client_secret_post must not set Authorization")
}

// TestCallback_ClientSecretBasic asserts the basic-auth path: Authorization
// header carries base64(qs(id):qs(secret)) and the form body carries
// neither client_id nor client_secret (RFC 6749 §2.3.1 forbids
// duplication).
func TestCallback_ClientSecretBasic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	o, _ := callbackConnFor(t, ctrl, sconfig.TokenEndpointAuthClientSecretBasic, "client-secret")

	var capturedBody, capturedAuth string
	genmock.
		New("https://example.com").
		Post("/oauth/token").
		MatchType("application/x-www-form-urlencoded").
		AddMatcher(captureBody(&capturedBody)).
		AddMatcher(captureHeader("Authorization", &capturedAuth)).
		Reply(200).
		AddHeader("Content-Type", "application/json").
		BodyString(`{"access_token":"a","expires_in":3600}`)

	_, err := o.CallbackFrom3rdParty(context.Background(), url.Values{"code": {"auth-code"}})
	require.NoError(t, err)

	form, err := url.ParseQuery(capturedBody)
	require.NoError(t, err)
	assert.Empty(t, form.Get("client_id"), "basic must not duplicate creds in body")
	assert.Empty(t, form.Get("client_secret"))
	assert.Equal(t, "auth-code", form.Get("code"))
	wantEnc := base64.StdEncoding.EncodeToString([]byte(
		url.QueryEscape("client-id") + ":" + url.QueryEscape("client-secret"),
	))
	assert.Equal(t, "Basic "+wantEnc, capturedAuth)
}

// TestCallback_None asserts the public-client path: client_id only in the
// form body, no client_secret, no Authorization header.
func TestCallback_None(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	o, _ := callbackConnFor(t, ctrl, sconfig.TokenEndpointAuthNone, "")

	var capturedBody, capturedAuth string
	genmock.
		New("https://example.com").
		Post("/oauth/token").
		MatchType("application/x-www-form-urlencoded").
		AddMatcher(captureBody(&capturedBody)).
		AddMatcher(captureHeader("Authorization", &capturedAuth)).
		Reply(200).
		AddHeader("Content-Type", "application/json").
		BodyString(`{"access_token":"a","expires_in":3600}`)

	_, err := o.CallbackFrom3rdParty(context.Background(), url.Values{"code": {"auth-code"}})
	require.NoError(t, err)

	form, err := url.ParseQuery(capturedBody)
	require.NoError(t, err)
	assert.Equal(t, "client-id", form.Get("client_id"))
	assert.Empty(t, form.Get("client_secret"), "none must not send client_secret")
	assert.Empty(t, capturedAuth)
}

// TestAuthOAuth2_Validate_BasicRequiresClientSecret guards the validator
// invariant that client_secret_basic must have client_secret set.
func TestAuthOAuth2_Validate_BasicRequiresClientSecret(t *testing.T) {
	a := &cschema.AuthOAuth2{
		Type:                    cschema.AuthTypeOAuth2,
		TokenEndpointAuthMethod: cschema.NewTokenEndpointAuthMethod(cschema.TokenEndpointAuthClientSecretBasic),
		ClientId:                common.NewStringValueDirect("client-id"),
		Authorization: cschema.AuthOauth2Authorization{
			Endpoint: "https://example.com/oauth/authorize",
		},
	}
	err := a.Validate(&common.ValidationContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client_secret")
}

// TestAuthOAuth2_Validate_NoneRejectsClientSecret guards the invariant that
// the none method must NOT carry a client_secret.
func TestAuthOAuth2_Validate_NoneRejectsClientSecret(t *testing.T) {
	a := &cschema.AuthOAuth2{
		Type:                    cschema.AuthTypeOAuth2,
		TokenEndpointAuthMethod: cschema.NewTokenEndpointAuthMethod(cschema.TokenEndpointAuthNone),
		ClientId:                common.NewStringValueDirect("client-id"),
		ClientSecret:            common.NewStringValueDirect("client-secret"),
		Authorization: cschema.AuthOauth2Authorization{
			Endpoint: "https://example.com/oauth/authorize",
			PKCE:     &cschema.AuthOauth2PKCE{Method: cschema.PKCEMethodS256},
		},
	}
	err := a.Validate(&common.ValidationContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client_secret")
}

// TestAuthOAuth2_Validate_NoneRequiresPKCE guards the invariant that the
// none method without PKCE is rejected (public client without
// proof-of-possession would be a security regression).
func TestAuthOAuth2_Validate_NoneRequiresPKCE(t *testing.T) {
	a := &cschema.AuthOAuth2{
		Type:                    cschema.AuthTypeOAuth2,
		TokenEndpointAuthMethod: cschema.NewTokenEndpointAuthMethod(cschema.TokenEndpointAuthNone),
		ClientId:                common.NewStringValueDirect("client-id"),
		Authorization: cschema.AuthOauth2Authorization{
			Endpoint: "https://example.com/oauth/authorize",
		},
	}
	err := a.Validate(&common.ValidationContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pkce")
}

// TestAuthOAuth2_Validate_NoneAcceptsWhenPKCESet exercises the happy path
// for public-client validation.
func TestAuthOAuth2_Validate_NoneAcceptsWhenPKCESet(t *testing.T) {
	a := &cschema.AuthOAuth2{
		Type:                    cschema.AuthTypeOAuth2,
		TokenEndpointAuthMethod: cschema.NewTokenEndpointAuthMethod(cschema.TokenEndpointAuthNone),
		ClientId:                common.NewStringValueDirect("client-id"),
		Authorization: cschema.AuthOauth2Authorization{
			Endpoint: "https://example.com/oauth/authorize",
			PKCE:     &cschema.AuthOauth2PKCE{Method: cschema.PKCEMethodS256},
		},
	}
	require.NoError(t, a.Validate(&common.ValidationContext{}))
}

// TestAuthOAuth2_Validate_RejectsExplicitEmptyTokenEndpointAuthMethod
// asserts that explicitly setting the field to "" is rejected — the
// nil-vs-empty distinction is load-bearing. nil means "use the default",
// empty string means the YAML author wrote something meaningless, and the
// validator must surface that rather than silently falling through.
func TestAuthOAuth2_Validate_RejectsExplicitEmptyTokenEndpointAuthMethod(t *testing.T) {
	a := &cschema.AuthOAuth2{
		Type:                    cschema.AuthTypeOAuth2,
		TokenEndpointAuthMethod: cschema.NewTokenEndpointAuthMethod(""),
		ClientId:                common.NewStringValueDirect("client-id"),
		ClientSecret:            common.NewStringValueDirect("client-secret"),
		Authorization: cschema.AuthOauth2Authorization{
			Endpoint: "https://example.com/oauth/authorize",
		},
	}
	err := a.Validate(&common.ValidationContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token_endpoint_auth_method")
	assert.Contains(t, err.Error(), "empty")
}

// TestAuthOAuth2_Validate_NilMethodAcceptedAsDefault asserts that omitting
// the field (nil pointer) is accepted as "use the default" — preserves
// the no-change behavior for every existing connector.
func TestAuthOAuth2_Validate_NilMethodAcceptedAsDefault(t *testing.T) {
	a := &cschema.AuthOAuth2{
		Type:         cschema.AuthTypeOAuth2,
		ClientId:     common.NewStringValueDirect("client-id"),
		ClientSecret: common.NewStringValueDirect("client-secret"),
		Authorization: cschema.AuthOauth2Authorization{
			Endpoint: "https://example.com/oauth/authorize",
		},
	}
	require.NoError(t, a.Validate(&common.ValidationContext{}))
	assert.Equal(t, cschema.TokenEndpointAuthClientSecretPost, a.GetTokenEndpointAuthMethodOrDefault())
}

func TestAuthOAuth2_Validate_ClientCredentialsRejectsAuthorizationBlock(t *testing.T) {
	a := &cschema.AuthOAuth2{
		Type:         cschema.AuthTypeOAuth2,
		GrantType:    cschema.NewOAuth2GrantType(cschema.OAuth2GrantClientCredentials),
		ClientId:     common.NewStringValueDirect("client-id"),
		ClientSecret: common.NewStringValueDirect("client-secret"),
		Authorization: cschema.AuthOauth2Authorization{
			Endpoint: "https://example.com/oauth/authorize",
		},
	}
	err := a.Validate(&common.ValidationContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authorization")
}

func TestAuthOAuth2_Validate_ClientCredentialsRequiresSecretUnlessNone(t *testing.T) {
	a := &cschema.AuthOAuth2{
		Type:      cschema.AuthTypeOAuth2,
		GrantType: cschema.NewOAuth2GrantType(cschema.OAuth2GrantClientCredentials),
		ClientId:  common.NewStringValueDirect("client-id"),
	}
	err := a.Validate(&common.ValidationContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client_secret")

	a.TokenEndpointAuthMethod = cschema.NewTokenEndpointAuthMethod(cschema.TokenEndpointAuthNone)
	require.NoError(t, a.Validate(&common.ValidationContext{}))
}

func TestAuthOAuth2_Validate_RejectsUnknownGrantType(t *testing.T) {
	a := &cschema.AuthOAuth2{
		Type:         cschema.AuthTypeOAuth2,
		GrantType:    cschema.NewOAuth2GrantType(cschema.OAuth2GrantType("implicit")),
		ClientId:     common.NewStringValueDirect("client-id"),
		ClientSecret: common.NewStringValueDirect("client-secret"),
	}
	err := a.Validate(&common.ValidationContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "grant_type")
}

// TestAuthOAuth2_Validate_RejectsUnknownTokenEndpointAuthMethod guards
// the validator against typos / unsupported RFC 7591 methods (client_secret_jwt
// etc., out of scope).
func TestAuthOAuth2_Validate_RejectsUnknownTokenEndpointAuthMethod(t *testing.T) {
	a := &cschema.AuthOAuth2{
		Type:                    cschema.AuthTypeOAuth2,
		TokenEndpointAuthMethod: cschema.NewTokenEndpointAuthMethod(cschema.TokenEndpointAuthMethod("bogus")),
		ClientId:                common.NewStringValueDirect("client-id"),
		ClientSecret:            common.NewStringValueDirect("client-secret"),
		Authorization: cschema.AuthOauth2Authorization{
			Endpoint: "https://example.com/oauth/authorize",
		},
	}
	err := a.Validate(&common.ValidationContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token_endpoint_auth_method")
}
