package routes

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	schemaapiopenapi "github.com/rmorlok/authproxy/internal/schema/api/openapi"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

const (
	resourceSearchModeQuery           = "query"
	resourceSearchModeSeed            = "seed"
	resourceSearchMaxLimit            = 50
	resourceSearchDefaultQueryLimit   = 30
	resourceSearchUntypedPerTypeLimit = 10
	resourceSearchTypeTimeout         = 750 * time.Millisecond
	resourceSearchOverallTimeout      = 1500 * time.Millisecond
)

type ResourceSearchRoutes struct {
	auth           auth.A
	db             database.ResourceSearcher
	logger         *slog.Logger
	typeTimeout    time.Duration
	overallTimeout time.Duration
}

type SearchResourcesRequestQuery struct {
	Mode          string   `form:"mode"`
	ResourceTypes []string `form:"resource_type"`
	Query         string   `form:"q"`
	LabelSelector string   `form:"label_selector"`
	Namespace     string   `form:"namespace"`
	Limit         *int     `form:"limit"`
}

type OpenAPISearchResourcesResponseJson = schemaapiopenapi.SearchResourcesResponseJson

type searchTypeResult struct {
	resourceType database.SearchResourceType
	result       database.SearchResourcesResult
	err          error
}

var searchPermissionResources = map[database.SearchResourceType]string{
	database.SearchResourceTypeActor:      "actors",
	database.SearchResourceTypeConnection: "connections",
	database.SearchResourceTypeConnector:  "connectors",
	database.SearchResourceTypeNamespace:  "namespaces",
	database.SearchResourceTypeKey:        "keys",
	database.SearchResourceTypeRateLimit:  "rate_limits",
}

