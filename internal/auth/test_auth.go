package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	jwt2 "github.com/rmorlok/authproxy/internal/jwt"
	"github.com/rmorlok/authproxy/internal/util"
)

// AuthTestUtil provides utility functions and helpers for testing authentication-related functionality.
type AuthTestUtil struct {
	cfg       config.C
	s         *service
	serviceId config.ServiceId
}

func TestAuthService(t *testing.T, serviceId config.ServiceId, cfg config.C) (config.C, A, *AuthTestUtil) {
	testName := "unknown"
	if t != nil {
		testName = t.Name()
	}

	cfg, db := database.MustApplyBlankTestDbConfig(testName, cfg)
	return TestAuthServiceWithDb(serviceId, cfg, db)
}

func TestAuthServiceWithDb(serviceId config.ServiceId, cfg config.C, db database.DB) (config.C, A, *AuthTestUtil) {
	if cfg == nil {
		cfg = config.FromRoot(&config.Root{})
	}

	root := cfg.GetRoot()
	if root == nil {
		panic("No root in config")
	}

	if root.SystemAuth.GlobalAESKey == nil {
		root.SystemAuth.GlobalAESKey = config.NewKeyDataRandomBytes()
	}
	if root.SystemAuth.JwtSigningKey == nil {
		root.SystemAuth.JwtSigningKey = &config.KeyShared{
			SharedKey: config.NewKeyDataRandomBytes(),
		}
	}

	cfg, r := apredis.MustApplyTestConfig(cfg)
	s := cfg.MustGetService(serviceId)
	e := encrypt.NewFakeEncryptService(false)

	hs := NewService(cfg, s.(config.HttpService), db, r, e, cfg.GetRootLogger())

	return cfg, hs, &AuthTestUtil{cfg: cfg, s: hs.(*service), serviceId: serviceId}
}

func (atu *AuthTestUtil) NewSignedRequestForActorId(method, url string, body io.Reader, actorId string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req, err = atu.SignRequestHeaderAs(
		context.Background(),
		req,
		jwt2.Actor{
			ID: actorId,
		},
	)
	if err != nil {
		return nil, err
	}

	return req, nil
}

func (atu *AuthTestUtil) claimsForActor(a jwt2.Actor) *jwt2.AuthProxyClaims {
	claims := &jwt2.AuthProxyClaims{
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

func (atu *AuthTestUtil) SignRequestHeaderAs(ctx context.Context, req *http.Request, a jwt2.Actor) (*http.Request, error) {
	claims := atu.claimsForActor(a)

	tokenString, err := atu.s.Token(ctx, claims)
	if err != nil {
		return req, errors.Wrap(err, "failed to generate jwt")
	}

	req.Header.Set(JwtHeaderKey, fmt.Sprintf("Bearer %s", tokenString))

	return req, nil
}

func (atu *AuthTestUtil) SignRequestQueryAs(ctx context.Context, req *http.Request, a jwt2.Actor) (*http.Request, error) {
	claims := atu.claimsForActor(a)
	claims.Nonce = util.ToPtr(uuid.New())
	claims.ExpiresAt = &jwt.NumericDate{apctx.GetClock(ctx).Now().Add(time.Hour)}

	tokenString, err := atu.s.Token(ctx, claims)
	if err != nil {
		return req, errors.Wrap(err, "failed to generate jwt")
	}

	q := req.URL.Query()
	q.Set(JwtQueryParam, tokenString)
	req.URL.RawQuery = q.Encode()

	return req, nil
}
