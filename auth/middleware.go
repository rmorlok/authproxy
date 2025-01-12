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

// MustGetActorInfoFromRequest gets actor info and panics if can't extract it from the request.
// should be called from authenticated controllers only
func MustGetActorInfoFromRequest(r *http.Request) *jwt2.Actor {
	actor := GetActorInfoFromRequest(r)
	if actor == nil {
		panic("actor is not present on request")
	}
	return actor
}

// GetActorInfoFromRequest returns actor info from request if present, otherwise returns nil
func GetActorInfoFromRequest(r *http.Request) *jwt2.Actor {

	ctx := r.Context()
	if ctx == nil {
		return nil
	}

	return jwt2.GetActorFromContext(context.AsContext(ctx))
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
			claims, _, err := j.Get(ctx, r)
			if err != nil {
				onError(h, w, r, fmt.Errorf("can't get token: %w", err))
				return
			}

			if claims.Actor == nil {
				onError(h, w, r, fmt.Errorf("no actor info presented in the claim"))
				return
			}

			if claims.IsExpired(ctx) {
				onError(h, w, r, fmt.Errorf("claim is expired"))
				return
				// TODO: look at token refresh for appropriate cases
				//if claims, err = j.refreshExpiredToken(ctx, w, claims, tkn); err != nil {
				//	j.Reset(w)
				//	onError(h, w, r, fmt.Errorf("can't refresh token: %w", err))
				//	return
				//}
			}

			if claims.Nonce != nil {
				if claims.ExpiresAt == nil {
					onError(h, w, r, fmt.Errorf("cannot use nonce without expiration time"))
					return
				}

				j.logf("[DEBUG] using nonce: %s", claims.Nonce.String())

				wasValid, err := j.Db.CheckNonceValidAndMarkUsed(ctx, *claims.Nonce, claims.ExpiresAt.Time)
				if err != nil {
					onError(h, w, r, errors.Wrap(err, "can't check nonce"))
					return
				}

				if !wasValid {
					onError(h, w, r, fmt.Errorf("nonce already used"))
					return
				}
			}

			r = SetActorInfoOnRequest(r, claims.Actor) // populate actor info to request context

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
