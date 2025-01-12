package auth

/*
 This file implements middleware specific to the gin framework on top of what's provided in the middlewares.go file.
*/

import (
	"github.com/gin-gonic/gin"
	context2 "github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/jwt"
	"net/http"
)

const GinAuthActorKey = "auth_actor"

// GetActorInfoFromGinContext returns actor info from request if present, otherwise returns nil
func GetActorInfoFromGinContext(c *gin.Context) *jwt.Actor {
	if c == nil {
		return nil
	}

	if a, ok := c.Get(GinAuthActorKey); ok {
		return a.(*jwt.Actor)
	}

	if c.Request == nil {
		return nil
	}

	return jwt.GetActorFromContext(context2.AsContext(c.Request.Context()))
}

// MustGetActorInfoFromGinContext returns actor info from request if present, otherwise panics
// if there is not actor present (the request is unauthorized)
func MustGetActorInfoFromGinContext(c *gin.Context) *jwt.Actor {
	actor := GetActorInfoFromGinContext(c)
	if actor == nil {
		panic("actor is not present on request")
	}
	return actor
}

// SetActorInfoOnRequest sets actor into request util
func SetActorInfoOnRequest(r *http.Request, actor *jwt.Actor) *http.Request {
	ctx := r.Context()
	ctx = actor.ContextWith(ctx)
	return r.WithContext(ctx)
}

func (j *service) Required() gin.HandlerFunc {
	return func(c *gin.Context) {
		success := false
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			a := GetActorInfoFromRequest(r)
			success = a != nil
			if success {
				c.Set(GinAuthActorKey, a)
			}

			c.Next()
		})
		j.Auth(_next).ServeHTTP(c.Writer, c.Request)
		if !success {
			c.Abort()
		}
	}
}

func (j *service) Optional() gin.HandlerFunc {
	return func(c *gin.Context) {
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := GetActorInfoFromRequest(r)
			if a != nil {
				c.Set(GinAuthActorKey, a)
			}

			c.Next()
		})
		j.Trace(_next).ServeHTTP(c.Writer, c.Request)
	}
}

// AdminOnly middleware allows access for admins only
func (j *service) AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		success := false
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := GetActorInfoFromRequest(r)
			if actor == nil {
				http.Error(c.Writer, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if !actor.IsAdmin() {
				http.Error(c.Writer, "Access denied", http.StatusForbidden)
				return
			}

			// This is really duplicative of the above
			success = actor != nil && actor.IsAdmin()
			if success {
				c.Set(GinAuthActorKey, actor)
			}

			c.Next()
		})
		j.Auth(_next).ServeHTTP(c.Writer, c.Request)
		if !success {
			c.Abort()
		}
	}
}

// RBAC middleware allows role based control for routes
// this handler internally wrapped with auth(true) to avoid situation if RBAC defined without prior Auth
//func (j *service) RBAC(roles ...string) gin.HandlerFunc {
//	return func(c *gin.Context) {
//		success := false
//		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//			actor := GetActorInfoFromRequest(r)
//			if actor == nil {
//				http.Error(c.Writer, "Unauthorized", http.StatusUnauthorized)
//				return
//			}
//
//			var matched bool
//			for _, role := range roles {
//				if strings.EqualFold(role, actor.Role) {
//					matched = true
//					break
//				}
//			}
//			if !matched {
//				http.Error(c.Writer, "Access denied", http.StatusForbidden)
//				return
//			}
//
//			// This is really duplicative of the above
//			success = actor != nil && matched
//			if success {
//				c.Set(GinAuthActorKey, actor)
//			}
//
//			c.Next()
//		})
//
//		j.Auth(_next).ServeHTTP(c.Writer, c.Request)
//		if !success {
//			c.Abort()
//		}
//	}
//}
