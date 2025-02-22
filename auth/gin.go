package auth

/*
 This file implements middleware specific to the gin framework on top of what's provided in the middlewares.go file.
*/

import (
	"github.com/gin-gonic/gin"
	context2 "github.com/rmorlok/authproxy/context"
	"net/http"
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

	return GetAuthFromContext(context2.AsContext(c.Request.Context()))
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

func (j *service) Required() gin.HandlerFunc {
	return func(c *gin.Context) {
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := GetAuthFromRequest(r)
			if a.IsAuthenticated() {
				c.Set(authContextKey, a)
				c.Next()
			}
		})
		j.Auth(_next, c.Abort).ServeHTTP(c.Writer, c.Request)
	}
}

func (j *service) Optional() gin.HandlerFunc {
	return func(c *gin.Context) {
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := GetAuthFromRequest(r)
			if a.IsAuthenticated() {
				c.Set(authContextKey, a)
			}

			c.Next()
		})
		j.Trace(_next, c.Abort).ServeHTTP(c.Writer, c.Request)
	}
}

// AdminOnly middleware allows access for admins only
func (j *service) AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		success := false
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := GetAuthFromRequest(r)
			if !a.IsAuthenticated() {
				http.Error(c.Writer, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if !a.MustGetActor().Admin {
				http.Error(c.Writer, "Access denied", http.StatusForbidden)
				return
			}

			// This is really duplicative of the above
			success = a.MustGetActor().Admin
			if success {
				c.Set(authContextKey, a)
				c.Next()
			}
		})
		j.Auth(_next, c.Abort).ServeHTTP(c.Writer, c.Request)
	}
}

func (j *service) EstablishGinSession(c *gin.Context, ra RequestAuth) error {
	ctx := context2.AsContext(c.Request.Context())
	return j.EstablishSession(ctx, c.Writer, ra)
}

func (j *service) EndGinSession(c *gin.Context, ra RequestAuth) error {
	ctx := context2.AsContext(c.Request.Context())
	return j.EndSession(ctx, c.Writer, ra)
}
