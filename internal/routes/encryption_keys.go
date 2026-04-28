package routes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/routes/key_value"
	cfgschema "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type EncryptionKeyJson struct {
	Id          apid.ID                     `json:"id"`
	Namespace   string                      `json:"namespace"`
	State       database.EncryptionKeyState `json:"state"`
	Labels      map[string]string           `json:"labels,omitempty"`
	Annotations map[string]string           `json:"annotations,omitempty"`
	CreatedAt   time.Time                   `json:"created_at"`
	UpdatedAt   time.Time                   `json:"updated_at"`
}

type CreateEncryptionKeyRequestJson struct {
	Namespace   string             `json:"namespace"`
	KeyData     *cfgschema.KeyData `json:"key_data,omitempty"`
	Labels      map[string]string  `json:"labels,omitempty"`
	Annotations map[string]string  `json:"annotations,omitempty"`
}

type UpdateEncryptionKeyRequestJson struct {
	State       *database.EncryptionKeyState `json:"state,omitempty"`
	Labels      *map[string]string           `json:"labels,omitempty"`
	Annotations *map[string]string           `json:"annotations,omitempty"`
}

type ListEncryptionKeysRequestQueryParams struct {
	Cursor        *string                      `form:"cursor"`
	LimitVal      *int32                       `form:"limit"`
	StateVal      *database.EncryptionKeyState `form:"state"`
	NamespaceVal  *string                      `form:"namespace"`
	LabelSelector *string                      `form:"label_selector"`
	OrderByVal    *string                      `form:"order_by"`
}

type ListEncryptionKeysResponseJson struct {
	Items  []EncryptionKeyJson `json:"items"`
	Cursor string              `json:"cursor,omitempty"`
}

func EncryptionKeyToJson(ek coreIface.EncryptionKey) EncryptionKeyJson {
	return EncryptionKeyJson{
		Id:          ek.GetId(),
		Namespace:   ek.GetNamespace(),
		State:       ek.GetState(),
		Labels:      ek.GetLabels(),
		Annotations: ek.GetAnnotations(),
		CreatedAt:   ek.GetCreatedAt(),
		UpdatedAt:   ek.GetUpdatedAt(),
	}
}

type EncryptionKeysRoutes struct {
	cfg           config.C
	core          coreIface.C
	authService   auth.A
	labelsAdapter key_value.Adapter[apid.ID]
	annotsAdapter key_value.Adapter[apid.ID]
}

// @Summary		Get encryption key
// @Description	Get a specific encryption key by ID
// @Tags			encryption_keys
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Encryption key ID"
// @Success		200		{object}	SwaggerEncryptionKeyJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id} [get]
func (r *EncryptionKeysRoutes) get(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := apid.ID(gctx.Param("id"))

	if id.IsNil() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	ek, err := r.core.GetEncryptionKey(ctx, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("encryption key '%s' not found", id), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(ek); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	gctx.PureJSON(http.StatusOK, EncryptionKeyToJson(ek))
}

// @Summary		Create encryption key
// @Description	Create a new encryption key
// @Tags			encryption_keys
// @Accept			json
// @Produce		json
// @Param			request	body		CreateEncryptionKeyRequestJson	true	"Encryption key creation request"
// @Success		200		{object}	SwaggerEncryptionKeyJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys [post]
func (r *EncryptionKeysRoutes) create(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req CreateEncryptionKeyRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	if req.Namespace == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("namespace is required"))
		val.MarkErrorReturn()
		return
	}

	if err := val.ValidateNamespace(req.Namespace); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	ek, err := r.core.CreateEncryptionKey(ctx, req.Namespace, req.KeyData, req.Labels)
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

		ek, err = r.core.UpdateEncryptionKeyAnnotations(ctx, ek.GetId(), req.Annotations)
		if err != nil {
			apgin.WriteErr(gctx, nil, err)
			val.MarkErrorReturn()
			return
		}
	}

	gctx.PureJSON(http.StatusOK, EncryptionKeyToJson(ek))
}

