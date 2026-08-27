//go:build integration

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/apauth/service"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

func TestJWTSigningKeyFallbackForActorWithKey(t *testing.T) {
	const actorExternalID = "jwt-fallback-actor"

	env := helpers.Setup(t, helpers.SetupOptions{
		Service: helpers.ServiceTypeAdminAPI,
		ConfigureRoot: func(root *sconfig.Root) {
			root.SystemAuth.JwtSigningKey = publicPrivateKey(
				"./test_data/system_keys/system.pub",
				"./test_data/system_keys/system",
			)
			root.SystemAuth.Actors = &sconfig.ConfiguredActors{
				InnerVal: sconfig.ConfiguredActorsList{
					&sconfig.ConfiguredActor{
						ExternalId: actorExternalID,
						Key: publicPrivateKey(
							"./test_data/admin_user_keys/bobdole.pub",
							"",
						),
						Permissions: aschema.AllPermissions(),
					},
				},
			}
		},
	})
	defer env.Cleanup()

	actor, err := env.Db.GetActorByExternalId(context.Background(), sconfig.RootNamespace, actorExternalID)
	require.NoError(t, err)
	require.True(t, actor.CanSelfSign(), "integration fixture must have an actor-specific verification key")

	token, err := jwt.NewJwtTokenBuilder().
		WithActorExternalId(actorExternalID).
		WithPrivateKeyPath("./test_data/system_keys/system").
		WithAudience(string(sconfig.ServiceIdAdminApi)).
		TokenCtx(context.Background())
	require.NoError(t, err)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/actors/external-id/"+actorExternalID+"?namespace=root",
		nil,
	)
	service.SetJwtRequestHeader(req, token)
	recorder := httptest.NewRecorder()
	env.ApiGin.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
}

func publicPrivateKey(publicPath, privatePath string) *sconfig.Key {
	key := &sconfig.KeyPublicPrivate{
		PublicKey: &sconfig.KeyData{
			InnerVal: &sconfig.KeyDataFile{Path: publicPath},
		},
	}
	if privatePath != "" {
		key.PrivateKey = &sconfig.KeyData{
			InnerVal: &sconfig.KeyDataFile{Path: privatePath},
		}
	}

	return &sconfig.Key{InnerVal: key}
}
