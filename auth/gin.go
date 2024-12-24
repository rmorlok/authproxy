package auth

/*
 This file implements middleware specific to the gin framework on top of what's provided in the middlewares.go file.
*/

import (
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/jwt"
	"net/http"
	"strings"
)

func (j *Service) Required() gin.HandlerFunc {
	return func(c *gin.Context) {
		success := false
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			a := jwt.GetActorInfoFromRequest(c.Request)
			c.Set("actor", a)
			success = a != nil

			c.Next()
		})
		j.Auth(_next).ServeHTTP(c.Writer, c.Request)
		if !success {
			c.Abort()
		}
	}
}

func (j *Service) Optional() gin.HandlerFunc {
	return func(c *gin.Context) {
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := jwt.GetActorInfoFromRequest(c.Request)
			c.Set("actor", a)
			c.Next()
		})
		j.Trace(_next).ServeHTTP(c.Writer, c.Request)
	}
}

// AdminOnly middleware allows access for admins only
func (j *Service) AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		success := false
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := jwt.GetActorInfoFromRequest(r)
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
func (j *Service) RBAC(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		success := false
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := jwt.GetActorInfoFromRequest(c.Request)
			if actor == nil {
				http.Error(c.Writer, "Unauthorized", http.StatusUnauthorized)
				return
			}

			var matched bool
			for _, role := range roles {
				if strings.EqualFold(role, actor.Role) {
					matched = true
					break
				}
			}
			if !matched {
				http.Error(c.Writer, "Access denied", http.StatusForbidden)
				return
			}

			// This is really duplicative of the above
			success = actor != nil && matched

			c.Next()
		})

		j.Auth(_next).ServeHTTP(c.Writer, c.Request)
		if !success {
			c.Abort()
		}
	}
}
