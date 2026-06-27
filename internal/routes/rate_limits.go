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
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type RateLimitJson = schemaapi.RateLimitJson
type CreateRateLimitRequestJson = schemaapi.CreateRateLimitRequestJson
type UpdateRateLimitRequestJson = schemaapi.UpdateRateLimitRequestJson
type ListRateLimitsResponseJson = schemaapi.ListRateLimitsResponseJson
type DryRunRequestJson = schemaapi.DryRunRequestJson
type DryRunContextJson = schemaapi.DryRunContextJson
type DryRunResponseJson = schemaapi.DryRunResponseJson
type DryRunMatchJson = schemaapi.DryRunMatchJson
type DryRunNotMatchedJson = schemaapi.DryRunNotMatchedJson

type OpenAPIRateLimitJson = schemaapiopenapi.RateLimitJson
type OpenAPIListRateLimitsResponseJson = schemaapiopenapi.ListRateLimitsResponseJson
type OpenAPICreateRateLimitRequestJson = schemaapiopenapi.CreateRateLimitRequestJson
type OpenAPIUpdateRateLimitRequestJson = schemaapiopenapi.UpdateRateLimitRequestJson
type OpenAPIDryRunRequestJson = schemaapiopenapi.DryRunRequestJson
type OpenAPIDryRunResponseJson = schemaapiopenapi.DryRunResponseJson

type ListRateLimitsRequestQueryParams struct {
	Cursor        *string `form:"cursor"`
	LimitVal      *int32  `form:"limit"`
	NamespaceVal  *string `form:"namespace"`
	LabelSelector *string `form:"label_selector"`
	OrderByVal    *string `form:"order_by"`
}

func RateLimitToJson(r coreIface.RateLimit) RateLimitJson {
	return RateLimitJson{
		Id:          r.GetId(),
		Namespace:   r.GetNamespace(),
		Definition:  r.GetDefinition(),
		Labels:      r.GetLabels(),
		Annotations: r.GetAnnotations(),
		CreatedAt:   r.GetCreatedAt(),
		UpdatedAt:   r.GetUpdatedAt(),
	}
}

type RateLimitsRoutes struct {
	cfg           config.C
	core          coreIface.C
	authService   auth.A
	labelsAdapter key_value.Adapter[apid.ID]
	annotsAdapter key_value.Adapter[apid.ID]
}

// dryRunRequestToCore translates the wire request to the structured input the core
// service consumes. Nothing here does business logic — it's just shape.
func dryRunRequestToCore(r DryRunRequestJson) coreIface.DryRunRateLimitRequest {
	return coreIface.DryRunRateLimitRequest{
		Request: coreIface.ProxyRequest{
			URL:      r.Request.URL,
			Method:   r.Request.Method,
			Headers:  r.Request.Headers,
			Labels:   r.Request.Labels,
			BodyRaw:  r.Request.BodyRaw,
			BodyJson: r.Request.BodyJson,
		},
		RequestType: r.RequestType,
		Context: coreIface.DryRunRequestContext{
			ConnectionId: r.Context.ConnectionId,
			ActorId:      r.Context.ActorId,
			Namespace:    r.Context.Namespace,
		},
	}
}

func dryRunResponseFromCore(res coreIface.DryRunRateLimitResult) DryRunResponseJson {
	matched := make([]DryRunMatchJson, len(res.Matched))
	for i, m := range res.Matched {
		matched[i] = DryRunMatchJson{
			RateLimitId:      m.RateLimitId,
			Namespace:        m.Namespace,
			EffectiveMode:    m.EffectiveMode,
			BucketKey:        m.BucketKey,
			AlgorithmSummary: m.AlgorithmSummary,
			WouldAllow:       m.WouldAllow,
			Remaining:        m.Remaining,
			RetryAfterMs:     m.RetryAfterMs,
			PeekFailed:       m.PeekFailed,
		}
	}
	notMatched := make([]DryRunNotMatchedJson, len(res.NotMatched))
	for i, nm := range res.NotMatched {
		notMatched[i] = DryRunNotMatchedJson{
			RateLimitId: nm.RateLimitId,
			Namespace:   nm.Namespace,
			Reason:      nm.Reason,
		}
	}
	return DryRunResponseJson{
		RequestLabelSnapshot: res.RequestLabelSnapshot,
		Matched:              matched,
		NotMatched:           notMatched,
	}
}

