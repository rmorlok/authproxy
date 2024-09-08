package auth

/*
 This file implements middleware specific to the gin framework on top of what's provided in the middlewares.go file.
*/

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func (j *Service) Required() gin.HandlerFunc {
	return func(c *gin.Context) {
		success := false
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			a := GetActorInfoFromRequest(c.Request)
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
			a := GetActorInfoFromRequest(c.Request)
			c.Set("actor", a)
			c.Next()
		})
		j.Trace(_next).ServeHTTP(c.Writer, c.Request)
	}
}
