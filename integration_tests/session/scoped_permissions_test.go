//go:build integration

package session

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

func TestScopedPermissionsPersistInBrowserSession(t *testing.T) {
	env := helpers.Setup(t, helpers.SetupOptions{
		Service:         helpers.ServiceTypeAPI,
		StartHTTPServer: true,
		IncludePublic:   true,
	})
	defer env.Cleanup()

	ctx := context.Background()
	actorPermissions := aschema.AllPermissions()
	tokenPermissions := aschema.PermissionsSingle("root.**", "connectors", "list")

	t.Run("new session retains jwt restrictions", func(t *testing.T) {
		client := newBrowserClient(t)
		token, err := env.PublicAuthUtil.GenerateScopedBearerToken(
			ctx,
			fmt.Sprintf("new-scoped-session-%d", time.Now().UnixNano()),
			config.RootNamespace,
			actorPermissions,
			tokenPermissions,
		)
		require.NoError(t, err)

		initiateSession(t, client, env.PublicURL, token)
		requireStatus(t, client, env.PublicURL+"/api/v1/connectors?limit=1", http.StatusOK)
		requireStatus(t, client, env.PublicURL+"/api/v1/connections?limit=1", http.StatusForbidden)
	})

	t.Run("scoped jwt narrows existing same actor session", func(t *testing.T) {
		client := newBrowserClient(t)
		externalID := fmt.Sprintf("existing-scoped-session-%d", time.Now().UnixNano())

		unscopedToken, err := env.PublicAuthUtil.GenerateScopedBearerToken(
			ctx,
			externalID,
			config.RootNamespace,
			actorPermissions,
			nil,
		)
		require.NoError(t, err)
		initiateSession(t, client, env.PublicURL, unscopedToken)
		requireStatus(t, client, env.PublicURL+"/api/v1/connections?limit=1", http.StatusOK)

		scopedToken, err := env.PublicAuthUtil.GenerateScopedBearerToken(
			ctx,
			externalID,
			config.RootNamespace,
			actorPermissions,
			tokenPermissions,
		)
		require.NoError(t, err)
		initiateSession(t, client, env.PublicURL, scopedToken)

		requireStatus(t, client, env.PublicURL+"/api/v1/connectors?limit=1", http.StatusOK)
		requireStatus(t, client, env.PublicURL+"/api/v1/connections?limit=1", http.StatusForbidden)
	})
}

func newBrowserClient(t *testing.T) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	return &http.Client{Jar: jar}
}

func initiateSession(t *testing.T, client *http.Client, publicURL, token string) {
	t.Helper()

	body, err := json.Marshal(schemaapi.SessionInitiateParams{
		ReturnToUrl: publicURL + "/connectors",
	})
	require.NoError(t, err)
	req, err := http.NewRequest(
		http.MethodPost,
		publicURL+"/api/v1/session/_initiate",
		bytes.NewReader(body),
	)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	require.NoError(t, err)
	respBody, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	require.NoError(t, resp.Body.Close())
	require.Equalf(t, http.StatusOK, resp.StatusCode, "session response: %s", respBody)

	parsedPublicURL, err := url.Parse(publicURL)
	require.NoError(t, err)
	hasSessionCookie := false
	for _, cookie := range client.Jar.Cookies(parsedPublicURL) {
		if cookie.Name == "SESSION-ID" {
			hasSessionCookie = true
			break
		}
	}
	require.True(t, hasSessionCookie, "session initiation should set a SESSION-ID cookie")
}

func requireStatus(t *testing.T, client *http.Client, endpoint string, expected int) {
	t.Helper()

	resp, err := client.Get(endpoint)
	require.NoError(t, err)
	respBody, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	require.NoError(t, resp.Body.Close())
	require.Equalf(t, expected, resp.StatusCode, "response body: %s", respBody)
}
