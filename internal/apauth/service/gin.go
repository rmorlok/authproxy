package service

/*
 This file implements middleware specific to the gin framework on top of what's provided in the middlewares.go file.
*/

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/api_common"
)

// GetAuthFromGinContext returns auth info from a request. This auth info can be authenticated or unauthenticated.
func GetAuthFromGinContext(c *gin.Context) *core.RequestAuth {
	if c == nil {
		return nil
	}

	if c.Request == nil {
		return core.NewUnauthenticatedRequestAuth()
	}

	return core.GetAuthFromContext(c.Request.Context())
}

// applyAuthToGinContext applies the auth info to the request context.
func applyAuthToGinContext(c *gin.Context, ra *core.RequestAuth) {
	if c == nil || c.Request == nil {
		return
	}

	ctx := c.Request.Context()
	ctx = ra.ContextWith(ctx)
	c.Request = c.Request.WithContext(ctx)
}

// MustGetAuthFromGinContext returns an authenticated request info. If the request is not authenticated, this
// method panics.
func MustGetAuthFromGinContext(c *gin.Context) *core.RequestAuth {
	ra := GetAuthFromGinContext(c)
	if ra == nil || !ra.IsAuthenticated() {
		panic("request is not authenticated")
	}
	return ra
}

// Required middleware requires authentication and validates the actor. There must be an authenticated actor, and
// the actor must pass the validators passed here and defaulted in the service.
func (j *service) Required(validators ...AuthValidator) gin.HandlerFunc {
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

			combinedValidators := combineAuthValidators(j.defaultAuthValidators, validators)
			valid, reason := validateAllAuthValidators(combinedValidators, c, a)
			if !valid {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusForbidden().
					WithResponseMsg(reason).
					BuildStatusError().
					WriteResponse(j.config, w)
				c.Abort()
				return
			}

			applyAuthToGinContext(c, a)
			c.Next()
		})
		j.Auth(_next, c.Abort).ServeHTTP(c.Writer, c.Request)
	}
}

// Optional middleware allows access for unauthenticated users. If the user is authenticated, it validates the
// actor with the supplied validators and the defaults for the service.
func (j *service) Optional(validators ...AuthValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := GetAuthFromRequest(r)
			if a.IsAuthenticated() {

				combinedValidators := combineAuthValidators(j.defaultAuthValidators, validators)
				valid, reason := validateAllAuthValidators(combinedValidators, c, a)
				if !valid {
					api_common.NewHttpStatusErrorBuilder().
						WithStatusForbidden().
						WithResponseMsg(reason).
						BuildStatusError().
						WriteResponse(j.config, w)
					c.Abort()
					return
				}
				applyAuthToGinContext(c, a)
			}

			c.Next()
		})
		j.Trace(_next, c.Abort).ServeHTTP(c.Writer, c.Request)
	}
}

// OptionalXsrfNotRequired middleware allows access for unauthenticated users and requests in session that do not have
// Xsrf. If the user is authenticated, it validates the actor with the supplied validators and the defaults for the
// service.
func (j *service) OptionalXsrfNotRequired(validators ...AuthValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := GetAuthFromRequest(r)
			if a.IsAuthenticated() {
				combinedValidators := combineAuthValidators(j.defaultAuthValidators, validators)
				valid, reason := validateAllAuthValidators(combinedValidators, c, a)
				if !valid {
					api_common.NewHttpStatusErrorBuilder().
						WithStatusForbidden().
						WithResponseMsg(reason).
						BuildStatusError().
						WriteResponse(j.config, w)
					c.Abort()
					return
				}
				applyAuthToGinContext(c, a)
			}

			c.Next()
		})
		j.TraceXsrfNotRequired(_next, c.Abort).ServeHTTP(c.Writer, c.Request)
	}
}

// AdminOnly middleware requires and authenticates an admin actor. It applies the validators passed in addition to the
// admin validator and the default validators for the service.
func (j *service) AdminOnly(validators ...AuthValidator) gin.HandlerFunc {
	combined := combineAuthValidators(validators, []AuthValidator{AuthValidatorActorIsAdmin})
	return j.Required(combined...)
}

func (j *service) EstablishGinSession(c *gin.Context, ra *core.RequestAuth) error {
	return j.EstablishSession(c.Request.Context(), c.Writer, ra)
}

func (j *service) EndGinSession(c *gin.Context, ra *core.RequestAuth) error {
	return j.EndSession(c.Request.Context(), c.Writer, ra)
}
