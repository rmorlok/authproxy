package routes

import (
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/app_metrics"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/httpf"
	sapi "github.com/rmorlok/authproxy/internal/schema/api"
	schemaapiopenapi "github.com/rmorlok/authproxy/internal/schema/api/openapi"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type OpenAPIRequestEventsEntry = schemaapiopenapi.RequestEventJson
type OpenAPIListRequestEventsResponse = schemaapiopenapi.ListRequestEventsResponseJson
type OpenAPIMetricsSchemaResponse = schemaapiopenapi.MetricsSchemaResponseJson

type RequestEventsRoutes struct {
	cfg  config.C
	auth auth.A
	rl   app_metrics.LogRetriever
}

type ListRequestEventsQuery struct {
	Cursor     *string `form:"cursor"`
	LimitVal   *int32  `form:"limit"`
	OrderByVal *string `form:"order_by"`

	/*
	 * Filters
	 */
	Namespace                *string  `form:"namespace"`
	RequestType              *string  `form:"request_type"`
	CorrelationId            *string  `form:"correlation_id"`
	ConnectionId             *apid.ID `form:"connection_id" swaggertype:"string"`
	ConnectorType            *string  `form:"connector_type"`
	ConnectorId              *apid.ID `form:"connector_id" swaggertype:"string"`
	ConnectorVersion         *uint64  `form:"connector_version"`
	Method                   *string  `form:"method"`
	StatusCode               *int     `form:"status_code"`
	StatusCodeRangeInclusive *string  `form:"status_code_range"`
	TimestampRange           *string  `form:"timestamp_range"`
	Path                     *string  `form:"path"`
	PathRegex                *string  `form:"path_regex"`
	LabelSelector            *string  `form:"label_selector"`
	ResponseSource           *string  `form:"response_source"`
	RateLimitId              *apid.ID `form:"rate_limit_id" swaggertype:"string"`
}

func (q *ListRequestEventsQuery) ApplyToBuilder(
	b app_metrics.ListRequestBuilder,
) (_ app_metrics.ListRequestBuilder, err error) {
	if q.Namespace != nil {
		b = b.ForNamespaceMatcher(*q.Namespace)
	}

	if q.RequestType != nil {
		b = b.ForRequestType(httpf.RequestType(*q.RequestType))
	}

	if q.CorrelationId != nil {
		b = b.ForCorrelationId(*q.CorrelationId)
	}

	if q.ConnectionId != nil {
		b = b.ForConnectionId(*q.ConnectionId)
	}

	if q.ConnectorType != nil {
		b = b.ForConnectorType(*q.ConnectorType)
	}

	if q.ConnectorId != nil {
		b = b.ForConnectorId(*q.ConnectorId)
	}

	if q.ConnectorVersion != nil {
		b = b.ForConnectorVersion(*q.ConnectorVersion)
	}

	if q.Method != nil {
		b = b.ForMethod(*q.Method)
	}

	if q.StatusCode != nil && q.StatusCodeRangeInclusive != nil {
		return nil, httperr.BadRequest("cannot specify both status_code and status_code_range")
	}

	if q.StatusCode != nil {
		b = b.ForStatusCode(*q.StatusCode)
	}

	if q.StatusCodeRangeInclusive != nil {
		b, err = b.ForParsedStatusCodeRange(*q.StatusCodeRangeInclusive)
		if err != nil {
			return nil, httperr.BadRequest("invalid status_code_range", httperr.WithInternalErr(err))
		}
	}

	if q.TimestampRange != nil {
		b, err = b.ForParsedTimestampRange(*q.TimestampRange)
		if err != nil {
			return nil, httperr.BadRequest("invalid timestamp_range", httperr.WithInternalErr(err))
		}
	}

	if q.Path != nil && q.PathRegex != nil {
		return nil, httperr.BadRequest("cannot specify both path and path_regex")
	}

	if q.Path != nil {
		b = b.ForPath(*q.Path)
	}

	if q.PathRegex != nil {
		b, err = b.ForPathRegex(*q.PathRegex)
		if err != nil {
			return nil, httperr.BadRequest("invalid path_regex", httperr.WithInternalErr(err))
		}
	}

	if q.LabelSelector != nil {
		b, err = b.ForLabelSelector(*q.LabelSelector)
		if err != nil {
			return nil, httperr.BadRequest("invalid label_selector", httperr.WithInternalErr(err))
		}
	}

	if q.ResponseSource != nil {
		src := app_metrics.ResponseSource(*q.ResponseSource)
		if !app_metrics.IsValidResponseSource(src) {
			return nil, httperr.BadRequestf("invalid response_source %q", *q.ResponseSource)
		}
		b = b.ForResponseSource(src)
	}

	if q.RateLimitId != nil {
		b = b.ForRateLimitId(*q.RateLimitId)
	}

	return b, nil
}

type ListRequestEventsResponseJson = sapi.ListRequestEventsResponseJson

func requestEventToJson(r *app_metrics.LogRecord) *sapi.RequestEventJson {
	if r == nil {
		return nil
	}

	matches := make([]sapi.RequestEventRateLimit, len(r.RateLimitMatched))
	for i, m := range r.RateLimitMatched {
		matches[i] = sapi.RequestEventRateLimit{
			Id:     m.Id,
			Mode:   m.Mode,
			Bucket: m.Bucket,
		}
	}

	return &sapi.RequestEventJson{
		Namespace:           r.Namespace,
		Type:                string(r.Type),
		RequestId:           r.RequestId,
		CorrelationId:       r.CorrelationId,
		Timestamp:           r.Timestamp,
		MillisecondDuration: int64(r.MillisecondDuration.Duration() / time.Millisecond),
		ConnectionId:        r.ConnectionId,
		ConnectorId:         r.ConnectorId,
		ConnectorVersion:    r.ConnectorVersion,
		Method:              r.Method,
		Host:                r.Host,
		Scheme:              r.Scheme,
		Path:                r.Path,
		RequestHttpVersion:  r.RequestHttpVersion,
		RequestSizeBytes:    r.RequestSizeBytes,
		RequestMimeType:     r.RequestMimeType,
		RequestBodySkipped:  string(r.RequestBodySkipped),
		ResponseStatusCode:  r.ResponseStatusCode,
		ResponseError:       r.ResponseError,
		ResponseHttpVersion: r.ResponseHttpVersion,
		ResponseSizeBytes:   r.ResponseSizeBytes,
		ResponseMimeType:    r.ResponseMimeType,
		ResponseBodySkipped: string(r.ResponseBodySkipped),
		InternalTimeout:     r.InternalTimeout,
		RequestCancelled:    r.RequestCancelled,
		FullRequestRecorded: r.FullRequestRecorded,
		Labels:              r.Labels,
		ResponseSource:      string(r.ResponseSource),
		RateLimitId:         r.RateLimitId,
		RateLimitMode:       r.RateLimitMode,
		RateLimitBucket:     r.RateLimitBucket,
		RateLimitMatched:    matches,
	}
}

func metricsQueryToRequestEventQuery(
	req sapi.MetricsQueryRequestJson,
	ref sapi.MetricsQueryRefJson,
	effectiveNamespaceMatchers []string,
	step time.Duration,
) (app_metrics.RequestEventMetricsQuery, error) {
	metric, err := requestEventMetricFromAPI(ref.Metric, ref.Aggregation)
	if err != nil {
		return app_metrics.RequestEventMetricsQuery{}, err
	}

	groupBy := make([]app_metrics.RequestEventGroupBy, 0, len(ref.GroupBy))
	for _, raw := range ref.GroupBy {
		gb := app_metrics.RequestEventGroupBy(raw)
		switch gb {
		case app_metrics.RequestEventGroupByType,
			app_metrics.RequestEventGroupByMethod,
			app_metrics.RequestEventGroupByResponseStatusCode,
			app_metrics.RequestEventGroupByResponseSource,
			app_metrics.RequestEventGroupByConnectorID:
			groupBy = append(groupBy, gb)
		default:
			return app_metrics.RequestEventMetricsQuery{}, httperr.BadRequestf("invalid group_by %q", raw)
		}
	}

	query := app_metrics.RequestEventMetricsQuery{
		RefID:             ref.RefID,
		Metric:            metric,
		Start:             req.Range.Start,
		End:               req.Range.End,
		Step:              step,
		NamespaceMatchers: effectiveNamespaceMatchers,
		GroupBy:           groupBy,
	}
	if req.LabelSelector != nil {
		query.LabelSelector = *req.LabelSelector
	}
	return query, nil
}

func requestEventMetricFromAPI(metric, aggregation string) (app_metrics.RequestEventMetric, error) {
	switch metric {
	case "request_events":
		if aggregation == "count" {
			return app_metrics.RequestEventMetricCount, nil
		}
	case "request_events.errors":
		if aggregation == "count" {
			return app_metrics.RequestEventMetricErrorsCount, nil
		}
	case "request_events.duration_ms":
		switch aggregation {
		case "avg":
			return app_metrics.RequestEventMetricDurationAvgMS, nil
		case "p95":
			return app_metrics.RequestEventMetricDurationP95MS, nil
		}
	}
	return "", httperr.BadRequestf("unsupported metric aggregation %q/%q", metric, aggregation)
}

func metricsQueryToResourceQuery(
	req sapi.MetricsQueryRequestJson,
	ref sapi.MetricsQueryRefJson,
	effectiveNamespaceMatchers []string,
	step time.Duration,
) (app_metrics.ResourceMetricsQuery, error) {
	metric, err := resourceMetricFromAPI(ref.Metric, ref.Aggregation)
	if err != nil {
		return app_metrics.ResourceMetricsQuery{}, err
	}

	groupBy := make([]app_metrics.ResourceGroupBy, 0, len(ref.GroupBy))
	for _, raw := range ref.GroupBy {
		gb := app_metrics.ResourceGroupBy(raw)
		if !app_metrics.IsValidResourceGroupBy(metric, gb) {
			return app_metrics.ResourceMetricsQuery{}, httperr.BadRequestf("invalid group_by %q", raw)
		}
		groupBy = append(groupBy, gb)
	}

	query := app_metrics.ResourceMetricsQuery{
		RefID:             ref.RefID,
		Metric:            metric,
		Start:             req.Range.Start,
		End:               req.Range.End,
		Step:              step,
		NamespaceMatchers: effectiveNamespaceMatchers,
		GroupBy:           groupBy,
	}
	if req.LabelSelector != nil {
		query.LabelSelector = *req.LabelSelector
	}
	return query, nil
}

func resourceMetricFromAPI(metric, aggregation string) (app_metrics.ResourceMetric, error) {
	switch metric {
	case "resources.connections":
		if aggregation == "count" {
			return app_metrics.ResourceMetricConnectionsCount, nil
		}
	case "resources.actors":
		if aggregation == "count" {
			return app_metrics.ResourceMetricActorsCount, nil
		}
	case "resources.connectors":
		if aggregation == "count" {
			return app_metrics.ResourceMetricConnectorsCount, nil
		}
	case "resources.connector_versions":
		if aggregation == "count" {
			return app_metrics.ResourceMetricConnectorVersionsCount, nil
		}
	case "resources.namespaces":
		if aggregation == "count" {
			return app_metrics.ResourceMetricNamespacesCount, nil
		}
	case "resources.rate_limits":
		if aggregation == "count" {
			return app_metrics.ResourceMetricRateLimitsCount, nil
		}
	}
	return "", httperr.BadRequestf("unsupported metric aggregation %q/%q", metric, aggregation)
}

func metricsSchemaResponse() sapi.MetricsSchemaResponseJson {
	requestEventGroupBy := []string{
		string(app_metrics.RequestEventGroupByType),
		string(app_metrics.RequestEventGroupByMethod),
		string(app_metrics.RequestEventGroupByResponseStatusCode),
		string(app_metrics.RequestEventGroupByResponseSource),
		string(app_metrics.RequestEventGroupByConnectorID),
	}

	return sapi.MetricsSchemaResponseJson{
		Metrics: []sapi.MetricsSchemaMetricJson{
			{
				Metric:       "request_events",
				Kind:         "counter",
				Aggregations: []string{"count"},
				GroupBy:      requestEventGroupBy,
			},
			{
				Metric:       "request_events.errors",
				Kind:         "counter",
				Aggregations: []string{"count"},
				GroupBy:      requestEventGroupBy,
			},
			{
				Metric:       "request_events.duration_ms",
				Kind:         "gauge",
				Aggregations: []string{"avg", "p95"},
				GroupBy:      requestEventGroupBy,
			},
			{
				Metric:       "resources.connections",
				Kind:         "gauge",
				Aggregations: []string{"count"},
				GroupBy: []string{
					string(app_metrics.ResourceGroupByState),
					string(app_metrics.ResourceGroupByHealthState),
					string(app_metrics.ResourceGroupByConnectorID),
					string(app_metrics.ResourceGroupByConnectorVersion),
				},
			},
			{
				Metric:       "resources.actors",
				Kind:         "gauge",
				Aggregations: []string{"count"},
				GroupBy: []string{
					string(app_metrics.ResourceGroupByNamespace),
				},
			},
			{
				Metric:       "resources.connectors",
				Kind:         "gauge",
				Aggregations: []string{"count"},
				GroupBy: []string{
					string(app_metrics.ResourceGroupByState),
					string(app_metrics.ResourceGroupByConnectorVersion),
					string(app_metrics.ResourceGroupByNamespace),
				},
			},
			{
				Metric:       "resources.connector_versions",
				Kind:         "gauge",
				Aggregations: []string{"count"},
				GroupBy: []string{
					string(app_metrics.ResourceGroupByState),
					string(app_metrics.ResourceGroupByConnectorID),
					string(app_metrics.ResourceGroupByConnectorVersion),
					string(app_metrics.ResourceGroupByNamespace),
				},
			},
			{
				Metric:       "resources.namespaces",
				Kind:         "gauge",
				Aggregations: []string{"count"},
				GroupBy: []string{
					string(app_metrics.ResourceGroupByState),
					string(app_metrics.ResourceGroupByNamespace),
				},
			},
			{
				Metric:       "resources.rate_limits",
				Kind:         "gauge",
				Aggregations: []string{"count"},
				GroupBy: []string{
					string(app_metrics.ResourceGroupByMode),
					string(app_metrics.ResourceGroupByNamespace),
				},
			},
		},
	}
}

type metricsResponseSeries struct {
	RefID  string
	Labels map[string]string
	Points []sapi.MetricsPointJson
}

func requestEventMetricsResponseSeries(series []app_metrics.RequestEventMetricSeries) []metricsResponseSeries {
	out := make([]metricsResponseSeries, 0, len(series))
	for _, s := range series {
		points := make([]sapi.MetricsPointJson, 0, len(s.Points))
		for _, p := range s.Points {
			points = append(points, sapi.MetricsPointJson{
				Timestamp: p.Timestamp,
				Value:     p.Value,
			})
		}
		out = append(out, metricsResponseSeries{
			RefID:  s.RefID,
			Labels: s.Labels,
			Points: points,
		})
	}
	return out
}

func resourceMetricsResponseSeries(series []app_metrics.ResourceMetricSeries) []metricsResponseSeries {
	out := make([]metricsResponseSeries, 0, len(series))
	for _, s := range series {
		points := make([]sapi.MetricsPointJson, 0, len(s.Points))
		for _, p := range s.Points {
			points = append(points, sapi.MetricsPointJson{
				Timestamp: p.Timestamp,
				Value:     p.Value,
			})
		}
		out = append(out, metricsResponseSeries{
			RefID:  s.RefID,
			Labels: s.Labels,
			Points: points,
		})
	}
	return out
}

func metricsResponseFromAPIRequest(req sapi.MetricsQueryRequestJson, series []metricsResponseSeries) sapi.MetricsQueryResponseJson {
	refsByID := make(map[string]sapi.MetricsQueryRefJson, len(req.Queries))
	refOrder := make(map[string]int, len(req.Queries))
	for i, ref := range req.Queries {
		refsByID[ref.RefID] = ref
		refOrder[ref.RefID] = i
	}
	sort.SliceStable(series, func(i, j int) bool {
		leftOrder := refOrder[series[i].RefID]
		rightOrder := refOrder[series[j].RefID]
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return metricsSeriesGroupKey(series[i].Labels) < metricsSeriesGroupKey(series[j].Labels)
	})

	out := sapi.MetricsQueryResponseJson{
		Series: make([]sapi.MetricsSeriesJson, 0, len(series)),
	}
	for _, s := range series {
		ref := refsByID[s.RefID]
		out.Series = append(out.Series, sapi.MetricsSeriesJson{
			RefID:       s.RefID,
			Metric:      ref.Metric,
			Aggregation: ref.Aggregation,
			Labels:      s.Labels,
			Points:      s.Points,
		})
	}
	return out
}

func metricsSeriesGroupKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := ""
	for _, key := range keys {
		out += key + "=" + labels[key] + "\x00"
	}
	return out
}