// @Summary		Get rate limit
// @Description	Get a specific rate limit by ID
// @Tags			rate_limits
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Rate limit ID"
// @Success		200		{object}	OpenAPIRateLimitJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/rate-limits/{id} [get]
func (r *RateLimitsRoutes) get(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := apid.ID(gctx.Param("id"))
	if id.IsNil() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	rl, err := r.core.GetRateLimit(ctx, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("rate limit '%s' not found", id), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(rl); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, RateLimitToJson(rl))
}

// @Summary		Create rate limit
// @Description	Create a new rate limit resource
// @Tags			rate_limits
// @Accept			json
// @Produce		json
// @Param			request	body		OpenAPICreateRateLimitRequestJson	true	"Rate limit creation request"
// @Success		200		{object}	OpenAPIRateLimitJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/rate-limits [post]
func (r *RateLimitsRoutes) create(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req CreateRateLimitRequestJson
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

	if err := req.Definition.Validate(); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid definition: %s", err.Error()))
		val.MarkErrorReturn()
		return
	}

	if req.Labels != nil {
		if err := database.ValidateUserLabels(req.Labels); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid labels: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
	}

	if req.Annotations != nil {
		if err := database.Annotations(req.Annotations).Validate(); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid annotations: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
	}

	rl, err := r.core.CreateRateLimit(ctx, req.Namespace, req.Definition, req.Labels, req.Annotations)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, RateLimitToJson(rl))
}

