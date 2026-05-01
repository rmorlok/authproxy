package helpers

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	auth2 "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/rmorlok/authproxy/internal/service"
	"github.com/rmorlok/authproxy/internal/service/admin_api"
	api_service "github.com/rmorlok/authproxy/internal/service/api"
	public_service "github.com/rmorlok/authproxy/internal/service/public"
	"github.com/stretchr/testify/require"
)

// ServiceType specifies which authproxy service to start.
type ServiceType string

const (
	// ServiceTypeAPI starts the API service (includes proxy, connections, connectors, namespaces, actors).
	ServiceTypeAPI ServiceType = "api"

	// ServiceTypeAdminAPI starts the admin API service (CRUD management, no proxy).
	ServiceTypeAdminAPI ServiceType = "admin-api"
)

// IntegrationTestEnv holds all the components needed for an integration test.
type IntegrationTestEnv struct {
	// Gin is the HTTP router for in-process request testing (when StartHTTPServer is false).
	Gin *gin.Engine

	// PublicGin is the public service router. Only populated when SetupOptions.IncludePublic
	// is true. Tests that drive an OAuth flow need this to deliver `/oauth2/redirect` and
	// `/oauth2/callback` requests, which only the public service hosts.
	PublicGin *gin.Engine

	// Cfg is the loaded configuration.
	Cfg config.C

	// AuthUtil provides JWT signing helpers for test requests.
	AuthUtil *auth2.AuthTestUtil

	// PublicAuthUtil signs requests for the public service (audience=public). Only
	// populated when SetupOptions.IncludePublic is true.
	PublicAuthUtil *auth2.AuthTestUtil

	// Db is the database connection.
	Db database.DB

	// Core is the core business logic service.
	Core coreIface.C

	// Logger is the test logger.
	Logger *slog.Logger

	// ServerURL is the base URL of the real HTTP server (e.g. "http://127.0.0.1:54321").
	// Only set when StartHTTPServer is true.
	ServerURL string

	// BearerToken is a pre-signed admin JWT for making HTTP requests to ServerURL.
	BearerToken string

	// DM provides access to all service dependencies.
	DM *service.DependencyManager

	// Cleanup tears down the test environment. Always defer this.
	Cleanup func()
}

// SetupOptions configures how the integration test environment is created.
type SetupOptions struct {
	// Connectors to load into the config (merged with config file connectors).
	Connectors []sconfig.Connector

	// ConfigPath overrides the default integration config path.
	// Defaults to integration_tests/config/integration.yaml.
	ConfigPath string

	// Service specifies which service to start. Defaults to ServiceTypeAPI.
	Service ServiceType

	// StartHTTPServer starts a real HTTP server on a random port.
	// When true, ServerURL and BearerToken are populated.
	// When false, use Gin with httptest.ResponseRecorder for in-process testing.
	StartHTTPServer bool

	// IncludePublic also wires up the public service's gin engine so tests can
	// drive `/oauth2/redirect` and `/oauth2/callback`. The public engine shares
	// the same DependencyManager (DB, Redis, core, encryption) as the primary
	// service. Only meaningful for in-process testing (StartHTTPServer=false).
	IncludePublic bool
}

// repoRoot returns the absolute path to the repository root.
func repoRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	// helpers/setup.go -> integration_tests/ -> repo root
	return filepath.Dir(filepath.Dir(filepath.Dir(filename)))
}

