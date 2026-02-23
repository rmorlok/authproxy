package routes

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/api_common"
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
	Labels    map[string]string       `json:"labels,omitempty"`
	CreatedAt time.Time               `json:"created_at"`
	UpdatedAt time.Time               `json:"updated_at"`
}

type CreateNamespaceRequestJson struct {
	Path   string            `json:"path"`
	Labels map[string]string `json:"labels"`
}

type UpdateNamespaceRequestJson struct {
	Labels map[string]string `json:"labels"`
}

type PutNamespaceLabelRequestJson struct {
	Value string `json:"value"`
}

type NamespaceLabelJson struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func NamespaceToJson(ns coreIface.Namespace) NamespaceJson {
	return NamespaceJson{
		Path:      ns.GetPath(),
		State:     ns.GetState(),
		Labels:    ns.GetLabels(),
		CreatedAt: ns.GetCreatedAt(),
		UpdatedAt: ns.GetUpdatedAt(),
	}
}

type ListNamespacesRequestQueryParams struct {
	Cursor        *string                  `form:"cursor"`
	LimitVal      *int32                   `form:"limit"`
	StateVal      *database.NamespaceState `form:"state"`
	ChildrenOf    *string                  `form:"children_of"`
	NamespaceVal  *string                  `form:"namespace"`
	LabelSelector *string                  `form:"label_selector"`
	OrderByVal    *string                  `form:"order_by"`
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

// @Summary		Get namespace
// @Description	Get a specific namespace by its path
// @Tags			namespaces
// @Accept			json
// @Produce		json
// @Param			path	path		string	true	"Namespace path"
// @Success		200		{object}	SwaggerNamespaceJson
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
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("path is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
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
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(ns); httpErr != nil {
		httpErr.WriteGinResponse(nil, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, NamespaceToJson(ns))
}

// @Summary		Create namespace
// @Description	Create a new namespace
// @Tags			namespaces
// @Accept			json
// @Produce		json
// @Param			request	body		CreateNamespaceRequestJson	true	"Namespace creation request"
// @Success		200		{object}	SwaggerNamespaceJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/namespaces [post]
func (r *NamespacesRoutes) create(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req CreateNamespaceRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if err := database.ValidateNamespacePath(req.Path); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsgf("invalid namespace path '%s': %s", req.Path, err.Error()).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	ns, err := r.core.GetNamespace(ctx, req.Path)
	if err == nil {
		// This means the namespace already exists
		api_common.NewHttpStatusErrorBuilder().
			WithStatus(http.StatusConflict).
			WithResponseMsgf("namespace '%s' already exists", req.Path).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if !errors.Is(err, core.ErrNotFound) {
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if err := val.ValidateNamespace(req.Path); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithPublicErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	ns, err = r.core.CreateNamespace(ctx, req.Path, req.Labels)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, NamespaceToJson(ns))
}

// @Summary		List namespaces
// @Description	List namespaces with optional filtering and pagination
// @Tags			namespaces
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"Pagination cursor"
// @Param			limit			query		integer	false	"Maximum number of results to return"
// @Param			state			query		string	false	"Filter by namespace state"
// @Param			children_of		query		string	false	"Filter to children of a parent namespace"
// @Param			namespace		query		string	false	"Filter by namespace path pattern"
// @Param			label_selector	query		string	false	"Filter by label selector"
// @Param			order_by		query		string	false	"Order by field (e.g., 'path:asc')"
// @Success		200				{object}	SwaggerListNamespacesResponse
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
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg(err.Error()).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
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
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
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

		b = b.ForNamespaceMatchers(val.GetEffectiveNamespaceMatchers(req.NamespaceVal))

		if req.LabelSelector != nil {
			b = b.ForLabelSelector(*req.LabelSelector)
		}

		if req.OrderByVal != nil {
			field, order, err := pagination.SplitOrderByParam[database.NamespaceOrderByField](*req.OrderByVal)
			if err != nil {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithInternalErr(err).
					WithResponseMsg(err.Error()).
					BuildStatusError().
					WriteGinResponse(nil, gctx)
				val.MarkErrorReturn()
				return
			}

			if !database.IsValidNamespaceOrderByField(field) {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithResponseMsgf("invalid sort field '%s'", field).
					BuildStatusError().
					WriteGinResponse(nil, gctx)
				val.MarkErrorReturn()
				return
			}

			b.OrderBy(field, order)
		}

		ex = b
	}

	result := ex.FetchPage(ctx)

	if result.Error != nil {
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(result.Error).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, ListNamespacesResponseJson{
		Items:  util.Map(auth.FilterForValidatedResources(val, result.Results), NamespaceToJson),
		Cursor: result.Cursor,
	})
}

// @Summary		Update namespace
// @Description	Update a namespace's labels
// @Tags			namespaces
// @Accept			json
// @Produce		json
// @Param			path	path		string						true	"Namespace path"
// @Param			request	body		UpdateNamespaceRequestJson	true	"Namespace update request"
// @Success		200		{object}	SwaggerNamespaceJson
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
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("path is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	var req UpdateNamespaceRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("invalid request body").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	// Validate labels if provided
	if req.Labels != nil {
		if err := database.Labels(req.Labels).Validate(); err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithInternalErr(err).
				WithResponseMsgf("invalid labels: %s", err.Error()).
				BuildStatusError().
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}
	}

	// Get the existing namespace for authorization check
	ns, err := r.core.GetNamespace(ctx, path)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsgf("namespace '%s' not found", path).
				WithInternalErr(err).
				BuildStatusError().
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(ns); httpErr != nil {
		httpErr.WriteGinResponse(nil, gctx)
		return
	}

	// Only update labels if provided in the request
	if req.Labels != nil {
		ns, err = r.core.UpdateNamespaceLabels(ctx, path, req.Labels)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusNotFound().
					WithResponseMsgf("namespace '%s' not found", path).
					WithInternalErr(err).
					BuildStatusError().
					WriteGinResponse(nil, gctx)
				val.MarkErrorReturn()
				return
			}

			api_common.NewHttpStatusErrorBuilder().
				WithStatusInternalServerError().
				WithInternalErr(err).
				BuildStatusError().
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}
	}

	gctx.PureJSON(http.StatusOK, NamespaceToJson(ns))
}

