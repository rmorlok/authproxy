package auth

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	jwt2 "github.com/rmorlok/authproxy/internal/jwt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
)

// keyForToken loads an appropriate key to sign or verify a given token. This accounts for the
// fact that admin users will verify with different keys to sign/verify tokens.
func (s *service) keyForToken(claims *jwt2.AuthProxyClaims) (config.Key, error) {
	if claims.IsAdmin() {
		adminUsername, err := claims.AdminUsername()
		if err != nil {
			return nil, errors.Wrap(err, "failed to establish admin username to sign jwt")
		}

		adminUser, ok := s.config.GetRoot().SystemAuth.AdminUsers.GetByUsername(adminUsername)
		if !ok {
			return nil, errors.Errorf("admin user '%s' not found", adminUsername)
		}

		return adminUser.Key, nil
	} else {
		return s.config.GetRoot().SystemAuth.JwtSigningKey, nil
	}
}

// Token mints a signed JWT with the specified claims. The token will be self-signed using the GlobalAESKey. The
// claims will be modified prior to signing to indicate which service signed this token and that it is self-signed.
func (s *service) Token(ctx context.Context, claims *jwt2.AuthProxyClaims) (string, error) {
	claimsClone := *claims
	claimsClone.Issuer = string(s.service.GetId())
	claimsClone.IssuedAt = jwt.NewNumericDate(apctx.GetClock(ctx).Now())
	claimsClone.SelfSigned = true

	audiences, err := claimsClone.GetAudience()
	if err != nil {
		return "", errors.Wrap(err, "failed to get aud")
	}

	if !config.AllValidServiceIds(audiences) {
		return "", errors.New("some service ids in aud are invalid")
	}

	data, err := s.config.GetRoot().SystemAuth.GlobalAESKey.GetData(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get global aes key")
	}

	return jwt2.
		NewJwtTokenBuilder().
		WithClaims(&claimsClone).
		WithSecretKey(data).
		WithSelfSigned().
		TokenCtx(ctx)
}

// Parse token string and verify.
func (s *service) Parse(ctx context.Context, tokenString string) (*jwt2.AuthProxyClaims, error) {
	claims, err := jwt2.NewJwtTokenParserBuilder().
		WithKeySelector(func(ctx context.Context, unverified *jwt2.AuthProxyClaims) (kd config.KeyData, isShared bool, err error) {
			if unverified.SelfSigned {
				return s.config.GetRoot().SystemAuth.GlobalAESKey, true, nil
			}

			key, err := s.keyForToken(unverified)
			if err != nil {
				return nil, false, errors.Wrap(err, "failed to get key")
			}

			if pk, ok := key.(*config.KeyPublicPrivate); ok {
				return pk.PublicKey, false, nil
			}

			if sk, ok := key.(*config.KeyShared); ok {
				return sk.SharedKey, true, nil
			}

			return nil, false, errors.New("invalid key type")
		}).
		ParseCtx(ctx, tokenString)
	if err != nil {
		return nil, errors.Wrap(err, "can't parse token")
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
			return nil, errors.Errorf("aud '%s' not valid for service '%s'", strings.Join(claims.Audience, ","), s.service.GetId())
		}
		return nil, errors.Errorf("aud not specified for service '%s'", s.service.GetId())
	}

	return claims, s.validate(ctx, claims)
}

