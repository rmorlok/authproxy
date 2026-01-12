package routes

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/config"
	connIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"

	"net/http"
)

type ConnectorJson struct {
	Id          uuid.UUID                      `json:"id"`
	Version     uint64                         `json:"version"`
	Namespace   string                         `json:"namespace"`
	State       database.ConnectorVersionState `json:"state"`
	Type        string                         `json:"type"`
	DisplayName string                         `json:"display_name"`
	Highlight   string                         `json:"highlight,omitempty"`
	Description string                         `json:"description"`
	Logo        string                         `json:"logo"`
	CreatedAt   time.Time                      `json:"created_at"`
	UpdatedAt   time.Time                      `json:"updated_at"`

	Versions int64                           `json:"versions,omitempty"`
	States   database.ConnectorVersionStates `json:"states,omitempty"`
}

func ConnectorToJson(c connIface.Connector) ConnectorJson {
	result := ConnectorVersionToConnectorJson(c)
	result.Versions = c.GetTotalVersions()
	result.States = c.GetStates()
	return result
}

func ConnectorVersionToConnectorJson(cv connIface.ConnectorVersion) ConnectorJson {
	def := cv.GetDefinition()
	logo := ""
	if def.Logo != nil {
		logo = def.Logo.GetUrl()
	}

	return ConnectorJson{
		Id:          cv.GetId(),
		Version:     cv.GetVersion(),
		Namespace:   cv.GetNamespace(),
		State:       cv.GetState(),
		Type:        cv.GetType(),
		Highlight:   def.Highlight,
		DisplayName: def.DisplayName,
		Description: def.Description,
		Logo:        logo,
		CreatedAt:   cv.GetCreatedAt(),
		UpdatedAt:   cv.GetUpdatedAt(),
	}
}

type ListConnectorsRequestQueryParams struct {
	Cursor       *string                         `form:"cursor"`
	LimitVal     *int32                          `form:"limit"`
	StateVal     *database.ConnectorVersionState `form:"state"`
	NamespaceVal *string                         `form:"namespace"`
	TypeVal      *string                         `form:"type"`
	OrderByVal   *string                         `form:"order_by"`
}

type ListConnectorsResponseJson struct {
	Items  []ConnectorJson `json:"items"`
	Cursor string          `json:"cursor,omitempty"`
}

type ConnectorVersionJson struct {
	Id         uuid.UUID                      `json:"id"`
	Version    uint64                         `json:"version"`
	State      database.ConnectorVersionState `json:"state"`
	Type       string                         `json:"type"`
	Definition cschema.Connector              `json:"definition"`
	CreatedAt  time.Time                      `json:"created_at"`
	UpdatedAt  time.Time                      `json:"updated_at"`
}

func ConnectorVersionToJson(cv connIface.ConnectorVersion) ConnectorVersionJson {
	def := cv.GetDefinition()

	return ConnectorVersionJson{
		Id:         cv.GetId(),
		Version:    cv.GetVersion(),
		State:      cv.GetState(),
		Type:       cv.GetType(),
		Definition: *def,
		CreatedAt:  cv.GetCreatedAt(),
		UpdatedAt:  cv.GetUpdatedAt(),
	}
}

type ListConnectorVersionsRequestQueryParams struct {
	Cursor       *string                         `form:"cursor"`
	LimitVal     *int32                          `form:"limit"`
	StateVal     *database.ConnectorVersionState `form:"state"`
	NamespaceVal *string                         `form:"namespace"`
	OrderByVal   *string                         `form:"order_by"`
}

type ListConnectorVersionsResponseJson struct {
	Items  []ConnectorVersionJson `json:"items"`
	Cursor string                 `json:"cursor,omitempty"`
}

type ConnectorsRoutes struct {
	cfg         config.C
	connectors  connIface.C
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
	var ex connIface.ListConnectorsExecutor

	if req.Cursor != nil {
		ex, err = r.connectors.ListConnectorsFromCursor(ctx, *req.Cursor)
		if err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusInternalServerError().
				WithInternalErr(err).
				WithResponseMsg("failed to list core from cursor").
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
			b = b.ForState(*req.StateVal)
		}

		if req.NamespaceVal != nil {
			b = b.ForNamespaceMatcher(*req.NamespaceVal)
		}

		if req.OrderByVal != nil {
			field, order, err := pagination.SplitOrderByParam[database.ConnectorOrderByField](*req.OrderByVal)
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

func (r *ConnectorsRoutes) getVersion(gctx *gin.Context) {
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

	b := r.connectors.
		ListConnectorVersionsBuilder().
		ForId(connectorId).
		Limit(1)

	versionStr := gctx.Param("version")

	if versionStr == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("version is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	version, err := strconv.ParseUint(versionStr, 10, 64)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("failed to parse version as an integer").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	b = b.ForVersion(version)

	// TODO: support lookup by certain states

	result := b.FetchPage(ctx)
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
			WithResponseMsgf("connector version '%s:%d' not found", connectorId, version).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	cv := result.Results[0]
	gctx.PureJSON(http.StatusOK, ConnectorVersionToJson(cv))
}

func (r *ConnectorsRoutes) listVersions(gctx *gin.Context) {
	var err error
	var ex connIface.ListConnectorVersionsExecutor

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

	ctx := gctx.Request.Context()
	var req ListConnectorVersionsRequestQueryParams
	if err := gctx.ShouldBindQuery(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg(err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if req.Cursor != nil {
		ex, err = r.connectors.ListConnectorVersionsFromCursor(ctx, *req.Cursor)
		if err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusInternalServerError().
				WithInternalErr(err).
				WithResponseMsg("failed to list connector versions from cursor").
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			return
		}
	} else {
		b := r.connectors.ListConnectorVersionsBuilder().
			ForId(connectorId)

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
			field, order, err := pagination.SplitOrderByParam[database.ConnectorVersionOrderByField](*req.OrderByVal)
			if err != nil {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithInternalErr(err).
					WithResponseMsg(err.Error()).
					BuildStatusError().
					WriteGinResponse(r.cfg, gctx)
				return
			}

			if !database.IsValidConnectorVersionOrderByField(field) {
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
		gctx.PureJSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
	}

	gctx.PureJSON(http.StatusOK, ListConnectorVersionsResponseJson{
		Items:  util.Map(result.Results, ConnectorVersionToJson),
		Cursor: result.Cursor,
	})
}

func (r *ConnectorsRoutes) Register(g gin.IRouter) {
	g.GET(
		"/connectors",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForVerb("list").
			Build(),
		r.list,
	)
	g.GET(
		"/connectors/:id",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("get").
			Build(),
		r.get,
	)
	g.GET("/connectors/:id/versions",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("list/versions").
			Build(),
		r.listVersions,
	)
	g.GET(
		"/connectors/:id/versions/:version",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("list/versions").
			Build(),
		r.getVersion,
	)
}

func NewConnectorsRoutes(cfg config.C, authService auth.A, c connIface.C) *ConnectorsRoutes {
	return &ConnectorsRoutes{
		cfg:         cfg,
		authService: authService,
		connectors:  c,
	}
}
