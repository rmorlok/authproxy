package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"net/http"
)

type Connector struct {
	Id          string `json:"id"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Logo        string `json:"logo"`
}

type GetConnectorRequestPath struct {
	Id string `json:"id"`
}

type ListConnectorsRequest struct {
	Continue string `json:"continue,omitempty"`
}

type ListConnectorsResponse struct {
	Connectors []Connector `json:"connectors"`
	Next       string      `json:"next,omitempty"`
}

type ConnectorsRoutes struct {
	cfg         config.C
	authService *auth.Service
}

func connectorResponseFromConfig(configConn *config.Connector) Connector {
	return Connector{
		Id:          configConn.Id,
		DisplayName: configConn.DisplayName,
		Description: configConn.Description,
		Logo:        configConn.Logo.GetUrl(),
	}
}

func (r *ConnectorsRoutes) get(ctx *gin.Context) {
	var req GetConnectorRequestPath
	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Id == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
	}

	for _, c := range r.cfg.GetRoot().Connectors {
		if c.Id == req.Id {
			ctx.JSON(http.StatusOK, connectorResponseFromConfig(&c))
			return
		}
	}

	ctx.JSON(http.StatusNotFound, gin.H{"error": "connector not found"})
}

func (r *ConnectorsRoutes) list(ctx *gin.Context) {
	var req ListConnectorsRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	connectors := make([]Connector, 0, len(r.cfg.GetRoot().Connectors))
	for _, c := range r.cfg.GetRoot().Connectors {
		connectors = append(connectors, connectorResponseFromConfig(&c))
	}

	ctx.JSON(http.StatusOK, ListConnectorsResponse{
		Connectors: connectors,
	})
}

func (r *ConnectorsRoutes) Register(g *gin.RouterGroup) {
	g.GET("/connectors", r.authService.Required(), r.list)
	g.GET("/connectors/:id", r.authService.Required(), r.get)
}

func NewConnectorsRoutes(cfg config.C, authService *auth.Service) *ConnectorsRoutes {
	return &ConnectorsRoutes{
		cfg:         cfg,
		authService: authService,
	}
}
