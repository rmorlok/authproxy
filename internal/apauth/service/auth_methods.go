package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rmorlok/authproxy/internal/apauth/core"
	jwt2 "github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/schema/config"

	"github.com/golang-jwt/jwt/v5"
)

// keyForToken loads an appropriate key to sign or verify a given token. This accounts for the
// fact that admin users will verify with different keys to sign/verify tokens.
// For admin users, the key is retrieved from the database where it is stored encrypted.
func (s *service) keyForToken(ctx context.Context, claims *jwt2.AuthProxyClaims) (*config.Key, error) {
	// If no database is configured, fall back to JWT signing key
	if s.db == nil {
		return s.config.GetRoot().SystemAuth.JwtSigningKey, nil
	}

	// Check actor cache first, then fall back to database
	cache := getActorCache(ctx)
	actor := cache.GetByExternalId(claims.GetNamespace(), claims.Subject)
	if actor == nil {
		var err error
		actor, err = s.db.GetActorByExternalId(ctx, claims.GetNamespace(), claims.Subject)
		if err != nil {
			if errors.Is(err, database.ErrNotFound) {
				// Actor does not exist. Fall back to JWT signing key
				return s.config.GetRoot().SystemAuth.JwtSigningKey, nil
			}
			return nil, fmt.Errorf("failed to get actor: %w", err)
		}
		cache.Put(actor)
	}

	if actor.EncryptedKey == nil {
		// Actor is not configured with self-signing key
		return s.config.GetRoot().SystemAuth.JwtSigningKey, nil
	}

	// Decrypt the key JSON
	decrypted, err := s.encrypt.DecryptString(ctx, *actor.EncryptedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt actor key: %w", err)
	}

	// Unmarshal to *config.Key
	var key config.Key
	if err := json.Unmarshal([]byte(decrypted), &key); err != nil {
		return nil, fmt.Errorf("failed to unmarshal actor key: %w", err)
	}

	return &key, nil
}

// keyForActorSignedToken returns the key for verifying an actor-signed token.
// For actors with their own key, returns the actor's key.
// Falls back to GlobalAESKey if no key is found.
func (s *service) keyForActorSignedToken(ctx context.Context, claims *jwt2.AuthProxyClaims) (*config.Key, error) {
	// If no database or encrypt service, fall back to GlobalAESKey
	if s.db == nil || s.encrypt == nil {
		return &config.Key{
			InnerVal: &config.KeyShared{
				SharedKey: s.config.GetRoot().SystemAuth.GlobalAESKey,
			},
		}, nil
	}

	// Check actor cache first, then fall back to database
	cache := getActorCache(ctx)
	actor := cache.GetByExternalId(claims.GetNamespace(), claims.Subject)
	if actor == nil {
		var err error
		actor, err = s.db.GetActorByExternalId(ctx, claims.GetNamespace(), claims.Subject)
		if err != nil {
			if errors.Is(err, database.ErrNotFound) {
				// Actor does not exist. Fall back to GlobalAESKey
				return &config.Key{
					InnerVal: &config.KeyShared{
						SharedKey: s.config.GetRoot().SystemAuth.GlobalAESKey,
					},
				}, nil
			}
			return nil, fmt.Errorf("failed to get actor: %w", err)
		}
		cache.Put(actor)
	}

	if actor.EncryptedKey == nil {
		// Actor is not configured with self-signing key
		return &config.Key{
			InnerVal: &config.KeyShared{
				SharedKey: s.config.GetRoot().SystemAuth.GlobalAESKey,
			},
		}, nil
	}

	// Decrypt the key JSON
	decrypted, err := s.encrypt.DecryptString(ctx, *actor.EncryptedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt actor key: %w", err)
	}

	// Unmarshal to *config.Key
	var key config.Key
	if err := json.Unmarshal([]byte(decrypted), &key); err != nil {
		return nil, fmt.Errorf("failed to unmarshal actor key: %w", err)
	}

	return &key, nil
}

// Token mints a signed JWT with the specified claims. The token will be self-signed using the GlobalAESKey. The
// claims will be modified prior to signing to indicate which service signed this token and that it is self-signed.
func (s *service) Token(ctx context.Context, claims *jwt2.AuthProxyClaims) (string, error) {
	claimsClone := *claims
	claimsClone.Issuer = string(s.service.GetId())
	claimsClone.IssuedAt = jwt.NewNumericDate(apctx.GetClock(ctx).Now())
	claimsClone.SystemSigned = true

	audiences, err := claimsClone.GetAudience()
	if err != nil {
		return "", fmt.Errorf("failed to get aud: %w", err)
	}

	if !config.AllValidServiceIds(audiences) {
		return "", errors.New("some service ids in aud are invalid")
	}

	ver, err := s.config.GetRoot().SystemAuth.GlobalAESKey.GetCurrentVersion(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get global aes key: %w", err)
	}

	return jwt2.
		NewJwtTokenBuilder().
		WithClaims(&claimsClone).
		WithSecretKey(ver.Data).
		WithSystemSigned().
		TokenCtx(ctx)
}

