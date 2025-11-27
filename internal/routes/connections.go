package routes

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/auth"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/config/common"
	"github.com/rmorlok/authproxy/internal/core"
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

type InitiateConnectionRequest struct {
	// Id of the connector to initiate the connector for
	ConnectorId uuid.UUID `json:"connector_id"`

	// Version of the connector to initiate connection for; if not specified defaults to primary version.
	ConnectorVersion uint64 `json:"connector_version,omitempty"`

	// The namespace to create the connection in. Must be the namespace of connector or a child namespace of that
	// namespace. Defaults to the connector namespace if not specified.
	IntoNamespace string `json:"into_namespace,omitempty"`

	// The URL to return to after connection is completed.
	ReturnToUrl string `json:"return_to_url"`
}

func (icr *InitiateConnectionRequest) Validate() error {
	result := &multierror.Error{}

	if icr.ConnectorId == uuid.Nil {
		result = multierror.Append(result, fmt.Errorf("connector_id is required"))
	}

	if icr.HasIntoNamespace() {
		if err := common.ValidateNamespacePath(icr.IntoNamespace); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result.ErrorOrNil()
}

func (icr *InitiateConnectionRequest) HasVersion() bool {
	return icr.ConnectorVersion > 0
}

func (icr *InitiateConnectionRequest) HasIntoNamespace() bool {
	return icr.IntoNamespace != ""
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
	var cv coreIface.ConnectorVersion
	if req.HasVersion() {
		cv, err = r.core.GetConnectorVersion(ctx, req.ConnectorId, req.ConnectorVersion)
	} else {
		cv, err = r.core.GetConnectorVersionForState(ctx, req.ConnectorId, database.ConnectorVersionStatePrimary)
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

	targetNamespace := cv.GetNamespace()
	if req.HasIntoNamespace() {
		targetNamespace = req.IntoNamespace
	}

	if err := common.ValidateNamespacePath(targetNamespace); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid namespace '%s'", targetNamespace).
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if !common.NamespaceIsSameOrChild(cv.GetNamespace(), targetNamespace) {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("target namespace '%s' is not a child of the connector's namespace '%s'", targetNamespace, cv.GetNamespace()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	_, err = r.core.EnsureNamespaceAncestorPath(ctx, targetNamespace)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	connection := database.Connection{
		ID:               uuid.New(),
		Namespace:        targetNamespace,
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
	if _, ok := connector.Auth.Inner().(*config.AuthOAuth2); ok {
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

func DatabaseConnectionToJson(cv coreIface.ConnectorVersion, conn database.Connection) ConnectionJson {
	connector := ConnectorJson{
		Id:          conn.ConnectorId,
		DisplayName: "Unknown",
		Type:        "unknown",
		Description: "Unknown connector",
	}

	if cv != nil {
		connector = ConnectorVersionToConnectorJson(cv)
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
			b = b.ForState(*req.StateVal)
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

	connectorVersions, err := r.core.GetConnectorVersions(ctx, core.GetConnectorVersionIdsForConnections(result.Results))
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
				connectorVersions[coreIface.ConnectorVersionId{c.ConnectorId, c.ConnectorVersion}],
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

	cv, err := r.core.GetConnectorVersion(ctx, c.ConnectorId, c.ConnectorVersion)
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

	cv, err := r.core.GetConnectorVersion(ctx, c.ConnectorId, c.ConnectorVersion)
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

	c, err := r.db.GetConnection(ctx, id)
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

	cv, err := r.core.GetConnectorVersion(ctx, c.ConnectorId, c.ConnectorVersion)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if c.State == req.State {
		gctx.PureJSON(http.StatusOK, DatabaseConnectionToJson(cv, *c))
		return
	}

	err = r.db.SetConnectionState(ctx, id, req.State)
	if err != nil {
		api_common.HttpStatusErrorBuilderFromError(err).
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	c, err = r.db.GetConnection(ctx, id)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, DatabaseConnectionToJson(cv, *c))
}

func (r *ConnectionsRoutes) Register(g gin.IRouter) {
	g.POST("/connections/_initiate", r.auth.Required(), r.initiate)
	g.GET("/connections", r.auth.Required(), r.list)
	g.GET("/connections/:id", r.auth.Required(), r.get)
	g.POST("/connections/:id/_disconnect", r.auth.Required(), r.disconnect)
	g.PUT("/connections/:id/_force_state", r.auth.Required(), r.forceState)
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
