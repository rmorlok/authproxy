package routes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/ratelimit"
	"github.com/rmorlok/authproxy/internal/routes/key_value"
	"github.com/rmorlok/authproxy/internal/schema/common"
	rlschema "github.com/rmorlok/authproxy/internal/schema/rate_limit"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type RateLimitJson struct {
	Id          apid.ID            `json:"id"`
	Namespace   string             `json:"namespace"`
	Definition  rlschema.RateLimit `json:"definition"`
	Labels      map[string]string  `json:"labels,omitempty"`
	Annotations map[string]string  `json:"annotations,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

type CreateRateLimitRequestJson struct {
	Namespace   string             `json:"namespace"`
	Definition  rlschema.RateLimit `json:"definition"`
	Labels      map[string]string  `json:"labels,omitempty"`
	Annotations map[string]string  `json:"annotations,omitempty"`
}

type UpdateRateLimitRequestJson struct {
	Definition  *rlschema.RateLimit `json:"definition,omitempty"`
	Labels      *map[string]string  `json:"labels,omitempty"`
	Annotations *map[string]string  `json:"annotations,omitempty"`
}

type ListRateLimitsRequestQueryParams struct {
	Cursor        *string `form:"cursor"`
	LimitVal      *int32  `form:"limit"`
	NamespaceVal  *string `form:"namespace"`
	LabelSelector *string `form:"label_selector"`
	OrderByVal    *string `form:"order_by"`
}

type ListRateLimitsResponseJson struct {
	Items  []RateLimitJson `json:"items"`
	Cursor string          `json:"cursor,omitempty"`
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
	db            database.DB
	rlCache       ratelimit.Cache
	redis         apredis.Client
	logger        *slog.Logger
	labelsAdapter key_value.Adapter[apid.ID]
	annotsAdapter key_value.Adapter[apid.ID]
}

// DryRunRequestJson is the input shape for POST /rate-limits/_dry_run.
// The two halves cleanly separate the request the operator wants to
// simulate (Request) from the actor / connection identity it runs under
// (Context).
type DryRunRequestJson struct {
	Request DryRunRequestPayloadJson `json:"request"`
	Context DryRunContextJson        `json:"context"`
}

// DryRunRequestPayloadJson mirrors the fields ratelimit.RequestContext
// consumes from the in-flight proxy request: method, path, request type.
// Headers are accepted but unused by the matcher today — kept on the wire
// so the same form can drive future "actually proxy this" workflows.
type DryRunRequestPayloadJson struct {
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	RequestType string            `json:"request_type"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// DryRunContextJson is the identity + label context the request runs
// under. Either a Connection (everything is hydrated from it) or raw
// fields. Manual Labels always override carry-forward.
type DryRunContextJson struct {
	ConnectionId *apid.ID          `json:"connection_id,omitempty"`
	ActorId      *apid.ID          `json:"actor_id,omitempty"`
	Namespace    *string           `json:"namespace,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
}

type DryRunResponseJson struct {
	RequestLabelSnapshot map[string]string       `json:"request_label_snapshot"`
	Matched              []DryRunMatchJson       `json:"matched"`
	NotMatched           []DryRunNotMatchedJson  `json:"not_matched"`
}

type DryRunMatchJson struct {
	RateLimitId      apid.ID `json:"rate_limit_id"`
	Namespace        string  `json:"namespace"`
	EffectiveMode    string  `json:"effective_mode"`
	BucketKey        string  `json:"bucket_key"`
	AlgorithmSummary string  `json:"algorithm_summary"`
	WouldAllow       bool    `json:"would_allow"`
	Remaining        int     `json:"remaining"`
	RetryAfterMs     int64   `json:"retry_after_ms"`
	PeekFailed       bool    `json:"peek_failed"`
}

type DryRunNotMatchedJson struct {
	RateLimitId apid.ID `json:"rate_limit_id"`
	Namespace   string  `json:"namespace"`
	Reason      string  `json:"reason"`
}

// @Summary		Get rate limit
// @Description	Get a specific rate limit by ID
// @Tags			rate_limits
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Rate limit ID"
// @Success		200		{object}	SwaggerRateLimitJson
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

	gctx.PureJSON(http.StatusOK, RateLimitToJson(rl))
}

// @Summary		Create rate limit
// @Description	Create a new rate limit resource
// @Tags			rate_limits
// @Accept			json
// @Produce		json
// @Param			request	body		CreateRateLimitRequestJson	true	"Rate limit creation request"
// @Success		200		{object}	SwaggerRateLimitJson
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

	gctx.PureJSON(http.StatusOK, RateLimitToJson(rl))
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
// @Success		200				{object}	SwaggerListRateLimitsResponse
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

	gctx.PureJSON(http.StatusOK, ListRateLimitsResponseJson{
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
// @Param			request	body		UpdateRateLimitRequestJson		true	"Update request"
// @Success		200		{object}	SwaggerRateLimitJson
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

	gctx.PureJSON(http.StatusOK, RateLimitToJson(rl))
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
// @Param			request	body		SwaggerDryRunRequest	true	"Dry-run input"
// @Success		200		{object}	SwaggerDryRunResponse
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
	logger := aplog.NewBuilder(r.logger).WithCtx(ctx).Build()

	var req DryRunRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	if req.Request.Method == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("request.method is required"))
		val.MarkErrorReturn()
		return
	}
	if req.Request.RequestType == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("request.request_type is required"))
		val.MarkErrorReturn()
		return
	}
	if !common.IsValidRequestType(req.Request.RequestType) {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid request.request_type %q", req.Request.RequestType))
		val.MarkErrorReturn()
		return
	}
	if req.Context.ConnectionId == nil && req.Context.Namespace == nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("context.connection_id or context.namespace is required"))
		val.MarkErrorReturn()
		return
	}

	reqCtx, namespace, httpErr := r.hydrateDryRunContext(ctx, req)
	if httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		val.MarkErrorReturn()
		return
	}

	if err := val.ValidateNamespace(namespace); err != nil {
		apgin.WriteError(gctx, nil, httperr.Forbidden(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Run against the same ruleset the enforcer sees so operators
	// observe propagation delay if any. The cache is empty until the
	// refresher has installed its first snapshot — we treat that as
	// "no rules to evaluate" rather than an error.
	rules := r.rlCache.All()
	rules = filterRulesInScope(rules, namespace)

	matched := make([]DryRunMatchJson, 0)
	notMatched := make([]DryRunNotMatchedJson, 0)

	for _, rule := range rules {
		if rule == nil {
			continue
		}
		ok, bucket, reason, err := ratelimit.MatchExplain(rule.Definition, reqCtx)
		if err != nil {
			// Malformed rule (e.g. uncompilable regex that escaped
			// validation) — surface as a miss with the error text so
			// operators can see something went wrong without 500ing the
			// whole dry-run.
			notMatched = append(notMatched, DryRunNotMatchedJson{
				RateLimitId: rule.Id,
				Namespace:   rule.Namespace,
				Reason:      fmt.Sprintf("malformed rule: %s", err.Error()),
			})
			continue
		}
		if !ok {
			notMatched = append(notMatched, DryRunNotMatchedJson{
				RateLimitId: rule.Id,
				Namespace:   rule.Namespace,
				Reason:      reason,
			})
			continue
		}

		limiter, lerr := ratelimit.NewLimiter(rule, r.redis, logger)
		if lerr != nil {
			logger.Warn("dry-run: failed to construct limiter; skipping rule",
				slog.String("rule_id", string(rule.Id)),
				slog.String("error", lerr.Error()),
			)
			continue
		}

		decision, peekErr := limiter.Peek(ctx, bucket)
		match := DryRunMatchJson{
			RateLimitId:      rule.Id,
			Namespace:        rule.Namespace,
			EffectiveMode:    string(rule.Definition.EffectiveMode()),
			BucketKey:        bucket.String(),
			AlgorithmSummary: algorithmSummary(rule.Definition),
			WouldAllow:       decision.Allowed,
			Remaining:        decision.Remaining,
			RetryAfterMs:     decision.RetryAfter.Milliseconds(),
			PeekFailed:       decision.FailedOpen,
		}
		// Peek's fail-open returns the underlying error so logs stay
		// useful — but the user-facing field is decision.FailedOpen,
		// already populated by the limiter.
		_ = peekErr
		matched = append(matched, match)
	}

	gctx.PureJSON(http.StatusOK, DryRunResponseJson{
		RequestLabelSnapshot: reqCtx.Labels,
		Matched:              matched,
		NotMatched:           notMatched,
	})
}

// hydrateDryRunContext fills a RequestContext from the dry-run input.
// When a connection_id is supplied, namespace / connector / labels come
// from it (mirroring how httpf.ForConnection populates a real request).
// Otherwise raw context fields are used; manual labels always win over
// any other source.
func (r *RateLimitsRoutes) hydrateDryRunContext(ctx context.Context, req DryRunRequestJson) (*ratelimit.RequestContext, string, *httperr.Error) {
	rc := &ratelimit.RequestContext{
		Type:   common.RequestType(req.Request.RequestType),
		Method: strings.ToUpper(req.Request.Method),
	}

	if req.Request.Path != "" {
		u, err := url.Parse(req.Request.Path)
		if err != nil {
			return nil, "", httperr.BadRequestf("invalid request.path: %s", err.Error())
		}
		rc.UpstreamURL = u
	}

	if req.Context.ConnectionId != nil && !req.Context.ConnectionId.IsNil() {
		conn, err := r.core.GetConnection(ctx, *req.Context.ConnectionId)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return nil, "", httperr.NotFound(fmt.Sprintf("connection '%s' not found", *req.Context.ConnectionId))
			}
			return nil, "", httperr.InternalServerError(httperr.WithInternalErr(err))
		}
		rc.Namespace = conn.GetNamespace()
		rc.ConnectionID = conn.GetId()
		rc.ConnectorID = conn.GetConnectorId()
		rc.ConnectorVersion = conn.GetConnectorVersion()
		// Connection labels already include carry-forward — copy so
		// downstream overrides don't mutate the core's map.
		if connLabels := conn.GetLabels(); len(connLabels) > 0 {
			rc.Labels = make(map[string]string, len(connLabels))
			for k, v := range connLabels {
				rc.Labels[k] = v
			}
		}
	} else if req.Context.Namespace != nil {
		rc.Namespace = *req.Context.Namespace
	}

	if req.Context.ActorId != nil && !req.Context.ActorId.IsNil() {
		rc.ActorID = *req.Context.ActorId
	}

	if len(req.Context.Labels) > 0 {
		if rc.Labels == nil {
			rc.Labels = make(map[string]string, len(req.Context.Labels))
		}
		for k, v := range req.Context.Labels {
			rc.Labels[k] = v
		}
	}

	if rc.Namespace == "" {
		return nil, "", httperr.BadRequest("could not determine namespace from context")
	}
	return rc, rc.Namespace, nil
}

// filterRulesInScope keeps rules whose namespace is the request's
// namespace or any ancestor — same cascading visibility the enforcer
// gets at runtime (rules at root.foo apply to requests in root.foo.bar).
func filterRulesInScope(rules []*database.RateLimit, requestNamespace string) []*database.RateLimit {
	out := make([]*database.RateLimit, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		if rule.Namespace == requestNamespace || strings.HasPrefix(requestNamespace, rule.Namespace+".") {
			out = append(out, rule)
		}
	}
	return out
}

// algorithmSummary formats the chosen algorithm for the dry-run UI.
// Kept symmetric with the frontend's per-row summary so operators see
// the same short string on the list page and the dry-run result.
func algorithmSummary(def rlschema.RateLimit) string {
	a := def.Algorithm
	switch {
	case a.TokenBucket != nil:
		return fmt.Sprintf("token bucket %d @ %g/s", a.TokenBucket.Capacity, a.TokenBucket.RefillRate)
	case a.FixedWindow != nil:
		return fmt.Sprintf("fixed window %d / %s", a.FixedWindow.Limit, a.FixedWindow.Window.Duration)
	case a.SlidingWindow != nil:
		return fmt.Sprintf("sliding window (%s) %d / %s",
			a.SlidingWindow.Mode, a.SlidingWindow.Limit, a.SlidingWindow.Window.Duration,
		)
	}
	return "—"
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
// @Success		200		{object}	SwaggerKeyValueJson
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
// @Param			request	body		SwaggerPutKeyValueRequest	true	"Label value"
// @Success		200		{object}	SwaggerKeyValueJson
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
// @Success		200			{object}	SwaggerKeyValueJson
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
// @Param			request		body		SwaggerPutKeyValueRequest	true	"Annotation value"
// @Success		200			{object}	SwaggerKeyValueJson
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

func NewRateLimitsRoutes(
	cfg config.C,
	authService auth.A,
	c coreIface.C,
	db database.DB,
	rlCache ratelimit.Cache,
	redis apredis.Client,
	logger *slog.Logger,
) *RateLimitsRoutes {
	if logger == nil {
		logger = slog.Default()
	}
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
		db:            db,
		rlCache:       rlCache,
		redis:         redis,
		logger:        logger,
		labelsAdapter: labelsAdapter,
		annotsAdapter: annotsAdapter,
	}
}
