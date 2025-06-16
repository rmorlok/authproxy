package routes

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/connectors"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/oauth2"
	"github.com/rmorlok/authproxy/redis"
	"github.com/rmorlok/authproxy/util"
	"log/slog"
	"net/http"
	"time"
)

type ConnectionsRoutes struct {
	cfg        config.C
	auth       auth.A
	connectors connectors.C
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
		gctx.PureJSON(http.StatusUnauthorized, Error{"unauthorized"})
		return
	}

	var req InitiateConnectionRequest
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		gctx.PureJSON(http.StatusBadRequest, Error{err.Error()})
		return
	}

	if err := req.Validate(); err != nil {
		gctx.PureJSON(http.StatusBadRequest, Error{err.Error()})
		return
	}

	var err error
	var cv *connectors.ConnectorVersion
	if req.HasVersion() {
		cv, err = r.connectors.GetConnectorVersion(ctx, req.ConnectorId, req.ConnectorVersion)
	} else {
		cv, err = r.connectors.GetConnectorVersionForState(ctx, req.ConnectorId, database.ConnectorVersionStatePrimary)
	}

	if err != nil {
		gctx.PureJSON(http.StatusInternalServerError, Error{err.Error()})
		return
	}

	if cv == nil {
		gctx.PureJSON(http.StatusBadRequest, Error{fmt.Sprintf("connector '%s' not found", req.ConnectorId)})
		return
	}

	connection := database.Connection{
		ID:               uuid.New(),
		ConnectorId:      cv.ID,
		ConnectorVersion: cv.Version,
		State:            database.ConnectionStateCreated,
	}

	err = r.db.CreateConnection(ctx, &connection)
	if err != nil {
		gctx.PureJSON(http.StatusInternalServerError, Error{err.Error()})
	}

	connector := cv.GetDefinition()
	if _, ok := connector.Auth.(*config.AuthOAuth2); ok {
		if req.ReturnToUrl == "" {
			gctx.PureJSON(http.StatusBadRequest, Error{"must specify return_to_url"})
			return
		}

		o2 := r.oauthf.NewOAuth2(connection, cv)
		url, err := o2.SetStateAndGeneratePublicUrl(ctx, ra.MustGetActor(), req.ReturnToUrl)
		if err != nil {
			gctx.PureJSON(http.StatusInternalServerError, Error{err.Error()})
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

	gctx.PureJSON(http.StatusInternalServerError, Error{"unsupported connector auth type"})
}

type ConnectionJson struct {
	ID          uuid.UUID                `json:"id"`
	State       database.ConnectionState `json:"state"`
	ConnectorId uuid.UUID                `json:"connector_id"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
}

func DatabaseConnectionToJson(conn database.Connection) ConnectionJson {
	return ConnectionJson{
		ID:          conn.ID,
		State:       conn.State,
		ConnectorId: conn.ConnectorId,
		CreatedAt:   conn.CreatedAt,
		UpdatedAt:   conn.UpdatedAt,
	}
}

type ListConnectionRequestQuery struct {
	Cursor     *string                   `form:"cursor"`
	LimitVal   *int32                    `form:"limit"`
	StateVal   *database.ConnectionState `form:"state"`
	OrderByVal *string                   `json:"order_by"`
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
		gctx.PureJSON(http.StatusBadRequest, Error{err.Error()})
		return
	}

	var ex database.ListConnectionsExecutor

	if req.Cursor != nil {
		ex, err = r.db.ListConnectionsFromCursor(ctx, *req.Cursor)
		if err != nil {
			gctx.PureJSON(http.StatusBadRequest, Error{err.Error()})
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
			field, order, err := database.SplitOrderByParam(*req.OrderByVal)
			if err != nil {
				gctx.PureJSON(http.StatusBadRequest, Error{err.Error()})
				return
			}

			if field != string(database.ConnectionOrderByCreatedAt) {
				gctx.PureJSON(http.StatusBadRequest, Error{fmt.Sprintf("invalid sort field '%s'", field)})
				return
			}

			b.OrderBy(database.ConnectionOrderByField(field), order)
		}

		ex = b
	}

	result := ex.FetchPage(ctx)

	if result.Error != nil {
		gctx.PureJSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
	}

	gctx.PureJSON(http.StatusOK, ListConnectionResponseJson{
		Items:  util.Map(result.Results, DatabaseConnectionToJson),
		Cursor: result.Cursor,
	})
}

func (r *ConnectionsRoutes) get(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	id, err := uuid.Parse(gctx.Param("id"))
	if err != nil {
		gctx.PureJSON(http.StatusBadRequest, Error{err.Error()})
		return
	}

	if id == uuid.Nil {
		gctx.PureJSON(http.StatusBadRequest, Error{"id is required"})
	}

	c, err := r.db.GetConnection(ctx, id)
	if err != nil {
		gctx.PureJSON(http.StatusBadRequest, Error{err.Error()})
		return
	}

	if c == nil {
		gctx.PureJSON(http.StatusNotFound, Error{"connection not found"})
		return
	}

	// TODO: add security checking for ownership

	gctx.PureJSON(http.StatusOK, DatabaseConnectionToJson(*c))
}

func (r *ConnectionsRoutes) Register(g gin.IRouter) {
	g.POST("/connections/_initiate", r.auth.Required(), r.initiate)
	g.GET("/connections", r.auth.Required(), r.list)
	g.GET("/connections/:id", r.auth.Required(), r.get)
	g.POST("/connections/:id/_proxy", r.auth.Required(), r.proxy)
}

func NewConnectionsRoutes(
	cfg config.C,
	authService auth.A,
	db database.DB,
	redis redis.R,
	c connectors.C,
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
