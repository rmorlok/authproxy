package jwt

import (
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/util"
	"strings"
	"time"
)

// ClaimsBuilder is an object to build Jwts to properly construct claims as expected
// with the actor/subject etc properly constructed.
type ClaimsBuilder interface {
	WithIssuer(issuer string) ClaimsBuilder
	WithAudience(audience string) ClaimsBuilder
	WithServiceId(serviceId config.ServiceId) ClaimsBuilder
	WithServiceIds(serviceIds []config.ServiceId) ClaimsBuilder
	WithExpiration(expiration time.Time) ClaimsBuilder
	WithExpiresIn(expiresIn time.Duration) ClaimsBuilder
	WithExpiresInCtx(ctx context.Context, expiresIn time.Duration) ClaimsBuilder
	WithSuperAdmin() ClaimsBuilder
	WithAdmin() ClaimsBuilder
	WithActorEmail(email string) ClaimsBuilder
	WithActorId(id string) ClaimsBuilder
	WithSessionOnly() ClaimsBuilder
	BuildCtx(context.Context) (*AuthProxyClaims, error)
	Build() (*AuthProxyClaims, error)
	MustBuild() AuthProxyClaims
	MustBuildCtx(context.Context) AuthProxyClaims
}

type claimsBuilder struct {
	issuer      *string
	audience    *string
	expiration  *time.Time
	superAdmin  *bool
	admin       *bool
	email       *string
	id          *string
	sessionOnly bool
}

func (b *claimsBuilder) WithIssuer(issuer string) ClaimsBuilder {
	b.issuer = &issuer
	return b
}

func (b *claimsBuilder) WithAudience(audience string) ClaimsBuilder {
	b.audience = &audience
	return b
}

func (b *claimsBuilder) WithServiceId(serviceId config.ServiceId) ClaimsBuilder {
	return b.WithAudience(string(serviceId))
}

func (b *claimsBuilder) WithServiceIds(serviceIds []config.ServiceId) ClaimsBuilder {
	return b.WithAudience(strings.Join(util.Map(serviceIds, func(serviceId config.ServiceId) string {
		return string(serviceId)
	}), ","))
}

func (b *claimsBuilder) WithExpiration(expiration time.Time) ClaimsBuilder {
	b.expiration = &expiration
	return b
}

func (b *claimsBuilder) WithExpiresIn(expiresIn time.Duration) ClaimsBuilder {
	return b.WithExpiresInCtx(context.Background(), expiresIn)
}

func (b *claimsBuilder) WithExpiresInCtx(ctx context.Context, expiresIn time.Duration) ClaimsBuilder {
	t := ctx.Clock().Now().Add(expiresIn)
	b.expiration = &t
	return b
}

func (b *claimsBuilder) WithSuperAdmin() ClaimsBuilder {
	b.admin = util.ToPtr(false)
	b.superAdmin = util.ToPtr(true)
	return b
}

func (b *claimsBuilder) WithAdmin() ClaimsBuilder {
	b.admin = util.ToPtr(true)
	b.superAdmin = util.ToPtr(false)
	return b
}

func (b *claimsBuilder) WithActorEmail(email string) ClaimsBuilder {
	b.email = &email
	return b
}

func (b *claimsBuilder) WithActorId(id string) ClaimsBuilder {
	b.id = &id
	return b
}

func (b *claimsBuilder) WithSessionOnly() ClaimsBuilder {
	b.sessionOnly = true
	return b
}

func (b *claimsBuilder) BuildCtx(ctx context.Context) (*AuthProxyClaims, error) {
	if util.CoerceBool(b.admin) && util.CoerceBool(b.superAdmin) {
		return nil, errors.New("cannot be both an admin and superadmin")
	}

	if util.CoerceBool(b.superAdmin) {
		b.id = util.ToPtr("superadmin/superadmin")
	}

	if b.id == nil {
		return nil, errors.New("id is required")
	}

	if util.CoerceBool(b.admin) {
		b.id = util.ToPtr(fmt.Sprintf("admin/%s", *b.id))
	}

	c := AuthProxyClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:  util.CoerceString(b.id),
			IssuedAt: &jwt.NumericDate{ctx.Clock().Now()},
			ID:       ctx.UuidGenerator().NewString(),
		},
		Actor: &Actor{
			ID:         util.CoerceString(b.id),
			Admin:      util.CoerceBool(b.admin),
			SuperAdmin: util.CoerceBool(b.superAdmin),
		},
		SessionOnly: b.sessionOnly,
	}

	if b.issuer != nil {
		c.Issuer = *b.issuer
	}

	if b.audience != nil {
		c.Audience = ClaimString(*b.audience)
		c.Actor.Audience = ClaimString(*b.audience)
	}

	if b.expiration != nil {
		c.ExpiresAt = &jwt.NumericDate{*b.expiration}
	}

	return &c, nil
}

func (b *claimsBuilder) Build() (*AuthProxyClaims, error) {
	return b.BuildCtx(context.Background())
}

func (b *claimsBuilder) MustBuildCtx(ctx context.Context) AuthProxyClaims {
	c, err := b.BuildCtx(ctx)
	if err != nil {
		panic(err)
	}

	return *c
}

func (b *claimsBuilder) MustBuild() AuthProxyClaims {
	return b.MustBuildCtx(context.Background())
}

func NewJwtBuilder() ClaimsBuilder {
	return &claimsBuilder{}
}
