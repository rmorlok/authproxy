package auth

import (
	"fmt"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
	jwt2 "github.com/rmorlok/authproxy/jwt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
)

func verificationKeyData(key config.Key) config.KeyData {
	switch v := key.(type) {
	case *config.KeyPublicPrivate:
		return v.PublicKey
	case *config.KeyShared:
		return v.SharedKey
	default:
		panic("key does not support verification")
	}
}

// keyDataForToken loads an appropriate key to sign or verify a given token. This accounts for the
// fact that admin users will verify with different keys to sign/verify tokens.
func (s *service) keyForToken(claims *jwt2.AuthProxyClaims) (config.Key, error) {
	if claims.IsAdmin() {
		adminUsername, err := claims.AdminUsername()
		if err != nil {
			return nil, errors.Wrap(err, "failed to establish admin username to sign jwt")
		}

		adminUser, ok := s.Config.GetRoot().SystemAuth.AdminUsers.GetByUsername(adminUsername)
		if !ok {
			return nil, errors.Errorf("admin user '%s' not found", adminUsername)
		}

		return adminUser.Key, nil
	} else {
		return s.Config.GetRoot().SystemAuth.JwtSigningKey, nil
	}
}

// Token mints a signed JWT with the specified claims. The token will be self-signed using the GlobalAESKey. The
// claims will be modified prior to signing to indicate which service signed this token and that it is self-signed.
func (s *service) Token(ctx context.Context, claims *jwt2.AuthProxyClaims) (string, error) {
	claimsClone := *claims
	claimsClone.Issuer = string(s.Opts.Service.GetId())
	claimsClone.IssuedAt = jwt.NewNumericDate(ctx.Clock().Now())
	claimsClone.SelfSigned = true

	audiences, err := claimsClone.GetAudience()
	if err != nil {
		return "", errors.Wrap(err, "failed to get aud")
	}

	if !config.AllValidServiceIds(audiences) {
		return "", errors.New("some service ids in aud are invalid")
	}

	data, err := s.Config.GetRoot().SystemAuth.GlobalAESKey.GetData(ctx)
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
				return s.Config.GetRoot().SystemAuth.GlobalAESKey, true, nil
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
		if aud == string(s.Opts.Service.GetId()) {
			found = true
			break
		}
	}
	if !found {
		if len(claims.Audience) > 0 {
			return nil, errors.Errorf("aud '%s' not valid for service '%s'", strings.Join(claims.Audience, ","), s.Opts.Service.GetId())
		}
		return nil, errors.Errorf("aud not specified for service '%s'", s.Opts.Service.GetId())
	}

	return claims, s.validate(ctx, claims)
}

func (s *service) validate(ctx context.Context, claims *jwt2.AuthProxyClaims) error {
	v := jwt.NewValidator(
		jwt.WithTimeFunc(func() time.Time {
			return ctx.Clock().Now()
		}),
	)

	return v.Validate(claims)
}

func JwtBearerHeaderVal(tokenString string) string {
	return fmt.Sprintf("Bearer %s", tokenString)
}

