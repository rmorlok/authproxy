package routes

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/oauth2"
	"github.com/rmorlok/authproxy/redis"
	"github.com/rmorlok/authproxy/util"
	"net/http"
	"time"
)

type ConnectionsRoutes struct {
	cfg         config.C
	authService auth.A
	db          database.DB
	redis       *redis.Wrapper
}

type InitiateConnectionRequest struct {
	ConnectorId string `json:"connector_id"`
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
	ctx := context.AsContext(gctx.Request.Context())

	actor := auth.GetActorInfoFromGinContext(gctx)
	if actor == nil {
		gctx.JSON(http.StatusUnauthorized, Error{"unauthorized"})
		return
	}

	var req InitiateConnectionRequest
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		gctx.JSON(http.StatusBadRequest, Error{err.Error()})
		return
	}

	var connector config.Connector
	found := false
	for _, c := range r.cfg.GetRoot().Connectors {
		if c.Id == req.ConnectorId {
			connector = c
			found = true
		}
	}

	if !found {
		gctx.JSON(http.StatusBadRequest, Error{fmt.Sprintf("connector '%s' not found", req.ConnectorId)})
		return
	}

	connection := database.Connection{
		ID:    uuid.New(),
		State: database.ConnectionStateCreated,
	}

	err := r.db.CreateConnection(ctx, &connection)
	if err != nil {
		gctx.JSON(http.StatusInternalServerError, Error{err.Error()})
	}

	if _, ok := connector.Auth.(*config.AuthOAuth2); ok {
		oAuth2 := oauth2.NewOAuth2(r.cfg, r.db, r.redis, connection, connector)
		url, err := oAuth2.SetStateAndGeneratePublicUrl(ctx, *actor)
		if err != nil {
			gctx.JSON(http.StatusInternalServerError, Error{err.Error()})
			return
		}

		gctx.JSON(http.StatusOK, InitiateConnectionRedirect{
			InitiateConnectionResponse: InitiateConnectionResponse{
				Id:   connection.ID,
				Type: PreconnectionResponseTypeRedirect,
			},
			RedirectUrl: url,
		})
		return
	}

	gctx.JSON(http.StatusInternalServerError, Error{"unsupported connector auth type"})
}

type ConnectionJson struct {
	ID        uuid.UUID                `json:"id"`
	State     database.ConnectionState `json:"state"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
}

func DatabaseConnectionToJson(conn database.Connection) ConnectionJson {
	return ConnectionJson{
		ID:        conn.ID,
		State:     conn.State,
		CreatedAt: conn.CreatedAt,
		UpdatedAt: conn.UpdatedAt,
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
	ctx := context.AsContext(gctx.Request.Context())

	var req ListConnectionRequestQuery
	var err error

	if err = gctx.ShouldBindQuery(&req); err != nil {
		gctx.JSON(http.StatusBadRequest, Error{err.Error()})
		return
	}

	var ex database.ListConnectionsExecutor

	if req.Cursor != nil {
		ex, err = r.db.ListConnectionsFromCursor(ctx, *req.Cursor)
		if err != nil {
			gctx.JSON(http.StatusBadRequest, Error{err.Error()})
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
				gctx.JSON(http.StatusBadRequest, Error{err.Error()})
				return
			}

			if field != string(database.ConnectionOrderByCreatedAt) {
				gctx.JSON(http.StatusBadRequest, Error{fmt.Sprintf("invalid sort field '%s'", field)})
				return
			}

			b.OrderBy(database.ConnectionOrderByField(field), order)
		}

		ex = b
	}

	result := ex.FetchPage(ctx)

	if result.Error != nil {
		gctx.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
	}

	gctx.JSON(http.StatusOK, ListConnectionResponseJson{
		Items:  util.Map(result.Results, DatabaseConnectionToJson),
		Cursor: result.Cursor,
	})
}

func (r *ConnectionsRoutes) get(gctx *gin.Context) {
	ctx := context.AsContext(gctx.Request.Context())
	id, err := uuid.Parse(gctx.Param("id"))
	if err != nil {
		gctx.JSON(http.StatusBadRequest, Error{err.Error()})
		return
	}

	if id == uuid.Nil {
		gctx.JSON(http.StatusBadRequest, Error{"id is required"})
	}

	c, err := r.db.GetConnection(ctx, id)
	if err != nil {
		gctx.JSON(http.StatusBadRequest, Error{err.Error()})
		return
	}

	if c == nil {
		gctx.JSON(http.StatusNotFound, Error{"connection not found"})
		return
	}

	gctx.JSON(http.StatusOK, DatabaseConnectionToJson(*c))
}

func (r *ConnectionsRoutes) Register(g gin.IRouter) {
	g.POST("/connections/_initiate", r.authService.Required(), r.initiate)
	g.GET("/connections", r.authService.Required(), r.list)
	g.GET("/connections/:id", r.authService.Required(), r.get)
}

func NewConnectionsRoutes(
	cfg config.C,
	authService auth.A,
	db database.DB,
	redis *redis.Wrapper,
) *ConnectionsRoutes {
	return &ConnectionsRoutes{
		cfg:         cfg,
		authService: authService,
		db:          db,
		redis:       redis,
	}
}