// @Summary		List rate limits
// @Description	List rate limits with optional filtering and pagination
// @Tags			rate_limits
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"Pagination cursor"
// @Param			limit			query		integer	false	"Maximum number of results to return"
// @Param			namespace		query		string	false	"Filter by namespace"
// @Param			label_selector	query		string	false	"Filter by label selector"
// @Param			order_by		query		string	false	"Order by field (e.g., 'created_at:desc')"
// @Success		200				{object}	OpenAPIListRateLimitsResponseJson
// @Failure		400				{object}	ErrorResponse
// @Failure		401				{object}	ErrorResponse
// @Failure		500				{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/rate-limits [get]
func (r *RateLimitsRoutes) list(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req ListRateLimitsRequestQueryParams
	if err := gctx.ShouldBindQuery(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	var err error
	var ex coreIface.ListRateLimitsExecutor

	if req.Cursor != nil {
		ex, err = r.core.ListRateLimitsFromCursor(ctx, *req.Cursor)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to list rate limits from cursor", httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	} else {
		b := r.core.ListRateLimitsBuilder()

		if req.LimitVal != nil {
			b = b.Limit(*req.LimitVal)
		}

		b = b.ForNamespaceMatchers(val.GetEffectiveNamespaceMatchers(req.NamespaceVal))

		if req.LabelSelector != nil {
			b = b.ForLabelSelector(*req.LabelSelector)
		}

		if req.OrderByVal != nil {
			field, order, err := pagination.SplitOrderByParam[database.RateLimitOrderByField](*req.OrderByVal)
			if err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			if !database.IsValidRateLimitOrderByField(field) {
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

	apgin.APIJSON(gctx, http.StatusOK, ListRateLimitsResponseJson{
		Items:  util.Map(auth.FilterForValidatedResources(val, result.Results), RateLimitToJson),
		Cursor: result.Cursor,
	})
}

// @Summary		Update rate limit
// @Description	Update a rate limit's definition, labels, or annotations
// @Tags			rate_limits
// @Accept			json
// @Produce		json
// @Param			id		path		string							true	"Rate limit ID"
// @Param			request	body		OpenAPIUpdateRateLimitRequestJson		true	"Update request"
// @Success		200		{object}	OpenAPIRateLimitJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/rate-limits/{id} [patch]
func (r *RateLimitsRoutes) update(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := apid.ID(gctx.Param("id"))
	if id.IsNil() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	var req UpdateRateLimitRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if req.Definition != nil {
		if err := req.Definition.Validate(); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid definition: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
	}

	if req.Labels != nil {
		if err := database.ValidateUserLabels(*req.Labels); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid labels: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
	}

	if req.Annotations != nil {
		if err := database.Annotations(*req.Annotations).Validate(); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid annotations: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
	}

	rl, err := r.core.GetRateLimit(ctx, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("rate limit '%s' not found", id), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(rl); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	if req.Definition != nil {
		if _, err := r.core.UpdateRateLimitDefinition(ctx, id, *req.Definition); err != nil {
			r.handleMutateError(gctx, val, id, err)
			return
		}
	}

	if req.Labels != nil {
		if _, err := r.core.UpdateRateLimitLabels(ctx, id, *req.Labels); err != nil {
			r.handleMutateError(gctx, val, id, err)
			return
		}
	}

	if req.Annotations != nil {
		if _, err := r.core.UpdateRateLimitAnnotations(ctx, id, *req.Annotations); err != nil {
			r.handleMutateError(gctx, val, id, err)
			return
		}
	}

	rl, err = r.core.GetRateLimit(ctx, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("rate limit '%s' not found", id), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, RateLimitToJson(rl))
}

func (r *RateLimitsRoutes) handleMutateError(gctx *gin.Context, val *auth.ResourcePermissionValidator, id apid.ID, err error) {
	if errors.Is(err, core.ErrNotFound) {
		apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("rate limit '%s' not found", id), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}
	apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
	val.MarkErrorReturn()
}

// @Summary		Delete rate limit
// @Description	Soft delete a rate limit
// @Tags			rate_limits
// @Accept			json
// @Produce		json
// @Param			id	path	string	true	"Rate limit ID"
// @Success		204		"No Content"
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/rate-limits/{id} [delete]
func (r *RateLimitsRoutes) delete(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := apid.ID(gctx.Param("id"))
	if id.IsNil() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	rl, err := r.core.GetRateLimit(ctx, id)
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

	if httpErr := val.ValidateHttpStatusError(rl); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	if err := r.core.DeleteRateLimit(ctx, id); err != nil {
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

// @Summary		Dry-run a rate-limit evaluation
// @Description	Evaluate which rate-limit rules would apply to a synthesized request, and whether each would limit it. Counters are NOT incremented — the endpoint uses Limiter.Peek to inspect counter state without writing. Useful for validating selectors / buckets / algorithms without sending real traffic.
// @Tags			rate_limits
// @Accept			json
// @Produce		json
// @Param			request	body		OpenAPIDryRunRequestJson	true	"Dry-run input"
// @Success		200		{object}	OpenAPIDryRunResponseJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/rate-limits/_dry_run [post]
func (r *RateLimitsRoutes) dryRun(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req DryRunRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	result, err := r.core.DryRunRateLimit(ctx, dryRunRequestToCore(req))
	if err != nil {
		switch {
		case errors.Is(err, core.ErrInvalidArgument):
			apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		case errors.Is(err, core.ErrNotFound):
			apgin.WriteError(gctx, nil, httperr.NotFound(err.Error(), httperr.WithInternalErr(err)))
		default:
			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		}
		val.MarkErrorReturn()
		return
	}

	// Namespace permission check happens *after* hydration so a
	// connection-driven dry-run is validated against the connection's
	// namespace, not whatever the caller guessed.
	if err := val.ValidateNamespace(result.Namespace); err != nil {
		apgin.WriteError(gctx, nil, httperr.Forbidden(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, dryRunResponseFromCore(result))
}

// Label and annotation handlers delegate to the shared key_value adapter.

// @Summary		Get all labels for a rate limit
// @Description	Get all labels associated with a specific rate limit
// @Tags			rate_limits
// @Produce		json
// @Param			id	path		string	true	"Rate limit ID"
// @Success		200	{object}	map[string]string
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/rate-limits/{id}/labels [get]
func (r *RateLimitsRoutes) getLabels(gctx *gin.Context) { r.labelsAdapter.HandleList(gctx) }

// @Summary		Get a specific label for a rate limit
// @Description	Get a specific label value by key for a rate limit
// @Tags			rate_limits
// @Produce		json
// @Param			id		path		string	true	"Rate limit ID"
// @Param			label	path		string	true	"Label key"
// @Success		200		{object}	KeyValueJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/rate-limits/{id}/labels/{label} [get]
func (r *RateLimitsRoutes) getLabel(gctx *gin.Context) { r.labelsAdapter.HandleGet(gctx) }

// @Summary		Set a label for a rate limit
// @Description	Set or update a specific label value by key for a rate limit
// @Tags			rate_limits
// @Accept			json
// @Produce		json
// @Param			id		path		string						true	"Rate limit ID"
// @Param			label	path		string						true	"Label key"
// @Param			request	body		PutKeyValueRequestJson	true	"Label value"
// @Success		200		{object}	KeyValueJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/rate-limits/{id}/labels/{label} [put]
func (r *RateLimitsRoutes) putLabel(gctx *gin.Context) { r.labelsAdapter.HandlePut(gctx) }

// @Summary		Delete a label from a rate limit
// @Description	Delete a specific label by key from a rate limit
// @Tags			rate_limits
// @Param			id		path	string	true	"Rate limit ID"
// @Param			label	path	string	true	"Label key"
// @Success		204		"No Content"
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/rate-limits/{id}/labels/{label} [delete]
func (r *RateLimitsRoutes) deleteLabel(gctx *gin.Context) { r.labelsAdapter.HandleDelete(gctx) }

// @Summary		Get all annotations for a rate limit
// @Description	Get all annotations associated with a specific rate limit
// @Tags			rate_limits
// @Produce		json
// @Param			id	path		string	true	"Rate limit ID"
// @Success		200	{object}	map[string]string
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/rate-limits/{id}/annotations [get]
func (r *RateLimitsRoutes) getAnnotations(gctx *gin.Context) { r.annotsAdapter.HandleList(gctx) }

// @Summary		Get a specific annotation for a rate limit
// @Description	Get a specific annotation value by key for a rate limit
// @Tags			rate_limits
// @Produce		json
// @Param			id			path		string	true	"Rate limit ID"
// @Param			annotation	path		string	true	"Annotation key"
// @Success		200			{object}	KeyValueJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/rate-limits/{id}/annotations/{annotation} [get]
func (r *RateLimitsRoutes) getAnnotation(gctx *gin.Context) { r.annotsAdapter.HandleGet(gctx) }

// @Summary		Set an annotation for a rate limit
// @Description	Set or update a specific annotation value by key for a rate limit
// @Tags			rate_limits
// @Accept			json
// @Produce		json
// @Param			id			path		string						true	"Rate limit ID"
// @Param			annotation	path		string						true	"Annotation key"
// @Param			request		body		PutKeyValueRequestJson	true	"Annotation value"
// @Success		200			{object}	KeyValueJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/rate-limits/{id}/annotations/{annotation} [put]
func (r *RateLimitsRoutes) putAnnotation(gctx *gin.Context) { r.annotsAdapter.HandlePut(gctx) }

// @Summary		Delete an annotation from a rate limit
// @Description	Delete a specific annotation by key from a rate limit
// @Tags			rate_limits
// @Param			id			path	string	true	"Rate limit ID"
// @Param			annotation	path	string	true	"Annotation key"
// @Success		204			"No Content"
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/rate-limits/{id}/annotations/{annotation} [delete]
func (r *RateLimitsRoutes) deleteAnnotation(gctx *gin.Context) {
	r.annotsAdapter.HandleDelete(gctx)
}

func (r *RateLimitsRoutes) Register(g gin.IRouter) {
	idExtractor := func(rl interface{}) string {
		return string(rl.(coreIface.RateLimit).GetId())
	}

	g.GET(
		"/rate-limits",
		r.authService.NewRequiredBuilder().
			ForResource("rate_limits").
			ForIdExtractor(idExtractor).
			ForVerb("list").
			Build(),
		r.list,
	)
	g.POST(
		"/rate-limits",
		r.authService.NewRequiredBuilder().
			ForResource("rate_limits").
			ForIdExtractor(idExtractor).
			ForVerb("create").
			Build(),
		r.create,
	)
	g.POST(
		"/rate-limits/_dry_run",
		r.authService.NewRequiredBuilder().
			ForResource("rate_limits").
			ForIdExtractor(idExtractor).
			ForVerb("get").
			Build(),
		r.dryRun,
	)
	g.GET(
		"/rate-limits/:id",
		r.authService.NewRequiredBuilder().
			ForResource("rate_limits").
			ForIdField("id").
			ForIdExtractor(idExtractor).
			ForVerb("get").
			Build(),
		r.get,
	)
	g.PATCH(
		"/rate-limits/:id",
		r.authService.NewRequiredBuilder().
			ForResource("rate_limits").
			ForIdField("id").
			ForIdExtractor(idExtractor).
			ForVerb("update").
			Build(),
		r.update,
	)
	g.DELETE(
		"/rate-limits/:id",
		r.authService.NewRequiredBuilder().
			ForResource("rate_limits").
			ForIdField("id").
			ForIdExtractor(idExtractor).
			ForVerb("delete").
			Build(),
		r.delete,
	)
	g.GET(
		"/rate-limits/:id/labels",
		r.authService.NewRequiredBuilder().
			ForResource("rate_limits").
			ForIdField("id").
			ForIdExtractor(idExtractor).
			ForVerb("get").
			Build(),
		r.getLabels,
	)
	g.GET(
		"/rate-limits/:id/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("rate_limits").
			ForIdField("id").
			ForIdExtractor(idExtractor).
			ForVerb("get").
			Build(),
		r.getLabel,
	)
	g.PUT(
		"/rate-limits/:id/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("rate_limits").
			ForIdField("id").
			ForIdExtractor(idExtractor).
			ForVerb("update").
			Build(),
		r.putLabel,
	)
	g.DELETE(
		"/rate-limits/:id/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("rate_limits").
			ForIdField("id").
			ForIdExtractor(idExtractor).
			ForVerb("update").
			Build(),
		r.deleteLabel,
	)
	g.GET(
		"/rate-limits/:id/annotations",
		r.authService.NewRequiredBuilder().
			ForResource("rate_limits").
			ForIdField("id").
			ForIdExtractor(idExtractor).
			ForVerb("get").
			Build(),
		r.getAnnotations,
	)
	g.GET(
		"/rate-limits/:id/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("rate_limits").
			ForIdField("id").
			ForIdExtractor(idExtractor).
			ForVerb("get").
			Build(),
		r.getAnnotation,
	)
	g.PUT(
		"/rate-limits/:id/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("rate_limits").
			ForIdField("id").
			ForIdExtractor(idExtractor).
			ForVerb("update").
			Build(),
		r.putAnnotation,
	)
	g.DELETE(
		"/rate-limits/:id/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("rate_limits").
			ForIdField("id").
			ForIdExtractor(idExtractor).
			ForVerb("update").
			Build(),
		r.deleteAnnotation,
	)
}

func NewRateLimitsRoutes(cfg config.C, authService auth.A, c coreIface.C) *RateLimitsRoutes {
	parseRateLimitID := func(gctx *gin.Context) (apid.ID, *httperr.Error) {
		id := apid.ID(gctx.Param("id"))
		if id.IsNil() {
			return apid.Nil, httperr.BadRequest("id is required")
		}
		return id, nil
	}

	getRateLimit := func(ctx context.Context, id apid.ID) (key_value.Resource, error) {
		rl, err := c.GetRateLimit(ctx, id)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return nil, database.ErrNotFound
			}
			return nil, err
		}
		if rl == nil {
			return nil, nil
		}
		return rl, nil
	}

	idExtractor := func(rl interface{}) string {
		return string(rl.(coreIface.RateLimit).GetId())
	}

	authGet := authService.NewRequiredBuilder().
		ForResource("rate_limits").
		ForIdField("id").
		ForIdExtractor(idExtractor).
		ForVerb("get").
		Build()
	authMutate := authService.NewRequiredBuilder().
		ForResource("rate_limits").
		ForIdField("id").
		ForIdExtractor(idExtractor).
		ForVerb("update").
		Build()

	labelsAdapter := key_value.Adapter[apid.ID]{
		Kind:         key_value.Label,
		ResourceName: "rate limit",
		PathPrefix:   "/rate-limits/:id",
		AuthGet:      authGet,
		AuthMutate:   authMutate,
		ParseID:      parseRateLimitID,
		Get:          getRateLimit,
		Put: func(ctx context.Context, id apid.ID, kv map[string]string) (key_value.Resource, error) {
			return c.PutRateLimitLabels(ctx, id, kv)
		},
		Delete: func(ctx context.Context, id apid.ID, keys []string) (key_value.Resource, error) {
			return c.DeleteRateLimitLabels(ctx, id, keys)
		},
	}

	annotsAdapter := key_value.Adapter[apid.ID]{
		Kind:         key_value.Annotation,
		ResourceName: "rate limit",
		PathPrefix:   "/rate-limits/:id",
		AuthGet:      authGet,
		AuthMutate:   authMutate,
		ParseID:      parseRateLimitID,
		Get:          getRateLimit,
		Put: func(ctx context.Context, id apid.ID, kv map[string]string) (key_value.Resource, error) {
			return c.PutRateLimitAnnotations(ctx, id, kv)
		},
		Delete: func(ctx context.Context, id apid.ID, keys []string) (key_value.Resource, error) {
			return c.DeleteRateLimitAnnotations(ctx, id, keys)
		},
	}

	return &RateLimitsRoutes{
		cfg:           cfg,
		authService:   authService,
		core:          c,
		labelsAdapter: labelsAdapter,
		annotsAdapter: annotsAdapter,
	}
}
