package key_value

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
)

// Adapter wires the generic label/annotation handlers to a specific
// resource type. ID is the resource's identifier type (e.g. apid.ID,
// string, or a struct for composite keys).
type Adapter[ID any] struct {
	// Kind selects between Label and Annotation behavior.
	Kind Kind

	// ResourceName is the singular resource name used in error messages
	// (e.g. "connection", "namespace").
	ResourceName string

	// PathPrefix is the gin path prefix that identifies a single
	// resource, including any path parameters.
	// Examples:
	//   "/connections/:id"
	//   "/namespaces/:path"
	//   "/connectors/:id/versions/:version"
	PathPrefix string

	// AuthGet is the auth middleware for read endpoints (GET).
	AuthGet gin.HandlerFunc

	// AuthMutate is the auth middleware for write endpoints (PUT/DELETE).
	AuthMutate gin.HandlerFunc

	// ParseID extracts and validates the resource identifier from the
	// gin context. Returns a non-nil *httperr.Error to terminate the
	// request with that error response.
	ParseID func(*gin.Context) (ID, *httperr.Error)

	// Get fetches the resource for read endpoints and for auth
	// validation on write endpoints. Should return database.ErrNotFound
	// when the resource is missing.
	Get func(ctx context.Context, id ID) (Resource, error)

	// Put adds or updates the supplied keys on the resource.
	Put func(ctx context.Context, id ID, kv map[string]string) (Resource, error)

	// Delete removes the supplied keys from the resource.
	Delete func(ctx context.Context, id ID, keys []string) (Resource, error)

	// Logger is the optional logger forwarded to apgin.WriteError when the
	// adapter writes error responses. May be nil.
	Logger *slog.Logger
}

// Register adds the four endpoints for this kind to the router.
func (a Adapter[ID]) Register(g gin.IRouter) {
	listPath := a.PathPrefix + "/" + a.Kind.PathSegment
	itemPath := listPath + "/:" + a.Kind.ParamName

	g.GET(listPath, a.AuthGet, a.HandleList)
	g.GET(itemPath, a.AuthGet, a.HandleGet)
	g.PUT(itemPath, a.AuthMutate, a.HandlePut)
	g.DELETE(itemPath, a.AuthMutate, a.HandleDelete)
}

// fetchAndAuthorize loads the resource and runs the permission
// validator.
//
// On success it returns (resource, true) and the caller continues
// handling the request.
//
// On failure it returns (nil, false) and has already written the
// terminal HTTP response (error body or 204) and updated the validator
// state. The caller MUST return immediately without writing anything
// further to gctx.
//
// returnNotFound selects the response when the resource is missing:
//   - true: 404 Not Found (used by GET and PUT)
//   - false: 204 No Content (used by DELETE for idempotent semantics)
func (a Adapter[ID]) fetchAndAuthorize(gctx *gin.Context, id ID, val *auth.ResourcePermissionValidator, returnNotFound bool) (resource Resource, ok bool) {
	ctx := gctx.Request.Context()
	r, err := a.Get(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			if returnNotFound {
				apgin.WriteError(gctx, a.Logger, httperr.NotFoundf("%s not found", a.ResourceName))
				val.MarkErrorReturn()
			} else {
				gctx.Status(http.StatusNoContent)
				val.MarkValidated()
			}
			return nil, false
		}
		apgin.WriteError(gctx, a.Logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return nil, false
	}

	if r == nil {
		if returnNotFound {
			apgin.WriteError(gctx, a.Logger, httperr.NotFoundf("%s not found", a.ResourceName))
			val.MarkErrorReturn()
		} else {
			gctx.Status(http.StatusNoContent)
			val.MarkValidated()
		}
		return nil, false
	}

	if httpErr := val.ValidateHttpStatusError(r); httpErr != nil {
		apgin.WriteError(gctx, a.Logger, httpErr)
		return nil, false
	}

	return r, true
}

