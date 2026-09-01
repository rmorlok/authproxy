package routes

import (
	"context"
	"errors"
	"fmt"
	"net/http"

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
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	schemaapiopenapi "github.com/rmorlok/authproxy/internal/schema/api/openapi"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// Swagger annotations do not count as Go references, so retain compile-time
// references to the documentation-only projections used below.
var (
	_ = schemaapiopenapi.KeyJson{}
	_ = schemaapiopenapi.KeyPatchJson{}
	_ = schemaapiopenapi.ListKeysResponseJson{}
)

type ListKeysRequestQueryParams struct {
	Cursor        *string             `form:"cursor"`
	LimitVal      *int32              `form:"limit"`
	StateVal      *keyschema.KeyState `form:"state"`
	NamespaceVal  *string             `form:"namespace"`
	NameVal       *string             `form:"name"`
	LabelSelector *string             `form:"labelSelector"`
	OrderByVal    *string             `form:"orderBy"`
}

func KeyToResource(ctx context.Context, c coreIface.C, ek coreIface.Key) (*keyschema.Key, error) {
	return keyToResource(ctx, c, ek, false)
}

func KeyToResourceOmitUnconfiguredData(ctx context.Context, c coreIface.C, ek coreIface.Key) (*keyschema.Key, error) {
	return keyToResource(ctx, c, ek, true)
}

func keyToResource(ctx context.Context, c coreIface.C, ek coreIface.Key, allowUnconfiguredKeyData bool) (*keyschema.Key, error) {
	resource := ek.GetResource()
	keyData, err := c.GetKeyData(ctx, ek.GetId())
	if err != nil {
		if allowUnconfiguredKeyData && errors.Is(err, core.ErrKeyDataNotConfigured) {
			return resource, nil
		}
		return nil, err
	}

	redacted, err := keyschema.RedactKeyData(keyData)
	if err != nil {
		return nil, err
	}
	resource.Spec.KeyData = redacted
	return resource, nil
}

type KeysRoutes struct {
	cfg           config.C
	core          coreIface.C
	authService   auth.A
	labelsAdapter key_value.Adapter[apid.ID]
	annotsAdapter key_value.Adapter[apid.ID]
}

// @Summary		Get key
// @Description	Get a specific key by ID
// @Tags			keys
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Key ID"
// @Success		200		{object}	schemaapiopenapi.KeyJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/keys/{id} [get]
func (r *KeysRoutes) get(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := apid.ID(gctx.Param("id"))

	if id.IsNil() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	ek, err := r.core.GetKey(ctx, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("key '%s' not found", id), httperr.WithInternalErr(err)))
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

	resp, err := KeyToResourceOmitUnconfiguredData(ctx, r.core, ek)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, resp); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		Create key
