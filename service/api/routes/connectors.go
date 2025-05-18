package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"net/http"
)

type ConnectorJson struct {
	Id          uuid.UUID `json:"id"`
	Type        string    `json:"type"`
	DisplayName string    `json:"display_name"`
	Description string    `json:"description"`
	Logo        string    `json:"logo"`
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
	authService auth.A
}

func connectorResponseFromConfig(cfg config.C, configConn *config.Connector) ConnectorJson {
	logo := cfg.GetFallbackConnectorLogo()
	if configConn.Logo != nil {
		logo = configConn.Logo.GetUrl()
	}

	return ConnectorJson{
		Id:          configConn.Id,
		Type:        configConn.Type,
		DisplayName: configConn.DisplayName,
		Description: configConn.Description,
		Logo:        logo,
	}
}

func (r *ConnectorsRoutes) get(ctx *gin.Context) {
	connectorIdStr := ctx.Param("id")

	if connectorIdStr == "" {
		ctx.PureJSON(http.StatusBadRequest, Error{"id is required"})
		return
	}

	connectorId, err := uuid.Parse(connectorIdStr)
	if err != nil {
		ctx.PureJSON(http.StatusBadRequest, Error{errors.Wrap(err, "failed to parse connector id").Error()})
		return
	}

	if connectorId == uuid.Nil {
		ctx.PureJSON(http.StatusBadRequest, Error{"id is required"})
	}

	for _, c := range r.cfg.GetRoot().Connectors {
		if c.Id == connectorId {
			ctx.PureJSON(http.StatusOK, connectorResponseFromConfig(r.cfg, &c))
			return
		}
	}

	ctx.PureJSON(http.StatusNotFound, Error{"connector not found"})
}

func (r *ConnectorsRoutes) list(ctx *gin.Context) {
	var req ListConnectorsRequestQueryParams
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.PureJSON(http.StatusBadRequest, Error{err.Error()})
		return
	}

	connectors := make([]ConnectorJson, 0, len(r.cfg.GetRoot().Connectors))
	for _, c := range r.cfg.GetRoot().Connectors {
		connectors = append(connectors, connectorResponseFromConfig(r.cfg, &c))
	}

	ctx.PureJSON(http.StatusOK, ListConnectorsResponse{
		Items: connectors,
	})
}

func (r *ConnectorsRoutes) Register(g gin.IRouter) {
	g.GET("/connectors", r.authService.Required(), r.list)
	g.GET("/connectors/:id", r.authService.Required(), r.get)
}

func NewConnectorsRoutes(cfg config.C, authService auth.A) *ConnectorsRoutes {
	return &ConnectorsRoutes{
		cfg:         cfg,
		authService: authService,
	}
}
