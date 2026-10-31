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
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

var (
	_ = schemaapiopenapi.ToolJson{}
	_ = schemaapiopenapi.ToolPatchJson{}
	_ = schemaapiopenapi.ListToolsResponseJson{}
)

// ListToolsRequestQueryParams are the supported filters for GET /tools.
type ListToolsRequestQueryParams struct {
	Cursor        *string `form:"cursor"`
	LimitVal      *int32  `form:"limit"`
	NamespaceVal  *string `form:"namespace"`
	NameVal       *string `form:"name"`
	ConnectionId  *string `form:"connectionId"`
	LabelSelector *string `form:"labelSelector"`
	OrderByVal    *string `form:"orderBy"`
}

// ToolsRoutes serves management CRUD for standalone Tools. These routes use
// the `tools` permission resource; they never authorize Tool invocation, and
// invocation grants never authorize them. Generated Tools are readable here
// but can only be changed through their owning ToolSet.
type ToolsRoutes struct {
	cfg         config.C
	core        coreIface.C
	authService auth.A
}

// writeToolError maps core Tool errors to the API error contract and marks the
// validator so the permission middleware does not report a missing check.
func writeToolError(gctx *gin.Context, val *auth.ResourcePermissionValidator, err error, id apid.ID) {
	switch {
	case errors.Is(err, core.ErrNotFound):
		apgin.WriteError(gctx, nil, httperr.NotFound(fmt.Sprintf("tool '%s' not found", id), httperr.WithInternalErr(err)))
	case errors.Is(err, core.ErrToolManaged):
		apgin.WriteError(gctx, nil, httperr.Conflict(err.Error(), httperr.WithInternalErr(err)))
	case errors.Is(err, core.ErrToolRevisionConflict):
		apgin.WriteError(gctx, nil, httperr.Conflict("tool changed concurrently; reload it and retry", httperr.WithInternalErr(err)))
	case errors.Is(err, core.ErrInvalidArgument):
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
	default:
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
	}
	val.MarkErrorReturn()
}

// managedToolConflict reports a write to a generated Tool before any patch is
// applied, so the caller learns which ToolSet owns it.
func managedToolConflict(tool coreIface.Tool) *httperr.Error {
	owner := tool.GetManagedByToolSetId()
	if owner.IsNil() {
		return nil
	}
	return httperr.Conflictf("tool is managed by ToolSet '%s'; update the owning ToolSet instead", owner)
}

// parseListOrderBy splits an orderBy query value of "<field> [asc|desc]" and
// checks the field with valid, shared by the Tool and ToolSet list routes. It
// writes the error response and returns false when the value is invalid.
func parseListOrderBy[T ~string](
	gctx *gin.Context,
	val *auth.ResourcePermissionValidator,
	raw string,
	valid func(T) bool,
) (T, pagination.OrderBy, bool) {
	field, order, err := pagination.SplitOrderByParam[T](raw)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return "", "", false
	}

	if !valid(field) {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid sort field '%s'", field))
		val.MarkErrorReturn()
		return "", "", false
	}

	return field, order, true
}

// loadTool reads one Tool for an ID route and checks the caller's permission on
// its namespace. It writes the error response and returns nil on failure.
func (r *ToolsRoutes) loadTool(gctx *gin.Context, val *auth.ResourcePermissionValidator) coreIface.Tool {
	id := apid.ID(gctx.Param("id"))
	if id.IsNil() || toolschema.ValidateID(id.String()) != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("a tool id is required"))
		val.MarkErrorReturn()
		return nil
	}

	tool, err := r.core.GetTool(gctx.Request.Context(), id)
	if err != nil {
		writeToolError(gctx, val, err, id)
		return nil
	}

	if httpErr := val.ValidateHttpStatusError(tool); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return nil
	}

	return tool
}

// renderTool writes a canonical Tool resource response.
func renderTool(gctx *gin.Context, val *auth.ResourcePermissionValidator, tool coreIface.Tool) {
	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, tool.GetResource()); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		Get tool
// @Description	Get a Tool's management resource, including its active definition and revision
// @Tags			tools
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Tool ID"
// @Success		200	{object}	schemaapiopenapi.ToolJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		403	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/tools/{id} [get]
func (r *ToolsRoutes) get(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	if tool := r.loadTool(gctx, val); tool != nil {
		renderTool(gctx, val, tool)
	}
}

// @Summary		Create tool
// @Description	Create a standalone Tool bound to a connection in the same namespace. The definition is compiled before revision 1 is published.
// @Tags			tools
// @Accept			json
// @Produce		json
// @Param			request	body		schemaapiopenapi.ToolJson	true	"Tool to create"
// @Success		200		{object}	schemaapiopenapi.ToolJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/tools [post]
func (r *ToolsRoutes) create(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req toolschema.Tool
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

	tool, err := r.core.CreateTool(ctx, &req)
	if err != nil {
		if conflictErr := resourceNameConflictError(err, "tool", req.Metadata.Name, req.Metadata.Namespace); conflictErr != nil {
			apgin.WriteError(gctx, nil, conflictErr)
			val.MarkErrorReturn()
			return
		}
		writeToolError(gctx, val, err, apid.Nil)
		return
	}

	renderTool(gctx, val, tool)
}

