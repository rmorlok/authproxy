package service

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	jwt2 "github.com/rmorlok/authproxy/internal/apauth/jwt"
)

const (
	JwtHeaderKey  = "Authorization"
	JwtQueryParam = "auth_token"
)

type A interface {
	/*
	 * Gin middlewares for establishing auth
	 */

	NewRequiredBuilder() *PermissionValidatorBuilder
	Required(validators ...AuthValidator) gin.HandlerFunc
	Optional(validators ...AuthValidator) gin.HandlerFunc
	OptionalXsrfNotRequired(validators ...AuthValidator) gin.HandlerFunc

	/*
	 * Other helpers to set and get authentication.
	 */

	// Token signs claims to a JWT token using the GlobalAESKey. This is intended to generate tokens that are used
	// to roundtrip from 3rd parties, transfer authentication between services, etc.
	Token(ctx context.Context, claims *jwt2.AuthProxyClaims) (string, error)
	Parse(ctx context.Context, tokenString string) (*jwt2.AuthProxyClaims, error)

	/*
	 * Session management
	 */

	// EstablishSession is used to start a new session explicitly from a service that is using auth. Generally this
	// will be used to session a user after that request has already been authenticated using a JWT. This method does
	// check for existing sessions and either extends them or cancels them if the auth is inconsistent.
	EstablishSession(ctx context.Context, w http.ResponseWriter, ra *core.RequestAuth) error

	// EstablishGinSession is used to start a new session explicitly from a service that is using auth. Generally this
	// will be used to session a user after that request has already been authenticated using a JWT. This method does
	// check for existing sessions and either extends them or cancels them if the auth is inconsistent. This method
	// provides a gin wrapper for the more generalized version of a similar name.
	EstablishGinSession(gctx *gin.Context, ra *core.RequestAuth) error

	// EndSession terminates a session that is in progress by clearing the session information from redis and clearing
	// session id cookies on the response.
	EndSession(ctx context.Context, w http.ResponseWriter, ra *core.RequestAuth) error

	// EndGinSession terminates a session that is in progress by clearing the session information from redis and clearing
	// session id cookies on the response. This method provides a gin wrapper for the more generalized version of a
	// similar name.
	EndGinSession(gctx *gin.Context, ra *core.RequestAuth) error

	// WithDefaultAuthValidators returns a new service with the given actor validators added to the list of validators
	// that are used to validate actors. The original service will not be modified. The validators are applied to all
	// requests that are authenticated. Unauthenticated requests will not be affected.
	WithDefaultAuthValidators(validators ...AuthValidator) A
}

var _ A = &service{}
