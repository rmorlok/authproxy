package routes

import (
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
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	schemaapiopenapi "github.com/rmorlok/authproxy/internal/schema/api/openapi"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	namespaceschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

var (
	_ = schemaapiopenapi.RateLimitJson{}
	_ = schemaapiopenapi.RateLimitPatchJson{}
	_ = schemaapiopenapi.ListRateLimitsResponseJson{}
	_ = schemaapiopenapi.RateLimitDryRunActionJson{}
)

type ListRateLimitsRequestQueryParams struct {
	Cursor        *string `form:"cursor"`
	LimitVal      *int32  `form:"limit"`
	NamespaceVal  *string `form:"namespace"`
	NameVal       *string `form:"name"`
	LabelSelector *string `form:"labelSelector"`
	OrderByVal    *string `form:"orderBy"`
}

func RateLimitToResource(r coreIface.RateLimit) *rlschema.RateLimit {
	return r.GetResource()
}

type RateLimitsRoutes struct {
	cfg         config.C
	core        coreIface.C
	authService auth.A
}

// dryRunRequestToCore translates the wire request to the structured input the core
// service consumes. Nothing here does business logic — it's just shape.
func dryRunRequestToCore(r schemaapi.RateLimitDryRunAction) coreIface.DryRunRateLimitRequest {
	var connectionID *apid.ID
	var namespace *string
	switch r.Metadata.Target.Kind {
	case connectionschema.ConnectionKind:
		value := apid.ID(r.Metadata.Target.ID)
		connectionID = &value
	case namespaceschema.NamespaceKind:
		value := r.Metadata.Target.ID
		namespace = &value
	}
	var actorID *apid.ID
	if r.Spec.ActorRef != nil && r.Spec.ActorRef.Kind == actorschema.ActorKind {
		value := apid.ID(r.Spec.ActorRef.ID)
		actorID = &value
	}
	return coreIface.DryRunRateLimitRequest{
		Request: coreIface.ProxyRequest{
			URL:      r.Spec.Request.URL,
			Method:   r.Spec.Request.Method,
			Headers:  r.Spec.Request.Headers,
			Labels:   r.Spec.Request.Labels,
			BodyRaw:  r.Spec.Request.BodyRaw,
			BodyJson: r.Spec.Request.BodyJson,
		},
		RequestType: r.Spec.RequestType,
		Context: coreIface.DryRunRequestContext{
			ConnectionId: connectionID,
			ActorId:      actorID,
			Namespace:    namespace,
		},
	}
}

func dryRunResponseFromCore(res coreIface.DryRunRateLimitResult) schemaapi.RateLimitDryRunStatus {
	matched := make([]schemaapi.DryRunMatchJson, len(res.Matched))
	for i, m := range res.Matched {
		matched[i] = schemaapi.DryRunMatchJson{
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
	notMatched := make([]schemaapi.DryRunNotMatchedJson, len(res.NotMatched))
	for i, nm := range res.NotMatched {
		notMatched[i] = schemaapi.DryRunNotMatchedJson{
			RateLimitId: nm.RateLimitId,
			Namespace:   nm.Namespace,
			Reason:      nm.Reason,
		}
	}
	return schemaapi.RateLimitDryRunStatus{
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
// @Success		200		{object}	schemaapiopenapi.RateLimitJson
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

	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, RateLimitToResource(rl)); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		Create rate limit
// @Description	Create a new rate limit resource
// @Tags			rate_limits
// @Accept			json
// @Produce		json
// @Param			request	body		schemaapiopenapi.RateLimitJson	true	"Rate limit creation request"
// @Success		200		{object}	schemaapiopenapi.RateLimitJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/rate-limits [post]
func (r *RateLimitsRoutes) create(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req rlschema.RateLimit
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

	rl, err := r.core.CreateRateLimit(ctx, &req)
	if err != nil {
		if conflictErr := resourceNameConflictError(err, "rate limit", req.Metadata.Name, req.Metadata.Namespace); conflictErr != nil {
			apgin.WriteError(gctx, nil, conflictErr)
			val.MarkErrorReturn()
			return
		}
		if errors.Is(err, core.ErrInvalidArgument) || errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, RateLimitToResource(rl)); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		List rate limits
// @Description	List rate limits with optional filtering and pagination
// @Tags			rate_limits
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"Pagination cursor"
// @Param			limit			query		integer	false	"Maximum number of results to return"
// @Param			namespace		query		string	false	"Filter by namespace"
// @Param			name			query		string	false	"Filter by exact name"
// @Param			labelSelector	query		string	false	"Filter by label selector"
// @Param			orderBy		query		string	false	"Order by field (e.g., 'created_at:desc')"
// @Success		200				{object}	schemaapiopenapi.ListRateLimitsResponseJson
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

		if req.NameVal != nil {
			name := scommon.ResourceName(*req.NameVal)
			if err := name.Validate(); err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid rate limit name: %s", err.Error()))
				val.MarkErrorReturn()
				return
			}
			b = b.ForName(name)
		}

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

	apgin.APIJSON(gctx, http.StatusOK, schemaapi.NewListRateLimitsResponseJson(
		util.Map(auth.FilterForValidatedResources(val, result.Results), func(value coreIface.RateLimit) rlschema.RateLimit {
			return *RateLimitToResource(value)
		}),
		result.Cursor,
	))
}

// @Summary		Update rate limit
// @Description	Update a rate limit's name, spec, labels, or annotations
// @Tags			rate_limits
// @Accept			json
// @Produce		json
// @Param			id		path		string							true	"Rate limit ID"
// @Param			request	body		schemaapiopenapi.RateLimitPatchJson	true	"Update request"
// @Success		200		{object}	schemaapiopenapi.RateLimitJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
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

	var req rlschema.RateLimitPatch
	if err := apgin.BindResourceJSON(gctx, &req, meta.ValidationModeUpdate); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
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

	updated, err := req.ApplyTo(rl.GetResource(), nil)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	originalNamespace := rl.GetNamespace()
	rl, err = r.core.UpdateRateLimit(ctx, id, updated)
	if err != nil {
		if req.Metadata.Name != nil {
			if conflictErr := resourceNameConflictError(err, "rate limit", *req.Metadata.Name, originalNamespace); conflictErr != nil {
				apgin.WriteError(gctx, nil, conflictErr)
				val.MarkErrorReturn()
				return
			}
		}
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("rate limit '%s' not found", id), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
		if errors.Is(err, core.ErrInvalidArgument) {
			apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, RateLimitToResource(rl)); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
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
// @Param			request	body		schemaapiopenapi.RateLimitDryRunActionJson	true	"Dry-run action"
// @Success		200		{object}	schemaapiopenapi.RateLimitDryRunActionJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/rate-limits/_dryRun [post]
func (r *RateLimitsRoutes) dryRun(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req schemaapi.RateLimitDryRunAction
	if err := apgin.BindActionJSON(gctx, &req, schemaapi.RateLimitDryRunActionKind); err != nil {
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

	response := schemaapi.NewRateLimitDryRunResponse(
		req.Metadata.Target,
		req.Spec,
		dryRunResponseFromCore(result),
	)
	if err := apgin.RenderActionJSON(gctx, http.StatusOK, &response, schemaapi.RateLimitDryRunActionKind); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
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
		"/rate-limits/_dryRun",
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
}

func NewRateLimitsRoutes(cfg config.C, authService auth.A, c coreIface.C) *RateLimitsRoutes {
	return &RateLimitsRoutes{
		cfg:         cfg,
		authService: authService,
		core:        c,
	}
}