// @Summary		List encryption keys
// @Description	List encryption keys with optional filtering and pagination
// @Tags			encryption_keys
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"Pagination cursor"
// @Param			limit			query		integer	false	"Maximum number of results to return"
// @Param			state			query		string	false	"Filter by state"
// @Param			namespace		query		string	false	"Filter by namespace"
// @Param			label_selector	query		string	false	"Filter by label selector"
// @Param			order_by		query		string	false	"Order by field (e.g., 'state:asc')"
// @Success		200				{object}	SwaggerListEncryptionKeysResponse
// @Failure		400				{object}	ErrorResponse
// @Failure		401				{object}	ErrorResponse
// @Failure		500				{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys [get]
func (r *EncryptionKeysRoutes) list(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req ListEncryptionKeysRequestQueryParams
	if err := gctx.ShouldBindQuery(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	var err error
	var ex coreIface.ListEncryptionKeysExecutor

	if req.Cursor != nil {
		ex, err = r.core.ListEncryptionKeysFromCursor(ctx, *req.Cursor)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to list encryption keys from cursor", httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	} else {
		b := r.core.ListEncryptionKeysBuilder()

		if req.LimitVal != nil {
			b = b.Limit(*req.LimitVal)
		}

		if req.StateVal != nil {
			b = b.ForState(*req.StateVal)
		}

		b = b.ForNamespaceMatchers(val.GetEffectiveNamespaceMatchers(req.NamespaceVal))

		if req.LabelSelector != nil {
			b = b.ForLabelSelector(*req.LabelSelector)
		}

		if req.OrderByVal != nil {
			field, order, err := pagination.SplitOrderByParam[database.EncryptionKeyOrderByField](*req.OrderByVal)
			if err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			if !database.IsValidEncryptionKeyOrderByField(field) {
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

	gctx.PureJSON(http.StatusOK, ListEncryptionKeysResponseJson{
		Items:  util.Map(auth.FilterForValidatedResources(val, result.Results), EncryptionKeyToJson),
		Cursor: result.Cursor,
	})
}

// @Summary		Update encryption key
// @Description	Update an encryption key's properties
// @Tags			encryption_keys
// @Accept			json
// @Produce		json
// @Param			id		path		string								true	"Encryption key ID"
// @Param			request	body		UpdateEncryptionKeyRequestJson		true	"Update request"
// @Success		200		{object}	SwaggerEncryptionKeyJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id} [patch]
func (r *EncryptionKeysRoutes) update(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := apid.ID(gctx.Param("id"))

	if id.IsNil() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	var req UpdateEncryptionKeyRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Validate state if provided
	if req.State != nil && !database.IsValidEncryptionKeyState(*req.State) {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid state '%s'", *req.State))
		val.MarkErrorReturn()
		return
	}

	// Validate labels if provided
	if req.Labels != nil {
		if err := database.Labels(*req.Labels).Validate(); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid labels: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
	}

	// Validate annotations if provided
	if req.Annotations != nil {
		if err := database.Annotations(*req.Annotations).Validate(); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid annotations: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
	}

	// Get existing key for authorization check
	ek, err := r.core.GetEncryptionKey(ctx, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("encryption key '%s' not found", id), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(ek); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	if req.State != nil {
		err = r.core.SetEncryptionKeyState(ctx, id, *req.State)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("encryption key '%s' not found", id), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
		}
	}

	if req.Labels != nil {
		_, err = r.core.UpdateEncryptionKeyLabels(ctx, id, *req.Labels)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("encryption key '%s' not found", id), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
		}
	}

	if req.Annotations != nil {
		_, err = r.core.UpdateEncryptionKeyAnnotations(ctx, id, *req.Annotations)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("encryption key '%s' not found", id), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
		}
	}

	ek, err = r.core.GetEncryptionKey(ctx, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("encryption key '%s' not found", id), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, EncryptionKeyToJson(ek))
}

// @Summary		Delete encryption key
// @Description	Soft delete an encryption key
// @Tags			encryption_keys
// @Accept			json
// @Produce		json
// @Param			id	path	string	true	"Encryption key ID"
// @Success		204		"No Content"
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id} [delete]
func (r *EncryptionKeysRoutes) delete(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := apid.ID(gctx.Param("id"))

	if id.IsNil() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	if id == database.GlobalEncryptionKeyID {
		apgin.WriteError(gctx, nil, httperr.BadRequest("the global encryption key cannot be deleted"))
		val.MarkErrorReturn()
		return
	}

	// Get existing key for authorization check
	ek, err := r.core.GetEncryptionKey(ctx, id)
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

	if httpErr := val.ValidateHttpStatusError(ek); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	err = r.core.DeleteEncryptionKey(ctx, id)
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

// Label and annotation handlers for encryption keys delegate to a shared
// generic adapter (see internal/routes/key_value). The doc comments below
// drive the OpenAPI spec; the bodies forward to the adapter.

// @Summary		Get all labels for an encryption key
// @Description	Get all labels associated with a specific encryption key
// @Tags			encryption_keys
// @Produce		json
// @Param			id	path		string	true	"Encryption key ID"
// @Success		200	{object}	map[string]string
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id}/labels [get]
func (r *EncryptionKeysRoutes) getLabels(gctx *gin.Context) { r.labelsAdapter.HandleList(gctx) }

// @Summary		Get a specific label for an encryption key
// @Description	Get a specific label value by key for an encryption key
// @Tags			encryption_keys
// @Produce		json
// @Param			id		path		string	true	"Encryption key ID"
// @Param			label	path		string	true	"Label key"
// @Success		200		{object}	SwaggerKeyValueJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id}/labels/{label} [get]
func (r *EncryptionKeysRoutes) getLabel(gctx *gin.Context) { r.labelsAdapter.HandleGet(gctx) }

// @Summary		Set a label for an encryption key
// @Description	Set or update a specific label value by key for an encryption key
// @Tags			encryption_keys
// @Accept			json
// @Produce		json
// @Param			id		path		string						true	"Encryption key ID"
// @Param			label	path		string						true	"Label key"
// @Param			request	body		SwaggerPutKeyValueRequest	true	"Label value"
// @Success		200		{object}	SwaggerKeyValueJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id}/labels/{label} [put]
func (r *EncryptionKeysRoutes) putLabel(gctx *gin.Context) { r.labelsAdapter.HandlePut(gctx) }

// @Summary		Delete a label from an encryption key
// @Description	Delete a specific label by key from an encryption key
// @Tags			encryption_keys
// @Param			id		path	string	true	"Encryption key ID"
// @Param			label	path	string	true	"Label key"
// @Success		204		"No Content"
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id}/labels/{label} [delete]
func (r *EncryptionKeysRoutes) deleteLabel(gctx *gin.Context) { r.labelsAdapter.HandleDelete(gctx) }

