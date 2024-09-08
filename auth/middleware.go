package auth

import (
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rmorlok/authproxy/common"
	"net/http"
	"strings"
	"time"
)

/*
 This file implements middleware not specific to any particular framework.
*/

// Auth middleware adds auth from session and populates actor info
func (j *Service) Auth(next http.Handler) http.Handler {
	return j.auth(true)(next)
}

// Trace middleware doesn't require valid actor but if actor info presented populates info
func (j *Service) Trace(next http.Handler) http.Handler {
	return j.auth(false)(next)
}

// auth implements all logic for authentication (reqAuth=true) and tracing (reqAuth=false)
func (j *Service) auth(reqAuth bool) func(http.Handler) http.Handler {

	onError := func(h http.Handler, w http.ResponseWriter, r *http.Request, err error) {
		if !reqAuth { // if no auth required allow to proceeded on error
			h.ServeHTTP(w, r)
			return
		}
		j.logf("[DEBUG] auth failed, %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}

	f := func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ctx := common.AsContext(r.Context())
			claims, tkn, err := j.Get(ctx, r)
			if err != nil {
				onError(h, w, r, fmt.Errorf("can't get token: %w", err))
				return
			}

			if claims.Actor == nil {
				onError(h, w, r, fmt.Errorf("no actor info presented in the claim"))
				return
			}

			if claims.Actor != nil { // if uinfo in token populate it to context
				// validator passed by client and performs check on token or/and claims
				if j.Validator != nil && !j.Validator.Validate(tkn, claims) {
					onError(h, w, r, fmt.Errorf("actor %s blocked", claims.Actor.ID))
					j.Reset(w)
					return
				}

				if j.IsExpired(ctx, claims) {
					if claims, err = j.refreshExpiredToken(ctx, w, claims, tkn); err != nil {
						j.Reset(w)
						onError(h, w, r, fmt.Errorf("can't refresh token: %w", err))
						return
					}
				}

				r = SetActorInfoOnRequest(r, claims.Actor) // populate actor info to request context
			}

			h.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
	return f
}

// refreshExpiredToken makes a new token with passed claims
func (j *Service) refreshExpiredToken(ctx common.Context, w http.ResponseWriter, claims Claims, tkn string) (Claims, error) {

	// cache refreshed claims for given token in order to eliminate multiple refreshes for concurrent requests
	if j.RefreshCache != nil {
		if c, ok := j.RefreshCache.Get(tkn); ok {
			// already in cache
			return c, nil
		}
	}

	claims.ExpiresAt = jwt.NewNumericDate(time.Time{}) // this will cause now+duration for refreshed token
	c, err := j.Set(ctx, w, claims)                    // Set changes token
	if err != nil {
		return Claims{}, err
	}

	if j.RefreshCache != nil {
		j.RefreshCache.Set(tkn, c)
	}

	j.logf("[DEBUG] token refreshed for %+v", claims.Actor)
	return c, nil
}

// AdminOnly middleware allows access for admins only
// this handler internally wrapped with auth(true) to avoid situation if AdminOnly defined without prior Auth
func (j *Service) AdminOnly(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		actor := GetActorInfoFromRequest(r)
		if actor == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if !actor.IsAdmin() {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}
	return j.auth(true)(http.HandlerFunc(fn)) // enforce auth
}

// RBAC middleware allows role based control for routes
// this handler internally wrapped with auth(true) to avoid situation if RBAC defined without prior Auth
func (j *Service) RBAC(roles ...string) func(http.Handler) http.Handler {

	f := func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			actor := GetActorInfoFromRequest(r)
			if actor == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
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
				http.Error(w, "Access denied", http.StatusForbidden)
				return
			}
			h.ServeHTTP(w, r)
		}
		return j.auth(true)(http.HandlerFunc(fn)) // enforce auth
	}
	return f
}
