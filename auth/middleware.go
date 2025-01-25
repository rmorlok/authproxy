package auth

import (
	"encoding/json"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/context"
	jwt2 "github.com/rmorlok/authproxy/jwt"
	"net/http"
	"time"
)

/*
 This file implements middleware not specific to any particular framework.
*/

// MustGetAuthFromRequest gets an authenticated info for the request. If the request is not authenticated, it
// panics.
func MustGetAuthFromRequest(r *http.Request) RequestAuth {
	a := GetAuthFromRequest(r)
	if a == nil || !a.IsAuthenticated() {
		panic("request is not authenticated")
	}
	return a
}

// GetAuthFromRequest returns auth info for the request. If the request is unauthenticated, it will return
// a value indicating not authenticated.
func GetAuthFromRequest(r *http.Request) RequestAuth {

	ctx := r.Context()
	if ctx == nil {
		return NewUnauthenticatedRequestAuth()
	}

	return GetAuthFromContext(context.AsContext(ctx))
}

// SetAuthOnRequestContext sets the auth information into the context for the request so that later handlers
// can retrieve the auth information.
func SetAuthOnRequestContext(r *http.Request, auth RequestAuth) *http.Request {
	ctx := r.Context()
	ctx = auth.ContextWith(ctx)
	return r.WithContext(ctx)
}

// Auth middleware adds auth from session and populates actor info
func (j *service) Auth(next http.Handler) http.Handler {
	return j.auth(true)(next)
}

// Trace middleware doesn't require valid actor but if actor info presented populates info
func (j *service) Trace(next http.Handler) http.Handler {
	return j.auth(false)(next)
}

// auth implements all logic for authentication (reqAuth=true) and tracing (reqAuth=false)
func (j *service) auth(reqAuth bool) func(http.Handler) http.Handler {

	onError := func(h http.Handler, w http.ResponseWriter, r *http.Request, err error) {
		if !requestHasAuth(r) && !reqAuth { // if no auth required allow to proceed on error
			h.ServeHTTP(w, r)
			return
		}
		j.logf("[DEBUG] auth failed, %v", err)

		errorResponse := struct {
			Error string `json:"error"`
		}{
			Error: "authorization failed",
		}

		if j.Config.IsDebugMode() {
			errorResponse.Error = fmt.Sprintf("authorization failed: %s", err.Error())
		}

		response, _ := json.Marshal(errorResponse)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(response)
	}

	f := func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ctx := context.AsContext(r.Context())
			reqAuth, err := j.establishAuthFromRequest(ctx, r)
			if err != nil {
				onError(h, w, r, fmt.Errorf("can't get token: %w", err))
				return
			}

			r = SetAuthOnRequestContext(r, reqAuth) // populate auth/actor info to request context

			h.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
	return f
}

// refreshExpiredToken makes a new token with passed claims
func (j *service) refreshExpiredToken(ctx context.Context, w http.ResponseWriter, claims *jwt2.AuthProxyClaims, tkn string) (*jwt2.AuthProxyClaims, error) {
	if claims.IsAdmin() || claims.IsSuperAdmin() {
		return nil, errors.New("cannot refresh admin tokens")
	}

	// cache refreshed claims for given token in order to eliminate multiple refreshes for concurrent requests
	if j.RefreshCache != nil {
		if c, ok := j.RefreshCache.Get(tkn); ok {
			// already in cache
			return &c, nil
		}
	}

	claims.ExpiresAt = jwt.NewNumericDate(time.Time{}) // this will cause now+duration for refreshed token
	c, err := j.Set(ctx, w, claims)                    // Set changes token
	if err != nil {
		return nil, err
	}

	if j.RefreshCache != nil {
		j.RefreshCache.Set(tkn, *c)
	}

	j.logf("[DEBUG] token refreshed for %+v", claims.Actor)
	return c, nil
}