// Parse token string and verify.
func (s *service) Parse(ctx context.Context, tokenString string) (*jwt2.AuthProxyClaims, error) {
	claims, err := jwt2.NewJwtTokenParserBuilder().
		WithKeySelector(func(ctx context.Context, unverified *jwt2.AuthProxyClaims) (kd config.KeyDataType, isShared bool, err error) {
			if unverified.SystemSigned {
				// System-signed tokens are minted internally by services
				// (e.g. OAuth redirect tokens, inter-service tokens) using GlobalAESKey.
				return s.config.GetRoot().SystemAuth.GlobalAESKey, true, nil
			}

			if unverified.ActorSigned {
				// Actor-signed tokens are minted externally (e.g. CLI) using
				// the actor's private key. Look up the actor's public key to verify.
				key, err := s.keyForActorSignedToken(ctx, unverified)
				if err != nil {
					return nil, false, fmt.Errorf("failed to get key: %w", err)
				}

				if pk, ok := key.InnerVal.(*config.KeyPublicPrivate); ok {
					return pk.PublicKey, false, nil
				}

				// Fall back to GlobalAESKey if no asymmetric key found
				return s.config.GetRoot().SystemAuth.GlobalAESKey, true, nil
			}

			// For non-self-signed tokens, use keyForToken
			key, err := s.keyForToken(ctx, unverified)
			if err != nil {
				return nil, false, fmt.Errorf("failed to get key: %w", err)
			}

			if pk, ok := key.InnerVal.(*config.KeyPublicPrivate); ok {
				return pk.PublicKey, false, nil
			}

			if sk, ok := key.InnerVal.(*config.KeyShared); ok {
				return sk.SharedKey, true, nil
			}

			return nil, false, errors.New("invalid key type")
		}).
		ParseCtx(ctx, tokenString)
	if err != nil {
		return nil, fmt.Errorf("can't parse token: %w", err)
	}

	found := false
	for _, aud := range claims.Audience {
		if aud == string(s.service.GetId()) {
			found = true
			break
		}
	}
	if !found {
		if len(claims.Audience) > 0 {
			return nil, fmt.Errorf("aud '%s' not valid for service '%s'", strings.Join(claims.Audience, ","), s.service.GetId())
		}
		return nil, fmt.Errorf("aud not specified for service '%s'", s.service.GetId())
	}

	return claims, s.validate(ctx, claims)
}

func (s *service) validate(ctx context.Context, claims *jwt2.AuthProxyClaims) error {
	v := jwt.NewValidator(
		jwt.WithTimeFunc(func() time.Time {
			return apctx.GetClock(ctx).Now()
		}),
	)

	return claims.Validate(v)
}

func JwtBearerHeaderVal(tokenString string) string {
	return fmt.Sprintf("Bearer %s", tokenString)
}

func SetJwtHeader(h http.Header, tokenString string) {
	h.Set(JwtHeaderKey, JwtBearerHeaderVal(tokenString))
}

func SetJwtRequestHeader(w *http.Request, tokenString string) {
	SetJwtHeader(w.Header, tokenString)
}

func SetJwtResponseHeader(w http.ResponseWriter, tokenString string) {
	SetJwtHeader(w.Header(), tokenString)
}

func (s *service) setJwtResponseHeader(w http.ResponseWriter, tokenString string) {
	SetJwtResponseHeader(w, tokenString)
}

// extractTokenFromBearer extracts the token v
func extractTokenFromBearer(authorizationHeader string) (string, error) {
	if strings.HasPrefix(authorizationHeader, "Bearer ") {
		return strings.TrimPrefix(authorizationHeader, "Bearer "), nil
	}

	return "", errors.New("no bearer token found")
}

func SetJwtQueryParm(q url.Values, tokenString string) {
	q.Set(JwtQueryParam, tokenString)
}

func getJwtTokenFromQuery(r *http.Request) (token string, hasValue bool) {
	tokenQuery := r.URL.Query().Get(JwtQueryParam)
	if tokenQuery == "" {
		return "", false
	}

	return tokenQuery, true
}

func getJwtTokenFromHeader(r *http.Request) (token string, hasValue bool, err error) {
	if tokenHeader := r.Header.Get(JwtHeaderKey); tokenHeader != "" {
		tokenString, err := extractTokenFromBearer(tokenHeader)
		if err != nil {
			return "", true, fmt.Errorf("failed to extract token from authorization header: %w", err)
		}

		if tokenString != "" {
			return tokenString, true, nil
		}
	}

	return "", false, nil
}