// @Summary		Get all labels for a namespace
// @Description	Get all labels associated with a specific namespace
// @Tags			namespaces
// @Accept			json
// @Produce		json
// @Param			path	path		string	true	"Namespace path"
// @Success		200		{object}	map[string]string
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/namespaces/{path}/labels [get]
func (r *NamespacesRoutes) getLabels(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	path := gctx.Param("path")

	if path == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("path is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
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
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(ns); httpErr != nil {
		httpErr.WriteGinResponse(nil, gctx)
		return
	}

	labels := ns.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	gctx.PureJSON(http.StatusOK, labels)
}

// @Summary		Get a specific label for a namespace
// @Description	Get a specific label value by key for a namespace
// @Tags			namespaces
// @Accept			json
// @Produce		json
// @Param			path	path		string	true	"Namespace path"
// @Param			label	path		string	true	"Label key"
// @Success		200		{object}	NamespaceLabelJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/namespaces/{path}/labels/{label} [get]
func (r *NamespacesRoutes) getLabel(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	path := gctx.Param("path")

	if path == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("path is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	labelKey := gctx.Param("label")
	if labelKey == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("label key is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
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
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(ns); httpErr != nil {
		httpErr.WriteGinResponse(nil, gctx)
		return
	}

	labels := ns.GetLabels()
	value, exists := labels[labelKey]
	if !exists {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsgf("label '%s' not found", labelKey).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, NamespaceLabelJson{
		Key:   labelKey,
		Value: value,
	})
}

// @Summary		Set a label for a namespace
// @Description	Set or update a specific label value by key for a namespace
// @Tags			namespaces
// @Accept			json
// @Produce		json
// @Param			path	path		string							true	"Namespace path"
// @Param			label	path		string							true	"Label key"
// @Param			request	body		PutNamespaceLabelRequestJson	true	"Label value"
// @Success		200		{object}	NamespaceLabelJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/namespaces/{path}/labels/{label} [put]
func (r *NamespacesRoutes) putLabel(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	path := gctx.Param("path")

	if path == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("path is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	labelKey := gctx.Param("label")
	if labelKey == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("label key is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	// Validate label key
	if err := database.ValidateLabelKey(labelKey); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsgf("invalid label key: %s", err.Error()).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	var req PutNamespaceLabelRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("invalid request body").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	// Validate label value
	if err := database.ValidateLabelValue(req.Value); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsgf("invalid label value: %s", err.Error()).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	// Get the existing namespace for authorization check
	ns, err := r.core.GetNamespace(ctx, path)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsgf("namespace '%s' not found", path).
				WithInternalErr(err).
				BuildStatusError().
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(ns); httpErr != nil {
		httpErr.WriteGinResponse(nil, gctx)
		return
	}

	// Use transactional PutNamespaceLabels to update
	updatedNs, err := r.core.PutNamespaceLabels(ctx, path, map[string]string{labelKey: req.Value})
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsgf("namespace '%s' not found", path).
				WithInternalErr(err).
				BuildStatusError().
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, NamespaceLabelJson{
		Key:   labelKey,
		Value: updatedNs.GetLabels()[labelKey],
	})
}

// @Summary		Delete a label from a namespace
// @Description	Delete a specific label by key from a namespace
// @Tags			namespaces
// @Accept			json
// @Produce		json
// @Param			path	path	string	true	"Namespace path"
// @Param			label	path	string	true	"Label key"
// @Success		204		"No Content"
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/namespaces/{path}/labels/{label} [delete]
func (r *NamespacesRoutes) deleteLabel(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	path := gctx.Param("path")

	if path == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("path is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	labelKey := gctx.Param("label")
	if labelKey == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("label key is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	// Get the existing namespace for authorization check
	ns, err := r.core.GetNamespace(ctx, path)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			// Namespace doesn't exist, return 204 (idempotent delete)
			gctx.Status(http.StatusNoContent)
			val.MarkValidated()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(ns); httpErr != nil {
		httpErr.WriteGinResponse(nil, gctx)
		return
	}

	// Use transactional DeleteNamespaceLabels to delete
	_, err = r.core.DeleteNamespaceLabels(ctx, path, []string{labelKey})
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			// Namespace was deleted between the check and the update, return 204
			gctx.Status(http.StatusNoContent)
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.Status(http.StatusNoContent)
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
	g.GET(
		"/namespaces/:path/labels",
		r.authService.NewRequiredBuilder().
			ForResource("namespaces").
			ForIdField("path").
			ForIdExtractor(func(ns interface{}) string { return ns.(coreIface.Namespace).GetPath() }).
			ForVerb("get").
			Build(),
		r.getLabels,
	)
	g.GET(
		"/namespaces/:path/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("namespaces").
			ForIdField("path").
			ForIdExtractor(func(ns interface{}) string { return ns.(coreIface.Namespace).GetPath() }).
			ForVerb("get").
			Build(),
		r.getLabel,
	)
	g.PUT(
		"/namespaces/:path/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("namespaces").
			ForIdField("path").
			ForIdExtractor(func(ns interface{}) string { return ns.(coreIface.Namespace).GetPath() }).
			ForVerb("update").
			Build(),
		r.putLabel,
	)
	g.DELETE(
		"/namespaces/:path/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("namespaces").
			ForIdField("path").
			ForIdExtractor(func(ns interface{}) string { return ns.(coreIface.Namespace).GetPath() }).
			ForVerb("update").
			Build(),
		r.deleteLabel,
	)
}

func NewNamespacesRoutes(cfg config.C, authService auth.A, c coreIface.C) *NamespacesRoutes {
	return &NamespacesRoutes{
		cfg:         cfg,
		authService: authService,
		core:        c,
	}
}
