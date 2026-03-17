package helpers

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/apid"
	asynqmock "github.com/rmorlok/authproxy/internal/apasynq/mock"
	auth2 "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/alicebob/miniredis/v2"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	httpf2 "github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/ratelimit"
	common_routes "github.com/rmorlok/authproxy/internal/routes"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/require"
)

// IntegrationTestEnv holds all the components needed for an integration test.
type IntegrationTestEnv struct {
	Gin            *gin.Engine
	Cfg            config.C
	AuthUtil       *auth2.AuthTestUtil
	Db             database.DB
	Core           coreIface.C
	Redis          apredis.Client
	RedisServer    *miniredis.Miniredis
	Logger         *slog.Logger
	Cleanup        func()
}

// SetupOptions configures how the integration test environment is created.
type SetupOptions struct {
	Connectors []sconfig.Connector
}

// Setup creates a full integration test environment with real database, Redis, and HTTP routing.
func Setup(t *testing.T, opts SetupOptions) *IntegrationTestEnv {
	t.Helper()

	cfg := config.FromRoot(&sconfig.Root{
		Connectors: &sconfig.Connectors{
			LoadFromList: opts.Connectors,
		},
	})

	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
	cfg, rds, rdsServer := apredis.MustApplyTestConfigWithServer(cfg)
	cfg, auth, authUtil := auth2.TestAuthServiceWithDb(sconfig.ServiceIdApi, cfg, db)
	logger := aplog.NewNoopLogger()

	// Create rate limit factory with real Redis for integration testing
	rlStore := ratelimit.NewStore(rds)
	rlFactory := ratelimit.NewFactory(rlStore, logger)

	h := httpf2.CreateFactory(cfg, rds, NewNoopRoundTripperFactory(), logger, rlFactory)
	cfg, e := encrypt.NewTestEncryptService(cfg, db)

	ctrl := gomock.NewController(t)
	ac := asynqmock.NewMockClient(ctrl)

	// Allow any async enqueue calls without failing
	ac.EXPECT().Enqueue(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	c := core.NewCoreService(cfg, db, e, rds, h, ac, logger)
	require.NoError(t, c.Migrate(context.Background()))

	// Set up gin with the routes we need
	r := api_common.GinForTest(nil)

	routesConnections := common_routes.NewConnectionsRoutes(cfg, auth, db, rds, c, h, e, logger)
	routesProxy := common_routes.NewConnectionsProxyRoutes(cfg, auth, db, rds, c, h, e, logger)

	api := r.Group("/api/v1")
	routesConnections.Register(api)
	routesProxy.Register(api)

	return &IntegrationTestEnv{
		Gin:         r,
		Cfg:         cfg,
		AuthUtil:    authUtil,
		Db:          db,
		Core:        c,
		Redis:       rds,
		RedisServer: rdsServer,
		Logger:      logger,
		Cleanup: func() {
			ctrl.Finish()
		},
	}
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
func (env *IntegrationTestEnv) DoProxyRequest(t *testing.T, connectionID, targetURL, method string) *httptest.ResponseRecorder {
	t.Helper()

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