// @Summary		List tools
// @Description	List Tool management resources visible to the caller
// @Tags			tools
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"Pagination cursor"
// @Param			limit			query		integer	false	"Maximum number of results to return (default 50, max 100)"
// @Param			namespace		query		string	false	"Filter by namespace matcher"
// @Param			name			query		string	false	"Filter by exact name"
// @Param			connectionId	query		string	false	"Filter by bound connection ID"
// @Param			labelSelector	query		string	false	"Filter by label selector"
// @Param			orderBy			query		string	false	"Order by id, namespace, name, created_at, or updated_at, optionally followed by asc or desc (e.g., 'created_at desc'). Defaults to id."
// @Success		200				{object}	schemaapiopenapi.ListToolsResponseJson
// @Failure		400				{object}	ErrorResponse
// @Failure		401				{object}	ErrorResponse
// @Failure		500				{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/tools [get]
func (r *ToolsRoutes) list(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req ListToolsRequestQueryParams
	if err := gctx.ShouldBindQuery(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	var page coreIface.ToolPage
	var err error
	if req.Cursor != nil {
		page, err = r.core.ListToolsFromCursor(ctx, *req.Cursor)
	} else {
		listReq := coreIface.ListToolsRequest{
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
				apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid tool name: %s", err.Error()))
				val.MarkErrorReturn()
				return
			}
			listReq.Name = name
		}
		if req.ConnectionId != nil {
			if err := connectionschema.ValidateID(*req.ConnectionId); err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid connectionId: %s", err.Error()))
				val.MarkErrorReturn()
				return
			}
			listReq.ConnectionId = apid.ID(*req.ConnectionId)
		}
		if req.LabelSelector != nil {
			listReq.LabelSelector = *req.LabelSelector
		}
		if req.OrderByVal != nil {
			field, order, ok := parseListOrderBy(gctx, val, *req.OrderByVal, database.IsValidToolOrderByField[database.ToolOrderByField])
			if !ok {
				return
			}
			listReq.OrderByField = field
			listReq.OrderBy = order
		}
		page, err = r.core.ListTools(ctx, listReq)
	}
	if err != nil {
		writeToolError(gctx, val, err, apid.Nil)
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, schemaapi.NewListToolsResponseJson(
		util.Map(auth.FilterForValidatedResources(val, page.Items), func(tool coreIface.Tool) toolschema.Tool {
			return *tool.GetResource()
		}),
		page.Cursor,
	))
}

// @Summary		Update tool
// @Description	Patch a standalone Tool. The complete merged definition is compiled and published atomically as the next revision; later invocations use it immediately. Generated Tools return 409.
// @Tags			tools
// @Accept			json
// @Produce		json
// @Param			id		path		string							true	"Tool ID"
// @Param			request	body		schemaapiopenapi.ToolPatchJson	true	"Update request"
// @Success		200		{object}	schemaapiopenapi.ToolJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/tools/{id} [patch]
func (r *ToolsRoutes) update(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req toolschema.ToolPatch
	if err := apgin.BindResourceJSON(gctx, &req, meta.ValidationModeUpdate); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	tool := r.loadTool(gctx, val)
	if tool == nil {
		return
	}
	if httpErr := managedToolConflict(tool); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	candidate, err := req.ApplyTo(tool.GetResource(), nil)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	// The revision read above is the expected revision, so an update that
	// commits between this read and the write is rejected rather than lost.
	updated, err := r.core.UpdateTool(ctx, tool.GetId(), tool.GetActiveRevision(), candidate)
	if err != nil {
		if conflictErr := resourceNameConflictError(err, "tool", candidate.Metadata.Name, tool.GetNamespace()); conflictErr != nil {
			apgin.WriteError(gctx, nil, conflictErr)
			val.MarkErrorReturn()
			return
		}
		writeToolError(gctx, val, err, tool.GetId())
		return
	}

	renderTool(gctx, val, updated)
}

// @Summary		Delete tool
// @Description	Soft delete a standalone Tool, retaining its revision history. Generated Tools return 409.
// @Tags			tools
// @Accept			json
// @Produce		json
// @Param			id	path	string	true	"Tool ID"
// @Success		204	"No Content"
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		403	{object}	ErrorResponse
// @Failure		409	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/tools/{id} [delete]
func (r *ToolsRoutes) delete(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := apid.ID(gctx.Param("id"))
	if id.IsNil() || toolschema.ValidateID(id.String()) != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("a tool id is required"))
		val.MarkErrorReturn()
		return
	}

	tool, err := r.core.GetTool(ctx, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			// Deletion is idempotent, matching other resource routes.
			gctx.Status(http.StatusNoContent)
			val.MarkValidated()
			return
		}
		writeToolError(gctx, val, err, id)
		return
	}

	if httpErr := val.ValidateHttpStatusError(tool); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}
	if httpErr := managedToolConflict(tool); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	if err := r.core.DeleteTool(ctx, id, tool.GetActiveRevision()); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			gctx.Status(http.StatusNoContent)
			return
		}
		writeToolError(gctx, val, err, id)
		return
	}

	gctx.Status(http.StatusNoContent)
}

// Register mounts the Tool management routes with `tools` resource permissions.
func (r *ToolsRoutes) Register(g gin.IRouter) {
	idExtractor := func(tool interface{}) string {
		return string(tool.(coreIface.Tool).GetId())
	}
	route := func(verb string, withID bool) gin.HandlerFunc {
		b := r.authService.NewRequiredBuilder().
			ForResource("tools").
			ForIdExtractor(idExtractor).
			ForVerb(verb)
		if withID {
			b = b.ForIdField("id")
		}
		return b.Build()
	}

	g.GET("/tools", route("list", false), r.list)
	g.POST("/tools", route("create", false), r.create)
	g.GET("/tools/:id", route("get", true), r.get)
	g.PATCH("/tools/:id", route("update", true), r.update)
	g.DELETE("/tools/:id", route("delete", true), r.delete)
}

// NewToolsRoutes constructs the Tool management routes.
func NewToolsRoutes(cfg config.C, authService auth.A, c coreIface.C) *ToolsRoutes {
	return &ToolsRoutes{
		cfg:         cfg,
		authService: authService,
		core:        c,
	}
}
