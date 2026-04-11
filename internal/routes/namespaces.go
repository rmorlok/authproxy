package routes

import (
	"errors"
	"time"

	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apgin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"

	"net/http"
)

type NamespaceJson struct {
	Path            string                  `json:"path"`
	State           database.NamespaceState `json:"state"`
	EncryptionKeyId *string                 `json:"encryption_key_id,omitempty"`
	Labels          map[string]string       `json:"labels,omitempty"`
	Annotations     map[string]string       `json:"annotations,omitempty"`
	CreatedAt       time.Time               `json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
}

type CreateNamespaceRequestJson struct {
	Path        string            `json:"path"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

type UpdateNamespaceRequestJson struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

type PutNamespaceLabelRequestJson struct {
	Value string `json:"value"`
}

type NamespaceLabelJson struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type PutNamespaceAnnotationRequestJson struct {
	Value string `json:"value"`
}

type NamespaceAnnotationJson struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func NamespaceToJson(ns coreIface.Namespace) NamespaceJson {
	var ekId *string
	if ns.GetEncryptionKeyId() != nil {
		s := string(*ns.GetEncryptionKeyId())
		ekId = &s
	}

	return NamespaceJson{
		Path:            ns.GetPath(),
		State:           ns.GetState(),
		EncryptionKeyId: ekId,
		Labels:          ns.GetLabels(),
		Annotations:     ns.GetAnnotations(),
		CreatedAt:       ns.GetCreatedAt(),
		UpdatedAt:       ns.GetUpdatedAt(),
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
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	if err := database.ValidateNamespacePath(req.Path); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid namespace path '%s': %s", req.Path, err.Error()))
		val.MarkErrorReturn()
		return
	}

	ns, err := r.core.GetNamespace(ctx, req.Path)
	if err == nil {
		// This means the namespace already exists
		apgin.WriteError(gctx, nil, httperr.Conflictf("namespace '%s' already exists", req.Path))
		val.MarkErrorReturn()
		return
	}

	if !errors.Is(err, core.ErrNotFound) {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	if err := val.ValidateNamespace(req.Path); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	ns, err = r.core.CreateNamespace(ctx, req.Path, req.Labels)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	// Set annotations if provided
	if req.Annotations != nil {
		if err := database.Annotations(req.Annotations).Validate(); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid annotations: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}

		ns, err = r.core.UpdateNamespaceAnnotations(ctx, req.Path, req.Annotations)
		if err != nil {
			apgin.WriteErr(gctx, nil, err)
			val.MarkErrorReturn()
			return
		}
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
		apgin.WriteError(gctx, nil, httperr.BadRequest("path is required"))
		val.MarkErrorReturn()
		return
	}

	var req UpdateNamespaceRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Validate labels if provided
	if req.Labels != nil {
		if err := database.Labels(req.Labels).Validate(); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid labels: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
	}

	// Validate annotations if provided
	if req.Annotations != nil {
		if err := database.Annotations(req.Annotations).Validate(); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid annotations: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
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

	// Only update labels if provided in the request
	if req.Labels != nil {
		ns, err = r.core.UpdateNamespaceLabels(ctx, path, req.Labels)
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
	if req.Annotations != nil {
		ns, err = r.core.UpdateNamespaceAnnotations(ctx, path, req.Annotations)
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
		apgin.WriteError(gctx, nil, httperr.BadRequest("path is required"))
		val.MarkErrorReturn()
		return
	}

	labelKey := gctx.Param("label")
	if labelKey == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("label key is required"))
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

	labels := ns.GetLabels()
	value, exists := labels[labelKey]
	if !exists {
		apgin.WriteError(gctx, nil, httperr.NotFoundf("label '%s' not found", labelKey))
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
		apgin.WriteError(gctx, nil, httperr.BadRequest("path is required"))
		val.MarkErrorReturn()
		return
	}

	labelKey := gctx.Param("label")
	if labelKey == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("label key is required"))
		val.MarkErrorReturn()
		return
	}

	// Validate label key
	if err := database.ValidateLabelKey(labelKey); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid label key: %s", err.Error()))
		val.MarkErrorReturn()
		return
	}

	var req PutNamespaceLabelRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Validate label value
	if err := database.ValidateLabelValue(req.Value); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid label value: %s", err.Error()))
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

	// Use transactional PutNamespaceLabels to update
	updatedNs, err := r.core.PutNamespaceLabels(ctx, path, map[string]string{labelKey: req.Value})
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
		apgin.WriteError(gctx, nil, httperr.BadRequest("path is required"))
		val.MarkErrorReturn()
		return
	}

	labelKey := gctx.Param("label")
	if labelKey == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("label key is required"))
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

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(ns); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
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

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	gctx.Status(http.StatusNoContent)
}