// Setup creates a full integration test environment backed by real infrastructure
// (Postgres, Redis, ClickHouse, MinIO) started via docker-compose.
func Setup(t *testing.T, opts SetupOptions) *IntegrationTestEnv {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)

	root := repoRoot()

	if opts.Service == "" {
		opts.Service = ServiceTypeAPI
	}

	// Load config
	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = filepath.Join(root, "integration_tests", "config", "integration.yaml")
	}

	// Change to repo root so relative key paths in config resolve correctly
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { os.Chdir(origDir) })

	cfg, err := config.LoadConfig(configPath)
	require.NoError(t, err, "failed to load integration test config from %s", configPath)

	// Create an isolated database for this test using pgtestdb.
	// This ensures each test gets a fresh, clean database.
	// MustApplyBlankTestDbConfig reads connection info from POSTGRES_TEST_* env vars
	// and updates cfg.GetRoot().Database to point at the new isolated database.
	cfg, _ = database.MustApplyBlankTestDbConfig(t, cfg)

	// Merge test-specific connectors into config
	if len(opts.Connectors) > 0 {
		cfgRoot := cfg.GetRoot()
		if cfgRoot.Connectors == nil {
			cfgRoot.Connectors = &sconfig.Connectors{}
		}
		cfgRoot.Connectors.LoadFromList = append(cfgRoot.Connectors.LoadFromList, opts.Connectors...)
	}

	// Create dependency manager and run migrations
	serviceId := string(opts.Service)
	dm := service.NewDependencyManager(serviceId, cfg)

	dm.AutoMigrateDatabase()

	// Each test gets an isolated database via pgtestdb but shares the same Redis.
	// The Redis sentinel in SyncKeysToDatabase ("recently synced") can cause a
	// concurrent test's sync to be skipped, leaving the new database without a
	// global AES key. Passing nil for Redis bypasses the sentinel so this test's
	// database is always seeded with the key, regardless of what other packages
	// are doing concurrently.
	//
	// This must happen before AutoMigrateLogStorageService because that call
	// can trigger GetEncryptService(), which starts the syncLoop goroutine that
	// immediately tries to read the global key from the database.
	require.NoError(t, encrypt.SyncKeysToDatabase(context.Background(), cfg, dm.GetDatabase(), dm.GetLogger(), nil))

	dm.AutoMigrateLogStorageService()
	dm.AutoMigrateCore()
	dm.AutoMigratePredefinedActors()

	// Get the appropriate server
	var httpServer *http.Server
	var serviceIdForAuth sconfig.ServiceId

	switch opts.Service {
	case ServiceTypeAdminAPI:
		httpServer, _, err = admin_api.GetGinServer(dm)
		serviceIdForAuth = sconfig.ServiceIdAdminApi
	default:
		httpServer, _, err = api_service.GetGinServer(dm)
		serviceIdForAuth = sconfig.ServiceIdApi
	}
	require.NoError(t, err, "failed to create %s server", opts.Service)

	// Build the auth test utility for signing requests
	httpSvc := cfg.MustGetService(serviceIdForAuth).(sconfig.HttpService)
	authService := auth2.NewService(
		cfg,
		httpSvc,
		dm.GetDatabase(),
		dm.GetRedisClient(),
		dm.GetEncryptService(),
		dm.GetLogger(),
	)
	authUtil := auth2.NewAuthTestUtil(cfg, authService, serviceIdForAuth)

	env := &IntegrationTestEnv{
		Cfg:      cfg,
		AuthUtil: authUtil,
		Db:       dm.GetDatabase(),
		Core:     dm.GetCoreService(),
		Logger:   dm.GetLogger(),
		DM:       dm,
		Cleanup:  func() {},
	}

	if opts.IncludePublic {
		publicServer, _, publicErr := public_service.GetGinServer(dm)
		require.NoError(t, publicErr, "failed to create public server")
		if handler, ok := publicServer.Handler.(*gin.Engine); ok {
			env.PublicGin = handler
		}
		publicHttpSvc := cfg.MustGetService(sconfig.ServiceIdPublic).(sconfig.HttpService)
		publicAuthService := auth2.NewService(
			cfg,
			publicHttpSvc,
			dm.GetDatabase(),
			dm.GetRedisClient(),
			dm.GetEncryptService(),
			dm.GetLogger(),
		)
		env.PublicAuthUtil = auth2.NewAuthTestUtil(cfg, publicAuthService, sconfig.ServiceIdPublic)
	}

	if opts.StartHTTPServer {
		// Start a real HTTP server on a random port
		listener, listenErr := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, listenErr)
		port := listener.Addr().(*net.TCPAddr).Port
		env.ServerURL = fmt.Sprintf("http://127.0.0.1:%d", port)

		httpServer.Addr = listener.Addr().String()
		go func() {
			if serveErr := httpServer.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
				// Server stopped
			}
		}()

		// Generate a bearer token for admin access
		token, tokenErr := authUtil.GenerateBearerToken(
			context.Background(),
			"integration-test-admin",
			sconfig.RootNamespace,
			aschema.AllPermissions(),
		)
		require.NoError(t, tokenErr, "failed to generate admin bearer token")
		env.BearerToken = token

		env.Cleanup = func() {
			httpServer.Close()
			dm.GetEncryptService().Shutdown()
		}
	} else {
		// Extract the gin engine from the http.Server handler for in-process testing
		if handler, ok := httpServer.Handler.(*gin.Engine); ok {
			env.Gin = handler
		}
		env.Cleanup = func() {
			dm.GetEncryptService().Shutdown()
		}
	}

	return env
}

// NewNoAuthConnector creates a connector configuration for a NoAuth service.
func NewNoAuthConnector(connectorID apid.ID, displayName string, rateLimiting *connectors.RateLimiting) sconfig.Connector {
	return sconfig.Connector{
		Id:          connectorID,
		Version:     1,
		Labels:      map[string]string{"type": displayName},
		DisplayName: displayName,
		Auth: &connectors.Auth{
			InnerVal: &connectors.AuthNoAuth{
				Type: connectors.AuthTypeNoAuth,
			},
		},
		RateLimiting: rateLimiting,
	}
}

