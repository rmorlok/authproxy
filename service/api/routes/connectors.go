package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"net/http"
)

type ConnectorJson struct {
	Id          string `json:"id"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Logo        string `json:"logo"`
}

type GetConnectorRequestPath struct {
	Id string `uri:"id"`
}

type ListConnectorsRequestQueryParams struct {
	Continue string `form:"continue,omitempty"`
}

type ListConnectorsResponse struct {
	Items  []ConnectorJson `json:"items"`
	Cursor string          `json:"cursor,omitempty"`
}

type ConnectorsRoutes struct {
	cfg         config.C
	authService *auth.Service
}

func connectorResponseFromConfig(cfg config.C, configConn *config.Connector) ConnectorJson {
	logo := cfg.GetFallbackConnectorLogo()
	if configConn.Logo != nil {
		logo = configConn.Logo.GetUrl()
	}

	return ConnectorJson{
		Id:          configConn.Id,
		DisplayName: configConn.DisplayName,
		Description: configConn.Description,
		Logo:        logo,
	}
}

func (r *ConnectorsRoutes) get(ctx *gin.Context) {
	var req GetConnectorRequestPath
	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, Error{err.Error()})
		return
	}

	if req.Id == "" {
		ctx.JSON(http.StatusBadRequest, Error{"id is required"})
	}

	for _, c := range r.cfg.GetRoot().Connectors {
		if c.Id == req.Id {
			ctx.JSON(http.StatusOK, connectorResponseFromConfig(r.cfg, &c))
			return
		}
	}

	ctx.JSON(http.StatusNotFound, Error{"connector not found"})
}

func (r *ConnectorsRoutes) list(ctx *gin.Context) {
	var req ListConnectorsRequestQueryParams
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, Error{err.Error()})
		return
	}

	connectors := make([]ConnectorJson, 0, len(r.cfg.GetRoot().Connectors))
	for _, c := range r.cfg.GetRoot().Connectors {
		connectors = append(connectors, connectorResponseFromConfig(r.cfg, &c))
	}

	ctx.JSON(http.StatusOK, ListConnectorsResponse{
		Items: connectors,
	})
}

func (r *ConnectorsRoutes) Register(g gin.IRouter) {
	g.GET("/connectors", r.authService.Required(), r.list)
	g.GET("/connectors/:id", r.authService.Required(), r.get)
}

func NewConnectorsRoutes(cfg config.C, authService *auth.Service) *ConnectorsRoutes {
	return &ConnectorsRoutes{
		cfg:         cfg,
		authService: authService,
	}
}