// @Summary		Get all annotations for an encryption key
// @Description	Get all annotations associated with a specific encryption key
// @Tags			encryption_keys
// @Produce		json
// @Param			id	path		string	true	"Encryption key ID"
// @Success		200	{object}	map[string]string
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id}/annotations [get]
func (r *EncryptionKeysRoutes) getAnnotations(gctx *gin.Context) { r.annotsAdapter.HandleList(gctx) }

// @Summary		Get a specific annotation for an encryption key
// @Description	Get a specific annotation value by key for an encryption key
// @Tags			encryption_keys
// @Produce		json
// @Param			id			path		string	true	"Encryption key ID"
// @Param			annotation	path		string	true	"Annotation key"
// @Success		200			{object}	SwaggerKeyValueJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id}/annotations/{annotation} [get]
func (r *EncryptionKeysRoutes) getAnnotation(gctx *gin.Context) { r.annotsAdapter.HandleGet(gctx) }

// @Summary		Set an annotation for an encryption key
// @Description	Set or update a specific annotation value by key for an encryption key
// @Tags			encryption_keys
// @Accept			json
// @Produce		json
// @Param			id			path		string						true	"Encryption key ID"
// @Param			annotation	path		string						true	"Annotation key"
// @Param			request		body		SwaggerPutKeyValueRequest	true	"Annotation value"
// @Success		200			{object}	SwaggerKeyValueJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id}/annotations/{annotation} [put]
func (r *EncryptionKeysRoutes) putAnnotation(gctx *gin.Context) { r.annotsAdapter.HandlePut(gctx) }

// @Summary		Delete an annotation from an encryption key
// @Description	Delete a specific annotation by key from an encryption key
// @Tags			encryption_keys
// @Param			id			path	string	true	"Encryption key ID"
// @Param			annotation	path	string	true	"Annotation key"
// @Success		204			"No Content"
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id}/annotations/{annotation} [delete]
func (r *EncryptionKeysRoutes) deleteAnnotation(gctx *gin.Context) {
	r.annotsAdapter.HandleDelete(gctx)
}

