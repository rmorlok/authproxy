package routes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	authservice "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/require"
)

type resourceSearchTestDB struct {
	database.DB
	search func(context.Context, database.SearchResourcesParams) (database.SearchResourcesResult, error)
}

func (d resourceSearchTestDB) SearchResources(ctx context.Context, params database.SearchResourcesParams) (database.SearchResourcesResult, error) {
	if d.search != nil {
		return d.search(ctx, params)
	}
	return d.DB.SearchResources(ctx, params)
}

type resourceSearchRouteSetup struct {
	gin      *gin.Engine
	db       database.DB
	authUtil *authservice.AuthTestUtil
	routes   *ResourceSearchRoutes
}

func setupResourceSearchRoute(t *testing.T, decorate func(database.DB) database.DB) resourceSearchRouteSetup {
	t.Helper()
	cfg, realDB := database.MustApplyBlankTestDbConfig(t, config.FromRoot(&sconfig.Root{}))
	db := realDB
	if decorate != nil {
		db = decorate(realDB)
	}
	_, auth, authUtil := authservice.TestAuthServiceWithDb(sconfig.ServiceIdAdminApi, cfg, db)
	router := apgin.ForTest(nil)
	searchRoutes := NewResourceSearchRoutes(auth, db, test_utils.NewTestLogger())
	searchRoutes.Register(router)
	return resourceSearchRouteSetup{gin: router, db: db, authUtil: authUtil, routes: searchRoutes}
}

func createSearchActor(t *testing.T, db database.DB, namespace string, labels database.Labels) *database.Actor {
	t.Helper()
	require.NoError(t, db.EnsureNamespaceByPath(t.Context(), namespace))
	actor := &database.Actor{
		Id:         apid.New(apid.PrefixActor),
		Namespace:  namespace,
		ExternalId: "search-" + apid.New(apid.PrefixActor).String(),
		Labels:     labels,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	require.NoError(t, db.CreateActor(t.Context(), actor))
	return actor
}

func signedSearchRequest(t *testing.T, setup resourceSearchRouteSetup, rawURL string, permissions []aschema.Permission) *http.Request {
	t.Helper()
	req, err := setup.authUtil.NewSignedRequestForActorExternalId(
		http.MethodGet,
		rawURL,
		nil,
		"root",
		"search-caller",
		permissions,
	)
	require.NoError(t, err)
	return req
}

func TestResourceSearchRouteQueryAndPermissions(t *testing.T) {
	setup := setupResourceSearchRoute(t, nil)
	allowed := createSearchActor(t, setup.db, "root.team", database.Labels{"name": "payments-service", "env": "prod"})
	_ = createSearchActor(t, setup.db, "root.other", database.Labels{"name": "payments-service", "env": "prod"})

	t.Run("requires authentication", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/search/resources?q=payments&resource_type=actor", nil)
		require.NoError(t, err)
		setup.gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("intersects namespace and resource id permissions", func(t *testing.T) {
		w := httptest.NewRecorder()
		permissions := []aschema.Permission{{
			Namespace:   "root.**",
			Resources:   []string{"actors"},
			ResourceIds: []string{allowed.Id.String()},
			Verbs:       []string{"list", "get"},
		}}
		req := signedSearchRequest(t, setup, "/search/resources?q=payments&resource_type=actor&namespace=root.team.**", permissions)
		setup.gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var response schemaapi.SearchResourcesResponseJson
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Len(t, response.Items, 1)
		require.Equal(t, allowed.Id.String(), response.Items[0].ResourceId)
		require.Equal(t, map[string]string{"env": "prod", "name": "payments-service"}, response.Items[0].Labels)
	})
}

func TestResourceSearchRouteValidation(t *testing.T) {
	setup := setupResourceSearchRoute(t, nil)
	permissions := aschema.AllPermissions()
	tests := []string{
		"/search/resources",
		"/search/resources?q=ab",
		"/search/resources?mode=invalid&q=valid",
		"/search/resources?mode=seed&q=invalid",
		"/search/resources?q=valid&resource_type=unknown",
		"/search/resources?label_selector=" + url.QueryEscape("bad key=value"),
		"/search/resources?label_selector=" + url.QueryEscape(","),
		"/search/resources?label_selector=" + url.QueryEscape("env=prod,"),
		"/search/resources?q=valid&limit=51",
	}
	for _, rawURL := range tests {
		t.Run(rawURL, func(t *testing.T) {
			w := httptest.NewRecorder()
			setup.gin.ServeHTTP(w, signedSearchRequest(t, setup, rawURL, permissions))
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}
}

func TestResourceSearchRouteSeedCoversRemainingTypes(t *testing.T) {
	var mu sync.Mutex
	seen := make(map[database.SearchResourceType]database.SearchResourcesParams)
	setup := setupResourceSearchRoute(t, func(db database.DB) database.DB {
		return resourceSearchTestDB{
			DB: db,
			search: func(_ context.Context, params database.SearchResourcesParams) (database.SearchResourcesResult, error) {
				mu.Lock()
				seen[params.ResourceType] = params
				mu.Unlock()
				resourceID := map[database.SearchResourceType]string{
					database.SearchResourceTypeNamespace: "root.seed",
					database.SearchResourceTypeKey:       "key_seed0000000000001",
					database.SearchResourceTypeRateLimit: "rl_seed00000000000001",
				}[params.ResourceType]
				return database.SearchResourcesResult{Items: []database.SearchResource{{
					ResourceType: params.ResourceType,
					ResourceID:   resourceID,
					Namespace:    "root.seed",
					Labels:       database.Labels{"name": "seed"},
					UpdatedAt:    time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC),
				}}}, nil
			},
		}
	})

	w := httptest.NewRecorder()
	setup.gin.ServeHTTP(w, signedSearchRequest(
		t,
		setup,
		"/search/resources?mode=seed&resource_type=namespace&resource_type=key&resource_type=rate_limit&limit=50",
		aschema.AllPermissions(),
	))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var response schemaapi.SearchResourcesResponseJson
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Items, 3)
	require.Empty(t, response.TruncatedTypes)
	require.Empty(t, response.IncompleteTypes)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, seen, 3)
	for _, resourceType := range []database.SearchResourceType{
		database.SearchResourceTypeNamespace,
		database.SearchResourceTypeKey,
		database.SearchResourceTypeRateLimit,
	} {
		params, ok := seen[resourceType]
		require.True(t, ok)
		require.Empty(t, params.Query)
		require.Empty(t, params.LabelSelector)
		require.Equal(t, 50, params.Limit)
	}
}

func TestResourceSearchRouteReturnsIncompleteTypes(t *testing.T) {
	setup := setupResourceSearchRoute(t, func(db database.DB) database.DB {
		return resourceSearchTestDB{
			DB: db,
			search: func(context.Context, database.SearchResourcesParams) (database.SearchResourcesResult, error) {
				return database.SearchResourcesResult{}, context.DeadlineExceeded
			},
		}
	})
	w := httptest.NewRecorder()
	setup.gin.ServeHTTP(w, signedSearchRequest(t, setup, "/search/resources?q=payments&resource_type=actor", aschema.AllPermissions()))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response schemaapi.SearchResourcesResponseJson
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, []schemaapi.SearchResourceType{schemaapi.SearchResourceTypeActor}, response.IncompleteTypes)
	require.Empty(t, response.Items)
}

