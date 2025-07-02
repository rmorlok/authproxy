package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/connectors"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/util"
	"net/http"
)

type ConnectorJson struct {
	Id          uuid.UUID                      `json:"id"`
	Version     uint64                         `json:"version"`
	State       database.ConnectorVersionState `json:"state"`
	Type        string                         `json:"type"`
	DisplayName string                         `json:"display_name"`
	Highlight   string                         `json:"highlight,omitempty"`
	Description string                         `json:"description"`
	Logo        string                         `json:"logo"`

	Versions int64                           `json:"versions,omitempty"`
	States   database.ConnectorVersionStates `json:"states,omitempty"`
}

func ConnectorToJson(c *connectors.Connector) ConnectorJson {
	def := c.GetDefinition()
	logo := ""
	if def.Logo != nil {
		logo = def.Logo.GetUrl()
	}

	return ConnectorJson{
		Id:          c.ID,
		Version:     c.Version,
		State:       c.State,
		Type:        c.Type,
		Highlight:   def.Highlight,
		DisplayName: def.DisplayName,
		Description: def.Description,
		Logo:        logo,

		Versions: c.TotalVersions,
		States:   c.States,
	}
}

type ListConnectorsRequestQueryParams struct {
	Cursor     *string                         `form:"cursor"`
	LimitVal   *int32                          `form:"limit"`
	StateVal   *database.ConnectorVersionState `form:"state"`
	TypeVal    *string                         `form:"type"`
	OrderByVal *string                         `form:"order_by"`
}

type ListConnectorsResponseJson struct {
	Items  []ConnectorJson `json:"items"`
	Cursor string          `json:"cursor,omitempty"`
}

type ConnectorsRoutes struct {
	cfg         config.C
	connectors  connectors.C
	authService auth.A
}

func (r *ConnectorsRoutes) get(gctx *gin.Context) {
	ctx := gctx.Request.Context()

	connectorIdStr := gctx.Param("id")

	if connectorIdStr == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	connectorId, err := uuid.Parse(connectorIdStr)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if connectorId == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
	}

	result := r.connectors.
		ListConnectorsBuilder().
		ForId(connectorId).
		Limit(1).
		FetchPage(ctx)

	if result.Error != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(result.Error).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if len(result.Results) == 0 {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsgf("connector '%s' not found", connectorId).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	c := result.Results[0]
	gctx.PureJSON(http.StatusOK, ConnectorToJson(c))
}

func (r *ConnectorsRoutes) list(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	var req ListConnectorsRequestQueryParams
	if err := gctx.ShouldBindQuery(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg(err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	var err error
	var ex connectors.ListConnectorsExecutor

	if req.Cursor != nil {
		ex, err = r.connectors.ListConnectorsFromCursor(ctx, *req.Cursor)
		if err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusInternalServerError().
				WithInternalErr(err).
				WithResponseMsg("failed to list connectors from cursor").
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			return
		}
	} else {
		b := r.connectors.ListConnectorsBuilder()

		if req.LimitVal != nil {
			b = b.Limit(*req.LimitVal)
		}

		if req.TypeVal != nil {
			b = b.ForType(*req.TypeVal)
		}

		if req.StateVal != nil {
			b = b.ForConnectorVersionState(*req.StateVal)
		}

		if req.OrderByVal != nil {
			field, order, err := database.SplitOrderByParam(*req.OrderByVal)
			if err != nil {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithInternalErr(err).
					WithResponseMsg(err.Error()).
					BuildStatusError().
					WriteGinResponse(r.cfg, gctx)
				return
			}

			if !database.IsValidConnectorOrderByField(field) {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithResponseMsgf("invalid sort field '%s'", field).
					BuildStatusError().
					WriteGinResponse(r.cfg, gctx)
				return
			}

			b.OrderBy(database.ConnectorOrderByField(field), order)
		}

		ex = b
	}

	result := ex.FetchPage(ctx)

	if result.Error != nil {
		gctx.PureJSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
	}

	gctx.PureJSON(http.StatusOK, ListConnectorsResponseJson{
		Items:  util.Map(result.Results, ConnectorToJson),
		Cursor: result.Cursor,
	})
}

func (r *ConnectorsRoutes) Register(g gin.IRouter) {
	g.GET("/connectors", r.authService.Required(), r.list)
	g.GET("/connectors/:id", r.authService.Required(), r.get)
}

func NewConnectorsRoutes(cfg config.C, authService auth.A, c connectors.C) *ConnectorsRoutes {
	return &ConnectorsRoutes{
		cfg:         cfg,
		authService: authService,
		connectors:  c,
	}
}