// OAuth2ConnectorOptions configures NewOAuth2Connector. Endpoints default to
// the test provider's URLs (provider.AuthorizationEndpoint(),
// provider.TokenEndpoint(), provider.RevocationEndpoint()) when zero.
type OAuth2ConnectorOptions struct {
	ClientID     string
	ClientSecret string
	Scopes       []string
	// IncludeRevocation, when true, fills in the standard /v1/oauth/revoke
	// endpoint so revoke flows are exercised.
	IncludeRevocation bool
	// AuthorizationEndpoint overrides provider.AuthorizationEndpoint().
	AuthorizationEndpoint string
	// TokenEndpoint overrides provider.TokenEndpoint().
	TokenEndpoint string
	// RevocationEndpoint overrides provider.RevocationEndpoint() (only used
	// when IncludeRevocation is true).
	RevocationEndpoint string
}

// NewOAuth2Connector builds an authproxy connector wired to the given
// OAuth2TestProvider. The endpoints default to the provider's standard
// /v1/oauth/* URLs.
func NewOAuth2Connector(connectorID apid.ID, displayName string, provider *OAuth2TestProvider, opts OAuth2ConnectorOptions) sconfig.Connector {
	authEndpoint := opts.AuthorizationEndpoint
	if authEndpoint == "" {
		authEndpoint = provider.AuthorizationEndpoint()
	}
	tokenEndpoint := opts.TokenEndpoint
	if tokenEndpoint == "" {
		tokenEndpoint = provider.TokenEndpoint()
	}

	scopes := make([]connectors.Scope, 0, len(opts.Scopes))
	for _, id := range opts.Scopes {
		scopes = append(scopes, connectors.Scope{Id: id, Reason: "integration test"})
	}

	auth := &connectors.AuthOAuth2{
		Type:         connectors.AuthTypeOAuth2,
		ClientId:     &common.StringValue{InnerVal: &common.StringValueDirect{Value: opts.ClientID}},
		ClientSecret: &common.StringValue{InnerVal: &common.StringValueDirect{Value: opts.ClientSecret}},
		Scopes:       scopes,
		Authorization: connectors.AuthOauth2Authorization{
			Endpoint: authEndpoint,
		},
		Token: connectors.AuthOauth2Token{
			Endpoint: tokenEndpoint,
		},
	}

	if opts.IncludeRevocation {
		revocationEndpoint := opts.RevocationEndpoint
		if revocationEndpoint == "" {
			revocationEndpoint = provider.RevocationEndpoint()
		}
		auth.Revocation = &connectors.AuthOauth2Revocation{
			Endpoint: revocationEndpoint,
		}
	}

	return sconfig.Connector{
		Id:          connectorID,
		Version:     1,
		Labels:      map[string]string{"type": displayName},
		DisplayName: displayName,
		Auth: &connectors.Auth{
			InnerVal: auth,
		},
	}
}