// @Description	Create a new key
// @Tags			keys
// @Accept			json
// @Produce		json
// @Param			request	body		schemaapiopenapi.KeyJson	true	"Key creation request"
// @Success		200		{object}	schemaapiopenapi.KeyJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/keys [post]
func (r *KeysRoutes) create(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req keyschema.Key
	if err := apgin.BindResourceJSON(gctx, &req, meta.ValidationModeCreate); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	if err := val.ValidateNamespace(req.Metadata.Namespace); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	ek, err := r.core.CreateKey(ctx, &req)
	if err != nil {
		if conflictErr := resourceNameConflictError(err, "key", req.Metadata.Name, req.Metadata.Namespace); conflictErr != nil {
			apgin.WriteError(gctx, nil, conflictErr)
			val.MarkErrorReturn()
			return
		}
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	resp, err := KeyToResource(ctx, r.core, ek)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, resp); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		List keys
// @Description	List keys with optional filtering and pagination
// @Tags			keys
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"Pagination cursor"
// @Param			limit			query		integer	false	"Maximum number of results to return"
// @Param			state			query		string	false	"Filter by state"
// @Param			namespace		query		string	false	"Filter by namespace"
// @Param			name			query		string	false	"Filter by exact resource name"
// @Param			labelSelector	query		string	false	"Filter by label selector"
// @Param			orderBy		query		string	false	"Order by field (e.g., 'state:asc')"
// @Success		200				{object}	schemaapiopenapi.ListKeysResponseJson
// @Failure		400				{object}	ErrorResponse
// @Failure		401				{object}	ErrorResponse
// @Failure		500				{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/keys [get]
func (r *KeysRoutes) list(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req ListKeysRequestQueryParams
	if err := gctx.ShouldBindQuery(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	var err error
	var ex coreIface.ListKeysExecutor

	if req.Cursor != nil {
		ex, err = r.core.ListKeysFromCursor(ctx, *req.Cursor)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to list keys from cursor", httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	} else {
		b := r.core.ListKeysBuilder()

		if req.LimitVal != nil {
			b = b.Limit(*req.LimitVal)
		}

		if req.StateVal != nil {
			if !keyschema.IsValidState(*req.StateVal) {
				apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid key state %q", *req.StateVal))
				val.MarkErrorReturn()
				return
			}
			b = b.ForState(database.KeyState(*req.StateVal))
		}

		b = b.ForNamespaceMatchers(val.GetEffectiveNamespaceMatchers(req.NamespaceVal))

		if req.NameVal != nil {
			name := scommon.ResourceName(*req.NameVal)
			if err := name.Validate(); err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid key name: %s", err.Error()))
				val.MarkErrorReturn()
				return
			}
			b = b.ForName(name)
		}

		if req.LabelSelector != nil {
			b = b.ForLabelSelector(*req.LabelSelector)
		}

		if req.OrderByVal != nil {
			field, order, err := pagination.SplitOrderByParam[database.KeyOrderByField](*req.OrderByVal)
			if err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			if !database.IsValidKeyOrderByField(field) {
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

	validated := auth.FilterForValidatedResources(val, result.Results)
	resources := make([]keyschema.Key, 0, len(validated))

	for _, ek := range validated {
		resp, err := KeyToResourceOmitUnconfiguredData(ctx, r.core, ek)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
		if err := resp.ValidateFor(meta.ValidationModeResponse, nil); err != nil {
			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
		resources = append(resources, *resp)
	}

	response := schemaapi.NewListKeysResponseJson(resources, result.Cursor)
	if err := response.Validate(keyschema.KeyKind); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}
	apgin.APIJSON(gctx, http.StatusOK, response)
}

// @Summary		Update key
// @Description	Update a key's desired state, provider configuration, name, labels, or annotations
// @Tags			keys
// @Accept			json
// @Produce		json
// @Param			id		path		string								true	"Key ID"
// @Param			request	body		schemaapiopenapi.KeyPatchJson		true	"Update request"
// @Success		200		{object}	schemaapiopenapi.KeyJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/keys/{id} [patch]
func (r *KeysRoutes) update(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := apid.ID(gctx.Param("id"))

	if id.IsNil() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	var req keyschema.KeyPatch
	if err := apgin.BindResourceJSON(gctx, &req, meta.ValidationModeUpdate); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Get existing key for authorization check
	ek, err := r.core.GetKey(ctx, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("key '%s' not found", id), httperr.WithInternalErr(err)))
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

	before, err := KeyToResourceOmitUnconfiguredData(ctx, r.core, ek)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}
	if _, err := req.ApplyTo(before, nil); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	if req.Metadata.Name != nil {
		name := *req.Metadata.Name
		originalNamespace := ek.GetNamespace()
		ek, err = r.core.UpdateKeyName(ctx, id, name)
		if err != nil {
			if conflictErr := resourceNameConflictError(err, "key", name, originalNamespace); conflictErr != nil {
				apgin.WriteError(gctx, nil, conflictErr)
				val.MarkErrorReturn()
				return
			}
			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	if req.Spec.DesiredState != nil {
		err = r.core.SetKeyState(ctx, id, database.KeyState(*req.Spec.DesiredState))
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("key '%s' not found", id), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	if req.Metadata.Labels != nil {
		_, err = r.core.UpdateKeyLabels(ctx, id, *req.Metadata.Labels)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("key '%s' not found", id), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	if req.Metadata.Annotations != nil {
		_, err = r.core.UpdateKeyAnnotations(ctx, id, *req.Metadata.Annotations)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("key '%s' not found", id), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	if req.Spec.HasKeyData() {
		_, err = r.core.UpdateKeyData(ctx, id, req.Spec.KeyData)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("key '%s' not found", id), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	ek, err = r.core.GetKey(ctx, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("key '%s' not found", id), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	resp, err := KeyToResourceOmitUnconfiguredData(ctx, r.core, ek)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, resp); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		Delete key
// @Description	Soft delete a key
// @Tags			keys
// @Accept			json
// @Produce		json
// @Param			id	path	string	true	"Key ID"
// @Success		204		"No Content"
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/keys/{id} [delete]
func (r *KeysRoutes) delete(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := apid.ID(gctx.Param("id"))

	if id.IsNil() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	if id == database.GlobalKeyID {
		apgin.WriteError(gctx, nil, httperr.BadRequest("the global key cannot be deleted"))
		val.MarkErrorReturn()
		return
	}

	// Get existing key for authorization check
	ek, err := r.core.GetKey(ctx, id)
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

	err = r.core.DeleteKey(ctx, id)
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

// Label and annotation handlers for keys delegate to a shared
// generic adapter (see internal/routes/key_value). The doc comments below
// drive the OpenAPI spec; the bodies forward to the adapter.

// @Summary		Get all labels for a key
// @Description	Get all labels associated with a specific key
// @Tags			keys
// @Produce		json
// @Param			id	path		string	true	"Key ID"
// @Success		200	{object}	map[string]string
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/keys/{id}/labels [get]
func (r *KeysRoutes) getLabels(gctx *gin.Context) { r.labelsAdapter.HandleList(gctx) }

// @Summary		Get a specific label for a key
// @Description	Get a specific label value by key for a key
// @Tags			keys
// @Produce		json
// @Param			id		path		string	true	"Key ID"
// @Param			label	path		string	true	"Label key"
// @Success		200		{object}	KeyValueJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/keys/{id}/labels/{label} [get]
func (r *KeysRoutes) getLabel(gctx *gin.Context) { r.labelsAdapter.HandleGet(gctx) }

// @Summary		Set a label for a key
// @Description	Set or update a specific label value by key for a key
// @Tags			keys
// @Accept			json
// @Produce		json
// @Param			id		path		string						true	"Key ID"
// @Param			label	path		string						true	"Label key"
// @Param			request	body		PutKeyValueRequestJson	true	"Label value"
// @Success		200		{object}	KeyValueJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/keys/{id}/labels/{label} [put]
func (r *KeysRoutes) putLabel(gctx *gin.Context) { r.labelsAdapter.HandlePut(gctx) }

// @Summary		Delete a label from a key
// @Description	Delete a specific label by key from a key
// @Tags			keys
// @Param			id		path	string	true	"Key ID"
// @Param			label	path	string	true	"Label key"
// @Success		204		"No Content"
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/keys/{id}/labels/{label} [delete]
func (r *KeysRoutes) deleteLabel(gctx *gin.Context) { r.labelsAdapter.HandleDelete(gctx) }

// @Summary		Get all annotations for a key
// @Description	Get all annotations associated with a specific key
// @Tags			keys
// @Produce		json
// @Param			id	path		string	true	"Key ID"
// @Success		200	{object}	map[string]string
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/keys/{id}/annotations [get]
func (r *KeysRoutes) getAnnotations(gctx *gin.Context) { r.annotsAdapter.HandleList(gctx) }

// @Summary		Get a specific annotation for a key
// @Description	Get a specific annotation value by key for a key
// @Tags			keys
// @Produce		json
// @Param			id			path		string	true	"Key ID"
// @Param			annotation	path		string	true	"Annotation key"
// @Success		200			{object}	KeyValueJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/keys/{id}/annotations/{annotation} [get]
func (r *KeysRoutes) getAnnotation(gctx *gin.Context) { r.annotsAdapter.HandleGet(gctx) }

// @Summary		Set an annotation for a key
// @Description	Set or update a specific annotation value by key for a key
// @Tags			keys
// @Accept			json
// @Produce		json
// @Param			id			path		string						true	"Key ID"
// @Param			annotation	path		string						true	"Annotation key"
// @Param			request		body		PutKeyValueRequestJson	true	"Annotation value"
// @Success		200			{object}	KeyValueJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/keys/{id}/annotations/{annotation} [put]
func (r *KeysRoutes) putAnnotation(gctx *gin.Context) { r.annotsAdapter.HandlePut(gctx) }

// @Summary		Delete an annotation from a key
// @Description	Delete a specific annotation by key from a key
// @Tags			keys
// @Param			id			path	string	true	"Key ID"
// @Param			annotation	path	string	true	"Annotation key"
// @Success		204			"No Content"
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/keys/{id}/annotations/{annotation} [delete]
func (r *KeysRoutes) deleteAnnotation(gctx *gin.Context) {
	r.annotsAdapter.HandleDelete(gctx)
}

func (r *KeysRoutes) Register(g gin.IRouter) {
	g.GET(
		"/keys",
		r.authService.NewRequiredBuilder().
			ForResource("keys").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.Key).GetId()) }).
			ForVerb("list").
			Build(),
		r.list,
	)
	g.POST(
		"/keys",
		r.authService.NewRequiredBuilder().
			ForResource("keys").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.Key).GetId()) }).
			ForVerb("create").
			Build(),
		r.create,
	)
	g.GET(
		"/keys/:id",
		r.authService.NewRequiredBuilder().
			ForResource("keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.Key).GetId()) }).
			ForVerb("get").
			Build(),
		r.get,
	)
	g.PATCH(
		"/keys/:id",
		r.authService.NewRequiredBuilder().
			ForResource("keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.Key).GetId()) }).
			ForVerb("update").
			Build(),
		r.update,
	)
	g.DELETE(
		"/keys/:id",
		r.authService.NewRequiredBuilder().
			ForResource("keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.Key).GetId()) }).
			ForVerb("delete").
			Build(),
		r.delete,
	)
	g.GET(
		"/keys/:id/labels",
		r.authService.NewRequiredBuilder().
			ForResource("keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.Key).GetId()) }).
			ForVerb("get").
			Build(),
		r.getLabels,
	)
	g.GET(
		"/keys/:id/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.Key).GetId()) }).
			ForVerb("get").
			Build(),
		r.getLabel,
	)
	g.PUT(
		"/keys/:id/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.Key).GetId()) }).
			ForVerb("update").
			Build(),
		r.putLabel,
	)
	g.DELETE(
		"/keys/:id/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.Key).GetId()) }).
			ForVerb("update").
			Build(),
		r.deleteLabel,
	)
	g.GET(
		"/keys/:id/annotations",
		r.authService.NewRequiredBuilder().
			ForResource("keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.Key).GetId()) }).
			ForVerb("get").
			Build(),
		r.getAnnotations,
	)
	g.GET(
		"/keys/:id/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.Key).GetId()) }).
			ForVerb("get").
			Build(),
		r.getAnnotation,
	)
	g.PUT(
		"/keys/:id/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.Key).GetId()) }).
			ForVerb("update").
			Build(),
		r.putAnnotation,
	)
	g.DELETE(
		"/keys/:id/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("keys").
			ForIdField("id").
			ForIdExtractor(func(ek interface{}) string { return string(ek.(coreIface.Key).GetId()) }).
			ForVerb("update").
			Build(),
		r.deleteAnnotation,
	)
}

func NewKeysRoutes(cfg config.C, authService auth.A, c coreIface.C) *KeysRoutes {
	parseKeyID := func(gctx *gin.Context) (apid.ID, *httperr.Error) {
		id := apid.ID(gctx.Param("id"))
		if id.IsNil() {
			return apid.Nil, httperr.BadRequest("id is required")
		}
		return id, nil
	}

	getKey := func(ctx context.Context, id apid.ID) (key_value.Resource, error) {
		ek, err := c.GetKey(ctx, id)
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

	idExtractor := func(ek interface{}) string { return string(ek.(coreIface.Key).GetId()) }

	authGet := authService.NewRequiredBuilder().
		ForResource("keys").
		ForIdField("id").
		ForIdExtractor(idExtractor).
		ForVerb("get").
		Build()
	authMutate := authService.NewRequiredBuilder().
		ForResource("keys").
		ForIdField("id").
		ForIdExtractor(idExtractor).
		ForVerb("update").
		Build()

	labelsAdapter := key_value.Adapter[apid.ID]{
		Kind:         key_value.Label,
		ResourceName: "key",
		PathPrefix:   "/keys/:id",
		AuthGet:      authGet,
		AuthMutate:   authMutate,
		ParseID:      parseKeyID,
		Get:          getKey,
		Put: func(ctx context.Context, id apid.ID, kv map[string]string) (key_value.Resource, error) {
			return c.PutKeyLabels(ctx, id, kv)
		},
		Delete: func(ctx context.Context, id apid.ID, keys []string) (key_value.Resource, error) {
			return c.DeleteKeyLabels(ctx, id, keys)
		},
	}

	annotsAdapter := key_value.Adapter[apid.ID]{
		Kind:         key_value.Annotation,
		ResourceName: "key",
		PathPrefix:   "/keys/:id",
		AuthGet:      authGet,
		AuthMutate:   authMutate,
		ParseID:      parseKeyID,
		Get:          getKey,
		Put: func(ctx context.Context, id apid.ID, kv map[string]string) (key_value.Resource, error) {
			return c.PutKeyAnnotations(ctx, id, kv)
		},
		Delete: func(ctx context.Context, id apid.ID, keys []string) (key_value.Resource, error) {
			return c.DeleteKeyAnnotations(ctx, id, keys)
		},
	}

	return &KeysRoutes{
		cfg:           cfg,
		authService:   authService,
		core:          c,
		labelsAdapter: labelsAdapter,
		annotsAdapter: annotsAdapter,
	}
}
