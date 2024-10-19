package auth

import (
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/util"
	"time"
)

// JwtBuilder is an object to build Jwts to properly construct claims as expected
// with the actor/subject etc properly constructed.
type JwtBuilder interface {
	WithIssuer(issuer string) JwtBuilder
	WithAudience(audience string) JwtBuilder
	WithExpiration(expiration time.Time) JwtBuilder
	WithExpiresIn(expiresIn time.Duration) JwtBuilder
	WithExpiresInCtx(ctx context.Context, expiresIn time.Duration) JwtBuilder
	WithSuperAdmin() JwtBuilder
	WithAdmin() JwtBuilder
	WithActorEmail(email string) JwtBuilder
	WithActorId(id string) JwtBuilder
	WithSessionOnly() JwtBuilder
	BuildCtx(context.Context) (*JwtTokenClaims, error)
	Build() (*JwtTokenClaims, error)
	MustBuild() JwtTokenClaims
	MustBuildCtx(context.Context) JwtTokenClaims
}

type jwtBuilder struct {
	issuer      *string
	audience    *string
	expiration  *time.Time
	superAdmin  *bool
	admin       *bool
	email       *string
	id          *string
	sessionOnly bool
}

func (b *jwtBuilder) WithIssuer(issuer string) JwtBuilder {
	b.issuer = &issuer
	return b
}

func (b *jwtBuilder) WithAudience(audience string) JwtBuilder {
	b.audience = &audience
	return b
}

func (b *jwtBuilder) WithExpiration(expiration time.Time) JwtBuilder {
	b.expiration = &expiration
	return b
}

func (b *jwtBuilder) WithExpiresIn(expiresIn time.Duration) JwtBuilder {
	return b.WithExpiresInCtx(context.Background(), expiresIn)
}

func (b *jwtBuilder) WithExpiresInCtx(ctx context.Context, expiresIn time.Duration) JwtBuilder {
	t := ctx.Clock().Now().Add(expiresIn)
	b.expiration = &t
	return b
}

func (b *jwtBuilder) WithSuperAdmin() JwtBuilder {
	b.admin = util.ToPtr(false)
	b.superAdmin = util.ToPtr(true)
	return b
}

func (b *jwtBuilder) WithAdmin() JwtBuilder {
	b.admin = util.ToPtr(true)
	b.superAdmin = util.ToPtr(false)
	return b
}

func (b *jwtBuilder) WithActorEmail(email string) JwtBuilder {
	b.email = &email
	return b
}

func (b *jwtBuilder) WithActorId(id string) JwtBuilder {
	b.id = &id
	return b
}

func (b *jwtBuilder) WithSessionOnly() JwtBuilder {
	b.sessionOnly = true
	return b
}

func (b *jwtBuilder) BuildCtx(ctx context.Context) (*JwtTokenClaims, error) {
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

	c := JwtTokenClaims{
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

func (b *jwtBuilder) Build() (*JwtTokenClaims, error) {
	return b.BuildCtx(context.Background())
}

func (b *jwtBuilder) MustBuildCtx(ctx context.Context) JwtTokenClaims {
	c, err := b.BuildCtx(ctx)
	if err != nil {
		panic(err)
	}

	return *c
}

func (b *jwtBuilder) MustBuild() JwtTokenClaims {
	return b.MustBuildCtx(context.Background())
}

func NewJwtBuilder() JwtBuilder {
	return &jwtBuilder{}
}