// DoProxyRequest performs a proxy request through the integration test environment using in-process gin.
func (env *IntegrationTestEnv) DoProxyRequest(t *testing.T, connectionID, targetURL, method string) *httptest.ResponseRecorder {
	t.Helper()
	require.NotNil(t, env.Gin, "DoProxyRequest requires in-process gin (StartHTTPServer must be false)")

	proxyReq := coreIface.ProxyRequest{
		URL:    targetURL,
		Method: method,
	}

	body, err := jsonMarshal(proxyReq)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, err := env.AuthUtil.NewSignedRequestForActorExternalId(
		http.MethodPost,
		"/api/v1/connections/"+connectionID+"/_proxy",
		body,
		sconfig.RootNamespace,
		"test-actor",
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	env.Gin.ServeHTTP(w, req)
	return w
}

// CreateConnection creates a connection directly in the database.
func (env *IntegrationTestEnv) CreateConnection(t *testing.T, connectorID apid.ID, connectorVersion uint64) string {
	t.Helper()

	id := apid.New(apid.PrefixConnection)

	err := env.Db.CreateConnection(context.Background(), &database.Connection{
		Id:               id,
		Namespace:        sconfig.RootNamespace,
		ConnectorId:      connectorID,
		ConnectorVersion: connectorVersion,
		State:            database.ConnectionStateCreated,
	})
	require.NoError(t, err)

	return id.String()
}

// PublicCallbackURL returns the URL the proxy emits as the OAuth `redirect_uri`.
// The OAuth provider must be configured with this exact URL so authorize matches
// (helper exists because public.GetBaseUrl() depends on resolved config — port 0
// in integration.yaml means the URL is "http://localhost:0/oauth2/callback").
func (env *IntegrationTestEnv) PublicCallbackURL() string {
	return env.Cfg.GetRoot().Public.GetBaseUrl() + "/oauth2/callback"
}

// InitiateOAuth2Connection POSTs to /api/v1/connections/_initiate and returns
// the new connection's ID and the redirect URL the user would be sent to. The
// redirect URL points at the public service's /oauth2/redirect endpoint.
func (env *IntegrationTestEnv) InitiateOAuth2Connection(t *testing.T, connectorID apid.ID, returnToUrl string) (connectionID, redirectURL string) {
	t.Helper()
	require.NotNil(t, env.Gin, "InitiateOAuth2Connection requires in-process gin (StartHTTPServer must be false)")

	body, err := jsonMarshal(coreIface.InitiateConnectionRequest{
		ConnectorId: connectorID,
		ReturnToUrl: returnToUrl,
	})
	require.NoError(t, err)

	req, err := env.AuthUtil.NewSignedRequestForActorExternalId(
		http.MethodPost,
		"/api/v1/connections/_initiate",
		body,
		sconfig.RootNamespace,
		"test-actor",
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	env.Gin.ServeHTTP(w, req)
	require.Equalf(t, http.StatusOK, w.Code, "initiate failed: %s", w.Body.String())

	var resp coreIface.ConnectionSetupRedirect
	require.NoError(t, jsonUnmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, coreIface.ConnectionSetupResponseTypeRedirect, resp.Type, "expected OAuth2 connector to return redirect")
	require.NotEmpty(t, resp.RedirectUrl)

	return resp.Id.String(), resp.RedirectUrl
}

// FollowOAuth2Redirect issues an in-process GET to the public service's
// `/oauth2/redirect` endpoint with the same state_id and signed JWT the user's
// browser would carry, and returns the Location header — the URL of the OAuth
// provider's authorize endpoint. The proxy generates the upstream URL from the
// connector config, so callers can assert on its query parameters.
func (env *IntegrationTestEnv) FollowOAuth2Redirect(t *testing.T, redirectURL string) string {
	t.Helper()
	require.NotNil(t, env.PublicGin, "FollowOAuth2Redirect requires SetupOptions.IncludePublic=true")

	parsed, err := url.Parse(redirectURL)
	require.NoError(t, err)

	req, err := env.PublicAuthUtil.NewSignedRequestForActorExternalId(
		http.MethodGet,
		parsed.RequestURI(),
		nil,
		sconfig.RootNamespace,
		"test-actor",
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	env.PublicGin.ServeHTTP(w, req)
	require.Equalf(t, http.StatusFound, w.Code, "/oauth2/redirect should 302; got %d body=%s", w.Code, w.Body.String())

	loc := w.Header().Get("Location")
	require.NotEmpty(t, loc, "/oauth2/redirect should set Location")
	return loc
}

// DeliverOAuth2Callback issues an in-process GET to the public service's
// `/oauth2/callback` endpoint with the code+state the OAuth provider would
// have redirected the user to. Returns the final Location header — typically
// the test's return_to_url, possibly augmented with setup=pending.
func (env *IntegrationTestEnv) DeliverOAuth2Callback(t *testing.T, callbackURL string) string {
	t.Helper()
	require.NotNil(t, env.PublicGin, "DeliverOAuth2Callback requires SetupOptions.IncludePublic=true")

	parsed, err := url.Parse(callbackURL)
	require.NoError(t, err)

	req, err := env.PublicAuthUtil.NewSignedRequestForActorExternalId(
		http.MethodGet,
		parsed.RequestURI(),
		nil,
		sconfig.RootNamespace,
		"test-actor",
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	env.PublicGin.ServeHTTP(w, req)
	require.Equalf(t, http.StatusFound, w.Code, "/oauth2/callback should 302; got %d body=%s", w.Code, w.Body.String())

	loc := w.Header().Get("Location")
	require.NotEmpty(t, loc, "/oauth2/callback should set Location")
	return loc
}

// GetOAuth2Token reads the most recent OAuth2 token row stored for the
// connection. Returns nil when no token exists yet.
func (env *IntegrationTestEnv) GetOAuth2Token(t *testing.T, connectionID string) *database.OAuth2Token {
	t.Helper()
	id, err := apid.Parse(connectionID)
	require.NoError(t, err)
	tok, err := env.Db.GetOAuth2Token(context.Background(), id)
	require.NoError(t, err)
	return tok
}

// GetConnection reads the connection row from the DB.
func (env *IntegrationTestEnv) GetConnection(t *testing.T, connectionID string) *database.Connection {
	t.Helper()
	id, err := apid.Parse(connectionID)
	require.NoError(t, err)
	conn, err := env.Db.GetConnection(context.Background(), id)
	require.NoError(t, err)
	return conn
}
