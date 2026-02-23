package service

import (
	"net/http"

	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/api_common"
)

/*
 This file implements middleware not specific to any particular framework.
*/

// MustGetAuthFromRequest gets a RequestInfo for the request. If the request is not authenticated, it
// panics.
func MustGetAuthFromRequest(r *http.Request) *core.RequestAuth {
	a := GetAuthFromRequest(r)
	if a == nil || !a.IsAuthenticated() {
		panic("request is not authenticated")
	}
	return a
}

// GetAuthFromRequest returns auth info for the request. If the request is unauthenticated, it will return
// a value indicating not authenticated.
func GetAuthFromRequest(r *http.Request) *core.RequestAuth {
	ctx := r.Context()
	if ctx == nil {
		return core.NewUnauthenticatedRequestAuth()
	}

	return core.GetAuthFromContext(ctx)
}

// SetAuthOnRequestContext sets the auth information into the context for the request so that later handlers
// can retrieve the auth information.
func SetAuthOnRequestContext(r *http.Request, auth *core.RequestAuth) *http.Request {
	ctx := r.Context()
	ctx = auth.ContextWith(ctx)
	return r.WithContext(ctx)
}

// Auth middleware adds auth from session and populates actor info
func (j *service) Auth(next http.Handler, abort func()) http.Handler {
	return j.auth(true, true, abort)(next)
}

// Trace middleware doesn't require a valid actor but if an actor is present it populates the actor info. If present
// the actor is validated against the supplied validators.
func (j *service) Trace(next http.Handler, abort func()) http.Handler {
	return j.auth(false, true, abort)(next)
}

// TraceXsrfNotRequired is the same as the Trace middleware except that it doesn't require a valid Xsrf token if session
// auth is being used.
func (j *service) TraceXsrfNotRequired(next http.Handler, abort func()) http.Handler {
	return j.auth(false, false, abort)(next)
}

// auth implements all logic for authentication (reqAuth=true) and tracing (reqAuth=false)
func (j *service) auth(requireAuth bool, requireSessionXsrf bool, abort func()) func(http.Handler) http.Handler {
	f := func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestAuth, err := j.establishAuthFromRequest(ctx, requireSessionXsrf, r, w)
			if err != nil {
				// We treat any errors as a failure, even if the resulting status is unauthorized. Not passing
				// a JWT will just result in you requesting this endpoint without authentication, but passing a bad
				// JWT will result in some sort of error -- unauthorized or otherwise.
				httpStatusErr := api_common.AsHttpStatusError(err)
				httpStatusErr.WriteResponse(r.Context(), nil, w)
				abort()
				return
			}

			if requireAuth && !requestAuth.IsAuthenticated() {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusUnauthorized().
					BuildStatusError().
					WriteResponse(r.Context(), nil, w)
				abort()
				return
			}

			r = SetAuthOnRequestContext(r, requestAuth) // populate auth/actor info to request context

			h.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
	return f
}
