package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/config"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"

	"log/slog"
	"net/http"
	"time"
)

type ConnectionsRoutes struct {
	cfg     config.C
	auth    auth.A
	core    coreIface.C
	db      database.DB
	r       apredis.Client
	httpf   httpf.F
	encrypt encrypt.E
	oauthf  oauth2.Factory
}

func (r *ConnectionsRoutes) initiate(gctx *gin.Context) {
	ctx := gctx.Request.Context()

	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusUnauthorized().
			WithResponseMsg("unauthorized").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	var req coreIface.InitiateConnectionRequest
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	resp, err := r.core.InitiateConnection(ctx, req)
	if err != nil {
		api_common.HttpStatusErrorBuilderFromError(err).
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, resp)
}

type ConnectionJson struct {
	Id        uuid.UUID                `json:"id"`
	Namespace string                   `json:"namespace"`
	State     database.ConnectionState `json:"state"`
	Connector ConnectorJson            `json:"connector"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
}

func ConnectionToJson(conn coreIface.Connection) ConnectionJson {
	connector := ConnectorVersionToConnectorJson(conn.GetConnectorVersionEntity())

	return ConnectionJson{
		Id:        conn.GetId(),
		Namespace: conn.GetNamespace(),
		State:     conn.GetState(),
		Connector: connector,
		CreatedAt: conn.GetCreatedAt(),
		UpdatedAt: conn.GetUpdatedAt(),
	}
}

type ListConnectionRequestQuery struct {
	Cursor       *string                   `form:"cursor"`
	LimitVal     *int32                    `form:"limit"`
	StateVal     *database.ConnectionState `form:"state"`
	NamespaceVal *string                   `form:"namespace"`
	OrderByVal   *string                   `form:"order_by"`
}

type ListConnectionResponseJson struct {
	Items  []ConnectionJson `json:"items"`
	Cursor string           `json:"cursor,omitempty"`
}

func (r *ConnectionsRoutes) list(gctx *gin.Context) {
	ctx := gctx.Request.Context()

	var req ListConnectionRequestQuery
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

	var ex coreIface.ListConnectionsExecutor

	if req.Cursor != nil {
		ex, err = r.core.ListConnectionsFromCursor(ctx, *req.Cursor)
		if err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithInternalErr(err).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			return
		}
	} else {
		b := r.core.ListConnectionsBuilder()

		if req.LimitVal != nil {
			b = b.Limit(*req.LimitVal)
		}

		if req.StateVal != nil {
			b = b.ForState(*req.StateVal)
		}

		if req.NamespaceVal != nil {
			b = b.ForNamespaceMatcher(*req.NamespaceVal)
		}

		if req.OrderByVal != nil {
			field, order, err := pagination.SplitOrderByParam[database.ConnectionOrderByField](*req.OrderByVal)
			if err != nil {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithInternalErr(err).
					WithResponseMsg(err.Error()).
					BuildStatusError().
					WriteGinResponse(r.cfg, gctx)
				return
			}

			if !database.IsValidConnectionOrderByField(field) {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithResponseMsgf("invalid sort field '%s'", field).
					BuildStatusError().
					WriteGinResponse(r.cfg, gctx)
				return
			}

			b.OrderBy(field, order)
		}

		ex = b
	}

	result := ex.FetchPage(ctx)

	if result.Error != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(result.Error).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, ListConnectionResponseJson{
		Items: util.Map(result.Results, func(c coreIface.Connection) ConnectionJson {
			return ConnectionToJson(c)
		}),
		Cursor: result.Cursor,
	})
}

func (r *ConnectionsRoutes) get(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	id, err := uuid.Parse(gctx.Param("id"))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if id == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("connection not found").
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

	if c == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg("connection not found").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, ConnectionToJson(c))
}

type DisconnectResponseJson struct {
	TaskId     string         `json:"task_id"`
	Connection ConnectionJson `json:"connection"`
}

func (r *ConnectionsRoutes) disconnect(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	id, err := uuid.Parse(gctx.Param("id"))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if id == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
	}

	// The Auth check is duplicative of the annotation for the route, but being done since are pulling auth anyway.
	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusUnauthorized().
			WithResponseMsg("unauthorized").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	ti, err := r.core.DisconnectConnection(ctx, id)
	if err != nil {
		api_common.HttpStatusErrorBuilderFromError(err).
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	taskId, err := ti.
		BindToActor(ra.MustGetActor()).
		ToSecureEncryptedString(ctx, r.encrypt)

	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	// Hard code the disconnecting state to avoid race condictions with task workers
	connJson := ConnectionToJson(c)
	connJson.State = database.ConnectionStateDisconnecting

	response := DisconnectResponseJson{
		TaskId:     taskId,
		Connection: connJson,
	}

	gctx.PureJSON(http.StatusOK, response)
}

type ForceStateRequestJson struct {
	State database.ConnectionState `json:"state"`
}

func (r *ConnectionsRoutes) forceState(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	id, err := uuid.Parse(gctx.Param("id"))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if id == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
	}

	// The Auth check is duplicative of the annotation for the route, but being done since are pulling auth anyway.
	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusUnauthorized().
			WithResponseMsg("unauthorized").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	req := ForceStateRequestJson{}
	err = gctx.BindJSON(&req)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if req.State == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("state is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("connection not found").
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

	if c.GetState() == req.State {
		gctx.PureJSON(http.StatusOK, ConnectionToJson(c))
		return
	}

	err = c.SetState(ctx, req.State)
	if err != nil {
		api_common.HttpStatusErrorBuilderFromError(err).
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, ConnectionToJson(c))
}

func (r *ConnectionsRoutes) Register(g gin.IRouter) {
	g.POST(
		"/connections/_initiate",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("create").
			Build(),
		r.initiate,
	)
	g.GET(
		"/connections",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("list").
			Build(),
		r.list,
	)
	g.GET(
		"/connections/:id",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("get").
			ForIdField("id").
			Build(),
		r.get,
	)
	g.POST(
		"/connections/:id/_disconnect",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("disconnect").
			ForIdField("id").
			Build(),
		r.disconnect,
	)
	g.PUT(
		"/connections/:id/_force_state",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("force_state").
			ForIdField("id").
			Build(),
		r.forceState,
	)
}

func NewConnectionsRoutes(
	cfg config.C,
	authService auth.A,
	db database.DB,
	r apredis.Client,
	c coreIface.C,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
) *ConnectionsRoutes {
	return &ConnectionsRoutes{
		cfg:     cfg,
		auth:    authService,
		core:    c,
		db:      db,
		r:       r,
		httpf:   httpf,
		encrypt: encrypt,
		oauthf:  oauth2.NewFactory(cfg, db, r, c, httpf, encrypt, logger),
	}
}