// HandleList serves GET /<resource>/<id>/<segment> — returns all
// labels (or annotations) for the resource as a flat key/value map.
func (a Adapter[ID]) HandleList(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, herr := a.ParseID(gctx)
	if herr != nil {
		apgin.WriteError(gctx, a.Logger, herr)
		val.MarkErrorReturn()
		return
	}

	r, ok := a.fetchAndAuthorize(gctx, id, val, true)
	if !ok {
		return
	}

	values := a.Kind.Get(r)
	if values == nil {
		values = make(map[string]string)
	}
	gctx.PureJSON(http.StatusOK, values)
}

// HandleGet serves GET /<resource>/<id>/<segment>/<key> — returns the
// single key/value pair, or 404 if the key is not set.
func (a Adapter[ID]) HandleGet(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, herr := a.ParseID(gctx)
	if herr != nil {
		apgin.WriteError(gctx, a.Logger, herr)
		val.MarkErrorReturn()
		return
	}

	key := gctx.Param(a.Kind.ParamName)
	if key == "" {
		apgin.WriteError(gctx, a.Logger, httperr.BadRequestf("%s key is required", a.Kind.Singular))
		val.MarkErrorReturn()
		return
	}

	r, ok := a.fetchAndAuthorize(gctx, id, val, true)
	if !ok {
		return
	}

	value, exists := a.Kind.Get(r)[key]
	if !exists {
		apgin.WriteError(gctx, a.Logger, httperr.NotFoundf("%s '%s' not found", a.Kind.Singular, key))
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, KeyValueJson{Key: key, Value: value})
}

// HandlePut serves PUT /<resource>/<id>/<segment>/<key> — adds or
// updates the value for a single key.
func (a Adapter[ID]) HandlePut(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, herr := a.ParseID(gctx)
	if herr != nil {
		apgin.WriteError(gctx, a.Logger, herr)
		val.MarkErrorReturn()
		return
	}

	key := gctx.Param(a.Kind.ParamName)
	if key == "" {
		apgin.WriteError(gctx, a.Logger, httperr.BadRequestf("%s key is required", a.Kind.Singular))
		val.MarkErrorReturn()
		return
	}

	if err := a.Kind.ValidateKey(key); err != nil {
		apgin.WriteError(gctx, a.Logger, httperr.BadRequestf("invalid %s key: %s", a.Kind.Singular, err.Error()))
		val.MarkErrorReturn()
		return
	}

	var req PutKeyValueRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, a.Logger, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if err := a.Kind.ValidateValue(req.Value); err != nil {
		apgin.WriteError(gctx, a.Logger, httperr.BadRequestf("invalid %s value: %s", a.Kind.Singular, err.Error()))
		val.MarkErrorReturn()
		return
	}

	if _, ok := a.fetchAndAuthorize(gctx, id, val, true); !ok {
		return
	}

	updated, err := a.Put(ctx, id, map[string]string{key: req.Value})
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			apgin.WriteError(gctx, a.Logger, httperr.NotFoundf("%s not found", a.ResourceName))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteErr(gctx, a.Logger, err)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, KeyValueJson{
		Key:   key,
		Value: a.Kind.Get(updated)[key],
	})
}

// HandleDelete serves DELETE /<resource>/<id>/<segment>/<key> —
// removes the key. Returns 204 on success or when the resource does
// not exist (idempotent).
func (a Adapter[ID]) HandleDelete(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, herr := a.ParseID(gctx)
	if herr != nil {
		apgin.WriteError(gctx, a.Logger, herr)
		val.MarkErrorReturn()
		return
	}

	key := gctx.Param(a.Kind.ParamName)
	if key == "" {
		apgin.WriteError(gctx, a.Logger, httperr.BadRequestf("%s key is required", a.Kind.Singular))
		val.MarkErrorReturn()
		return
	}

	if _, ok := a.fetchAndAuthorize(gctx, id, val, false); !ok {
		return
	}

	if _, err := a.Delete(ctx, id, []string{key}); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			gctx.Status(http.StatusNoContent)
			return
		}
		apgin.WriteErr(gctx, a.Logger, err)
		val.MarkErrorReturn()
		return
	}

	gctx.Status(http.StatusNoContent)
}
