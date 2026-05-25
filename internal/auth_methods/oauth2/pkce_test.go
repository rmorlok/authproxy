package oauth2

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
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
	gock "gopkg.in/h2non/gock.v1"
)

func TestGeneratePKCEVerifier_LengthAndAlphabet(t *testing.T) {
	for i := 0; i < 64; i++ {
		v, err := generatePKCEVerifier()
		require.NoError(t, err)
		require.Len(t, v, pkceVerifierLength, "verifier must be %d chars", pkceVerifierLength)
		for j := 0; j < len(v); j++ {
			require.True(t, strings.IndexByte(pkceVerifierAlphabet, v[j]) >= 0,
				"verifier contains illegal char %q at idx %d (full: %q)", v[j], j, v)
		}
	}
}

func TestGeneratePKCEVerifier_UniquePerCall(t *testing.T) {
	// Birthday-paradox sanity check; 256 bits of entropy means collisions
	// across 64 draws are statistically impossible — a duplicate here would
	// flag a bug in the generator, not bad luck.
	seen := make(map[string]struct{}, 64)
	for i := 0; i < 64; i++ {
		v, err := generatePKCEVerifier()
		require.NoError(t, err)
		_, dup := seen[v]
		require.False(t, dup, "duplicate verifier generated")
		seen[v] = struct{}{}
	}
}

func TestPKCEChallengeFor_S256(t *testing.T) {
	// RFC 7636 Appendix B test vector.
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const expected = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	got, err := pkceChallengeFor(sconfig.PKCEMethodS256, verifier)
	require.NoError(t, err)
	assert.Equal(t, expected, got)

	// Independent confirmation of the encoding choice (no padding, URL-safe).
	sum := sha256.Sum256([]byte(verifier))
	assert.Equal(t, base64.RawURLEncoding.EncodeToString(sum[:]), got)
}

func TestPKCEChallengeFor_Plain(t *testing.T) {
	const verifier = "the-verifier"
	got, err := pkceChallengeFor(sconfig.PKCEMethodPlain, verifier)
	require.NoError(t, err)
	assert.Equal(t, verifier, got)
}

func TestPKCEChallengeFor_UnknownMethodErrors(t *testing.T) {
	_, err := pkceChallengeFor(sconfig.PKCEMethod("bogus"), "x")
	require.Error(t, err)
}

