package routes

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/auth"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"

	"net/http"
)

type NamespaceJson struct {
	Path      string                  `json:"path"`
	State     database.NamespaceState `json:"state"`
	CreatedAt time.Time               `json:"created_at"`
	UpdatedAt time.Time               `json:"updated_at"`
}

type CreateNamespaceRequestJson struct {
	Path string `json:"path"`
}

func NamespaceToJson(ns coreIface.Namespace) NamespaceJson {
	return NamespaceJson{
		Path:      ns.GetPath(),
		State:     ns.GetState(),
		CreatedAt: ns.GetCreatedAt(),
		UpdatedAt: ns.GetUpdatedAt(),
	}
}

type ListNamespacesRequestQueryParams struct {
	Cursor     *string                  `form:"cursor"`
	LimitVal   *int32                   `form:"limit"`
	StateVal   *database.NamespaceState `form:"state"`
	ChildrenOf *string                  `form:"children_of"`
	OrderByVal *string                  `form:"order_by"`
}

type ListNamespacesResponseJson struct {
	Items  []NamespaceJson `json:"items"`
	Cursor string          `json:"cursor,omitempty"`
}

type NamespacesRoutes struct {
	cfg         config.C
	core        coreIface.C
	authService auth.A
}

func (r *NamespacesRoutes) get(gctx *gin.Context) {
	ctx := gctx.Request.Context()

	path := gctx.Param("path")

	if path == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("path is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	ns, err := r.core.GetNamespace(ctx, path)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsgf("namespace '%s' not found", path).
				WithInternalErr(err).
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

	gctx.PureJSON(http.StatusOK, NamespaceToJson(ns))
}

func (r *NamespacesRoutes) create(gctx *gin.Context) {
	ctx := gctx.Request.Context()

	var req CreateNamespaceRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if err := database.ValidateNamespacePath(req.Path); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsgf("invalid namespace path '%s': %s", req.Path, err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	ns, err := r.core.GetNamespace(ctx, req.Path)
	if err == nil {
		// This means the namespace already exists
		api_common.NewHttpStatusErrorBuilder().
			WithStatus(http.StatusConflict).
			WithResponseMsgf("namespace '%s' already exists", req.Path).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if !errors.Is(err, core.ErrNotFound) {
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	ns, err = r.core.CreateNamespace(ctx, req.Path)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}
	gctx.PureJSON(http.StatusOK, NamespaceToJson(ns))
}

func (r *NamespacesRoutes) list(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	var req ListNamespacesRequestQueryParams
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
	var ex coreIface.ListNamespacesExecutor

	if req.Cursor != nil {
		ex, err = r.core.ListNamespacesFromCursor(ctx, *req.Cursor)
		if err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusInternalServerError().
				WithInternalErr(err).
				WithResponseMsg("failed to list namespaces from cursor").
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			return
		}
	} else {
		b := r.core.ListNamespacesBuilder()

		if req.LimitVal != nil {
			b = b.Limit(*req.LimitVal)
		}

		if req.StateVal != nil {
			b = b.ForState(*req.StateVal)
		}

		if req.ChildrenOf != nil {
			b = b.ForChildrenOf(*req.ChildrenOf)
		}

		if req.OrderByVal != nil {
			field, order, err := pagination.SplitOrderByParam[database.NamespaceOrderByField](*req.OrderByVal)
			if err != nil {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithInternalErr(err).
					WithResponseMsg(err.Error()).
					BuildStatusError().
					WriteGinResponse(r.cfg, gctx)
				return
			}

			if !database.IsValidNamespaceOrderByField(field) {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithResponseMsgf("invalid sort field '%s'", field).
					BuildStatusError().
					WriteGinResponse(r.cfg, gctx)
				return
			}

			b.OrderBy(database.NamespaceOrderByField(field), order)
		}

		ex = b
	}

	result := ex.FetchPage(ctx)

	if result.Error != nil {
		gctx.PureJSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
	}

	gctx.PureJSON(http.StatusOK, ListNamespacesResponseJson{
		Items:  util.Map(result.Results, NamespaceToJson),
		Cursor: result.Cursor,
	})
}

func (r *NamespacesRoutes) Register(g gin.IRouter) {
	g.GET("/namespaces", r.authService.Required(), r.list)
	g.POST("/namespaces", r.authService.Required(), r.create)
	g.GET("/namespaces/:path", r.authService.Required(), r.get)
}

func NewNamespacesRoutes(cfg config.C, authService auth.A, c coreIface.C) *NamespacesRoutes {
	return &NamespacesRoutes{
		cfg:         cfg,
		authService: authService,
		core:        c,
	}
}
