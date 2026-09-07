package routes

import (
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	schemaapiopenapi "github.com/rmorlok/authproxy/internal/schema/api/openapi"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"

	"net/http"
)

// Swagger annotations do not count as Go references, so retain this compile-
// time reference to the documentation-only list projection used below.
var _ = schemaapiopenapi.ListNamespacesResponseJson{}

type ListNamespacesRequestQueryParams struct {
	Cursor        *string                   `form:"cursor"`
	LimitVal      *int32                    `form:"limit"`
	StateVal      *namespace.NamespaceState `form:"state"`
	ChildrenOf    *string                   `form:"childrenOf"`
	NamespaceVal  *string                   `form:"namespace"`
	NameVal       *string                   `form:"name"`
	LabelSelector *string                   `form:"labelSelector"`
	OrderByVal    *string                   `form:"orderBy"`
}

type NamespacesRoutes struct {
	cfg         config.C
	core        coreIface.C
	authService auth.A
}

func writeNamespaceKeyReferenceError(gctx *gin.Context, err error) {
	switch {
	case errors.Is(err, core.ErrInvalidArgument):
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
	case errors.Is(err, core.ErrNotFound):
		apgin.WriteError(gctx, nil, httperr.NotFound("key reference not found", httperr.WithInternalErr(err)))
	default:
		apgin.WriteErr(gctx, nil, err)
	}
}

// @Summary		Get namespace
// @Description	Get a specific namespace by its path
// @Tags			namespaces
// @Accept			json
// @Produce		json
// @Param			path	path		string	true	"Namespace path"
// @Success		200		{object}	namespace.Namespace
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/namespaces/{path} [get]
func (r *NamespacesRoutes) get(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	path := gctx.Param("path")

	if path == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("path is required"))
		val.MarkErrorReturn()
		return
	}

	ns, err := r.core.GetNamespace(ctx, path)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("namespace '%s' not found", path), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(ns); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	resource := ns.GetResource()
	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, resource); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		Create namespace
// @Description	Create a new namespace
// @Tags			namespaces
// @Accept			json
// @Produce		json
// @Param			request	body		namespace.Namespace	true	"Namespace creation request"
// @Success		200		{object}	namespace.Namespace
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/namespaces [post]
func (r *NamespacesRoutes) create(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req namespace.Namespace
	if err := apgin.BindResourceJSON(gctx, &req, meta.ValidationModeCreate); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	path, err := namespace.PathFromMetadata(req.Metadata)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	ns, err := r.core.GetNamespace(ctx, path)
	if err == nil {
		// This means the namespace already exists
		apgin.WriteError(gctx, nil, httperr.Conflictf("namespace '%s' already exists", path))
		val.MarkErrorReturn()
		return
	}

	if !errors.Is(err, core.ErrNotFound) {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	if err := val.ValidateNamespace(path); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	ns, err = r.core.CreateNamespace(ctx, &req)
	if err != nil {
		if req.Spec.EncryptionKeyRef != nil {
			writeNamespaceKeyReferenceError(gctx, err)
		} else {
			apgin.WriteErr(gctx, nil, err)
		}
		val.MarkErrorReturn()
		return
	}

	resource := ns.GetResource()
	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, resource); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		List namespaces
// @Description	List namespaces with optional filtering and pagination
// @Tags			namespaces
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"Pagination cursor"
// @Param			limit			query		integer	false	"Maximum number of results to return"
// @Param			state			query		string	false	"Filter by namespace state"
// @Param			childrenOf		query		string	false	"Filter to children of a parent namespace"
// @Param			namespace		query		string	false	"Filter by namespace path pattern"
// @Param			name			query		string	false	"Filter by exact final path segment"
// @Param			labelSelector	query		string	false	"Filter by label selector"
// @Param			orderBy		query		string	false	"Order by field (e.g., 'path:asc')"
// @Success		200				{object}	schemaapiopenapi.ListNamespacesResponseJson
// @Failure		400				{object}	ErrorResponse
// @Failure		401				{object}	ErrorResponse
// @Failure		500				{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/namespaces [get]
func (r *NamespacesRoutes) list(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req ListNamespacesRequestQueryParams
	if err := gctx.ShouldBindQuery(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	var err error
	var ex coreIface.ListNamespacesExecutor

	if req.Cursor != nil {
		ex, err = r.core.ListNamespacesFromCursor(ctx, *req.Cursor)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to list namespaces from cursor", httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	} else {
		b := r.core.ListNamespacesBuilder()

		if req.LimitVal != nil {
			b = b.Limit(*req.LimitVal)
		}

		if req.StateVal != nil {
			if !namespace.IsValidState(*req.StateVal) {
				apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid namespace state %q", *req.StateVal))
				val.MarkErrorReturn()
				return
			}
			b = b.ForState(*req.StateVal)
		}

		if req.ChildrenOf != nil {
			if err := namespace.ValidatePath(*req.ChildrenOf); err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid children_of namespace '%s': %s", *req.ChildrenOf, err.Error()))
				val.MarkErrorReturn()
				return
			}

			b = b.ForChildrenOf(*req.ChildrenOf)
		}

		if req.NamespaceVal != nil {
			if err := namespace.ValidateMatcher(*req.NamespaceVal); err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid namespace matcher '%s': %s", *req.NamespaceVal, err.Error()))
				val.MarkErrorReturn()
				return
			}
		}

		b = b.ForNamespaceMatchers(val.GetEffectiveNamespaceMatchers(req.NamespaceVal))

		if req.NameVal != nil {
			name := scommon.ResourceName(*req.NameVal)
			if err := namespace.ValidateName(*req.NameVal); err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid namespace name: %s", err.Error()))
				val.MarkErrorReturn()
				return
			}
			b = b.ForName(name)
		}

		if req.LabelSelector != nil {
			b = b.ForLabelSelector(*req.LabelSelector)
		}

		if req.OrderByVal != nil {
			field, order, err := pagination.SplitOrderByParam[database.NamespaceOrderByField](*req.OrderByVal)
			if err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			if !database.IsValidNamespaceOrderByField(field) {
				apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid sort field '%s'", field))
				val.MarkErrorReturn()
				return
			}

			b.OrderBy(field, order)
		}

		ex = b
	}

	result := ex.FetchPage(ctx)

	if result.Error != nil {
		apgin.WriteErr(gctx, nil, result.Error)
		val.MarkErrorReturn()
		return
	}

	response := schemaapi.NewListNamespacesResponseJson(
		util.Map(
			auth.FilterForValidatedResources(val, result.Results),
			func(ns coreIface.Namespace) namespace.Namespace {
				return *ns.GetResource()
			},
		),
		result.Cursor,
	)

	if err := response.Validate(namespace.NamespaceKind); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}
	apgin.APIJSON(gctx, http.StatusOK, response)
}