func TestAuthOAuth2_Validate_RejectsUnknownPKCEMethod(t *testing.T) {
	a := &cschema.AuthOAuth2{
		Type:         cschema.AuthTypeOAuth2,
		ClientId:     common.NewStringValueDirect("client-id"),
		ClientSecret: common.NewStringValueDirect("client-secret"),
		Authorization: cschema.AuthOauth2Authorization{
			Endpoint: "https://example.com/oauth/authorize",
			PKCE:     &cschema.AuthOauth2PKCE{Method: "bogus"},
		},
	}
	vc := &common.ValidationContext{}
	err := a.Validate(vc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pkce")
	assert.Contains(t, err.Error(), "method")
}

func TestAuthOAuth2_Validate_AcceptsS256(t *testing.T) {
	a := &cschema.AuthOAuth2{
		Type:         cschema.AuthTypeOAuth2,
		ClientId:     common.NewStringValueDirect("client-id"),
		ClientSecret: common.NewStringValueDirect("client-secret"),
		Authorization: cschema.AuthOauth2Authorization{
			Endpoint: "https://example.com/oauth/authorize",
			PKCE:     &cschema.AuthOauth2PKCE{Method: cschema.PKCEMethodS256},
		},
	}
	require.NoError(t, a.Validate(&common.ValidationContext{}))
}

func TestAuthOAuth2_Validate_AcceptsPlain(t *testing.T) {
	a := &cschema.AuthOAuth2{
		Type:         cschema.AuthTypeOAuth2,
		ClientId:     common.NewStringValueDirect("client-id"),
		ClientSecret: common.NewStringValueDirect("client-secret"),
		Authorization: cschema.AuthOauth2Authorization{
			Endpoint: "https://example.com/oauth/authorize",
			PKCE:     &cschema.AuthOauth2PKCE{Method: cschema.PKCEMethodPlain},
		},
	}
	require.NoError(t, a.Validate(&common.ValidationContext{}))
}

func TestAuthOAuth2_Validate_DefaultsMethodWhenBlockPresent(t *testing.T) {
	// Block present but method omitted: schema-level validation must accept;
	// runtime defaults to S256 via GetMethodOrDefault.
	a := &cschema.AuthOAuth2{
		Type:         cschema.AuthTypeOAuth2,
		ClientId:     common.NewStringValueDirect("client-id"),
		ClientSecret: common.NewStringValueDirect("client-secret"),
		Authorization: cschema.AuthOauth2Authorization{
			Endpoint: "https://example.com/oauth/authorize",
			PKCE:     &cschema.AuthOauth2PKCE{},
		},
	}
	require.NoError(t, a.Validate(&common.ValidationContext{}))
	assert.Equal(t, cschema.PKCEMethodS256, a.Authorization.PKCE.GetMethodOrDefault())
}

func TestAuthOauth2PKCE_GetMethodOrDefault_NilReturnsS256(t *testing.T) {
	var p *cschema.AuthOauth2PKCE
	assert.Equal(t, cschema.PKCEMethodS256, p.GetMethodOrDefault())
}

// TestGenerateAuthUrl_PKCEEmitsChallenge verifies the authorize URL carries
// code_challenge / code_challenge_method when PKCE is enabled and that the
// challenge matches the verifier persisted in state under S256.
func TestGenerateAuthUrl_PKCEEmitsChallenge(t *testing.T) {
	cfg := config.FromRoot(&sconfig.Root{
		Public: sconfig.ServicePublic{
			ServiceHttp: sconfig.ServiceHttp{
				PortVal: common.NewIntegerValueDirect(8080),
			},
		},
	})

	const verifier = "test-verifier-with-enough-entropy-1234567890"
	sum := sha256.Sum256([]byte(verifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(sum[:])

	o := &oAuth2Connection{
		cfg: cfg,
		connection: &configuredConnection{
			Connection: &mockCore.Connection{
				Id: apid.New(apid.PrefixConnection),
			},
			connectorVersion: &mockCore.ConnectorVersion{
				Id: apid.New(apid.PrefixConnectorVersion),
			},
		},
		auth: &cschema.AuthOAuth2{
			Type:     cschema.AuthTypeOAuth2,
			ClientId: common.NewStringValueDirect("client-id"),
			Authorization: cschema.AuthOauth2Authorization{
				Endpoint: "https://example.com/oauth/authorize",
				PKCE:     &cschema.AuthOauth2PKCE{Method: cschema.PKCEMethodS256},
			},
		},
		state: &state{
			Id:               apid.New(apid.PrefixOauth2State),
			PKCECodeVerifier: verifier,
			PKCEMethod:       cschema.PKCEMethodS256,
		},
	}

	authUrl, err := o.GenerateAuthUrl(context.Background(), &mockActorData{id: apid.New(apid.PrefixActor)})
	require.NoError(t, err)

	parsed, err := url.Parse(authUrl)
	require.NoError(t, err)
	q := parsed.Query()
	assert.Equal(t, wantChallenge, q.Get("code_challenge"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
}

// TestGenerateAuthUrl_PKCEOmittedWhenDisabled verifies the authorize URL stays
// unchanged when no PKCE block is configured — backwards-compatible behavior.
func TestGenerateAuthUrl_PKCEOmittedWhenDisabled(t *testing.T) {
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
				Id: apid.New(apid.PrefixConnection),
			},
			connectorVersion: &mockCore.ConnectorVersion{
				Id: apid.New(apid.PrefixConnectorVersion),
			},
		},
		auth: &cschema.AuthOAuth2{
			Type:     cschema.AuthTypeOAuth2,
			ClientId: common.NewStringValueDirect("client-id"),
			Authorization: cschema.AuthOauth2Authorization{
				Endpoint: "https://example.com/oauth/authorize",
			},
		},
		state: &state{
			Id: apid.New(apid.PrefixOauth2State),
		},
	}

	authUrl, err := o.GenerateAuthUrl(context.Background(), &mockActorData{id: apid.New(apid.PrefixActor)})
	require.NoError(t, err)

	parsed, err := url.Parse(authUrl)
	require.NoError(t, err)
	q := parsed.Query()
	assert.Empty(t, q.Get("code_challenge"))
	assert.Empty(t, q.Get("code_challenge_method"))
}

// captureBody is a gock matcher that records the raw request body so the
// test can assert on the form fields the proxy actually sent. The matcher
// always returns true — it inspects, it does not gate.
func captureBody(dst *string) func(*http.Request, *gock.Request) (bool, error) {
	return func(req *http.Request, _ *gock.Request) (bool, error) {
		if req.Body == nil {
			return true, nil
		}
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return false, err
		}
		*dst = string(b)
		req.Body = io.NopCloser(strings.NewReader(*dst))
		return true, nil
	}
}

// TestCallbackFrom3rdParty_PKCESendsVerifier asserts that when state carries
// a verifier, the token-exchange POST body includes code_verifier with the
// matching value.
func TestCallbackFrom3rdParty_PKCESendsVerifier(t *testing.T) {
	const verifier = "callback-verifier-1234567890abcdef-fixedlen"

	cfg := config.FromRoot(&sconfig.Root{
		Public: sconfig.ServicePublic{
			ServiceHttp: sconfig.ServiceHttp{
				PortVal: common.NewIntegerValueDirect(8080),
			},
		},
	})

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h := mockH.NewFactoryWithMockingClient(ctrl)
	db := mockDb.NewMockDB(ctrl)
	encrypt := mockEncrypt.NewMockE(ctrl)
	r := mockRedis.NewMockClient(ctrl)
	logger, _ := mockLog.NewTestLogger(t)

	r.EXPECT().Del(gomock.Any(), gomock.Any()).Return(redis.NewIntCmd(context.Background())).AnyTimes()

	connectionId := apid.New(apid.PrefixConnection)
	tokenId := apid.New(apid.PrefixOAuth2Token)

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
		auth: &cschema.AuthOAuth2{
			Type:         cschema.AuthTypeOAuth2,
			ClientId:     common.NewStringValueDirect("client-id"),
			ClientSecret: common.NewStringValueDirect("client-secret"),
			Token: cschema.AuthOauth2Token{
				Endpoint: "https://example.com/oauth/token",
			},
		},
		state: &state{
			Id:               apid.New(apid.PrefixOauth2State),
			ReturnToUrl:      "https://app.example.com/callback",
			PKCECodeVerifier: verifier,
			PKCEMethod:       cschema.PKCEMethodS256,
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
		BodyString(`{"access_token":"a","expires_in":3600}`)

	encrypt.EXPECT().EncryptStringForEntity(gomock.Any(), gomock.Any(), "a").
		Return(encfield.EncryptedField{ID: "ekv_test", Data: "enc-a"}, nil)
	db.EXPECT().InsertOAuth2Token(gomock.Any(), connectionId, nil, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&database.OAuth2Token{Id: tokenId}, nil)

	_, err := o.CallbackFrom3rdParty(context.Background(), url.Values{"code": {"auth-code"}})
	require.NoError(t, err)

	form, err := url.ParseQuery(capturedBody)
	require.NoError(t, err)
	assert.Equal(t, verifier, form.Get("code_verifier"))
	assert.Equal(t, "auth-code", form.Get("code"))
	assert.Equal(t, "authorization_code", form.Get("grant_type"))
}

// TestCallbackFrom3rdParty_PKCEOmittedWhenStateHasNoVerifier asserts that a
// connection set up without PKCE does not send code_verifier — backwards
// compat with existing connectors.
func TestCallbackFrom3rdParty_PKCEOmittedWhenStateHasNoVerifier(t *testing.T) {
	cfg := config.FromRoot(&sconfig.Root{
		Public: sconfig.ServicePublic{
			ServiceHttp: sconfig.ServiceHttp{
				PortVal: common.NewIntegerValueDirect(8080),
			},
		},
	})

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h := mockH.NewFactoryWithMockingClient(ctrl)
	db := mockDb.NewMockDB(ctrl)
	encrypt := mockEncrypt.NewMockE(ctrl)
	r := mockRedis.NewMockClient(ctrl)
	logger, _ := mockLog.NewTestLogger(t)

	r.EXPECT().Del(gomock.Any(), gomock.Any()).Return(redis.NewIntCmd(context.Background())).AnyTimes()

	connectionId := apid.New(apid.PrefixConnection)
	tokenId := apid.New(apid.PrefixOAuth2Token)

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
		auth: &cschema.AuthOAuth2{
			Type:         cschema.AuthTypeOAuth2,
			ClientId:     common.NewStringValueDirect("client-id"),
			ClientSecret: common.NewStringValueDirect("client-secret"),
			Token: cschema.AuthOauth2Token{
				Endpoint: "https://example.com/oauth/token",
			},
		},
		state: &state{
			Id:          apid.New(apid.PrefixOauth2State),
			ReturnToUrl: "https://app.example.com/callback",
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
		BodyString(`{"access_token":"a","expires_in":3600}`)

	encrypt.EXPECT().EncryptStringForEntity(gomock.Any(), gomock.Any(), "a").
		Return(encfield.EncryptedField{ID: "ekv_test", Data: "enc-a"}, nil)
	db.EXPECT().InsertOAuth2Token(gomock.Any(), connectionId, nil, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&database.OAuth2Token{Id: tokenId}, nil)

	_, err := o.CallbackFrom3rdParty(context.Background(), url.Values{"code": {"auth-code"}})
	require.NoError(t, err)

	form, err := url.ParseQuery(capturedBody)
	require.NoError(t, err)
	assert.Empty(t, form.Get("code_verifier"))
}
