package helpers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureRemoteAuthProxyClaims(
	t *testing.T,
	rig *RemoteAuthProxy,
	sharedKey string,
	actorExternalID string,
	actorNamespace string,
) *jwt.AuthProxyClaims {
	t.Helper()

	var claims *jwt.AuthProxyClaims
	var parseErr error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		claims, parseErr = jwt.NewJwtTokenParserBuilder().
			WithSharedKeyString(sharedKey).
			Parse(token)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	rig.doSigned(t, actorExternalID, actorNamespace, http.MethodGet, server.URL, nil, true, http.StatusNoContent, nil)
	require.NoError(t, parseErr)
	require.NotNil(t, claims)
	return claims
}

func TestRemoteAuthProxySignsWithGlobalKeyAndProvisionsUser(t *testing.T) {
	const globalKey = "test-global-key-material"
	keyPath := filepath.Join(t.TempDir(), "global_aes.key")
	require.NoError(t, os.WriteFile(keyPath, []byte(globalKey), 0o600))

	permissions := []aschema.Permission{
		{
			Namespace: "root.smoke",
			Resources: []string{"connectors"},
			Verbs:     []string{"list"},
		},
		{
			Namespace: "root.smoke.smoke-user-20260821T010203Z-a1b2c3d4",
			Resources: []string{"connections"},
			Verbs:     []string{"create", "get", "proxy"},
		},
	}
	rig := NewRemoteAuthProxy(t, RemoteAuthProxyOptions{
		BaseURL:              "https://demo.authproxy.net",
		GlobalKey:            keyPath,
		UserActorExternalID:  "smoke-user-20260821T010203Z-a1b2c3d4",
		UserActorNamespace:   "root.smoke",
		UserActorPermissions: permissions,
		ConnectorNamespace:   "root.smoke",
		ConnectionNamespace:  "root.smoke.smoke-user-20260821T010203Z-a1b2c3d4",
	})

	userClaims := captureRemoteAuthProxyClaims(t, rig, globalKey, rig.UserActorExternalID, rig.UserActorNamespace)
	assert.True(t, userClaims.SystemSigned)
	assert.False(t, userClaims.ActorSigned)
	assert.Equal(t, rig.UserActorExternalID, userClaims.Subject)
	assert.Equal(t, rig.UserActorNamespace, userClaims.Namespace)
	require.NotNil(t, userClaims.Actor)
	assert.Equal(t, rig.UserActorExternalID, userClaims.Actor.ExternalId)
	assert.Equal(t, rig.UserActorNamespace, userClaims.Actor.Namespace)
	assert.Equal(t, permissions, userClaims.Actor.Permissions)
	assert.Equal(t, permissions, userClaims.Permissions)

	adminClaims := captureRemoteAuthProxyClaims(t, rig, globalKey, rig.AdminActorExternalID, rig.AdminActorNamespace)
	assert.True(t, adminClaims.SystemSigned)
	assert.False(t, adminClaims.ActorSigned)
	assert.Nil(t, adminClaims.Actor)
	assert.Equal(t, aschema.AllPermissions(), adminClaims.Permissions)
}
