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
	// ApiGin is the HTTP router for in-process request testing against the
	// service named by SetupOptions.Service (default: API). Populated when
	// SetupOptions.StartHTTPServer is false.
	ApiGin *gin.Engine

	// PublicGin is the public service router. Only populated when SetupOptions.IncludePublic
	// is true. Tests that drive an OAuth flow need this to deliver `/oauth2/redirect` and
	// `/oauth2/callback` requests, which only the public service hosts.
	PublicGin *gin.Engine

	// Cfg is the loaded configuration.
	Cfg config.C

	// ApiAuthUtil provides JWT signing helpers for the primary service
	// (audience matches SetupOptions.Service).
	ApiAuthUtil *auth2.AuthTestUtil

	// PublicAuthUtil signs requests for the public service (audience=public). Only
	// populated when SetupOptions.IncludePublic is true.
	PublicAuthUtil *auth2.AuthTestUtil

	// Db is the database connection.
	Db database.DB

	// Core is the core business logic service.
	Core coreIface.C

	// Logger is the test logger.
	Logger *slog.Logger

	// ServerURL is the base URL of the real HTTP server hosting the primary
	// service (api or admin-api per SetupOptions.Service). Only set when
	// StartHTTPServer is true.
	ServerURL string

	// PublicURL is the base URL of the real HTTP server hosting the public
	// service. Only set when StartHTTPServer && IncludePublic.
	PublicURL string

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
	// When false, use ApiGin with httptest.ResponseRecorder for in-process testing.
	StartHTTPServer bool

	// IncludePublic also wires up the public service alongside the primary
	// service so tests can drive `/oauth2/redirect`, `/oauth2/callback`, and
	// (when ServeMarketplaceUI is true) the marketplace SPA. The public service
	// shares the same DependencyManager (DB, Redis, core, encryption) as the
	// primary service. With StartHTTPServer=true, public is started on its
	// own listener and exposed via env.PublicURL; otherwise it's served
	// in-process via env.PublicGin.
	IncludePublic bool

	// ServeMarketplaceUI configures the public service to serve the built
	// marketplace SPA at "/" (in addition to its API routes). Implies
	// IncludePublic. The vite build runs once per `go test` process if
	// `ui/marketplace/dist/index.html` is missing.
	ServeMarketplaceUI bool
}

// repoRoot returns the absolute path to the repository root.
func repoRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	// helpers/setup.go -> integration_tests/ -> repo root
	return filepath.Dir(filepath.Dir(filepath.Dir(filename)))
}

// mustListen reserves a random localhost port and hands back the listener.
// The caller is responsible for handing it to http.Server.Serve and for
// closing it via env.Cleanup.
func mustListen(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return l
}

// serviceIdFor maps the SetupOptions.Service knob to its config ServiceId.
func serviceIdFor(s ServiceType) sconfig.ServiceId {
	switch s {
	case ServiceTypeAdminAPI:
		return sconfig.ServiceIdAdminApi
	default:
		return sconfig.ServiceIdApi
	}
}

