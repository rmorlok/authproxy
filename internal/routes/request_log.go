package routes

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/auth"
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
	Items  []request_log.EntryRecord `json:"items"`
	Cursor string                    `json:"cursor,omitempty"`
	Total  *int64                    `json:"total,omitempty"`
}

func (r *RequestLogRoutes) get(gctx *gin.Context) {
	ctx := gctx.Request.Context()

	logIdStr := gctx.Param("id")

	if logIdStr == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	logId, err := uuid.Parse(logIdStr)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if logId == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
	}

	entry, err := r.rl.GetFullLog(ctx, logId)

	if err != nil {
		if errors.Is(err, request_log.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("request log not found").
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, entry)
}

func (r *RequestLogRoutes) list(gctx *gin.Context) {
	ctx := gctx.Request.Context()

	var req ListRequestsQuery
	var err error

	if err = gctx.ShouldBindQuery(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg(err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
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
			return
		}
	} else {
		b := r.rl.NewListRequestsBuilder()

		b, err = req.ApplyToBuilder(b)
		if err != nil {
			api_common.HttpStatusErrorBuilderFromError(err).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
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
		return
	}

	gctx.PureJSON(200, &ListRequestsResponseJson{
		Items:  result.Results,
		Cursor: result.Cursor,
		Total:  result.Total,
	})
}

func (r *RequestLogRoutes) Register(g gin.IRouter) {
	g.GET("/request-log/:id", r.auth.Required(), r.get)
	g.GET("/request-log", r.auth.Required(), r.list)
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
