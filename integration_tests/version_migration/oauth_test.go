//go:build integration

package version_migration

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const oauthMigrationTimeout = 20 * time.Second

type oauthMigrationRig struct {
	env          *helpers.IntegrationTestEnv
	provider     *helpers.OAuth2TestProvider
	connectorID  apid.ID
	clientKey    string
	clientSecret string
	userID       string
	returnToURL  string
}

func TestOAuth2VersionMigrationScopeExpansionRequiresReauth(t *testing.T) {
	rig := newOAuthMigrationRig(t, "oauth-migration-scope-reauth")
	connectionID := rig.createHealthyReadConnection(t)
	initialToken := rig.requireTokenScopes(t, connectionID, "read", "read")

	rig.publishRequiredScopeVersion(t, []string{"read", "write"})
	rig.provider.Script(rig.clientKey, helpers.EndpointRefresh, helpers.ScriptAction{
		ScopeOverride: stringPointer("read"),
	})

	rig.env.MigrateConnectionVersionAndWait(t, connectionID, 2, oauthMigrationTimeout)

	migrated := rig.env.GetConnection(t, connectionID)
	require.Equal(t, uint64(2), migrated.ConnectorVersion)
	require.Equal(t, database.ConnectionStateConfigured, migrated.State)
	require.Equal(t, database.ConnectionHealthStateUnhealthy, migrated.HealthState)
	require.Nil(t, migrated.SetupStep)

	migrationToken := rig.requireTokenScopes(t, connectionID, "read write", "read")
	assert.NotEqual(t, initialToken.Id, migrationToken.Id,
		"the target-version refresh should persist the provider's granted scope set")

	notification := rig.env.RequireSingleActiveConnectionNotification(
		t,
		connectionID,
		helpers.NotificationKeySuffixAuthRequired,
	)
	assertNoAuthProxyCopy(t, notification)
	assert.True(t, notification.CanAction)
	assert.Contains(t, notification.ActionUrl, "action=reauth")

	reauthRedirect := rig.env.ReauthOAuth2Connection(t, connectionID, rig.returnToURL)
	rig.requireRedirectScopes(t, reauthRedirect, "read write")
	rig.authorizeAndDeliverCallback(t, reauthRedirect, "read write")

	afterReauth := rig.env.GetConnection(t, connectionID)
	require.Equal(t, uint64(2), afterReauth.ConnectorVersion)
	require.Equal(t, database.ConnectionStateConfigured, afterReauth.State)
	require.Equal(t, database.ConnectionHealthStateHealthy, afterReauth.HealthState)
	require.Nil(t, afterReauth.SetupStep)
	require.Nil(t, afterReauth.SetupError)

	reauthedToken := rig.requireTokenScopes(t, connectionID, "read write", "read write")
	assert.NotEqual(t, migrationToken.Id, reauthedToken.Id,
		"successful reauthorization should replace the token from the failed migration refresh")
	assert.Equal(t, http.StatusOK, rig.proxyStatus(t, connectionID))

	rig.env.RequireNoActiveConnectionNotifications(t, connectionID)
	rig.env.RequireResolvedConnectionNotification(t, connectionID, helpers.NotificationKeySuffixAuthRequired)
}

func TestOAuth2VersionMigrationScopeExpansionRollbackRestoresConnection(t *testing.T) {
	rig := newOAuthMigrationRig(t, "oauth-migration-scope-rollback")
	connectionID := rig.createHealthyReadConnection(t)

	rig.publishRequiredScopeVersion(t, []string{"read", "write"})
	rig.provider.Script(rig.clientKey, helpers.EndpointRefresh, helpers.ScriptAction{
		ScopeOverride: stringPointer("read"),
	})
	rig.env.MigrateConnectionVersionAndWait(t, connectionID, 2, oauthMigrationTimeout)

	requiresReauth := rig.env.GetConnection(t, connectionID)
	require.Equal(t, uint64(2), requiresReauth.ConnectorVersion)
	require.Equal(t, database.ConnectionHealthStateUnhealthy, requiresReauth.HealthState)
	rig.env.RequireSingleActiveConnectionNotification(
		t,
		connectionID,
		helpers.NotificationKeySuffixAuthRequired,
	)

	rollback := rig.env.MigrateConnectionVersionAndWait(t, connectionID, 1, oauthMigrationTimeout)
	require.Equal(t, uint64(2), rollback.SourceVersion)
	require.Equal(t, uint64(1), rollback.TargetVersion)

	restored := rig.env.GetConnection(t, connectionID)
	require.Equal(t, uint64(1), restored.ConnectorVersion)
	require.Equal(t, database.ConnectionStateConfigured, restored.State)
	require.Equal(t, database.ConnectionHealthStateHealthy, restored.HealthState)
	require.Nil(t, restored.SetupStep)
	require.Nil(t, restored.SetupError)
	rig.requireTokenScopes(t, connectionID, "read", "read")
	assert.Equal(t, http.StatusOK, rig.proxyStatus(t, connectionID))

	rig.env.RequireNoActiveConnectionNotifications(t, connectionID)
	rig.env.RequireResolvedConnectionNotification(t, connectionID, helpers.NotificationKeySuffixAuthRequired)
}