// establishAuthFromRequest is the top-level method for managing auth. It looks for authentication in all the places
// supported by the configuration, makes sure any incoming JWTs are consistent with the auth information stored in the
// database, and returns the established internal actor. This method will error if no auth is present, or the
// information is somehow inconsistent with the auth state of the system.
//
// When establishing auth, a JWT always takes precedence over a session. If the session and JWT differ, the session will
// be terminated and the caller must decide to start a new session explicitly.
func (s *service) establishAuthFromRequest(ctx context.Context, requireSessionXsrf bool, r *http.Request, w http.ResponseWriter) (*core.RequestAuth, error) {
	// Initialize per-request actor cache to avoid redundant DB lookups
	ctx = withActorCache(ctx)

	var err error
	var claims *jwt2.AuthProxyClaims
	var ra = core.NewUnauthenticatedRequestAuth()
	tokenString := ""

	// try to get from "token" query param
	if tkQuery, hasValue := getJwtTokenFromQuery(r); hasValue {
		tokenString = tkQuery
	}

	// try to get from JWT header
	if tokenString == "" {
		if tokenHeader, hasValue, err := getJwtTokenFromHeader(r); hasValue || err != nil {
			if err != nil {
				return core.NewUnauthenticatedRequestAuth(), err
			}

			tokenString = tokenHeader
		}
	}

	if tokenString != "" {
		claims, err = s.Parse(ctx, tokenString)
		if err != nil {
			if errors.Is(err, jwt2.ErrInvalidClaims) {
				return core.NewUnauthenticatedRequestAuth(), httperr.Unauthorized(httperr.WithPublicErr(err))
			}

			return core.NewUnauthenticatedRequestAuth(), httperr.UnauthorizedMsg("invalid token", httperr.WithInternalErrorf("failed to get token: %w", err))
		}

		if claims.IsExpired(ctx) {
			return core.NewUnauthenticatedRequestAuth(), httperr.UnauthorizedMsg("token is expired")
		}

		if claims.Nonce != nil {
			if claims.ExpiresAt == nil {
				return core.NewUnauthenticatedRequestAuth(), httperr.UnauthorizedMsg("cannot use nonce in jwt without expiration time")
			}

			s.logger.Debug("using nonce", "nonce", claims.Nonce.String())

			wasValid, err := s.db.CheckNonceValidAndMarkUsed(ctx, *claims.Nonce, claims.ExpiresAt.Time)
			if err != nil {
				return core.NewUnauthenticatedRequestAuth(), httperr.UnauthorizedMsg("failed to verify jwt details", httperr.WithInternalErrorf("can't check nonce: %w", err))
			}

			if !wasValid {
				return core.NewUnauthenticatedRequestAuth(), httperr.UnauthorizedMsg("jwt nonce already used")
			}
		}

		cache := getActorCache(ctx)
		var actor *database.Actor
		if claims.Actor == nil {
			// This implies that the subject of the claim is the external id of the actor, and the actor must already
			// exist in the database. If the actor does not exist, the claim is invalid.
			// Check the cache first to avoid a redundant DB lookup (the actor may have been loaded during key selection).
			actor = cache.GetByExternalId(claims.GetNamespace(), claims.Subject)
			if actor == nil {
				actor, err = s.db.GetActorByExternalId(ctx, claims.GetNamespace(), claims.Subject)
				if err != nil {
					if errors.Is(err, database.ErrNotFound) {
						return core.NewUnauthenticatedRequestAuth(), httperr.UnauthorizedMsg("actor does not exist", httperr.WithInternalErrorf("actor '%s' not found", claims.Subject))
					}
					return core.NewUnauthenticatedRequestAuth(), httperr.InternalServerErrorMsg("database error", httperr.WithInternalErrorf("failed to get actor: %w", err))
				}
				cache.Put(actor)
			}
		} else {
			// The actor was specified in the JWT. This means that we can upsert an actor, either creating it or making
			// it consistent with the request's definition.
			actor, err = s.db.UpsertActor(ctx, claims.Actor)
			if err != nil {
				return core.NewUnauthenticatedRequestAuth(), httperr.InternalServerErrorMsg("database error", httperr.WithInternalErrorf("failed to upsert actor: %w", err))
			}
			cache.Put(actor)
		}

		ra = core.NewAuthenticatedRequestAuth(actor)
	}

	// Extend auth with session, or establish the user authed from session if not authenticated yet
	ra, err = s.establishAuthFromSession(ctx, requireSessionXsrf, r, w, ra)
	if err != nil {
		return core.NewUnauthenticatedRequestAuth(), httperr.FromErrorf("failed to establish auth from session: %w", err)
	}

	return ra, nil
}