func (s *service) validate(ctx context.Context, claims *jwt2.AuthProxyClaims) error {
	v := jwt.NewValidator(
		jwt.WithTimeFunc(func() time.Time {
			return apctx.GetClock(ctx).Now()
		}),
	)

	return v.Validate(claims)
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
			return "", true, errors.Wrap(err, "failed to extract token from authorization header")
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
func (s *service) establishAuthFromRequest(ctx context.Context, requireSessionXsrf bool, r *http.Request, w http.ResponseWriter) (RequestAuth, error) {
	var err error
	var claims *jwt2.AuthProxyClaims
	var ra = NewUnauthenticatedRequestAuth()
	tokenString := ""

	// try to get from "token" query param
	if tkQuery, hasValue := getJwtTokenFromQuery(r); hasValue {
		tokenString = tkQuery
	}

	// try to get from JWT header
	if tokenString == "" {
		if tokenHeader, hasValue, err := getJwtTokenFromHeader(r); hasValue || err != nil {
			if err != nil {
				return NewUnauthenticatedRequestAuth(), err
			}

			tokenString = tokenHeader
		}
	}

	if tokenString != "" {
		claims, err = s.Parse(ctx, tokenString)
		if err != nil {
			return NewUnauthenticatedRequestAuth(), api_common.NewHttpStatusErrorBuilder().
				WithStatusUnauthorized().
				WithResponseMsg("invalid token").
				WithInternalErr(errors.Wrap(err, "failed to get token")).
				Build()
		}

		if claims.IsExpired(ctx) {
			return NewUnauthenticatedRequestAuth(), api_common.NewHttpStatusErrorBuilder().
				WithStatusUnauthorized().
				WithResponseMsg("token is expired").
				Build()
		}

		if claims.Nonce != nil {
			if claims.ExpiresAt == nil {
				return NewUnauthenticatedRequestAuth(), api_common.NewHttpStatusErrorBuilder().
					WithStatusUnauthorized().
					WithResponseMsg("cannot use nonce in jwt without expiration time").
					Build()
			}

			s.logger.Debug("using nonce", "nonce", claims.Nonce.String())

			wasValid, err := s.db.CheckNonceValidAndMarkUsed(ctx, *claims.Nonce, claims.ExpiresAt.Time)
			if err != nil {
				return NewUnauthenticatedRequestAuth(), api_common.NewHttpStatusErrorBuilder().
					WithStatusUnauthorized().
					WithResponseMsg("failed to verify jwt details").
					WithInternalErr(errors.Wrap(err, "can't check nonce")).
					Build()
			}

			if !wasValid {
				return NewUnauthenticatedRequestAuth(), api_common.NewHttpStatusErrorBuilder().
					WithStatusUnauthorized().
					WithResponseMsg("jwt nonce already used").
					Build()
			}
		}

		var actor *database.Actor
		if claims.Actor == nil {
			// This implies that the subject of the claim is the external id of the actor, and the actor must already
			// exist. If the actor does not exist, the claim is invalid. Admins are special cased so that they can
			// be created dynamically based on information in the system.
			actor, err = s.db.GetActorByExternalId(ctx, claims.Subject)
			if err != nil {
				return NewUnauthenticatedRequestAuth(), api_common.NewHttpStatusErrorBuilder().
					WithStatusInternalServerError().
					WithResponseMsg("database error").
					WithInternalErr(errors.Wrap(err, "failed to get actor")).
					Build()
			}
			if actor == nil {
				unauthenticatedError := api_common.NewHttpStatusErrorBuilder().
					WithStatusUnauthorized().
					WithResponseMsg("actor does not exist").
					WithInternalErr(errors.Errorf("actor '%s' not found", claims.Subject)).
					Build()

				if claims.IsAdmin() {
					if cfgAdmin, ok := s.config.GetRoot().SystemAuth.AdminUsers.GetByJwtSubject(claims.Subject); ok {
						adminDomain := "local"
						if s.config.GetRoot().SystemAuth.AdminEmailDomain != "" {
							adminDomain = s.config.GetRoot().SystemAuth.AdminEmailDomain
						}

						email := fmt.Sprintf("%s@%s", cfgAdmin.Username, adminDomain)
						if cfgAdmin.Email != "" {
							email = cfgAdmin.Email
						}

						adminActor := database.Actor{
							ID:         uuid.New(),
							ExternalId: claims.Subject,
							Email:      email,
							Admin:      true,
						}

						err = s.db.CreateActor(ctx, &adminActor)
						if err != nil {
							return NewUnauthenticatedRequestAuth(), api_common.NewHttpStatusErrorBuilder().
								WithStatusInternalServerError().
								WithResponseMsg("database error").
								WithInternalErr(errors.Wrap(err, "failed to upsert admin actor")).
								Build()
						}

						// success
						actor = &adminActor

					} else {
						return NewUnauthenticatedRequestAuth(), unauthenticatedError
					}

				} else {
					return NewUnauthenticatedRequestAuth(), unauthenticatedError
				}
			}
		} else {
			// The actor was specified in the JWT. This means that we can upsert an actor, either creating it or making
			// it consistent with the request's definition.
			actor, err = s.db.UpsertActor(ctx, claims.Actor)
			if err != nil {
				return NewUnauthenticatedRequestAuth(), api_common.NewHttpStatusErrorBuilder().
					WithStatusInternalServerError().
					WithResponseMsg("database error").
					WithInternalErr(errors.Wrap(err, "failed to upsert actor")).
					Build()
			}
		}

		ra = &requestAuth{
			actor: actor,
		}
	}

	// Extend auth with session, or establish the user authed from session if not authenticated yet
	ra, err = s.establishAuthFromSession(ctx, requireSessionXsrf, r, w, ra)
	if err != nil {
		return NewUnauthenticatedRequestAuth(), api_common.NewHttpStatusErrorBuilder().
			WithStatusUnauthorized().
			WithResponseMsg("failed to establish auth from session").
			WithInternalErr(errors.Wrap(err, "failed to establish auth from session")).
			Build()
	}

	return ra, nil
}