// @Summary		Get all annotations for a namespace
// @Description	Get all annotations associated with a specific namespace
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
// @Router			/namespaces/{path}/annotations [get]
func (r *NamespacesRoutes) getAnnotations(gctx *gin.Context) {
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

	annotations := ns.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	gctx.PureJSON(http.StatusOK, annotations)
}

// @Summary		Get a specific annotation for a namespace
// @Description	Get a specific annotation value by key for a namespace
// @Tags			namespaces
// @Accept			json
// @Produce		json
// @Param			path		path		string	true	"Namespace path"
// @Param			annotation	path		string	true	"Annotation key"
// @Success		200			{object}	SwaggerNamespaceAnnotationJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/namespaces/{path}/annotations/{annotation} [get]
func (r *NamespacesRoutes) getAnnotation(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	path := gctx.Param("path")

	if path == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("path is required"))
		val.MarkErrorReturn()
		return
	}

	annotationKey := gctx.Param("annotation")
	if annotationKey == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("annotation key is required"))
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

	annotations := ns.GetAnnotations()
	value, exists := annotations[annotationKey]
	if !exists {
		apgin.WriteError(gctx, nil, httperr.NotFoundf("annotation '%s' not found", annotationKey))
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, NamespaceAnnotationJson{
		Key:   annotationKey,
		Value: value,
	})
}

// @Summary		Set an annotation for a namespace
// @Description	Set or update a specific annotation value by key for a namespace
// @Tags			namespaces
// @Accept			json
// @Produce		json
// @Param			path		path		string									true	"Namespace path"
// @Param			annotation	path		string									true	"Annotation key"
// @Param			request		body		SwaggerPutNamespaceAnnotationRequest	true	"Annotation value"
// @Success		200			{object}	SwaggerNamespaceAnnotationJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/namespaces/{path}/annotations/{annotation} [put]
func (r *NamespacesRoutes) putAnnotation(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	path := gctx.Param("path")

	if path == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("path is required"))
		val.MarkErrorReturn()
		return
	}

	annotationKey := gctx.Param("annotation")
	if annotationKey == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("annotation key is required"))
		val.MarkErrorReturn()
		return
	}

	// Validate annotation key
	if err := database.ValidateAnnotationKey(annotationKey); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid annotation key: %s", err.Error()))
		val.MarkErrorReturn()
		return
	}

	var req PutNamespaceAnnotationRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
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

	// Use transactional PutNamespaceAnnotations to update
	updatedNs, err := r.core.PutNamespaceAnnotations(ctx, path, map[string]string{annotationKey: req.Value})
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

	gctx.PureJSON(http.StatusOK, NamespaceAnnotationJson{
		Key:   annotationKey,
		Value: updatedNs.GetAnnotations()[annotationKey],
	})
}

// @Summary		Delete an annotation from a namespace
// @Description	Delete a specific annotation by key from a namespace
// @Tags			namespaces
// @Accept			json
// @Produce		json
// @Param			path		path	string	true	"Namespace path"
// @Param			annotation	path	string	true	"Annotation key"
// @Success		204			"No Content"
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/namespaces/{path}/annotations/{annotation} [delete]
func (r *NamespacesRoutes) deleteAnnotation(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	path := gctx.Param("path")

	if path == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("path is required"))
		val.MarkErrorReturn()
		return
	}

	annotationKey := gctx.Param("annotation")
	if annotationKey == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("annotation key is required"))
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

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(ns); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	// Use transactional DeleteNamespaceAnnotations to delete
	_, err = r.core.DeleteNamespaceAnnotations(ctx, path, []string{annotationKey})
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			// Namespace was deleted between the check and the update, return 204
			gctx.Status(http.StatusNoContent)
			return
		}

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	gctx.Status(http.StatusNoContent)
}

