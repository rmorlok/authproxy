package routes

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/app_metrics"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

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
		b = b.WithNamespaceMatcher(*q.Namespace)
	}

	if q.RequestType != nil {
		b = b.WithRequestType(httpf.RequestType(*q.RequestType))
	}

	if q.CorrelationId != nil {
		b = b.WithCorrelationId(*q.CorrelationId)
	}

	if q.ConnectionId != nil {
		b = b.WithConnectionId(*q.ConnectionId)
	}

	if q.ConnectorType != nil {
		b = b.WithConnectorType(*q.ConnectorType)
	}

	if q.ConnectorId != nil {
		b = b.WithConnectorId(*q.ConnectorId)
	}

	if q.ConnectorVersion != nil {
		b = b.WithConnectorVersion(*q.ConnectorVersion)
	}

	if q.Method != nil {
		b = b.WithMethod(*q.Method)
	}

	if q.StatusCode != nil && q.StatusCodeRangeInclusive != nil {
		return nil, httperr.BadRequest("cannot specify both status_code and status_code_range")
	}

	if q.StatusCode != nil {
		b = b.WithStatusCode(*q.StatusCode)
	}

	if q.StatusCodeRangeInclusive != nil {
		b, err = b.WithParsedStatusCodeRange(*q.StatusCodeRangeInclusive)
		if err != nil {
			return nil, err
		}
	}

	if q.TimestampRange != nil {
		b, err = b.WithParsedTimestampRange(*q.TimestampRange)
		if err != nil {
			return nil, err
		}
	}

	if q.Path != nil && q.PathRegex != nil {
		return nil, httperr.BadRequest("cannot specify both path and path_regex")
	}

	if q.Path != nil {
		b = b.WithPath(*q.Path)
	}

	if q.PathRegex != nil {
		b, err = b.WithPathRegex(*q.PathRegex)
		if err != nil {
			return nil, err
		}
	}

	if q.LabelSelector != nil {
		b, err = b.WithLabelSelector(*q.LabelSelector)
		if err != nil {
			return nil, err
		}
	}

	if q.ResponseSource != nil {
		src := app_metrics.ResponseSource(*q.ResponseSource)
		if !app_metrics.IsValidResponseSource(src) {
			return nil, httperr.BadRequestf("invalid response_source %q", *q.ResponseSource)
		}
		b = b.WithResponseSource(src)
	}

	if q.RateLimitId != nil {
		b = b.WithRateLimitId(*q.RateLimitId)
	}

	return b, nil
}

type ListRequestEventsResponseJson struct {
	Items  []*app_metrics.LogRecord `json:"items"`
	Cursor string                   `json:"cursor,omitempty"`
	Total  *int64                   `json:"total,omitempty"`
}

// @Summary		Get request events entry
// @Description	Get a specific request events entry by its UUID
// @Tags			request-events
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Request events entry UUID"
// @Success		200	{object}	SwaggerRequestEventsEntry
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

	gctx.PureJSON(http.StatusOK, entry)
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
// @Success		200					{object}	SwaggerListRequestEventsResponse
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

		b = b.WithNamespaceMatchers(val.GetEffectiveNamespaceMatchers(req.Namespace))

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

	gctx.PureJSON(200, &ListRequestEventsResponseJson{
		Items:  auth.FilterForValidatedResources(val, result.Results),
		Cursor: result.Cursor,
		Total:  result.Total,
	})
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