// applyListenerToService writes the listener's port back into the service's
// config so GetBaseUrl() returns a navigable URL. Without this the OAuth
// `redirect_uri` the proxy emits — which is built from Public.GetBaseUrl() —
// would be `http://localhost:0/...` because integration.yaml uses port 0.
func applyListenerToService(cfg config.C, id sconfig.ServiceId, l net.Listener) {
	port := l.Addr().(*net.TCPAddr).Port
	root := cfg.GetRoot()
	switch id {
	case sconfig.ServiceIdApi:
		root.Api.PortVal = common.NewIntegerValueDirectInline(int64(port))
	case sconfig.ServiceIdAdminApi:
		root.AdminApi.PortVal = common.NewIntegerValueDirectInline(int64(port))
	case sconfig.ServiceIdPublic:
		root.Public.PortVal = common.NewIntegerValueDirectInline(int64(port))
	default:
		panic(fmt.Sprintf("applyListenerToService: unsupported service id %s", id))
	}
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

	// ServeMarketplaceUI requires both the public service running and a real
	// HTTP listener (chromedp needs a navigable URL).
	if opts.ServeMarketplaceUI {
		require.True(t, opts.StartHTTPServer, "ServeMarketplaceUI requires StartHTTPServer=true")
		opts.IncludePublic = true
	}

	// When starting real HTTP servers, pre-allocate listeners and write the
	// resulting ports back into the config so each service's GetBaseUrl()
	// returns a navigable URL — the proxy uses Public.GetBaseUrl() to build
	// the OAuth `redirect_uri` it sends to the provider, and the marketplace
	// SPA's same-origin assumption depends on it.
	var primaryListener, publicListener net.Listener
	if opts.StartHTTPServer {
		primaryListener = mustListen(t)
		applyListenerToService(cfg, serviceIdFor(opts.Service), primaryListener)

		if opts.IncludePublic {
			publicListener = mustListen(t)
			applyListenerToService(cfg, sconfig.ServiceIdPublic, publicListener)
		}
	}

	if opts.ServeMarketplaceUI {
		EnsureMarketplaceBuilt(t)
		cfg.GetRoot().Public.StaticVal = &sconfig.ServicePublicStaticContentConfig{
			MountAtPath:   "/",
			ServeFromPath: MarketplaceDistPath(),
		}
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
		Cfg:         cfg,
		ApiAuthUtil: authUtil,
		Db:          dm.GetDatabase(),
		Core:        dm.GetCoreService(),
		Logger:      dm.GetLogger(),
		DM:          dm,
		Cleanup:     func() {},
	}

	var publicServer *http.Server
	if opts.IncludePublic {
		var publicErr error
		publicServer, _, publicErr = public_service.GetGinServer(dm)
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
		port := primaryListener.Addr().(*net.TCPAddr).Port
		env.ServerURL = fmt.Sprintf("http://127.0.0.1:%d", port)

		httpServer.Addr = primaryListener.Addr().String()
		go func() {
			if serveErr := httpServer.Serve(primaryListener); serveErr != nil && serveErr != http.ErrServerClosed {
				// Server stopped
			}
		}()

		if opts.IncludePublic {
			// Use localhost (not 127.0.0.1) so env.PublicURL matches what
			// Public.GetBaseUrl() returns — the proxy's OAuth `redirect_uri`
			// is built from GetBaseUrl(), and the SPA's same-origin
			// return_to_url depends on the browser starting from the same
			// host name we register at the provider.
			publicPort := publicListener.Addr().(*net.TCPAddr).Port
			env.PublicURL = fmt.Sprintf("http://localhost:%d", publicPort)
			publicServer.Addr = publicListener.Addr().String()
			go func() {
				if serveErr := publicServer.Serve(publicListener); serveErr != nil && serveErr != http.ErrServerClosed {
					// Server stopped
				}
			}()
		}

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
			if publicServer != nil {
				publicServer.Close()
			}
			dm.GetEncryptService().Shutdown()
		}
	} else {
		// Extract the gin engine from the http.Server handler for in-process testing
		if handler, ok := httpServer.Handler.(*gin.Engine); ok {
			env.ApiGin = handler
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

// DoProxyRequest performs a proxy request through the integration test environment.
// Routes through the in-process gin engine when StartHTTPServer=false, or hits the
// real HTTP server at env.ServerURL when StartHTTPServer=true. Returns a recorder
// either way so callers can read status/body uniformly.
func (env *IntegrationTestEnv) DoProxyRequest(t *testing.T, connectionID, targetURL, method string) *httptest.ResponseRecorder {
	t.Helper()
	require.Truef(t, env.ApiGin != nil || env.ServerURL != "", "DoProxyRequest requires either in-process gin or a running HTTP server")

	proxyReq := coreIface.ProxyRequest{
		URL:    targetURL,
		Method: method,
	}

	body, err := jsonMarshal(proxyReq)
	require.NoError(t, err)

	path := "/api/v1/connections/" + connectionID + "/_proxy"
	req, err := env.ApiAuthUtil.NewSignedRequestForActorExternalId(
		http.MethodPost,
		path,
		body,
		sconfig.RootNamespace,
		"test-actor",
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	if env.ApiGin != nil {
		env.ApiGin.ServeHTTP(w, req)
		return w
	}

	// HTTP mode: rewrite the path-only URL onto env.ServerURL and send.
	abs, err := url.Parse(env.ServerURL + path)
	require.NoError(t, err)
	req.URL = abs
	req.Host = abs.Host
	req.RequestURI = ""
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	w.Code = resp.StatusCode
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	if _, err := w.Body.ReadFrom(resp.Body); err != nil {
		require.NoError(t, err)
	}
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

// PublicOAuthCallbackURL returns the URL the proxy emits as the OAuth `redirect_uri`.
// The OAuth provider must be configured with this exact URL so authorize matches
// (helper exists because public.GetBaseUrl() depends on resolved config — port 0
// in integration.yaml means the URL is "http://localhost:0/oauth2/callback").
func (env *IntegrationTestEnv) PublicOAuthCallbackURL() string {
	return env.Cfg.GetRoot().Public.GetBaseUrl() + "/oauth2/callback"
}

// InitiateOAuth2Connection POSTs to /api/v1/connections/_initiate and returns
// the new connection's ID and the redirect URL the user would be sent to. The
// redirect URL points at the public service's /oauth2/redirect endpoint.
func (env *IntegrationTestEnv) InitiateOAuth2Connection(t *testing.T, connectorID apid.ID, returnToUrl string) (connectionID, redirectURL string) {
	t.Helper()
	require.NotNil(t, env.ApiGin, "InitiateOAuth2Connection requires in-process gin (StartHTTPServer must be false)")

	body, err := jsonMarshal(coreIface.InitiateConnectionRequest{
		ConnectorId: connectorID,
		ReturnToUrl: returnToUrl,
	})
	require.NoError(t, err)

	req, err := env.ApiAuthUtil.NewSignedRequestForActorExternalId(
		http.MethodPost,
		"/api/v1/connections/_initiate",
		body,
		sconfig.RootNamespace,
		"test-actor",
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	env.ApiGin.ServeHTTP(w, req)
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
