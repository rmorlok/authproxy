package helpers

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

	// LogCapture, when non-nil, replaces the configured logging block with
	// a buffered JSON sink so the test can assert on emitted slog records.
	// The swap happens before the dependency manager is built so every
	// derived logger routes here. nil leaves the configured logger
	// untouched.
	LogCapture *LogCapture
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

	// Hook the log capture before the dependency manager builds its cached
	// logger — once dm.GetLogger() runs, the logger is fixed and any later
	// swap would only affect new builders.
	if opts.LogCapture != nil {
		cfgRoot := cfg.GetRoot()
		if cfgRoot.Logging == nil {
			cfgRoot.Logging = &sconfig.LoggingConfig{}
		}
		cfgRoot.Logging.InnerVal = opts.LogCapture.asLoggingImpl()
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

// DoProxyRawRequest performs a streaming /_proxy_raw request through the
// integration test environment. The upstream URL is carried in the
// X-AuthProxy-Upstream-URL header per the raw-proxy contract; body and
// extraHeaders are forwarded as-is to the upstream. forceChunked, when
// true, sends the body with Content-Length omitted so net/http negotiates
// chunked transfer-encoding — used to exercise the streaming/skipped
// path in the request log.
func (env *IntegrationTestEnv) DoProxyRawRequest(
	t *testing.T,
	connectionID, targetURL, method string,
	body []byte,
	forceChunked bool,
	extraHeaders http.Header,
) *httptest.ResponseRecorder {
	t.Helper()
	require.Truef(t, env.ApiGin != nil || env.ServerURL != "", "DoProxyRawRequest requires either in-process gin or a running HTTP server")

	path := "/api/v1/connections/" + connectionID + "/_proxy_raw"

	var bodyReader io.Reader
	if body != nil {
		if forceChunked {
			// Wrapping bytes.NewReader in struct{ io.Reader } strips the
			// Len() method so http.NewRequest can't infer ContentLength,
			// forcing chunked transfer-encoding on the wire.
			bodyReader = struct{ io.Reader }{bytes.NewReader(body)}
		} else {
			bodyReader = bytes.NewReader(body)
		}
	}

	req, err := env.ApiAuthUtil.NewSignedRequestForActorExternalId(
		method,
		path,
		bodyReader,
		sconfig.RootNamespace,
		"test-actor",
		aschema.AllPermissions(),
	)
	require.NoError(t, err)
	if forceChunked {
		// net/http's server side reports ContentLength=-1 for chunked
		// transfer-encoding. For in-process tests the request struct
		// is shared rather than re-parsed off the wire, so set the
		// signal explicitly — the route handler propagates it to the
		// outbound request and the roundtripper keys its skip
		// decision off ContentLength<0.
		req.ContentLength = -1
		req.TransferEncoding = []string{"chunked"}
	}
	req.Header.Set("X-AuthProxy-Upstream-URL", targetURL)
	for k, vv := range extraHeaders {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	w := httptest.NewRecorder()
	if env.ApiGin != nil {
		env.ApiGin.ServeHTTP(w, req)
		return w
	}

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

// GetConnection reads the connection row from the DB.
func (env *IntegrationTestEnv) GetConnection(t *testing.T, connectionID string) *database.Connection {
	t.Helper()
	id, err := apid.Parse(connectionID)
	require.NoError(t, err)
	conn, err := env.Db.GetConnection(context.Background(), id)
	require.NoError(t, err)
	return conn
}
