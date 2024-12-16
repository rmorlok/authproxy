package auth

import (
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/logger"
	"io"
	"net/http"
)

// AuthTestUtil provides utility functions and helpers for testing authentication-related functionality.
type AuthTestUtil struct {
	cfg       config.C
	s         *Service
	serviceId config.ServiceId
}

func TestAuthService(serviceId config.ServiceId, cfg config.C) (config.C, *Service, *AuthTestUtil) {
	if cfg == nil {
		cfg = config.FromRoot(&config.Root{})
	}

	root := cfg.GetRoot()
	if root == nil {
		panic("No root in config")
	}

	if root.SystemAuth.CookieDomain == "" {
		root.SystemAuth.CookieDomain = "localhost:8080"
	}
	if root.SystemAuth.GlobalAESKey == nil {
		root.SystemAuth.GlobalAESKey = &config.KeyDataRandomBytes{}
	}
	if root.SystemAuth.JwtSigningKey == nil {
		root.SystemAuth.JwtSigningKey = &config.KeyShared{
			SharedKey: &config.KeyDataRandomBytes{},
		}
	}

	s := NewService(Opts{
		Config:    cfg,
		ServiceId: serviceId,
		Logger:    logger.Std,
	})

	return cfg, s, &AuthTestUtil{cfg: cfg, s: s, serviceId: serviceId}
}

func (atu *AuthTestUtil) NewSignedRequestForActorId(method, url string, body io.Reader, actorId string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req, err = atu.SignRequestAs(
		context.Background(),
		req,
		Actor{
			ID: actorId,
		},
	)
	if err != nil {
		return nil, err
	}

	return req, nil
}

func (atu *AuthTestUtil) SignRequestAs(ctx context.Context, req *http.Request, a Actor) (*http.Request, error) {
	if atu.s.UsesCookies {
		return atu.SignRequestCookieAs(ctx, req, a)
	} else {
		return atu.SignRequestHeaderAs(ctx, req, a)
	}
}

func (atu *AuthTestUtil) claimsForActor(a Actor) *JwtTokenClaims {
	claims := &JwtTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "test",
			Subject:  a.ID,
			Audience: []string{string(atu.serviceId)},
			ID:       uuid.UUID{}.String(),
		},
		Actor: &a,
	}

	if claims.Subject == "" {
		return nil
	}

	return claims
}

func (atu *AuthTestUtil) SignRequestHeaderAs(ctx context.Context, req *http.Request, a Actor) (*http.Request, error) {
	claims := atu.claimsForActor(a)

	tokenString, err := atu.s.Token(ctx, claims)
	if err != nil {
		return req, errors.Wrap(err, "failed to generate jwt")
	}

	req.Header.Set(jwtHeaderKey, fmt.Sprintf("Bearer %s", tokenString))

	return req, nil
}

func (atu *AuthTestUtil) SignRequestCookieAs(ctx context.Context, req *http.Request, a Actor) (*http.Request, error) {
	claims := atu.claimsForActor(a)

	tokenString, err := atu.s.Token(ctx, claims)
	if err != nil {
		return req, errors.Wrap(err, "failed to generate jwt")
	}

	jwtCookie := http.Cookie{
		Name:     jwtCookieName,
		Value:    tokenString,
		HttpOnly: true,
		Path:     "/",
		Domain:   atu.cfg.GetRoot().SystemAuth.CookieDomain,
		MaxAge:   0,
		Secure:   false,
		SameSite: cookieSameSite,
	}

	req.AddCookie(&jwtCookie)
	return req, nil
}
