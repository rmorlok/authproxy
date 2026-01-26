package service

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
	"github.com/rmorlok/authproxy/internal/apauth/core"
	jwt2 "github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
)

// AuthTestUtil provides utility functions and helpers for testing authentication-related functionality.
type AuthTestUtil struct {
	cfg       config.C
	s         *service
	serviceId sconfig.ServiceId
}

func TestAuthService(t *testing.T, serviceId sconfig.ServiceId, cfg config.C) (config.C, A, *AuthTestUtil) {
	testName := "unknown"
	if t != nil {
		testName = t.Name()
	}

	cfg, db := database.MustApplyBlankTestDbConfig(testName, cfg)
	return TestAuthServiceWithDb(serviceId, cfg, db)
}

func TestAuthServiceWithDb(serviceId sconfig.ServiceId, cfg config.C, db database.DB) (config.C, A, *AuthTestUtil) {
	if cfg == nil {
		cfg = config.FromRoot(&sconfig.Root{})
	}

	root := cfg.GetRoot()
	if root == nil {
		panic("No root in config")
	}

	if root.SystemAuth.GlobalAESKey == nil {
		root.SystemAuth.GlobalAESKey = sconfig.NewKeyDataRandomBytes()
	}
	if root.SystemAuth.JwtSigningKey == nil {
		root.SystemAuth.JwtSigningKey = &sconfig.Key{
			InnerVal: &sconfig.KeyShared{
				SharedKey: sconfig.NewKeyDataRandomBytes(),
			},
		}
	}

	cfg, r := apredis.MustApplyTestConfig(cfg)
	s := cfg.MustGetService(serviceId)
	e := encrypt.NewFakeEncryptService(false)

	hs := NewService(cfg, s.(sconfig.HttpService), db, r, e, cfg.GetRootLogger())

	return cfg, hs, &AuthTestUtil{cfg: cfg, s: hs.(*service), serviceId: serviceId}
}

func (atu *AuthTestUtil) NewSignedRequestForActorExternalId(method, url string, body io.Reader, namespace, actorExternalId string, permissions []aschema.Permission) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req, err = atu.SignRequestHeaderAs(
		context.Background(),
		req,
		core.Actor{
			ExternalId:  actorExternalId,
			Namespace:   namespace,
			Permissions: permissions,
		},
	)
	if err != nil {
		return nil, err
	}

	return req, nil
}

// NewSignedRequestForActor creates a signed request with a custom actor.
// This allows tests to specify exact permissions or admin/superadmin status.
func (atu *AuthTestUtil) NewSignedRequestForActor(method, url string, body io.Reader, actor core.Actor) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req, err = atu.SignRequestHeaderAs(context.Background(), req, actor)
	if err != nil {
		return nil, err
	}

	return req, nil
}

func (atu *AuthTestUtil) claimsForActor(ctx context.Context, a core.Actor) *jwt2.AuthProxyClaims {
	uuidGen := apctx.GetUuidGenerator(ctx)
	claims := &jwt2.AuthProxyClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "test",
			Subject:  a.ExternalId,
			Audience: []string{string(atu.serviceId)},
			ID:       uuidGen.NewString(),
		},
		Namespace: a.Namespace,
		Actor:     &a,
	}

	if claims.Subject == "" {
		return nil
	}

	return claims
}

func (atu *AuthTestUtil) SignRequestHeaderAs(ctx context.Context, req *http.Request, a core.Actor) (*http.Request, error) {
	claims := atu.claimsForActor(ctx, a)

	tokenString, err := atu.s.Token(ctx, claims)
	if err != nil {
		return req, errors.Wrap(err, "failed to generate jwt")
	}

	req.Header.Set(JwtHeaderKey, fmt.Sprintf("Bearer %s", tokenString))

	return req, nil
}

func (atu *AuthTestUtil) SignRequestQueryAs(ctx context.Context, req *http.Request, a core.Actor) (*http.Request, error) {
	claims := atu.claimsForActor(ctx, a)
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
