package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	connIface "github.com/rmorlok/authproxy/connectors/interface"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/redis"
	"github.com/rmorlok/authproxy/request_log"
	"github.com/rmorlok/authproxy/util/pagination"
)

type RequestLogRoutes struct {
	cfg        config.C
	auth       auth.A
	connectors connIface.C
	db         database.DB
	redis      redis.R
	encrypt    encrypt.E
}

type ListRequestsQuery struct {
	Cursor     *string `form:"cursor"`
	LimitVal   *int32  `form:"limit"`
	OrderByVal *string `json:"order_by"`
}

type ListRequestsResponseJson struct {
	Items  []request_log.EntryRecord `json:"items"`
	Cursor string                    `json:"cursor,omitempty"`
	Total  *int64                    `json:"total,omitempty"`
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

	rl := request_log.NewRetrievalService(r.redis, r.cfg.GetRoot().SystemAuth.GlobalAESKey)

	var ex request_log.ListRequestExecutor

	if req.Cursor != nil {
		ex, err = rl.ListRequestsFromCursor(gctx, *req.Cursor)
		if err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusInternalServerError().
				WithInternalErr(err).
				WithResponseMsg("failed to list requests from cursor").
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			return
		}
	} else {
		b := rl.NewListRequestsBuilder()

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
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(result.Error).
			WithResponseMsg("failed to list requests").
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
	g.GET("/request-log", r.auth.Required(), r.list)
}

func NewRequestLogRoutes(
	cfg config.C,
	auth auth.A,
	connectors connIface.C,
	db database.DB,
	redis redis.R,
	encrypt encrypt.E,
) *RequestLogRoutes {
	return &RequestLogRoutes{
		cfg:        cfg,
		auth:       auth,
		connectors: connectors,
		db:         db,
		redis:      redis,
		encrypt:    encrypt,
	}
}