// @Summary		Get request events entry
// @Description	Get a specific request events entry by its UUID
// @Tags			request-events
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Request events entry UUID"
// @Success		200	{object}	OpenAPIRequestEventsEntry
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/metrics/request-events/{id} [get]
func (r *RequestEventsRoutes) get(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	logIdStr := gctx.Param("id")

	if logIdStr == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	logId, err := apid.Parse(logIdStr)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format"))
		val.MarkErrorReturn()
		return
	}

	if logId == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	entry, err := r.rl.GetFullLog(ctx, logId)

	if err != nil {
		if errors.Is(err, app_metrics.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound("request events not found"))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(entry); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, entry)
}

// @Summary		List request events entries
// @Description	List request events entries with optional filtering and pagination
// @Tags			request-events
// @Accept			json
// @Produce		json
// @Param			cursor				query		string	false	"Pagination cursor"
// @Param			limit				query		integer	false	"Maximum number of results to return"
// @Param			order_by			query		string	false	"Order by field (e.g., 'timestamp:desc')"
// @Param			namespace			query		string	false	"Filter by namespace"
// @Param			request_type		query		string	false	"Filter by request type"
// @Param			correlation_id		query		string	false	"Filter by correlation ID"
// @Param			connection_id		query		string	false	"Filter by connection UUID"
// @Param			connector_type		query		string	false	"Filter by connector type"
// @Param			connector_id		query		string	false	"Filter by connector UUID"
// @Param			connector_version	query		integer	false	"Filter by connector version"
// @Param			method				query		string	false	"Filter by HTTP method"
// @Param			status_code			query		integer	false	"Filter by exact status code"
// @Param			status_code_range	query		string	false	"Filter by status code range (e.g., '200-299')"
// @Param			timestamp_range		query		string	false	"Filter by timestamp range"
// @Param			path				query		string	false	"Filter by exact path"
// @Param			path_regex			query		string	false	"Filter by path regex"
// @Param			label_selector		query		string	false	"Filter by label selector (e.g., 'env=prod,team=api')"
// @Success		200					{object}	OpenAPIListRequestEventsResponse
// @Failure		400					{object}	ErrorResponse
// @Failure		401					{object}	ErrorResponse
// @Failure		500					{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/metrics/request-events [get]
func (r *RequestEventsRoutes) list(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req ListRequestEventsQuery
	var err error

	if err = gctx.ShouldBindQuery(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	var ex app_metrics.ListRequestExecutor

	if req.Cursor != nil {
		ex, err = r.rl.ListRequestsFromCursor(gctx, *req.Cursor)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequest("failed to list requests from cursor", httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	} else {
		b := r.rl.NewListRequestsBuilder()

		b = b.ForNamespaceMatchers(val.GetEffectiveNamespaceMatchers(req.Namespace))

		b, err = req.ApplyToBuilder(b)
		if err != nil {
			apgin.WriteErr(gctx, nil, err)
			val.MarkErrorReturn()
			return
		}

		if req.LimitVal != nil {
			b = b.Limit(*req.LimitVal)
		}

		if req.OrderByVal != nil {
			field, ob, err := pagination.SplitOrderByParam[app_metrics.RequestOrderByField](*req.OrderByVal)
			if err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequest("invalid order by", httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			if !app_metrics.IsValidOrderByField(field) {
				apgin.WriteError(gctx, nil, httperr.BadRequest("invalid order by field"))
				val.MarkErrorReturn()
				return
			}

			b = b.OrderBy(field, ob)
		}

		ex = b
	}

	result := ex.FetchPage(ctx)
	if result.Error != nil {
		apgin.WriteErr(gctx, nil, result.Error)
		val.MarkErrorReturn()
		return
	}

	apgin.APIJSON(gctx, 200, &ListRequestEventsResponseJson{
		Items:  util.Map(auth.FilterForValidatedResources(val, result.Results), requestEventToJson),
		Cursor: result.Cursor,
		Total:  result.Total,
	})
}

// @Summary		Query application metrics
// @Description	Query application metrics over a time range
// @Tags			metrics
// @Accept			json
// @Produce		json
// @Param			request	body		object	true	"Metrics query request"
// @Success		200		{object}	object
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/metrics/query [post]
func (r *RequestEventsRoutes) queryMetrics(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req sapi.MetricsQueryRequestJson
	if err := gctx.ShouldBindJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if req.Range.Start.IsZero() || req.Range.End.IsZero() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("range.start and range.end are required"))
		val.MarkErrorReturn()
		return
	}
	if !req.Range.Start.Before(req.Range.End) {
		apgin.WriteError(gctx, nil, httperr.BadRequest("range.start must be before range.end"))
		val.MarkErrorReturn()
		return
	}
	step, err := time.ParseDuration(req.Range.Step)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("range.step must be a positive duration", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}
	if step <= 0 {
		apgin.WriteError(gctx, nil, httperr.BadRequest("range.step must be a positive duration"))
		val.MarkErrorReturn()
		return
	}
	if len(req.Queries) == 0 {
		apgin.WriteError(gctx, nil, httperr.BadRequest("queries is required"))
		val.MarkErrorReturn()
		return
	}
	if req.Namespace != nil {
		if err := namespace.ValidateMatcher(*req.Namespace); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequest("invalid namespace matcher", httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}
	if req.LabelSelector != nil {
		if _, err := database.ParseLabelSelector(*req.LabelSelector); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequest("invalid label selector", httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	effectiveNamespaceMatchers := val.GetEffectiveNamespaceMatchers(req.Namespace)
	requestEventQueries := make([]app_metrics.RequestEventMetricsQuery, 0, len(req.Queries))
	resourceQueries := make([]app_metrics.ResourceMetricsQuery, 0, len(req.Queries))
	for _, ref := range req.Queries {
		if _, err := requestEventMetricFromAPI(ref.Metric, ref.Aggregation); err == nil {
			q, err := metricsQueryToRequestEventQuery(req, ref, effectiveNamespaceMatchers, step)
			if err != nil {
				apgin.WriteErr(gctx, nil, err)
				val.MarkErrorReturn()
				return
			}
			requestEventQueries = append(requestEventQueries, q)
			continue
		}

		if _, err := resourceMetricFromAPI(ref.Metric, ref.Aggregation); err == nil {
			q, err := metricsQueryToResourceQuery(req, ref, effectiveNamespaceMatchers, step)
			if err != nil {
				apgin.WriteErr(gctx, nil, err)
				val.MarkErrorReturn()
				return
			}
			resourceQueries = append(resourceQueries, q)
			continue
		}

		apgin.WriteError(gctx, nil, httperr.BadRequestf("unsupported metric aggregation %q/%q", ref.Metric, ref.Aggregation))
		val.MarkErrorReturn()
		return
	}

	responseSeries := make([]metricsResponseSeries, 0)
	if len(requestEventQueries) > 0 {
		series, err := r.rl.QueryRequestEventMetrics(ctx, requestEventQueries)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequest("invalid metrics query", httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
		responseSeries = append(responseSeries, requestEventMetricsResponseSeries(series)...)
	}
	if len(resourceQueries) > 0 {
		series, err := r.rl.QueryResourceMetrics(ctx, resourceQueries)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequest("invalid metrics query", httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
		responseSeries = append(responseSeries, resourceMetricsResponseSeries(series)...)
	}

	// Metrics responses are aggregate series, not resource rows, so the request is validated once all query refs are
	// accepted and their namespace matchers are constrained by the actor's permissions.
	val.MarkValidated()
	apgin.APIJSON(gctx, http.StatusOK, metricsResponseFromAPIRequest(req, responseSeries))
}

// @Summary		Get application metrics schema
// @Description	Get supported application metrics, aggregations, group-by dimensions, and metric kinds
// @Tags			metrics
// @Produce		json
// @Success		200	{object}	OpenAPIMetricsSchemaResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		403	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/metrics/schema [get]
func (r *RequestEventsRoutes) schema(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	// The schema response is static metadata for the app-metrics API, so there are no resource rows to post-validate.
	val.MarkValidated()
	apgin.APIJSON(gctx, http.StatusOK, metricsSchemaResponse())
}

func (r *RequestEventsRoutes) Register(g gin.IRouter) {
	g.GET(
		"/metrics/request-events/:id",
		r.auth.NewRequiredBuilder().
			ForResource("request-events").
			ForIdField("id").
			ForVerb("get").
			Build(),
		r.get,
	)
	g.GET(
		"/metrics/request-events",
		r.auth.NewRequiredBuilder().
			ForResource("request-events").
			ForVerb("list").
			Build(),
		r.list,
	)
	g.POST(
		"/metrics/query",
		r.auth.NewRequiredBuilder().
			ForResource("app-metrics").
			ForVerb("query").
			Build(),
		r.queryMetrics,
	)
	g.GET(
		"/metrics/schema",
		r.auth.NewRequiredBuilder().
			ForResource("app-metrics").
			ForVerb("schema").
			Build(),
		r.schema,
	)
}

func NewRequestEventsRoutes(
	cfg config.C,
	auth auth.A,
	rl app_metrics.LogRetriever,
) *RequestEventsRoutes {
	return &RequestEventsRoutes{
		cfg:  cfg,
		auth: auth,
		rl:   rl,
	}
}
