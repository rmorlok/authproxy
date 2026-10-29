package routes

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httperr"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	schemaapiopenapi "github.com/rmorlok/authproxy/internal/schema/api/openapi"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/rmorlok/authproxy/internal/util"
)

var (
	_ = schemaapiopenapi.ToolSetJson{}
	_ = schemaapiopenapi.ToolSetPatchJson{}
	_ = schemaapiopenapi.ListToolSetsResponseJson{}
)

// ListToolSetsRequestQueryParams are the supported filters for GET /toolsets.
type ListToolSetsRequestQueryParams struct {
	Cursor        *string `form:"cursor"`
	LimitVal      *int32  `form:"limit"`
	NamespaceVal  *string `form:"namespace"`
	NameVal       *string `form:"name"`
	LabelSelector *string `form:"labelSelector"`
}

// ListToolSetGenerationsRequestQueryParams page GET /toolsets/{id}/generations.
type ListToolSetGenerationsRequestQueryParams struct {
	Cursor   *string `form:"cursor"`
	LimitVal *int32  `form:"limit"`
}

// ToolSetsRoutes serves ToolSet management: the logical resource, which owns
// identity, metadata, and the shared connection selector, and its generations,
// which own definitions and follow the Connector draft/primary/active lifecycle.
// These routes use the `toolsets` permission resource. They never install,
// migrate, or invoke generated Tools.
type ToolSetsRoutes struct {
	cfg         config.C
	core        coreIface.C
	authService auth.A
}

// writeToolSetError maps core ToolSet errors to the API error contract and
// marks the validator so the permission middleware does not report a missing check.
func writeToolSetError(gctx *gin.Context, val *auth.ResourcePermissionValidator, err error, id apid.ID) {
	switch {
	case errors.Is(err, core.ErrNotFound):
		apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("toolset '%s' not found", id), httperr.WithInternalErr(err)))
	case errors.Is(err, core.ErrDraftAlreadyExists):
		apgin.WriteError(gctx, nil, httperr.Conflict("toolset already has a draft generation; update it instead", httperr.WithInternalErr(err)))
	case errors.Is(err, core.ErrNotDraft):
		apgin.WriteError(gctx, nil, httperr.Conflict("published toolset generations are immutable; update the draft instead", httperr.WithInternalErr(err)))
	case errors.Is(err, core.ErrToolSetConflict):
		apgin.WriteError(gctx, nil, httperr.Conflict("toolset changed concurrently; reload it and retry", httperr.WithInternalErr(err)))
	case errors.Is(err, core.ErrInvalidArgument):
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
	default:
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
	}
	val.MarkErrorReturn()
}

// parseToolSetID reads and validates the :id path parameter. It writes the
// error response and returns apid.Nil on failure.
func parseToolSetID(gctx *gin.Context, val *auth.ResourcePermissionValidator) apid.ID {
	id := apid.ID(gctx.Param("id"))
	if id.IsNil() || toolsetschema.ValidateID(id.String()) != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("a toolset id is required"))
		val.MarkErrorReturn()
		return apid.Nil
	}
	return id
}

// parseToolSetGeneration reads the positive :generation path parameter. It
// writes the error response and returns zero on failure.
func parseToolSetGeneration(gctx *gin.Context, val *auth.ResourcePermissionValidator) uint64 {
	generation, err := strconv.ParseUint(gctx.Param("generation"), 10, 63)
	if err != nil || generation == 0 {
		apgin.WriteError(gctx, nil, httperr.BadRequest("generation must be a positive integer"))
		val.MarkErrorReturn()
		return 0
	}
	return generation
}

// loadToolSet reads a ToolSet's default projection for an ID route and checks
// the caller's permission on its namespace. Every generation shares the logical
// namespace, so this also authorizes generation routes. It writes the error
// response and returns nil on failure.
func (r *ToolSetsRoutes) loadToolSet(gctx *gin.Context, val *auth.ResourcePermissionValidator) coreIface.ToolSet {
	id := parseToolSetID(gctx, val)
	if id.IsNil() {
		return nil
	}

	toolSet, err := r.core.GetToolSet(gctx.Request.Context(), id)
	if err != nil {
		writeToolSetError(gctx, val, err, id)
		return nil
	}

	if httpErr := val.ValidateHttpStatusError(toolSet); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return nil
	}

	return toolSet
}

