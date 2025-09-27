package routes

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/connectors"
	connIface "github.com/rmorlok/authproxy/connectors/interface"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/oauth2"
	"github.com/rmorlok/authproxy/redis"
	"github.com/rmorlok/authproxy/util"
	"github.com/rmorlok/authproxy/util/pagination"

	"log/slog"
	"net/http"
	"time"
)

type ConnectionsRoutes struct {
	cfg        config.C
	auth       auth.A
	connectors connIface.C
	db         database.DB
	redis      redis.R
	httpf      httpf.F
	encrypt    encrypt.E
	oauthf     oauth2.Factory
}

type InitiateConnectionRequest struct {
	// Id of the connector to initiate the connector for
	ConnectorId uuid.UUID `json:"connector_id"`

	// Version of the connector to initiate connection for; if not specified defaults to primary version.
	ConnectorVersion uint64 `json:"connector_version,omitempty"`

	// The URL to return to after connection is completed.
	ReturnToUrl string `json:"return_to_url"`
}

func (icr *InitiateConnectionRequest) Validate() error {
	if icr.ConnectorId == uuid.Nil {
		return fmt.Errorf("connector_id is required")
	}

	return nil
}

func (icr *InitiateConnectionRequest) HasVersion() bool {
	return icr.ConnectorVersion > 0
}

type InitiateConnectionResponseType string

const (
	PreconnectionResponseTypeRedirect InitiateConnectionResponseType = "redirect"
)

type InitiateConnectionResponse struct {
	Id   uuid.UUID                      `json:"id"`
	Type InitiateConnectionResponseType `json:"type"`
}

type InitiateConnectionRedirect struct {
	// Type must be PreconnectionResponseTypeRedirect
	InitiateConnectionResponse
	RedirectUrl string `json:"redirect_url"`
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

	var req InitiateConnectionRequest
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if err := req.Validate(); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	var err error
	var cv connIface.ConnectorVersion
	if req.HasVersion() {
		cv, err = r.connectors.GetConnectorVersion(ctx, req.ConnectorId, req.ConnectorVersion)
	} else {
		cv, err = r.connectors.GetConnectorVersionForState(ctx, req.ConnectorId, database.ConnectorVersionStatePrimary)
	}

	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if cv == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("connector '%s' not found", req.ConnectorId).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	connection := database.Connection{
		ID:               uuid.New(),
		ConnectorId:      cv.GetID(),
		ConnectorVersion: cv.GetVersion(),
		State:            database.ConnectionStateCreated,
	}

	err = r.db.CreateConnection(ctx, &connection)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
	}

	connector := cv.GetDefinition()
	if _, ok := connector.Auth.(*config.AuthOAuth2); ok {
		if req.ReturnToUrl == "" {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithResponseMsg("must specify return_to_url").
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			return
		}

		o2 := r.oauthf.NewOAuth2(connection, cv)
		url, err := o2.SetStateAndGeneratePublicUrl(ctx, ra.MustGetActor(), req.ReturnToUrl)
		if err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusInternalServerError().
				WithInternalErr(err).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			return
		}

		gctx.PureJSON(http.StatusOK, InitiateConnectionRedirect{
			InitiateConnectionResponse: InitiateConnectionResponse{
				Id:   connection.ID,
				Type: PreconnectionResponseTypeRedirect,
			},
			RedirectUrl: url,
		})
		return
	}

	api_common.NewHttpStatusErrorBuilder().
		WithStatusInternalServerError().
		WithResponseMsg("unsupported connector auth type").
		BuildStatusError().
		WriteGinResponse(r.cfg, gctx)
}

type ConnectionJson struct {
	ID        uuid.UUID                `json:"id"`
	State     database.ConnectionState `json:"state"`
	Connector ConnectorJson            `json:"connector"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
}

func DatabaseConnectionToJson(cv connIface.ConnectorVersion, conn database.Connection) ConnectionJson {
	connector := ConnectorJson{
		Id:          conn.ConnectorId,
		DisplayName: "Unknown",
		Type:        "unknown",
		Description: "Unknown connector",
	}

	if cv != nil {
		connector = ConnectorVersionToJson(cv)
	}

	return ConnectionJson{
		ID:        conn.ID,
		State:     conn.State,
		Connector: connector,
		CreatedAt: conn.CreatedAt,
		UpdatedAt: conn.UpdatedAt,
	}
}

type ListConnectionRequestQuery struct {
	Cursor     *string                   `form:"cursor"`
	LimitVal   *int32                    `form:"limit"`
	StateVal   *database.ConnectionState `form:"state"`
	OrderByVal *string                   `form:"order_by"`
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

	var ex database.ListConnectionsExecutor

	if req.Cursor != nil {
		ex, err = r.db.ListConnectionsFromCursor(ctx, *req.Cursor)
		if err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithInternalErr(err).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			return
		}
	} else {
		b := r.db.ListConnectionsBuilder()

		if req.LimitVal != nil {
			b = b.Limit(*req.LimitVal)
		}

		if req.StateVal != nil {
			b = b.ForConnectionState(*req.StateVal)
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

			b.OrderBy(database.ConnectionOrderByField(field), order)
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

	connectorVersions, err := r.connectors.GetConnectorVersions(ctx, connectors.GetConnectorVersionIdsForConnections(result.Results))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, ListConnectionResponseJson{
		Items: util.Map(result.Results, func(c database.Connection) ConnectionJson {
			return DatabaseConnectionToJson(
				connectorVersions[connIface.ConnectorVersionId{c.ConnectorId, c.ConnectorVersion}],
				c,
			)
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

	c, err := r.db.GetConnection(ctx, id)
	if err != nil {
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

	cv, err := r.connectors.GetConnectorVersion(ctx, c.ConnectorId, c.ConnectorVersion)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	// TODO: add security checking for ownership

	gctx.PureJSON(http.StatusOK, DatabaseConnectionToJson(cv, *c))
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

	c, err := r.db.GetConnection(ctx, id)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	cv, err := r.connectors.GetConnectorVersion(ctx, c.ConnectorId, c.ConnectorVersion)
	if err != nil {
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

	ti, err := r.connectors.DisconnectConnection(ctx, id)
	if err != nil {
		api_common.HttpStatusErrorBuilderFromError(err).
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	taskId, err := ti.
		BindToActor(util.ToPtr(ra.MustGetActor())).
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
	c.State = database.ConnectionStateDisconnecting

	response := DisconnectResponseJson{
		TaskId:     taskId,
		Connection: DatabaseConnectionToJson(cv, *c),
	}

	gctx.PureJSON(http.StatusOK, response)
}

func (r *ConnectionsRoutes) Register(g gin.IRouter) {
	g.POST("/connections/_initiate", r.auth.Required(), r.initiate)
	g.GET("/connections", r.auth.Required(), r.list)
	g.GET("/connections/:id", r.auth.Required(), r.get)
	g.POST("/connections/:id/_disconnect", r.auth.Required(), r.disconnect)
}

func NewConnectionsRoutes(
	cfg config.C,
	authService auth.A,
	db database.DB,
	redis redis.R,
	c connIface.C,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
) *ConnectionsRoutes {
	return &ConnectionsRoutes{
		cfg:        cfg,
		auth:       authService,
		connectors: c,
		db:         db,
		redis:      redis,
		httpf:      httpf,
		encrypt:    encrypt,
		oauthf:     oauth2.NewFactory(cfg, db, redis, c, httpf, encrypt, logger),
	}
}