// @Summary      Search Admin resources
// @Description  Search durable resources by label value and exact label selector, or seed a bounded recent-resource cache
// @Tags         search
// @Produce      json
// @Param        mode            query string false "Search mode: query or seed"
// @Param        resource_type   query []string false "Resource types; may be repeated" collectionFormat(multi)
// @Param        q               query string false "Case-insensitive literal label-value substring (minimum 3 characters)"
// @Param        label_selector  query string false "Exact Kubernetes-style label selector"
// @Param        namespace       query string false "Namespace matcher"
// @Param        limit           query int false "Maximum results (query) or results per type (seed), up to 50"
// @Success      200 {object} OpenAPISearchResourcesResponseJson
// @Failure      400 {object} ErrorResponse
// @Failure      401 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Security     BearerAuth
// @Router       /search/resources [get]
func (r *ResourceSearchRoutes) search(gctx *gin.Context) {
	startedAt := time.Now()
	requestContext := gctx.Request.Context()
	ra := auth.MustGetAuthFromGinContext(gctx)

	var req SearchResourcesRequestQuery
	if err := gctx.ShouldBindQuery(&req); err != nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		return
	}

	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = resourceSearchModeQuery
	}
	if mode != resourceSearchModeQuery && mode != resourceSearchModeSeed {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("mode must be query or seed"))
		return
	}

	queryText := strings.TrimSpace(req.Query)
	labelSelector := strings.TrimSpace(req.LabelSelector)
	if labelSelector != "" {
		if _, err := database.ParseLabelSelector(labelSelector); err != nil {
			// Do not attach the parser error: it includes the raw selector and
			// label value, which search telemetry must never log.
			apgin.WriteError(gctx, r.logger, httperr.BadRequest("invalid label_selector"))
			return
		}
	}
	if mode == resourceSearchModeQuery {
		if queryText == "" && labelSelector == "" {
			apgin.WriteError(gctx, r.logger, httperr.BadRequest("query mode requires q or label_selector"))
			return
		}
		if queryText != "" && len([]rune(queryText)) < 3 && labelSelector == "" {
			apgin.WriteError(gctx, r.logger, httperr.BadRequest("q must contain at least 3 characters"))
			return
		}
	} else if queryText != "" || labelSelector != "" {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("seed mode does not accept q or label_selector"))
		return
	}

	if req.Namespace != "" {
		if err := namespace.ValidateMatcher(req.Namespace); err != nil {
			apgin.WriteError(gctx, r.logger, httperr.BadRequest("invalid namespace", httperr.WithInternalErr(err)))
			return
		}
	}

	resourceTypes, explicitTypes, err := parseSearchResourceTypes(req.ResourceTypes)
	if err != nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		return
	}
	limit := resourceSearchDefaultQueryLimit
	if mode == resourceSearchModeSeed {
		limit = resourceSearchMaxLimit
	}
	if req.Limit != nil {
		limit = *req.Limit
	}
	if limit <= 0 || limit > resourceSearchMaxLimit {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("limit must be between 1 and 50"))
		return
	}

	overallContext, cancelOverall := context.WithTimeout(requestContext, r.overallTimeout)
	defer cancelOverall()
	resultChannel := make(chan searchTypeResult, len(resourceTypes))
	semaphore := make(chan struct{}, 2)
	launchedTypes := make(map[database.SearchResourceType]struct{}, len(resourceTypes))

	for _, resourceType := range resourceTypes {
		permissionResource := searchPermissionResources[resourceType]
		matchers := permittedSearchNamespaceMatchers(ra, permissionResource, req.Namespace)
		if len(matchers) == 0 {
			continue
		}

		perTypeLimit := limit
		if mode == resourceSearchModeQuery && !explicitTypes {
			perTypeLimit = resourceSearchUntypedPerTypeLimit
		}
		launchedTypes[resourceType] = struct{}{}
		go func(resourceType database.SearchResourceType, permissionResource string, matchers []string, perTypeLimit int) {
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-overallContext.Done():
				resultChannel <- searchTypeResult{resourceType: resourceType, err: overallContext.Err()}
				return
			}

			typeContext, cancelType := context.WithTimeout(overallContext, r.typeTimeout)
			defer cancelType()
			result, err := r.db.SearchResources(typeContext, database.SearchResourcesParams{
				ResourceType:      resourceType,
				Query:             queryText,
				LabelSelector:     labelSelector,
				NamespaceMatchers: matchers,
				Limit:             perTypeLimit,
			})
			if err == nil {
				filtered := result.Items[:0]
				for _, item := range result.Items {
					if ra.Allows(item.Namespace, permissionResource, "list", item.ResourceID) &&
						ra.Allows(item.Namespace, permissionResource, "get", item.ResourceID) {
						filtered = append(filtered, item)
					}
				}
				result.Items = filtered
			}
			resultChannel <- searchTypeResult{resourceType: resourceType, result: result, err: err}
		}(resourceType, permissionResource, matchers, perTypeLimit)
	}

	var resources []database.SearchResource
	truncated := make(map[database.SearchResourceType]struct{})
	incomplete := make(map[database.SearchResourceType]struct{})
	receivedTypes := make(map[database.SearchResourceType]struct{}, len(launchedTypes))
	var unexpectedErr error
	consumeResult := func(typeResult searchTypeResult) {
		receivedTypes[typeResult.resourceType] = struct{}{}
		if typeResult.err != nil {
			if errors.Is(typeResult.err, context.DeadlineExceeded) || errors.Is(typeResult.err, context.Canceled) {
				incomplete[typeResult.resourceType] = struct{}{}
				return
			}
			unexpectedErr = typeResult.err
			return
		}
		resources = append(resources, typeResult.result.Items...)
		if typeResult.result.Truncated {
			truncated[typeResult.resourceType] = struct{}{}
		}
	}

	remaining := len(launchedTypes)
	overallTimedOut := false
	for remaining > 0 && unexpectedErr == nil {
		select {
		case typeResult := <-resultChannel:
			remaining--
			consumeResult(typeResult)
		case <-overallContext.Done():
			// Preserve any results that completed before the deadline but were
			// already buffered, then stop waiting for searchers that ignore
			// cancellation. The channel is sized for every worker, so late sends
			// cannot block after the handler returns.
			for {
				select {
				case typeResult := <-resultChannel:
					remaining--
					consumeResult(typeResult)
				default:
					overallTimedOut = true
					goto collectionComplete
				}
			}
		}
	}

