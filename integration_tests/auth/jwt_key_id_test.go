//go:build integration

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	gojwt "github.com/golang-jwt/jwt/v5"
	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestJWTKeyId(t *testing.T) {
	const (
		actorWithKey    = "jwt-kid-actor"
		actorWithoutKey = "jwt-kid-keyless-actor"
	)

	env := helpers.Setup(t, helpers.SetupOptions{
		Service: helpers.ServiceTypeAdminAPI,
		ConfigureRoot: func(root *sconfig.Root) {
			root.SystemAuth.JwtSigningKey = publicPrivateKey(
				"./test_data/system_keys/system.pub",
				"./test_data/system_keys/system",
			)
			root.SystemAuth.Actors = &sconfig.ConfiguredActors{
				InnerVal: sconfig.ConfiguredActorsList{
					&actorschema.Actor{
						TypeMeta: meta.NewTypeMeta(actorschema.ActorKind),
						Metadata: meta.ObjectMeta{Namespace: sconfig.RootNamespace},
						Spec: actorschema.ActorSpec{
							ExternalId:  actorWithKey,
							SigningKey:  publicPrivateKey("./test_data/admin_user_keys/bobdole.pub", ""),
							Permissions: aschema.AllPermissions(),
						},
					},
				},
			}
		},
	})
	defer env.Cleanup()

	// Configured actors always carry a signing key, so create an actor without one directly.
	require.NoError(t, env.Db.CreateActor(context.Background(), &database.Actor{
		Id:          apid.New(apid.PrefixActor),
		Namespace:   sconfig.RootNamespace,
		ExternalId:  actorWithoutKey,
		Permissions: aschema.AllPermissions(),
	}))

	request := func(t *testing.T, subject, token string) int {
		t.Helper()
		req := httptest.NewRequest(
			http.MethodGet,
			"/api/v1/actors/external-id/"+subject+"?namespace=root",
			nil,
		)
		service.SetJwtRequestHeader(req, token)
		recorder := httptest.NewRecorder()
		env.ApiGin.ServeHTTP(recorder, req)
		return recorder.Code
	}

	builderToken := func(t *testing.T, subject, privateKeyPath string) string {
		t.Helper()
		token, err := jwt.NewJwtTokenBuilder().
			WithActorExternalId(subject).
			WithPrivateKeyPath(privateKeyPath).
			WithAudience(string(sconfig.ServiceIdAdminApi)).
			TokenCtx(context.Background())
		require.NoError(t, err)

		parsed, _, err := gojwt.NewParser().ParseUnverified(token, &jwt.AuthProxyClaims{})
		require.NoError(t, err)
		require.NotEmpty(t, parsed.Header[jwt.KeyIdHeader])
		return token
	}

	// rawToken signs without the token builder so the kid header can be omitted or forged.
	rawToken := func(t *testing.T, subject, privateKeyPath string, kid *string) string {
		t.Helper()
		data, err := os.ReadFile(privateKeyPath)
		require.NoError(t, err)
		key, err := ssh.ParseRawPrivateKey(data)
		require.NoError(t, err)

		method := gojwt.SigningMethod(gojwt.SigningMethodRS256)
		if _, ok := key.(interface{ Seed() []byte }); ok {
			method = gojwt.SigningMethodEdDSA
		}

		token := gojwt.NewWithClaims(method, &jwt.AuthProxyClaims{
			RegisteredClaims: gojwt.RegisteredClaims{
				Subject:  subject,
				Audience: []string{string(sconfig.ServiceIdAdminApi)},
			},
		})
		if kid != nil {
			token.Header[jwt.KeyIdHeader] = *kid
		}
		s, err := token.SignedString(key)
		require.NoError(t, err)
		return s
	}

	unknownKid := "ap1.AAAAAAAAAAAAAAAAAAAAAA"

	t.Run("actor key with kid", func(t *testing.T) {
		token := builderToken(t, actorWithKey, "./test_data/admin_user_keys/bobdole")
		require.Equal(t, http.StatusOK, request(t, actorWithKey, token))
	})

	t.Run("global key with kid for actor with key", func(t *testing.T) {
		token := builderToken(t, actorWithKey, "./test_data/system_keys/system")
		require.Equal(t, http.StatusOK, request(t, actorWithKey, token))
	})

	t.Run("global key with kid for keyless actor", func(t *testing.T) {
		token := builderToken(t, actorWithoutKey, "./test_data/system_keys/system")
		require.Equal(t, http.StatusOK, request(t, actorWithoutKey, token))
	})

	t.Run("legacy actor key token without kid", func(t *testing.T) {
		token := rawToken(t, actorWithKey, "./test_data/admin_user_keys/bobdole", nil)
		require.Equal(t, http.StatusOK, request(t, actorWithKey, token))
	})

	t.Run("legacy global key token without kid", func(t *testing.T) {
		token := rawToken(t, actorWithoutKey, "./test_data/system_keys/system", nil)
		require.Equal(t, http.StatusOK, request(t, actorWithoutKey, token))
	})

	t.Run("unknown kid is rejected", func(t *testing.T) {
		token := rawToken(t, actorWithKey, "./test_data/admin_user_keys/bobdole", &unknownKid)
		require.Equal(t, http.StatusUnauthorized, request(t, actorWithKey, token))
	})

	t.Run("kid for a different key is rejected", func(t *testing.T) {
		globalToken := builderToken(t, actorWithKey, "./test_data/system_keys/system")
		parsed, _, err := gojwt.NewParser().ParseUnverified(globalToken, &jwt.AuthProxyClaims{})
		require.NoError(t, err)
		globalKid := parsed.Header[jwt.KeyIdHeader].(string)

		token := rawToken(t, actorWithKey, "./test_data/admin_user_keys/bobdole", &globalKid)
		require.Equal(t, http.StatusUnauthorized, request(t, actorWithKey, token))
	})
}
