package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/context"
	jwt2 "github.com/rmorlok/authproxy/jwt"
	"net/http"
)

type A interface {
	/*
	 * Gin middlewares for establishing auth
	 */

	Required() gin.HandlerFunc
	Optional() gin.HandlerFunc
	AdminOnly() gin.HandlerFunc
	// RBAC(roles ...string) gin.HandlerFunc

	/*
	 * Middleware not specific to a framework
	 */

	Auth(next http.Handler) http.Handler  // Auth middleware adds auth from session and populates actor info
	Trace(next http.Handler) http.Handler // Trace middleware doesn't require valid actor but if actor info presented populates info

	/*
	 * Other helpers to set and get authentication.
	 */

	// Token signs claims to a JWT token using the GlobalAESKey. This is intended to generate tokens that are used
	// to roundtrip from 3rd parties, transfer authentication between services, etc.
	Token(ctx context.Context, claims *jwt2.AuthProxyClaims) (string, error)
	Parse(ctx context.Context, tokenString string) (*jwt2.AuthProxyClaims, error)
	Set(ctx context.Context, w http.ResponseWriter, claims *jwt2.AuthProxyClaims) (*jwt2.AuthProxyClaims, error)
	Get(ctx context.Context, r *http.Request) (*jwt2.AuthProxyClaims, string, error)
	Reset(w http.ResponseWriter)
}

var _ A = &service{}
