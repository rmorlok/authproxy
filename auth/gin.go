package auth

/*
 This file implements middleware specific to the gin framework on top of what's provided in the middlewares.go file.
*/

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/api_common"
)

// GetAuthFromGinContext returns auth info from a request. This auth info can be authenticated or unauthenticated.
func GetAuthFromGinContext(c *gin.Context) RequestAuth {
	if c == nil {
		return nil
	}

	if a, ok := c.Get(authContextKey); ok {
		return a.(RequestAuth)
	}

	if c.Request == nil {
		return NewUnauthenticatedRequestAuth()
	}

	return GetAuthFromContext(c.Request.Context())
}

// MustGetAuthFromGinContext returns an authenticated request info. If the request is not authenticated, this
// method panics.
func MustGetAuthFromGinContext(c *gin.Context) RequestAuth {
	ra := GetAuthFromGinContext(c)
	if ra == nil || !ra.IsAuthenticated() {
		panic("request is not authenticated")
	}
	return ra
}

// Required middleware requires authentication and validates the actor. There must be an authenticated actor, and
// the actor must pass the validators passed here and defaulted in the service.
func (j *service) Required(validators ...ActorValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := GetAuthFromRequest(r)

			// This check is duplicative of the one in Auth, but it's here for clarity.
			if !a.IsAuthenticated() {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusUnauthorized().
					BuildStatusError().
					WriteResponse(j.config, w)
				c.Abort()
				return
			}

			c.Set(authContextKey, a)
			c.Next()
		})
		j.Auth(_next, c.Abort, validators...).ServeHTTP(c.Writer, c.Request)
	}
}

// Optional middleware allows access for unauthenticated users. If the user is authenticated, it validates the
// actor with the supplied validators and the defaults for the service.
func (j *service) Optional(validators ...ActorValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := GetAuthFromRequest(r)
			if a.IsAuthenticated() {
				c.Set(authContextKey, a)
			}

			c.Next()
		})
		j.Trace(_next, c.Abort, validators...).ServeHTTP(c.Writer, c.Request)
	}
}

// OptionalXsrfNotRequired middleware allows access for unauthenticated users and requests in session that do not have
// Xsrf. If the user is authenticated, it validates the actor with the supplied validators and the defaults for the
// service.
func (j *service) OptionalXsrfNotRequired(validators ...ActorValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := GetAuthFromRequest(r)
			if a.IsAuthenticated() {
				c.Set(authContextKey, a)
			}

			c.Next()
		})
		j.TraceXsrfNotRequired(_next, c.Abort, validators...).ServeHTTP(c.Writer, c.Request)
	}
}

// AdminOnly middleware requires and authenticates an admin actor. It applies the validators passed in addition to the
// admin validator and the default validators for the service.
func (j *service) AdminOnly(validators ...ActorValidator) gin.HandlerFunc {
	combined := combineActorValidators(validators, []ActorValidator{ActorValidatorIsAdmin})
	return j.Required(combined...)
}

func (j *service) EstablishGinSession(c *gin.Context, ra RequestAuth) error {
	return j.EstablishSession(c.Request.Context(), c.Writer, ra)
}

func (j *service) EndGinSession(c *gin.Context, ra RequestAuth) error {
	return j.EndSession(c.Request.Context(), c.Writer, ra)
}