collectionComplete:
	if overallTimedOut {
		for resourceType := range launchedTypes {
			if _, received := receivedTypes[resourceType]; !received {
				incomplete[resourceType] = struct{}{}
			}
		}
	}
	if unexpectedErr != nil {
		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(unexpectedErr)))
		return
	}

	sort.SliceStable(resources, func(i, j int) bool {
		if resources[i].MatchRank != resources[j].MatchRank {
			return resources[i].MatchRank < resources[j].MatchRank
		}
		if !resources[i].UpdatedAt.Equal(resources[j].UpdatedAt) {
			return resources[i].UpdatedAt.After(resources[j].UpdatedAt)
		}
		if resources[i].ResourceType != resources[j].ResourceType {
			return resources[i].ResourceType < resources[j].ResourceType
		}
		return resources[i].ResourceID < resources[j].ResourceID
	})
	if mode == resourceSearchModeQuery && len(resources) > limit {
		for _, dropped := range resources[limit:] {
			truncated[dropped.ResourceType] = struct{}{}
		}
		resources = resources[:limit]
	}

	response := schemaapi.SearchResourcesResponseJson{
		Items:           make([]schemaapi.SearchResourceSummaryJson, 0, len(resources)),
		TruncatedTypes:  schemaSearchTypes(truncated),
		IncompleteTypes: schemaSearchTypes(incomplete),
	}
	for _, resource := range resources {
		labels := map[string]string{}
		for key, value := range resource.Labels {
			labels[key] = value
		}
		matches := make([]schemaapi.SearchLabelMatchJson, 0, len(resource.MatchedLabels))
		for _, match := range resource.MatchedLabels {
			matches = append(matches, schemaapi.SearchLabelMatchJson{Key: match.Key, Value: match.Value})
		}
		response.Items = append(response.Items, schemaapi.SearchResourceSummaryJson{
			ResourceType:  schemaapi.SearchResourceType(resource.ResourceType),
			ResourceId:    resource.ResourceID,
			Namespace:     resource.Namespace,
			Labels:        labels,
			MatchedLabels: matches,
			UpdatedAt:     resource.UpdatedAt,
		})
	}

	r.logger.Info("admin resource search completed",
		"mode", mode,
		"resource_type_count", len(resourceTypes),
		"result_count", len(response.Items),
		"truncated_type_count", len(response.TruncatedTypes),
		"incomplete_type_count", len(response.IncompleteTypes),
		"duration", time.Since(startedAt),
	)
	apgin.APIJSON(gctx, http.StatusOK, response)
}

func parseSearchResourceTypes(rawTypes []string) ([]database.SearchResourceType, bool, error) {
	if len(rawTypes) == 0 {
		return database.SearchResourceTypes(), false, nil
	}
	seen := make(map[database.SearchResourceType]struct{}, len(rawTypes))
	result := make([]database.SearchResourceType, 0, len(rawTypes))
	for _, rawType := range rawTypes {
		resourceType := database.SearchResourceType(strings.TrimSpace(rawType))
		if !database.IsValidSearchResourceType(resourceType) {
			return nil, true, httperr.BadRequestf("invalid resource_type %q", rawType)
		}
		if _, ok := seen[resourceType]; ok {
			continue
		}
		seen[resourceType] = struct{}{}
		result = append(result, resourceType)
	}
	return result, true, nil
}

func permittedSearchNamespaceMatchers(ra *authcore.RequestAuth, resource, requested string) []string {
	matchers := intersectNamespaceMatchers(
		ra.GetNamespacesAllowed(resource, "list"),
		ra.GetNamespacesAllowed(resource, "get"),
	)
	if requested != "" {
		matchers = intersectNamespaceMatchers(matchers, []string{requested})
	}
	sort.Strings(matchers)
	return matchers
}

func intersectNamespaceMatchers(left, right []string) []string {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	for _, a := range left {
		for _, b := range right {
			if constrained, ok := namespace.ConstrainMatcher(a, b); ok {
				seen[constrained] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for matcher := range seen {
		result = append(result, matcher)
	}
	return result
}

func schemaSearchTypes(values map[database.SearchResourceType]struct{}) []schemaapi.SearchResourceType {
	result := make([]schemaapi.SearchResourceType, 0, len(values))
	for resourceType := range values {
		result = append(result, schemaapi.SearchResourceType(resourceType))
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func NewResourceSearchRoutes(a auth.A, db database.ResourceSearcher, logger *slog.Logger) *ResourceSearchRoutes {
	if logger == nil {
		logger = slog.Default()
	}
	return &ResourceSearchRoutes{
		auth:           a,
		db:             db,
		logger:         logger,
		typeTimeout:    resourceSearchTypeTimeout,
		overallTimeout: resourceSearchOverallTimeout,
	}
}

func (r *ResourceSearchRoutes) Register(g gin.IRouter) {
	g.GET("/search/resources", r.auth.Required(), r.search)
}
