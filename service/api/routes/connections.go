package routes

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/util"
	"net/http"
	"time"
)

type ConnectionsRoutes struct {
	cfg         config.C
	authService *auth.Service
	db          database.DB
}

type InitiateConnectionRequest struct {
	ConnectorId string `json:"connectors"`
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
	var req InitiateConnectionRequest
	if err := gctx.ShouldBindQuery(&req); err != nil {
		gctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		gctx.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("connector '%s' not found", req.ConnectorId)})
		return
	}

	connection := database.Connection{
		ID:    uuid.New(),
		State: database.ConnectionStateCreated,
	}

	err := r.db.CreateConnection(ctx, &connection)
	if err != nil {
		gctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}

	if oAuth2, ok := connector.Auth.(*config.AuthOAuth2); ok {
		gctx.JSON(http.StatusOK, InitiateConnectionRedirect{
			InitiateConnectionResponse: InitiateConnectionResponse{
				Id:   connection.ID,
				Type: PreconnectionResponseTypeRedirect,
			},
			RedirectUrl: oAuth2.AuthorizationEndpoint,
		})
		return
	}

	gctx.JSON(http.StatusInternalServerError, gin.H{"error": "unsupported connector auth type"})
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
		gctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var ex database.ListConnectionsExecutor

	if req.Cursor != nil {
		ex, err = r.db.ListConnectionsFromCursor(ctx, *req.Cursor)
		if err != nil {
			gctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
				gctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			if field != string(database.ConnectionOrderByCreatedAt) {
				gctx.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid sort field '%s'", field)})
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

type GetConnectionRequestPath struct {
	Id uuid.UUID `uri:"id"`
}

func (r *ConnectionsRoutes) get(gctx *gin.Context) {
	ctx := context.AsContext(gctx.Request.Context())
	var req GetConnectionRequestPath
	if err := gctx.ShouldBindUri(&req); err != nil {
		gctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Id == uuid.Nil {
		gctx.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
	}

	c, err := r.db.GetConnection(ctx, req.Id)
	if err != nil {
		gctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}

	gctx.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
}

func (r *ConnectionsRoutes) Register(g *gin.RouterGroup) {
	g.POST("/connections/initiate", r.authService.Required(), r.initiate)
	g.GET("/connections", r.authService.Required(), r.list)
	g.GET("/connections/:id", r.authService.Required(), r.get)
}

func NewConnectionsRoutes(cfg config.C, authService *auth.Service, db database.DB) *ConnectionsRoutes {
	return &ConnectionsRoutes{
		cfg:         cfg,
		authService: authService,
		db:          db,
	}
}