func newOAuthMigrationRig(t *testing.T, name string) *oauthMigrationRig {
	t.Helper()

	provider := helpers.NewOAuth2TestProvider(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	clientKey := name + "-client-" + suffix
	clientSecret := name + "-secret-" + suffix
	connectorID := apid.New(apid.PrefixConnectorVersion)

	env := helpers.Setup(t, helpers.SetupOptions{
		Service:       helpers.ServiceTypeAPI,
		IncludePublic: true,
	})
	t.Cleanup(env.Cleanup)
	helpers.StartCoreWorkflowWorker(t, env)

	rig := &oauthMigrationRig{
		env:          env,
		provider:     provider,
		connectorID:  connectorID,
		clientKey:    clientKey,
		clientSecret: clientSecret,
		returnToURL:  env.Cfg.GetRoot().Public.GetBaseUrl() + "/connections",
	}

	created := env.CreateConnector(t, rig.connectorDefinition(name+"-v1", []string{"read"}), nil, nil)
	require.Equal(t, uint64(1), created.Version)
	primary := env.ForceConnectorVersionState(t, created.Id, created.Version, schemaapi.ConnectorVersionStatePrimary)
	require.Equal(t, schemaapi.ConnectorVersionStatePrimary, primary.State)

	registered := provider.CreateClient(helpers.CreateClientRequest{
		Key:                     clientKey,
		Secret:                  clientSecret,
		RedirectURI:             env.PublicOAuthCallbackURL(),
		TokenEndpointAuthMethod: helpers.TokenEndpointAuthPost,
		Scope:                   "read write",
	})
	require.Equal(t, clientKey, registered.Key)

	user := provider.CreateUser(helpers.CreateUserRequest{
		Username: name + "-" + suffix + "@example.com",
		Password: "p4ssw0rd-" + suffix,
		Email:    name + "-" + suffix + "@example.com",
	})
	require.NotEmpty(t, user.ID)
	rig.userID = user.ID

	return rig
}

func (r *oauthMigrationRig) connectorDefinition(displayName string, scopes []string) sconfig.Connector {
	return helpers.NewOAuth2Connector(r.connectorID, displayName, r.provider, helpers.OAuth2ConnectorOptions{
		ClientID:     r.clientKey,
		ClientSecret: r.clientSecret,
		Scopes:       scopes,
	})
}

func (r *oauthMigrationRig) createHealthyReadConnection(t *testing.T) string {
	t.Helper()

	connectionID, redirectURL := r.env.InitiateOAuth2Connection(t, r.connectorID, r.returnToURL)
	r.requireRedirectScopes(t, redirectURL, "read")
	r.authorizeAndDeliverCallback(t, redirectURL, "read")

	conn := r.env.GetConnection(t, connectionID)
	require.Equal(t, uint64(1), conn.ConnectorVersion)
	require.Equal(t, database.ConnectionStateConfigured, conn.State)
	require.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState)
	require.Nil(t, conn.SetupStep)
	require.Nil(t, conn.SetupError)
	return connectionID
}

func (r *oauthMigrationRig) publishRequiredScopeVersion(t *testing.T, scopes []string) {
	t.Helper()

	published := r.env.PublishConnectorVersion(
		t,
		r.connectorID,
		r.connectorDefinition("oauth-migration-v2", scopes),
		nil,
		nil,
	)
	require.Equal(t, uint64(2), published.Version)
}

func (r *oauthMigrationRig) requireRedirectScopes(t *testing.T, redirectURL, want string) {
	t.Helper()

	authorizeURL := r.env.FollowOAuth2Redirect(t, redirectURL)
	parsed, err := url.Parse(authorizeURL)
	require.NoError(t, err)
	require.Equal(t, want, parsed.Query().Get("scope"))
}

func (r *oauthMigrationRig) authorizeAndDeliverCallback(t *testing.T, redirectURL, grantedScope string) {
	t.Helper()

	parsed, err := url.Parse(redirectURL)
	require.NoError(t, err)
	stateID := parsed.Query().Get("state_id")
	require.NotEmpty(t, stateID, "redirect should embed state_id: %s", redirectURL)

	authorize := r.provider.Authorize(helpers.AuthorizeRequest{
		ClientID:    r.clientKey,
		UserID:      r.userID,
		RedirectURI: r.env.PublicOAuthCallbackURL(),
		Scope:       grantedScope,
		State:       stateID,
		Decision:    helpers.AuthorizeApprove,
	})
	callback, err := url.Parse(authorize.RedirectURL)
	require.NoError(t, err)
	code := callback.Query().Get("code")
	require.NotEmpty(t, code)

	location := r.env.DeliverOAuth2Callback(t, r.env.ForgeOAuth2CallbackURL(stateID, code))
	require.Truef(t, strings.HasPrefix(location, r.returnToURL),
		"OAuth callback should return to %q, got %q", r.returnToURL, location)
}

func (r *oauthMigrationRig) requireTokenScopes(t *testing.T, connectionID, wantRequested, wantGranted string) *database.OAuth2Token {
	t.Helper()

	token := r.env.GetOAuth2Token(t, connectionID)
	require.NotNil(t, token)
	assert.Equal(t, wantRequested, token.RequestedScopes)
	assert.Equal(t, wantGranted, token.Scopes)
	return token
}

func (r *oauthMigrationRig) proxyStatus(t *testing.T, connectionID string) int {
	t.Helper()
	w := r.env.DoProxyRequest(t, connectionID, r.provider.ResourceURL("/echo"), http.MethodGet)
	return w.Code
}

func stringPointer(value string) *string {
	return &value
}