func (r *EncryptionKeysRoutes) Register(g gin.IRouter) {
	g.GET(
		"/encryption-keys",
		r.authService.NewRequiredBuilder().
			ForResource("encryption_keys").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.EncryptionKey).GetId()) }).
			ForVerb("list").
			Build(),
		r.list,
	)
	g.POST(
		"/encryption-keys",
		r.authService.NewRequiredBuilder().
			ForResource("encryption_keys").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.EncryptionKey).GetId()) }).
			ForVerb("create").
			Build(),
		r.create,
	)
	g.GET(
		"/encryption-keys/:id",
		r.authService.NewRequiredBuilder().
			ForResource("encryption_keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.EncryptionKey).GetId()) }).
			ForVerb("get").
			Build(),
		r.get,
	)
	g.PATCH(
		"/encryption-keys/:id",
		r.authService.NewRequiredBuilder().
			ForResource("encryption_keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.EncryptionKey).GetId()) }).
			ForVerb("update").
			Build(),
		r.update,
	)
	g.DELETE(
		"/encryption-keys/:id",
		r.authService.NewRequiredBuilder().
			ForResource("encryption_keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.EncryptionKey).GetId()) }).
			ForVerb("delete").
			Build(),
		r.delete,
	)
	g.GET(
		"/encryption-keys/:id/labels",
		r.authService.NewRequiredBuilder().
			ForResource("encryption_keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.EncryptionKey).GetId()) }).
			ForVerb("get").
			Build(),
		r.getLabels,
	)
	g.GET(
		"/encryption-keys/:id/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("encryption_keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.EncryptionKey).GetId()) }).
			ForVerb("get").
			Build(),
		r.getLabel,
	)
	g.PUT(
		"/encryption-keys/:id/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("encryption_keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.EncryptionKey).GetId()) }).
			ForVerb("update").
			Build(),
		r.putLabel,
	)
	g.DELETE(
		"/encryption-keys/:id/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("encryption_keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.EncryptionKey).GetId()) }).
			ForVerb("update").
			Build(),
		r.deleteLabel,
	)
	g.GET(
		"/encryption-keys/:id/annotations",
		r.authService.NewRequiredBuilder().
			ForResource("encryption_keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.EncryptionKey).GetId()) }).
			ForVerb("get").
			Build(),
		r.getAnnotations,
	)
	g.GET(
		"/encryption-keys/:id/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("encryption_keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.EncryptionKey).GetId()) }).
			ForVerb("get").
			Build(),
		r.getAnnotation,
	)
	g.PUT(
		"/encryption-keys/:id/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("encryption_keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.EncryptionKey).GetId()) }).
			ForVerb("update").
			Build(),
		r.putAnnotation,
	)
	g.DELETE(
		"/encryption-keys/:id/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("encryption_keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.EncryptionKey).GetId()) }).
			ForVerb("update").
			Build(),
		r.deleteAnnotation,
	)
}

func NewEncryptionKeysRoutes(cfg config.C, authService auth.A, c coreIface.C) *EncryptionKeysRoutes {
	parseEncryptionKeyID := func(gctx *gin.Context) (apid.ID, *httperr.Error) {
		id := apid.ID(gctx.Param("id"))
		if id.IsNil() {
			return apid.Nil, httperr.BadRequest("id is required")
		}
		return id, nil
	}

	getEncryptionKey := func(ctx context.Context, id apid.ID) (key_value.Resource, error) {
		ek, err := c.GetEncryptionKey(ctx, id)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return nil, database.ErrNotFound
			}
			return nil, err
		}
		if ek == nil {
			return nil, nil
		}
		return ek, nil
	}

	idExtractor := func(ek interface{}) string { return string(ek.(coreIface.EncryptionKey).GetId()) }

	authGet := authService.NewRequiredBuilder().
		ForResource("encryption_keys").
		ForIdField("id").
		ForIdExtractor(idExtractor).
		ForVerb("get").
		Build()
	authMutate := authService.NewRequiredBuilder().
		ForResource("encryption_keys").
		ForIdField("id").
		ForIdExtractor(idExtractor).
		ForVerb("update").
		Build()

	labelsAdapter := key_value.Adapter[apid.ID]{
		Kind:         key_value.Label,
		ResourceName: "encryption key",
		PathPrefix:   "/encryption-keys/:id",
		AuthGet:      authGet,
		AuthMutate:   authMutate,
		ParseID:      parseEncryptionKeyID,
		Get:          getEncryptionKey,
		Put: func(ctx context.Context, id apid.ID, kv map[string]string) (key_value.Resource, error) {
			return c.PutEncryptionKeyLabels(ctx, id, kv)
		},
		Delete: func(ctx context.Context, id apid.ID, keys []string) (key_value.Resource, error) {
			return c.DeleteEncryptionKeyLabels(ctx, id, keys)
		},
	}

	annotsAdapter := key_value.Adapter[apid.ID]{
		Kind:         key_value.Annotation,
		ResourceName: "encryption key",
		PathPrefix:   "/encryption-keys/:id",
		AuthGet:      authGet,
		AuthMutate:   authMutate,
		ParseID:      parseEncryptionKeyID,
		Get:          getEncryptionKey,
		Put: func(ctx context.Context, id apid.ID, kv map[string]string) (key_value.Resource, error) {
			return c.PutEncryptionKeyAnnotations(ctx, id, kv)
		},
		Delete: func(ctx context.Context, id apid.ID, keys []string) (key_value.Resource, error) {
			return c.DeleteEncryptionKeyAnnotations(ctx, id, keys)
		},
	}

	return &EncryptionKeysRoutes{
		cfg:           cfg,
		authService:   authService,
		core:          c,
		labelsAdapter: labelsAdapter,
		annotsAdapter: annotsAdapter,
	}
}