func SetJwtHeader(h http.Header, tokenString string) {
	h.Set(jwtHeaderKey, JwtBearerHeaderVal(tokenString))
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

// Set creates token cookie with xsrf cookie and put it to ResponseWriter
// accepts claims and sets expiration if none defined. permanent flag means long-living cookie,
// false makes it session only.
func (s *service) Set(ctx context.Context, w http.ResponseWriter, claims *jwt2.AuthProxyClaims) (*jwt2.AuthProxyClaims, error) {
	expiresAt, err := claims.GetExpirationTime()
	if err != nil {
		return nil, errors.Wrap(err, "can't get expiration time")
	}

	if expiresAt == nil {
		claims.ExpiresAt = jwt.NewNumericDate(ctx.Clock().Now().Add(s.Config.GetRoot().SystemAuth.JwtTokenDuration()))
	}

	claims.Issuer = string(s.Opts.Service.GetId())
	claims.IssuedAt = jwt.NewNumericDate(ctx.Clock().Now())

	tokenString, err := s.Token(ctx, claims)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make token token")
	}

	if s.SendJWTHeader {
		s.setJwtResponseHeader(w, tokenString)
		return claims, nil
	}

	cookieExpiration := 0 // session cookie
	if !claims.SessionOnly {
		cookieExpiration = int(s.Config.GetRoot().SystemAuth.CookieDuration().Seconds())
	}

	jwtCookie := http.Cookie{
		Name:     jwtCookieName,
		Value:    tokenString,
		HttpOnly: true,
		Path:     "/",
		Domain:   s.Config.GetRoot().SystemAuth.CookieDomain,
		MaxAge:   cookieExpiration,
		Secure:   s.Opts.Service.IsHttps(),
		SameSite: cookieSameSite,
	}
	http.SetCookie(w, &jwtCookie)

	xsrfCookie := http.Cookie{
		Name:     xsrfCookieName,
		Value:    claims.ID,
		HttpOnly: false,
		Path:     "/",
		Domain:   s.Config.GetRoot().SystemAuth.CookieDomain,
		MaxAge:   cookieExpiration,
		Secure:   s.Opts.Service.IsHttps(),
		SameSite: cookieSameSite,
	}
	http.SetCookie(w, &xsrfCookie)

	return claims, nil
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
	if tokenHeader := r.Header.Get(jwtHeaderKey); tokenHeader != "" {
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

func getJwtTokenFromCookie(r *http.Request) (token string, hasValue bool, err error) {
	jc, err := r.Cookie(jwtCookieName)

	if errors.Is(err, http.ErrNoCookie) {
		return "", false, nil
	}

	if err != nil {
		return "", true, errors.Wrap(err, "failed to get cookie")
	}

	return jc.Value, true, nil
}

func requestHasAuth(r *http.Request) bool {
	_, hasQueryVal := getJwtTokenFromQuery(r)
	_, hasHeaderVal, _ := getJwtTokenFromHeader(r)
	_, hasCookieVal, _ := getJwtTokenFromCookie(r)

	return hasQueryVal || hasHeaderVal || hasCookieVal
}

// establishAuthFromRequest is the top-level method for managing auth. It looks for authentication in all the places
// supported by the configuration, makes sure any incoming JWTs are consistent with the auth information stored in the
// database, and returns the established internal actor. This method will error if no auth is present, or the
// information is somehow inconsistent with the auth state of the system.
func (s *service) establishAuthFromRequest(ctx context.Context, r *http.Request) (RequestAuth, error) {

	fromCookie := false
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

	// try to get from JWT cookie
	if tokenString == "" {
		if tokenCookie, hasValue, err := getJwtTokenFromCookie(r); hasValue || err != nil {
			if err != nil {
				return NewUnauthenticatedRequestAuth(), err
			}

			fromCookie = true
			tokenString = tokenCookie
		}
	}

	claims, err := s.Parse(ctx, tokenString)
	if err != nil {
		return NewUnauthenticatedRequestAuth(), errors.Wrap(err, "failed to get token")
	}

	if !s.Config.GetRoot().SystemAuth.DisableXSRF && fromCookie && claims.Actor != nil {
		xsrf := r.Header.Get(xsrfHeaderKey)
		if claims.ID != xsrf {
			return NewUnauthenticatedRequestAuth(), errors.New("xsrf mismatch")
		}
	}

	if claims.IsExpired(ctx) {
		return NewUnauthenticatedRequestAuth(), fmt.Errorf("claim is expired")
	}

	if claims.Nonce != nil {
		if claims.ExpiresAt == nil {
			return NewUnauthenticatedRequestAuth(), fmt.Errorf("cannot use nonce without expiration time")
		}

		s.logf("[DEBUG] using nonce: %s", claims.Nonce.String())

		wasValid, err := s.Db.CheckNonceValidAndMarkUsed(ctx, *claims.Nonce, claims.ExpiresAt.Time)
		if err != nil {
			return NewUnauthenticatedRequestAuth(), errors.Wrap(err, "can't check nonce")
		}

		if !wasValid {
			return NewUnauthenticatedRequestAuth(), fmt.Errorf("nonce already used")
		}
	}

	var actor *database.Actor
	if claims.Actor == nil {
		// This implies that the subject of the claim is the external id of the actor, and the actor must already
		// exist. If the actor does not exist, the claim is invalid.
		actor, err = s.Opts.Db.GetActorByExternalId(ctx, claims.Subject)
		if err != nil {
			return NewUnauthenticatedRequestAuth(), errors.Wrap(err, "failed to get actor")
		}
		if actor == nil {
			return NewUnauthenticatedRequestAuth(), errors.Errorf("actor '%s' not found", claims.Subject)
		}
	} else {
		// The actor was specified in the JWT. This means that we can upsert an actor, either creating it or making
		// it consistent with the request's definition.
		actor, err = s.Opts.Db.UpsertActor(ctx, claims.Actor)
		if err != nil {
			return NewUnauthenticatedRequestAuth(), errors.Wrap(err, "failed to upsert actor")
		}
	}

	return &requestAuth{
		actor: actor,
	}, nil
}

// Reset token's cookies
func (s *service) Reset(w http.ResponseWriter) {
	jwtCookie := http.Cookie{Name: jwtCookieName, Value: "", HttpOnly: false, Path: "/", Domain: s.Config.GetRoot().SystemAuth.CookieDomain,
		MaxAge: -1, Expires: time.Unix(0, 0), Secure: s.Opts.Service.IsHttps(), SameSite: cookieSameSite}
	http.SetCookie(w, &jwtCookie)

	xsrfCookie := http.Cookie{Name: xsrfCookieName, Value: "", HttpOnly: false, Path: "/", Domain: s.Config.GetRoot().SystemAuth.CookieDomain,
		MaxAge: -1, Expires: time.Unix(0, 0), Secure: s.Opts.Service.IsHttps(), SameSite: cookieSameSite}
	http.SetCookie(w, &xsrfCookie)
}
