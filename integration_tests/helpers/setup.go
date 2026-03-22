package helpers

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
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
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/rmorlok/authproxy/internal/service"
	"github.com/rmorlok/authproxy/internal/service/admin_api"
	api_service "github.com/rmorlok/authproxy/internal/service/api"
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

	// Cfg is the loaded configuration.
	Cfg config.C

	// AuthUtil provides JWT signing helpers for test requests.
	AuthUtil *auth2.AuthTestUtil

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
	dm.AutoMigrateAll()

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
		}
	} else {
		// Extract the gin engine from the http.Server handler for in-process testing
		if handler, ok := httpServer.Handler.(*gin.Engine); ok {
			env.Gin = handler
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