// renderToolSet writes a canonical ToolSet resource response.
func renderToolSet(gctx *gin.Context, val *auth.ResourcePermissionValidator, status int, toolSet coreIface.ToolSet) {
	if err := apgin.RenderResourceJSON(gctx, status, toolSet.GetResource()); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// renderToolSetPage writes a ToolSetList of the permitted items.
func renderToolSetPage(gctx *gin.Context, val *auth.ResourcePermissionValidator, page coreIface.ToolSetPage) {
	apgin.APIJSON(gctx, http.StatusOK, schemaapi.NewListToolSetsResponseJson(
		util.Map(auth.FilterForValidatedResources(val, page.Items), func(toolSet coreIface.ToolSet) toolsetschema.ToolSet {
			return *toolSet.GetResource()
		}),
		page.Cursor,
	))
}

// @Summary		Get toolset
// @Description	Get a ToolSet's default projection: its primary generation, otherwise its draft. The connection selector is always the current shared selector.
// @Tags			toolsets
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"ToolSet ID"
// @Success		200	{object}	schemaapiopenapi.ToolSetJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		403	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/toolsets/{id} [get]
func (r *ToolSetsRoutes) get(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	if toolSet := r.loadToolSet(gctx, val); toolSet != nil {
		renderToolSet(gctx, val, http.StatusOK, toolSet)
	}
}

// @Summary		Create toolset
// @Description	Create a ToolSet and its first generation. Every explicit template is compiled first. The generation is a draft unless spec.release.desiredState is primary, which publishes it. Only explicit sources are supported.
// @Tags			toolsets
// @Accept			json
// @Produce		json
// @Param			request	body		schemaapiopenapi.ToolSetJson	true	"ToolSet to create"
// @Success		201		{object}	schemaapiopenapi.ToolSetJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/toolsets [post]
func (r *ToolSetsRoutes) create(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req toolsetschema.ToolSet
	if err := apgin.BindResourceJSON(gctx, &req, meta.ValidationModeCreate); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	if err := val.ValidateNamespace(req.Metadata.Namespace); err != nil {
		apgin.WriteError(gctx, nil, httperr.Forbidden(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	toolSet, err := r.core.CreateToolSet(ctx, &req)
	if err != nil {
		if conflictErr := resourceNameConflictError(err, "toolset", req.Metadata.Name, req.Metadata.Namespace); conflictErr != nil {
			apgin.WriteError(gctx, nil, conflictErr)
			val.MarkErrorReturn()
			return
		}
		writeToolSetError(gctx, val, err, apid.Nil)
		return
	}

	renderToolSet(gctx, val, http.StatusCreated, toolSet)
}

// @Summary		List toolsets
// @Description	List ToolSets visible to the caller, each in its default projection
// @Tags			toolsets
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"Pagination cursor"
// @Param			limit			query		integer	false	"Maximum number of results to return (default 50, max 100)"
// @Param			namespace		query		string	false	"Filter by namespace matcher"
// @Param			name			query		string	false	"Filter by exact name"
// @Param			labelSelector	query		string	false	"Filter by label selector"
// @Success		200				{object}	schemaapiopenapi.ListToolSetsResponseJson
// @Failure		400				{object}	ErrorResponse
// @Failure		401				{object}	ErrorResponse
// @Failure		500				{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/toolsets [get]
func (r *ToolSetsRoutes) list(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req ListToolSetsRequestQueryParams
	if err := gctx.ShouldBindQuery(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	var page coreIface.ToolSetPage
	var err error
	if req.Cursor != nil {
		page, err = r.core.ListToolSetsFromCursor(ctx, *req.Cursor)
	} else {
		listReq := coreIface.ListToolSetsRequest{
			NamespaceMatchers: val.GetEffectiveNamespaceMatchers(req.NamespaceVal),
		}
		if req.LimitVal != nil {
			if *req.LimitVal < 1 {
				apgin.WriteError(gctx, nil, httperr.BadRequest("limit must be positive"))
				val.MarkErrorReturn()
				return
			}
			listReq.Limit = *req.LimitVal
		}
		if req.NameVal != nil {
			name := scommon.ResourceName(*req.NameVal)
			if err := name.Validate(); err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid toolset name: %s", err.Error()))
				val.MarkErrorReturn()
				return
			}
			listReq.Name = name
		}
		if req.LabelSelector != nil {
			listReq.LabelSelector = *req.LabelSelector
		}
		page, err = r.core.ListToolSets(ctx, listReq)
	}
	if err != nil {
		writeToolSetError(gctx, val, err, apid.Nil)
		return
	}

	renderToolSetPage(gctx, val, page)
}

// @Summary		Update toolset
// @Description	Patch a ToolSet through its logical endpoint. Metadata and spec.connectionSelector change the logical ToolSet without creating a generation or migrating connections. spec.definition and spec.release edit the draft, cloning the newest generation into one if needed. A definition sent with desiredState primary is published; desiredState primary alone is already satisfied by an existing primary, so publish a pending draft through its generation endpoint.
// @Tags			toolsets
// @Accept			json
// @Produce		json
// @Param			id		path		string								true	"ToolSet ID"
// @Param			request	body		schemaapiopenapi.ToolSetPatchJson	true	"Update request"
// @Success		200		{object}	schemaapiopenapi.ToolSetJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/toolsets/{id} [patch]
func (r *ToolSetsRoutes) update(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req toolsetschema.ToolSetPatch
	if err := apgin.BindResourceJSON(gctx, &req, meta.ValidationModeUpdate); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	toolSet := r.loadToolSet(gctx, val)
	if toolSet == nil {
		return
	}

	updated, err := r.core.UpdateToolSet(ctx, toolSet.GetId(), &req)
	if err != nil {
		if req.Metadata != nil && req.Metadata.Name != nil {
			if conflictErr := resourceNameConflictError(err, "toolset", *req.Metadata.Name, toolSet.GetNamespace()); conflictErr != nil {
				apgin.WriteError(gctx, nil, conflictErr)
				val.MarkErrorReturn()
				return
			}
		}
		writeToolSetError(gctx, val, err, toolSet.GetId())
		return
	}

	renderToolSet(gctx, val, http.StatusOK, updated)
}

// @Summary		Delete toolset
// @Description	Soft delete a ToolSet. All of its generations become unreadable immediately.
// @Tags			toolsets
// @Accept			json
// @Produce		json
// @Param			id	path	string	true	"ToolSet ID"
// @Success		204	"No Content"
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		403	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/toolsets/{id} [delete]
func (r *ToolSetsRoutes) delete(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := parseToolSetID(gctx, val)
	if id.IsNil() {
		return
	}

	toolSet, err := r.core.GetToolSet(ctx, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			// Deletion is idempotent, matching other resource routes.
			gctx.Status(http.StatusNoContent)
			val.MarkValidated()
			return
		}
		writeToolSetError(gctx, val, err, id)
		return
	}

	if httpErr := val.ValidateHttpStatusError(toolSet); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	if err := r.core.DeleteToolSet(ctx, id); err != nil && !errors.Is(err, core.ErrNotFound) {
		writeToolSetError(gctx, val, err, id)
		return
	}

	gctx.Status(http.StatusNoContent)
}

// @Summary		List toolset generations
// @Description	List a ToolSet's generations, newest first. Each projects the current shared connection selector.
// @Tags			toolsets
// @Accept			json
// @Produce		json
// @Param			id		path		string	true	"ToolSet ID"
// @Param			cursor	query		string	false	"Pagination cursor"
// @Param			limit	query		integer	false	"Maximum number of results to return (default 50, max 100)"
// @Success		200		{object}	schemaapiopenapi.ListToolSetsResponseJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/toolsets/{id}/generations [get]
func (r *ToolSetsRoutes) listGenerations(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req ListToolSetGenerationsRequestQueryParams
	if err := gctx.ShouldBindQuery(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}
	if req.LimitVal != nil && *req.LimitVal < 1 {
		apgin.WriteError(gctx, nil, httperr.BadRequest("limit must be positive"))
		val.MarkErrorReturn()
		return
	}

	toolSet := r.loadToolSet(gctx, val)
	if toolSet == nil {
		return
	}

	var page coreIface.ToolSetPage
	var err error
	if req.Cursor != nil {
		page, err = r.core.ListToolSetGenerationsFromCursor(ctx, *req.Cursor)
		// A cursor carries its own ToolSet; refuse one minted for another path.
		if err == nil && len(page.Items) > 0 && page.Items[0].GetId() != toolSet.GetId() {
			apgin.WriteError(gctx, nil, httperr.BadRequest("cursor does not belong to this toolset"))
			val.MarkErrorReturn()
			return
		}
	} else {
		var limit int32
		if req.LimitVal != nil {
			limit = *req.LimitVal
		}
		page, err = r.core.ListToolSetGenerations(ctx, toolSet.GetId(), limit)
	}
	if err != nil {
		writeToolSetError(gctx, val, err, toolSet.GetId())
		return
	}

	renderToolSetPage(gctx, val, page)
}

// @Summary		Get toolset generation
// @Description	Get one ToolSet generation projected with the current shared connection selector
// @Tags			toolsets
// @Accept			json
// @Produce		json
// @Param			id			path		string	true	"ToolSet ID"
// @Param			generation	path		integer	true	"Generation number"
// @Success		200			{object}	schemaapiopenapi.ToolSetJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/toolsets/{id}/generations/{generation} [get]
func (r *ToolSetsRoutes) getGeneration(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	toolSet := r.loadToolSet(gctx, val)
	if toolSet == nil {
		return
	}
	generation := parseToolSetGeneration(gctx, val)
	if generation == 0 {
		return
	}

	result, err := r.core.GetToolSetGeneration(ctx, toolSet.GetId(), generation)
	if err != nil {
		writeToolSetError(gctx, val, err, toolSet.GetId())
		return
	}

	renderToolSet(gctx, val, http.StatusOK, result)
}

// @Summary		Create toolset generation
// @Description	Create a draft generation cloned from the newest generation. An optional patch may replace spec.definition or request desiredState primary to publish it. Metadata and connection selector changes are rejected here. Returns 409 if a draft already exists.
// @Tags			toolsets
// @Accept			json
// @Produce		json
// @Param			id		path		string								true	"ToolSet ID"
// @Param			request	body		schemaapiopenapi.ToolSetPatchJson	false	"Optional generation patch"
// @Success		201		{object}	schemaapiopenapi.ToolSetJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/toolsets/{id}/generations [post]
func (r *ToolSetsRoutes) createGeneration(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var patch *toolsetschema.ToolSetPatch
	if gctx.Request.ContentLength != 0 {
		var req toolsetschema.ToolSetPatch
		if err := apgin.BindResourceJSON(gctx, &req, meta.ValidationModeUpdate); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
			val.MarkErrorReturn()
			return
		}
		patch = &req
	}

	toolSet := r.loadToolSet(gctx, val)
	if toolSet == nil {
		return
	}

	result, err := r.core.CreateToolSetGeneration(ctx, toolSet.GetId(), patch)
	if err != nil {
		writeToolSetError(gctx, val, err, toolSet.GetId())
		return
	}

	renderToolSet(gctx, val, http.StatusCreated, result)
}

// @Summary		Update toolset generation
// @Description	Patch one draft generation's spec.definition or publish it with desiredState primary, which demotes the previous primary to active without migrating connections. Metadata and connection selector changes are rejected here. Published generations are immutable; an empty or primary-only patch on one is a no-op.
// @Tags			toolsets
// @Accept			json
// @Produce		json
// @Param			id			path		string								true	"ToolSet ID"
// @Param			generation	path		integer								true	"Generation number"
// @Param			request		body		schemaapiopenapi.ToolSetPatchJson	true	"Generation patch"
// @Success		200			{object}	schemaapiopenapi.ToolSetJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		409			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/toolsets/{id}/generations/{generation} [patch]
func (r *ToolSetsRoutes) updateGeneration(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req toolsetschema.ToolSetPatch
	if err := apgin.BindResourceJSON(gctx, &req, meta.ValidationModeUpdate); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	toolSet := r.loadToolSet(gctx, val)
	if toolSet == nil {
		return
	}
	generation := parseToolSetGeneration(gctx, val)
	if generation == 0 {
		return
	}

	result, err := r.core.UpdateToolSetGeneration(ctx, toolSet.GetId(), generation, &req)
	if err != nil {
		writeToolSetError(gctx, val, err, toolSet.GetId())
		return
	}

	renderToolSet(gctx, val, http.StatusOK, result)
}

// Register mounts the ToolSet management routes with `toolsets` resource
// permissions. Generation reads use list/generations, generation creation uses
// create, and draft edits and publication use update, mirroring Connectors.
func (r *ToolSetsRoutes) Register(g gin.IRouter) {
	idExtractor := func(toolSet interface{}) string {
		return string(toolSet.(coreIface.ToolSet).GetId())
	}
	route := func(verb string, withID bool) gin.HandlerFunc {
		b := r.authService.NewRequiredBuilder().
			ForResource("toolsets").
			ForIdExtractor(idExtractor).
			ForVerb(verb)
		if withID {
			b = b.ForIdField("id")
		}
		return b.Build()
	}

	g.GET("/toolsets", route("list", false), r.list)
	g.POST("/toolsets", route("create", false), r.create)
	g.GET("/toolsets/:id", route("get", true), r.get)
	g.PATCH("/toolsets/:id", route("update", true), r.update)
	g.DELETE("/toolsets/:id", route("delete", true), r.delete)
	g.GET("/toolsets/:id/generations", route("list/generations", true), r.listGenerations)
	g.POST("/toolsets/:id/generations", route("create", true), r.createGeneration)
	g.GET("/toolsets/:id/generations/:generation", route("list/generations", true), r.getGeneration)
	g.PATCH("/toolsets/:id/generations/:generation", route("update", true), r.updateGeneration)
}

// NewToolSetsRoutes constructs the ToolSet management routes.
func NewToolSetsRoutes(cfg config.C, authService auth.A, c coreIface.C) *ToolSetsRoutes {
	return &ToolSetsRoutes{
		cfg:         cfg,
		authService: authService,
		core:        c,
	}
}
