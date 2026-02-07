package routes

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/request_log"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type RequestLogRoutes struct {
	cfg  config.C
	auth auth.A
	rl   request_log.LogRetriever
}

type ListRequestsQuery struct {
	Cursor     *string `form:"cursor"`
	LimitVal   *int32  `form:"limit"`
	OrderByVal *string `form:"order_by"`

	/*
	 * Filters
	 */
	Namespace                *string    `form:"namespace"`
	RequestType              *string    `form:"request_type"`
	CorrelationId            *string    `form:"correlation_id"`
	ConnectionId             *uuid.UUID `form:"connection_id"`
	ConnectorType            *string    `form:"connector_type"`
	ConnectorId              *uuid.UUID `form:"connector_id"`
	ConnectorVersion         *uint64    `form:"connector_version"`
	Method                   *string    `form:"method"`
	StatusCode               *int       `form:"status_code"`
	StatusCodeRangeInclusive *string    `form:"status_code_range"`
	TimestampRange           *string    `form:"timestamp_range"`
	Path                     *string    `form:"path"`
	PathRegex                *string    `form:"path_regex"`
}

func (q *ListRequestsQuery) ApplyToBuilder(
	b request_log.ListRequestBuilder,
) (_ request_log.ListRequestBuilder, err error) {
	if q.Namespace != nil {
		b = b.WithNamespaceMatcher(*q.Namespace)
	}

	if q.RequestType != nil {
		b = b.WithRequestType(request_log.RequestType(*q.RequestType))
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
		return nil, api_common.
			NewHttpStatusErrorBuilder().
			DefaultStatusBadRequest().
			WithResponseMsg("cannot specify both status_code and status_code_range").
			Build()
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
		return nil, api_common.
			NewHttpStatusErrorBuilder().
			DefaultStatusBadRequest().
			WithResponseMsg("cannot specify both path and path_regex").
			Build()
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

	return b, nil
}

type ListRequestsResponseJson struct {
	Items  []*request_log.EntryRecord `json:"items"`
	Cursor string                     `json:"cursor,omitempty"`
	Total  *int64                     `json:"total,omitempty"`
}

// @Summary		Get request log entry
// @Description	Get a specific request log entry by its UUID
// @Tags			request-log
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Request log entry UUID"
// @Success		200	{object}	SwaggerRequestLogEntry
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/request-log/{id} [get]
func (r *RequestLogRoutes) get(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	logIdStr := gctx.Param("id")

	if logIdStr == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	logId, err := uuid.Parse(logIdStr)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if logId == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	entry, err := r.rl.GetFullLog(ctx, logId)

	if err != nil {
		if errors.Is(err, request_log.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("request log not found").
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(entry); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, entry)
}

// @Summary		List request log entries
// @Description	List request log entries with optional filtering and pagination
// @Tags			request-log
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
// @Success		200					{object}	SwaggerListRequestsResponse
// @Failure		400					{object}	ErrorResponse
// @Failure		401					{object}	ErrorResponse
// @Failure		500					{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/request-log [get]
func (r *RequestLogRoutes) list(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req ListRequestsQuery
	var err error

	if err = gctx.ShouldBindQuery(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg(err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	var ex request_log.ListRequestExecutor

	if req.Cursor != nil {
		ex, err = r.rl.ListRequestsFromCursor(gctx, *req.Cursor)
		if err != nil {
			api_common.HttpStatusErrorBuilderFromError(err).
				WithStatusBadRequest().
				WithInternalErr(err).
				WithResponseMsg("failed to list requests from cursor").
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
	} else {
		b := r.rl.NewListRequestsBuilder()

		b = b.WithNamespaceMatchers(val.GetEffectiveNamespaceMatchers(req.Namespace))

		b, err = req.ApplyToBuilder(b)
		if err != nil {
			api_common.HttpStatusErrorBuilderFromError(err).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}

		if req.LimitVal != nil {
			b = b.Limit(*req.LimitVal)
		}

		if req.OrderByVal != nil {
			field, ob, err := pagination.SplitOrderByParam[request_log.RequestOrderByField](*req.OrderByVal)
			if err != nil {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithInternalErr(err).
					WithResponseMsg("invalid order by").
					BuildStatusError().
					WriteGinResponse(r.cfg, gctx)
				val.MarkErrorReturn()
				return
			}

			if !request_log.IsValidOrderByField(field) {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithResponseMsg("invalid order by field").
					BuildStatusError().
					WriteGinResponse(r.cfg, gctx)
				val.MarkErrorReturn()
				return
			}

			b = b.OrderBy(field, ob)
		}

		ex = b
	}

	result := ex.FetchPage(ctx)
	if result.Error != nil {
		api_common.HttpStatusErrorBuilderFromError(result.Error).
			DefaultStatusBadRequest().
			DefaultResponseMsg("failed to list requests").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(200, &ListRequestsResponseJson{
		Items:  auth.FilterForValidatedResources(val, result.Results),
		Cursor: result.Cursor,
		Total:  result.Total,
	})
}

func (r *RequestLogRoutes) Register(g gin.IRouter) {
	g.GET(
		"/request-log/:id",
		r.auth.NewRequiredBuilder().
			ForResource("request-log").
			ForIdField("id").
			ForVerb("get").
			Build(),
		r.get,
	)
	g.GET(
		"/request-log",
		r.auth.NewRequiredBuilder().
			ForResource("request-log").
			ForVerb("list").
			Build(),
		r.list,
	)
}

func NewRequestLogRoutes(
	cfg config.C,
	auth auth.A,
	rl request_log.LogRetriever,
) *RequestLogRoutes {
	return &RequestLogRoutes{
		cfg:  cfg,
		auth: auth,
		rl:   rl,
	}
}