type SetNamespaceEncryptionKeyRequestJson struct {
	EncryptionKeyId string `json:"encryption_key_id"`
}

type NamespaceEncryptionKeyJson struct {
	EncryptionKeyId string `json:"encryption_key_id"`
}

func (r *NamespacesRoutes) getEncryptionKey(gctx *gin.Context) {
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

	ekId := ns.GetEncryptionKeyId()
	if ekId == nil {
		apgin.WriteError(gctx, nil, httperr.NotFoundf("namespace '%s' has no encryption key set", path))
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, NamespaceEncryptionKeyJson{
		EncryptionKeyId: string(*ekId),
	})
}

func (r *NamespacesRoutes) setEncryptionKey(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	path := gctx.Param("path")

	if path == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("path is required"))
		val.MarkErrorReturn()
		return
	}

	var req SetNamespaceEncryptionKeyRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if req.EncryptionKeyId == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("encryption_key_id is required"))
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

	ns, err = r.core.SetNamespaceEncryptionKey(ctx, path, apid.ID(req.EncryptionKeyId))
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("encryption key '%s' not found", req.EncryptionKeyId), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}

		var httpErr *httperr.Error
		if errors.As(err, &httpErr) {
			apgin.WriteError(gctx, nil, httpErr)
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, NamespaceToJson(ns))
}

func (r *NamespacesRoutes) clearEncryptionKey(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	path := gctx.Param("path")

	if path == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("path is required"))
		val.MarkErrorReturn()
		return
	}

	// Get the existing namespace for authorization check
	ns, err := r.core.GetNamespace(ctx, path)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			gctx.Status(http.StatusNoContent)
			val.MarkValidated()
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

	_, err = r.core.ClearNamespaceEncryptionKey(ctx, path)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			gctx.Status(http.StatusNoContent)
			return
		}

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
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
	g.GET(
		"/namespaces/:path/annotations",
		r.authService.NewRequiredBuilder().
			ForResource("namespaces").
			ForIdField("path").
			ForIdExtractor(func(ns interface{}) string { return ns.(coreIface.Namespace).GetPath() }).
			ForVerb("get").
			Build(),
		r.getAnnotations,
	)
	g.GET(
		"/namespaces/:path/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("namespaces").
			ForIdField("path").
			ForIdExtractor(func(ns interface{}) string { return ns.(coreIface.Namespace).GetPath() }).
			ForVerb("get").
			Build(),
		r.getAnnotation,
	)
	g.PUT(
		"/namespaces/:path/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("namespaces").
			ForIdField("path").
			ForIdExtractor(func(ns interface{}) string { return ns.(coreIface.Namespace).GetPath() }).
			ForVerb("update").
			Build(),
		r.putAnnotation,
	)
	g.DELETE(
		"/namespaces/:path/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("namespaces").
			ForIdField("path").
			ForIdExtractor(func(ns interface{}) string { return ns.(coreIface.Namespace).GetPath() }).
			ForVerb("update").
			Build(),
		r.deleteAnnotation,
	)
	g.GET(
		"/namespaces/:path/encryption-key",
		r.authService.NewRequiredBuilder().
			ForResource("namespaces").
			ForIdField("path").
			ForIdExtractor(func(ns interface{}) string { return ns.(coreIface.Namespace).GetPath() }).
			ForVerb("get").
			Build(),
		r.getEncryptionKey,
	)
	g.PUT(
		"/namespaces/:path/encryption-key",
		r.authService.NewRequiredBuilder().
			ForResource("namespaces").
			ForIdField("path").
			ForIdExtractor(func(ns interface{}) string { return ns.(coreIface.Namespace).GetPath() }).
			ForVerb("update").
			Build(),
		r.setEncryptionKey,
	)
	g.DELETE(
		"/namespaces/:path/encryption-key",
		r.authService.NewRequiredBuilder().
			ForResource("namespaces").
			ForIdField("path").
			ForIdExtractor(func(ns interface{}) string { return ns.(coreIface.Namespace).GetPath() }).
			ForVerb("update").
			Build(),
		r.clearEncryptionKey,
	)
}

func NewNamespacesRoutes(cfg config.C, authService auth.A, c coreIface.C) *NamespacesRoutes {
	return &NamespacesRoutes{
		cfg:         cfg,
		authService: authService,
		core:        c,
	}
}
