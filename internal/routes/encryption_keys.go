package routes

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apgin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
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
	Namespace   string            `json:"namespace"`
	KeyData     *cfgschema.KeyData `json:"key_data,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
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

type PutEncryptionKeyLabelRequestJson struct {
	Value string `json:"value"`
}

type EncryptionKeyLabelJson struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type PutEncryptionKeyAnnotationRequestJson struct {
	Value string `json:"value"`
}

type EncryptionKeyAnnotationJson struct {
	Key   string `json:"key"`
	Value string `json:"value"`
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
	cfg         config.C
	core        coreIface.C
	authService auth.A
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

// @Summary		Get all labels for an encryption key
// @Description	Get all labels associated with a specific encryption key
// @Tags			encryption_keys
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Encryption key ID"
// @Success		200		{object}	map[string]string
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id}/labels [get]
func (r *EncryptionKeysRoutes) getLabels(gctx *gin.Context) {
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

	labels := ek.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	gctx.PureJSON(http.StatusOK, labels)
}

// @Summary		Get a specific label for an encryption key
// @Description	Get a specific label value by key for an encryption key
// @Tags			encryption_keys
// @Accept			json
// @Produce		json
// @Param			id		path		string	true	"Encryption key ID"
// @Param			label	path		string	true	"Label key"
// @Success		200		{object}	EncryptionKeyLabelJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id}/labels/{label} [get]
func (r *EncryptionKeysRoutes) getLabel(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := apid.ID(gctx.Param("id"))

	if id.IsNil() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	labelKey := gctx.Param("label")
	if labelKey == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("label key is required"))
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

	labels := ek.GetLabels()
	value, exists := labels[labelKey]
	if !exists {
		apgin.WriteError(gctx, nil, httperr.NotFoundf("label '%s' not found", labelKey))
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, EncryptionKeyLabelJson{
		Key:   labelKey,
		Value: value,
	})
}

// @Summary		Set a label for an encryption key
// @Description	Set or update a specific label value by key for an encryption key
// @Tags			encryption_keys
// @Accept			json
// @Produce		json
// @Param			id		path		string								true	"Encryption key ID"
// @Param			label	path		string								true	"Label key"
// @Param			request	body		PutEncryptionKeyLabelRequestJson	true	"Label value"
// @Success		200		{object}	EncryptionKeyLabelJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id}/labels/{label} [put]
func (r *EncryptionKeysRoutes) putLabel(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := apid.ID(gctx.Param("id"))

	if id.IsNil() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	labelKey := gctx.Param("label")
	if labelKey == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("label key is required"))
		val.MarkErrorReturn()
		return
	}

	if err := database.ValidateLabelKey(labelKey); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid label key: %s", err.Error()))
		val.MarkErrorReturn()
		return
	}

	var req PutEncryptionKeyLabelRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if err := database.ValidateLabelValue(req.Value); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid label value: %s", err.Error()))
		val.MarkErrorReturn()
		return
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

	updatedEk, err := r.core.PutEncryptionKeyLabels(ctx, id, map[string]string{labelKey: req.Value})
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

	gctx.PureJSON(http.StatusOK, EncryptionKeyLabelJson{
		Key:   labelKey,
		Value: updatedEk.GetLabels()[labelKey],
	})
}

// @Summary		Delete a label from an encryption key
// @Description	Delete a specific label by key from an encryption key
// @Tags			encryption_keys
// @Accept			json
// @Produce		json
// @Param			id		path	string	true	"Encryption key ID"
// @Param			label	path	string	true	"Label key"
// @Success		204		"No Content"
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id}/labels/{label} [delete]
func (r *EncryptionKeysRoutes) deleteLabel(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := apid.ID(gctx.Param("id"))

	if id.IsNil() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	labelKey := gctx.Param("label")
	if labelKey == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("label key is required"))
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

	_, err = r.core.DeleteEncryptionKeyLabels(ctx, id, []string{labelKey})
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

// @Summary		Get all annotations for an encryption key
// @Description	Get all annotations associated with a specific encryption key
// @Tags			encryption_keys
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Encryption key ID"
// @Success		200		{object}	map[string]string
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id}/annotations [get]
func (r *EncryptionKeysRoutes) getAnnotations(gctx *gin.Context) {
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

	annotations := ek.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	gctx.PureJSON(http.StatusOK, annotations)
}

// @Summary		Get a specific annotation for an encryption key
// @Description	Get a specific annotation value by key for an encryption key
// @Tags			encryption_keys
// @Accept			json
// @Produce		json
// @Param			id			path		string	true	"Encryption key ID"
// @Param			annotation	path		string	true	"Annotation key"
// @Success		200			{object}	SwaggerEncryptionKeyAnnotationJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id}/annotations/{annotation} [get]
func (r *EncryptionKeysRoutes) getAnnotation(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := apid.ID(gctx.Param("id"))

	if id.IsNil() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	annotationKey := gctx.Param("annotation")
	if annotationKey == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("annotation key is required"))
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

	annotations := ek.GetAnnotations()
	value, exists := annotations[annotationKey]
	if !exists {
		apgin.WriteError(gctx, nil, httperr.NotFoundf("annotation '%s' not found", annotationKey))
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, EncryptionKeyAnnotationJson{
		Key:   annotationKey,
		Value: value,
	})
}

// @Summary		Set an annotation for an encryption key
// @Description	Set or update a specific annotation value by key for an encryption key
// @Tags			encryption_keys
// @Accept			json
// @Produce		json
// @Param			id			path		string										true	"Encryption key ID"
// @Param			annotation	path		string										true	"Annotation key"
// @Param			request		body		SwaggerPutEncryptionKeyAnnotationRequest	true	"Annotation value"
// @Success		200			{object}	SwaggerEncryptionKeyAnnotationJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id}/annotations/{annotation} [put]
func (r *EncryptionKeysRoutes) putAnnotation(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := apid.ID(gctx.Param("id"))

	if id.IsNil() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	annotationKey := gctx.Param("annotation")
	if annotationKey == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("annotation key is required"))
		val.MarkErrorReturn()
		return
	}

	if err := database.ValidateAnnotationKey(annotationKey); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid annotation key: %s", err.Error()))
		val.MarkErrorReturn()
		return
	}

	var req PutEncryptionKeyAnnotationRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if err := database.ValidateAnnotationValue(req.Value); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid annotation value: %s", err.Error()))
		val.MarkErrorReturn()
		return
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

	updatedEk, err := r.core.PutEncryptionKeyAnnotations(ctx, id, map[string]string{annotationKey: req.Value})
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

	gctx.PureJSON(http.StatusOK, EncryptionKeyAnnotationJson{
		Key:   annotationKey,
		Value: updatedEk.GetAnnotations()[annotationKey],
	})
}

// @Summary		Delete an annotation from an encryption key
// @Description	Delete a specific annotation by key from an encryption key
// @Tags			encryption_keys
// @Accept			json
// @Produce		json
// @Param			id			path	string	true	"Encryption key ID"
// @Param			annotation	path	string	true	"Annotation key"
// @Success		204			"No Content"
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/encryption-keys/{id}/annotations/{annotation} [delete]
func (r *EncryptionKeysRoutes) deleteAnnotation(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := apid.ID(gctx.Param("id"))

	if id.IsNil() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	annotationKey := gctx.Param("annotation")
	if annotationKey == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("annotation key is required"))
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

	_, err = r.core.DeleteEncryptionKeyAnnotations(ctx, id, []string{annotationKey})
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
	return &EncryptionKeysRoutes{
		cfg:         cfg,
		authService: authService,
		core:        c,
	}
}
