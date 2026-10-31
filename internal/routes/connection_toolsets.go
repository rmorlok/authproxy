package routes

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	schemaapiopenapi "github.com/rmorlok/authproxy/internal/schema/api/openapi"
)

// Swaggo resolves the documented response through this package-level reference.
var _ = schemaapiopenapi.ConnectionToolSetBindingListJson{}

// ListConnectionToolSetBindingsQueryParams page GET /connections/{id}/toolsets.
// There is no sort option: bindings have a single keyset order by ToolSet ID.
type ListConnectionToolSetBindingsQueryParams struct {
	Cursor   *string `form:"cursor"`
	LimitVal *int32  `form:"limit"`
}

// @Summary		List connection toolsets
// @Description	List the ToolSets assigned to a connection with their installation status. Requires connections:get on the connection; only ToolSets the caller can read with toolsets:get are returned, so a ToolSet in an ancestor namespace is omitted without that grant. This reports status and grants no Tool invocation.
// @Tags			connections
// @Produce		json
// @Param			id		path		string	true	"Connection ID"
// @Param			cursor	query		string	false	"Pagination cursor"
// @Param			limit	query		integer	false	"Maximum number of results to return (default 50, max 100)"
// @Success		200		{object}	schemaapiopenapi.ConnectionToolSetBindingListJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/toolsets [get]
func (r *ConnectionsRoutes) listToolSetBindings(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	// writeError marks the validator so middleware does not report a missing check.
	writeError := func(err *httperr.Error) {
		apgin.WriteError(gctx, nil, err)
		val.MarkErrorReturn()
	}

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		writeError(httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		return
	}
	if err := id.ValidatePrefix(apid.PrefixConnection); err != nil {
		writeError(httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		return
	}

	var req ListConnectionToolSetBindingsQueryParams
	if err := gctx.ShouldBindQuery(&req); err != nil {
		writeError(httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		return
	}

	// Only the stored namespace is needed, so skip hydrating the connector.
	connection, err := r.db.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeError(httperr.NotFound("connection not found"))
		} else {
			writeError(httperr.InternalServerError(httperr.WithInternalErr(err)))
		}
		return
	}

	if httpErr := val.ValidateHttpStatusError(connection); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	// The route validator only covers connections:get. Parent ToolSets are checked
	// against toolsets:get here, in SQL so scope is applied before paging.
	requestAuth := auth.MustGetAuthFromGinContext(gctx)

	var ex coreIface.ListConnectionToolSetBindingsExecutor
	if req.Cursor != nil {
		ex, err = r.core.ListConnectionToolSetBindingsFromCursor(ctx, *req.Cursor)
		if err != nil {
			writeError(httperr.BadRequest("invalid cursor", httperr.WithInternalErr(err)))
			return
		}
	} else {
		b := r.core.
			ListConnectionToolSetBindingsBuilder().
			ForConnectionId(id).
			ForToolSetNamespaceMatchers(requestAuth.GetNamespacesAllowed("toolsets", "get"))
		if req.LimitVal != nil {
			if *req.LimitVal < 1 {
				writeError(httperr.BadRequest("limit must be positive"))
				return
			}
			b = b.Limit(*req.LimitVal)
		}
		ex = b
	}

	page := ex.FetchPage(ctx)
	if page.Error != nil {
		writeError(httperr.InternalServerError(httperr.WithInternalErr(page.Error)))
		return
	}

	items := make([]schemaapi.ConnectionToolSetBinding, 0, len(page.Results))
	for _, binding := range page.Results {
		// A cursor carries its own connection, so one issued for another connection
		// must not read through this route's connection check.
		if binding.GetConnectionId() != id {
			writeError(httperr.BadRequest("cursor belongs to a different connection"))
			return
		}

		// Namespace scope cannot express ID restrictions, and a cursor can outlive
		// the grants of the request that created it, so each row is rechecked.
		if requestAuth.Allows(binding.GetToolSetNamespace(), "toolsets", "get", binding.GetToolSetId().String()) {
			items = append(items, *binding.GetResource())
		}
	}

	list := schemaapi.NewConnectionToolSetBindingList(items, page.Cursor)
	if err := list.Validate(); err != nil {
		writeError(httperr.InternalServerError(httperr.WithInternalErr(err)))
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, list)
}
