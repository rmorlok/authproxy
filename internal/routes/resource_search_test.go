package routes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/rmorlok/authproxy/internal/encfield"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
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

func createSearchActor(t *testing.T, db database.DB, namespace, name string, labels database.Labels) *database.Actor {
	t.Helper()
	require.NoError(t, db.EnsureNamespaceByPath(t.Context(), namespace))
	actor := &database.Actor{
		Id:         apid.New(apid.PrefixActor),
		Name:       scommon.ResourceName(name),
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
	allowed := createSearchActor(t, setup.db, "root.team", "payments-service", database.Labels{"env": "prod"})
	_ = createSearchActor(t, setup.db, "root.other", "payments-service", database.Labels{"env": "prod"})

	t.Run("requires authentication", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/search/resources?q=payments&resourceType=actor", nil)
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
		req := signedSearchRequest(t, setup, "/search/resources?q=payments&resourceType=actor&namespace=root.team.**", permissions)
		setup.gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var response schemaapi.SearchResourcesResponseJson
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Len(t, response.Items, 1)
		require.Equal(t, allowed.Id.String(), response.Items[0].ResourceId)
		require.Equal(t, "payments-service", response.Items[0].Name)
		require.Equal(t, map[string]string{"env": "prod"}, response.Items[0].Labels)
	})
}

func TestResourceNamesEndToEndAcrossVersionsAndAuthorization(t *testing.T) {
	setup := setupResourceSearchRoute(t, nil)
	ctx := t.Context()
	for _, ns := range []string{"root.allowed", "root.hidden"} {
		require.NoError(t, setup.db.EnsureNamespaceByPath(ctx, ns))
	}

	createConnectorVersion := func(id apid.ID, ns, name string, version uint64, state database.ConnectorDefinitionVersionState) {
		t.Helper()
		require.NoError(t, setup.db.UpsertConnectorDefinitionVersion(ctx, &database.ConnectorWithDefinition{
			Id:        id,
			Name:      scommon.ResourceName(name),
			Namespace: ns,
			Version:   version,
			State:     state,
			EncryptedDefinition: encfield.EncryptedField{
				ID:   apid.New(apid.PrefixDataEncryptionKey),
				Data: fmt.Sprintf("definition-%d", version),
			},
		}))
	}

	allowedConnectorID := apid.New(apid.PrefixConnector)
	createConnectorVersion(allowedConnectorID, "root.allowed", "payments-provider", 1, database.ConnectorDefinitionVersionStatePrimary)
	createConnectorVersion(allowedConnectorID, "root.allowed", "", 2, database.ConnectorDefinitionVersionStateDraft)
	hiddenConnectorID := apid.New(apid.PrefixConnector)
	createConnectorVersion(hiddenConnectorID, "root.hidden", "billing-provider", 1, database.ConnectorDefinitionVersionStatePrimary)

	connectionID := apid.New(apid.PrefixConnection)
	require.NoError(t, setup.db.CreateConnection(ctx, &database.Connection{
		Id:               connectionID,
		Name:             "payments-live",
		Namespace:        "root.allowed",
		ConnectorId:      allowedConnectorID,
		ConnectorVersion: 1,
		State:            database.ConnectionStateConfigured,
	}))

	// Rename by immutable IDs, then drive the same reconciliation that the
	// targeted background task performs for connector descendants.
	require.NoError(t, setup.db.UpdateConnectorName(ctx, allowedConnectorID, "billing-provider"))
	_, err := setup.db.UpdateConnectionName(ctx, connectionID, "billing-live")
	require.NoError(t, err)
	require.NoError(t, setup.db.RefreshConnectionsForConnector(ctx, allowedConnectorID))

	versions := setup.db.ListConnectorDefinitionVersionsBuilder().
		ForName("billing-provider").
		ForNamespaceMatchers([]string{"root.allowed"}).
		FetchPage(ctx)
	require.NoError(t, versions.Error)
	require.Len(t, versions.Results, 2)
	for _, version := range versions.Results {
		require.Equal(t, allowedConnectorID, version.Id)
		require.Equal(t, scommon.ResourceName("billing-provider"), version.Name)
		require.Equal(t, "billing-provider", version.Labels["apxy/cxr/-/name"])
	}

	connections := setup.db.ListConnectionsBuilder().
		ForName("billing-live").
		ForNamespaceMatchers([]string{"root.allowed"}).
		FetchPage(ctx)
	require.NoError(t, connections.Error)
	require.Len(t, connections.Results, 1)
	require.Equal(t, connectionID, connections.Results[0].Id)
	require.Equal(t, "billing-live", connections.Results[0].Labels["apxy/cxn/-/name"])
	require.Equal(t, "billing-provider", connections.Results[0].Labels["apxy/cxr/-/name"])

	permissions := []aschema.Permission{{
		Namespace: "root.allowed",
		Resources: []string{"connectors", "connections"},
		Verbs:     []string{"list", "get"},
	}}
	w := httptest.NewRecorder()
	setup.gin.ServeHTTP(w, signedSearchRequest(
		t,
		setup,
		"/search/resources?q=billing-provider&resourceType=connector&namespace=root.**",
		permissions,
	))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response schemaapi.SearchResourcesResponseJson
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Items, 1, "the hidden duplicate and extra connector version must not leak")
	require.Equal(t, allowedConnectorID.String(), response.Items[0].ResourceId)
	require.Equal(t, "billing-provider", response.Items[0].Name)
	require.Equal(t, "root.allowed", response.Items[0].Namespace)

	w = httptest.NewRecorder()
	setup.gin.ServeHTTP(w, signedSearchRequest(
		t,
		setup,
		"/search/resources?q=billing-live&resourceType=connection&namespace=root.**",
		permissions,
	))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Items, 1)
	require.Equal(t, connectionID.String(), response.Items[0].ResourceId)
	require.Equal(t, "billing-live", response.Items[0].Name)
}

func TestResourceSearchRouteValidation(t *testing.T) {
	setup := setupResourceSearchRoute(t, nil)
	permissions := aschema.AllPermissions()
	tests := []string{
		"/search/resources",
		"/search/resources?q=ab",
		"/search/resources?mode=invalid&q=valid",
		"/search/resources?mode=seed&q=invalid",
		"/search/resources?q=valid&resourceType=unknown",
		"/search/resources?labelSelector=" + url.QueryEscape("bad key=value"),
		"/search/resources?labelSelector=" + url.QueryEscape(","),
		"/search/resources?labelSelector=" + url.QueryEscape("env=prod,"),
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
					Name:         "seed",
					Namespace:    "root.seed",
					Labels:       database.Labels{},
					UpdatedAt:    time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC),
				}}}, nil
			},
		}
	})

	w := httptest.NewRecorder()
	setup.gin.ServeHTTP(w, signedSearchRequest(
		t,
		setup,
		"/search/resources?mode=seed&resourceType=namespace&resourceType=key&resourceType=rate_limit&limit=50",
		aschema.AllPermissions(),
	))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var response schemaapi.SearchResourcesResponseJson
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Items, 3)
	for _, item := range response.Items {
		require.Equal(t, "seed", item.Name)
	}
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
	setup.gin.ServeHTTP(w, signedSearchRequest(t, setup, "/search/resources?q=payments&resourceType=actor", aschema.AllPermissions()))
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
	setup.gin.ServeHTTP(w, signedSearchRequest(t, setup, "/search/resources?q=payments&resourceType=actor", aschema.AllPermissions()))
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
	setup.gin.ServeHTTP(w, signedSearchRequest(t, setup, "/search/resources?q=payments&resourceType=actor", aschema.AllPermissions()))
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}