// @Summary		Update namespace
// @Description	Update a namespace's desired encryption key, labels, and annotations. Set spec.encryptionKeyRef to null to clear it. Namespace identity cannot be changed.
// @Tags			namespaces
// @Accept			json
// @Produce		json
// @Param			path	path		string						true	"Namespace path"
// @Param			request	body		namespace.NamespacePatch	true	"Namespace update request"
// @Success		200		{object}	namespace.Namespace
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/namespaces/{path} [patch]
func (r *NamespacesRoutes) update(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	path := gctx.Param("path")

	if path == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("path is required"))
		val.MarkErrorReturn()
		return
	}

	var req namespace.NamespacePatch
	if err := apgin.BindResourceJSON(gctx, &req, meta.ValidationModeUpdate); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Get the existing namespace for authorization check
	ns, err := r.core.GetNamespace(ctx, path)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("namespace '%s' not found", path), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(ns); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	before := ns.GetResource()
	_, err = req.ApplyTo(before, nil)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	// Only update labels if provided in the request
	if req.Metadata.Labels != nil {
		ns, err = r.core.UpdateNamespaceLabels(ctx, path, *req.Metadata.Labels)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("namespace '%s' not found", path), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	// Only update annotations if provided in the request
	if req.Metadata.Annotations != nil {
		ns, err = r.core.UpdateNamespaceAnnotations(ctx, path, *req.Metadata.Annotations)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("namespace '%s' not found", path), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	if req.Spec.HasEncryptionKeyRef() {
		if ref := req.Spec.EncryptionKeyRef; ref != nil {
			key, resolveErr := r.core.ResolveKeyReference(ctx, *ref)
			if resolveErr != nil {
				writeNamespaceKeyReferenceError(gctx, resolveErr)
				val.MarkErrorReturn()
				return
			}
			ns, err = r.core.SetNamespaceKey(ctx, path, key.GetId())
		} else {
			ns, err = r.core.ClearNamespaceKey(ctx, path)
		}
		if err != nil {
			apgin.WriteErr(gctx, nil, err)
			val.MarkErrorReturn()
			return
		}
	}

	resource := ns.GetResource()
	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, resource); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

func (r *NamespacesRoutes) Register(g gin.IRouter) {
	g.GET(
		"/namespaces",
		r.authService.NewRequiredBuilder().
			ForResource("namespaces").
			ForIdExtractor(func(ns interface{}) string { return ns.(coreIface.Namespace).GetPath() }).
			ForVerb("list").
			Build(),
		r.list,
	)
	g.POST(
		"/namespaces",
		r.authService.NewRequiredBuilder().
			ForResource("namespaces").
			ForIdExtractor(func(ns interface{}) string { return ns.(coreIface.Namespace).GetPath() }).
			ForVerb("create").
			Build(),
		r.create,
	)
	g.GET(
		"/namespaces/:path",
		r.authService.NewRequiredBuilder().
			ForResource("namespaces").
			ForIdField("path").
			ForIdExtractor(func(ns interface{}) string { return ns.(coreIface.Namespace).GetPath() }).
			ForVerb("get").
			Build(),
		r.get,
	)
	g.PATCH(
		"/namespaces/:path",
		r.authService.NewRequiredBuilder().
			ForResource("namespaces").
			ForIdField("path").
			ForIdExtractor(func(ns interface{}) string { return ns.(coreIface.Namespace).GetPath() }).
			ForVerb("update").
			Build(),
		r.update,
	)
}

func NewNamespacesRoutes(cfg config.C, authService auth.A, c coreIface.C) *NamespacesRoutes {
	return &NamespacesRoutes{
		cfg:         cfg,
		authService: authService,
		core:        c,
	}
}