func TestResourceSearchRouteOverallDeadlineDoesNotWaitForIgnoredCancellation(t *testing.T) {
	release := make(chan struct{})
	setup := setupResourceSearchRoute(t, func(db database.DB) database.DB {
		return resourceSearchTestDB{
			DB: db,
			search: func(context.Context, database.SearchResourcesParams) (database.SearchResourcesResult, error) {
				select {
				case <-release:
				case <-time.After(250 * time.Millisecond):
				}
				return database.SearchResourcesResult{}, nil
			},
		}
	})
	setup.routes.typeTimeout = 5 * time.Millisecond
	setup.routes.overallTimeout = 20 * time.Millisecond

	w := httptest.NewRecorder()
	setup.gin.ServeHTTP(w, signedSearchRequest(t, setup, "/search/resources?q=payments&resource_type=actor", aschema.AllPermissions()))
	close(release)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response schemaapi.SearchResourcesResponseJson
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, []schemaapi.SearchResourceType{schemaapi.SearchResourceTypeActor}, response.IncompleteTypes)
}

func TestResourceSearchRouteUnexpectedDatabaseFailure(t *testing.T) {
	setup := setupResourceSearchRoute(t, func(db database.DB) database.DB {
		return resourceSearchTestDB{
			DB: db,
			search: func(context.Context, database.SearchResourcesParams) (database.SearchResourcesResult, error) {
				return database.SearchResourcesResult{}, errors.New("search failed")
			},
		}
	})
	w := httptest.NewRecorder()
	setup.gin.ServeHTTP(w, signedSearchRequest(t, setup, "/search/resources?q=payments&resource_type=actor", aschema.AllPermissions()))
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}
