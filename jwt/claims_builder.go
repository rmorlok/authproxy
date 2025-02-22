package jwt

import (
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
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
	WithAudience(audience string) ClaimsBuilder    // Specifies the audience of the claims; normally a service id
	WithAudiences(audience []string) ClaimsBuilder // Specifies the service that is intended to consume the claims. Communicated as aud.
	WithServiceId(serviceId config.ServiceId) ClaimsBuilder
	WithServiceIds(serviceIds []config.ServiceId) ClaimsBuilder
	WithExpiration(expiration time.Time) ClaimsBuilder
	WithExpiresIn(expiresIn time.Duration) ClaimsBuilder
	WithExpiresInCtx(ctx context.Context, expiresIn time.Duration) ClaimsBuilder
	WithSuperAdmin() ClaimsBuilder
	WithAdmin() ClaimsBuilder
	WithSelfSigned() ClaimsBuilder
	WithActorEmail(email string) ClaimsBuilder
	WithActorId(id string) ClaimsBuilder
	WithActor(actor *Actor) ClaimsBuilder
	WithNonce() ClaimsBuilder
	BuildCtx(context.Context) (*AuthProxyClaims, error)
	Build() (*AuthProxyClaims, error)
	MustBuild() AuthProxyClaims
	MustBuildCtx(context.Context) AuthProxyClaims
}

type claimsBuilder struct {
	issuer     *string
	audiences  []string
	expiresIn  *time.Duration
	expiration *time.Time
	superAdmin *bool
	admin      *bool
	email      *string
	id         *string
	actor      *Actor
	selfSigned bool
	nonce      *uuid.UUID
}

func (b *claimsBuilder) WithIssuer(issuer string) ClaimsBuilder {
	b.issuer = &issuer
	return b
}

func (b *claimsBuilder) WithAudience(audience string) ClaimsBuilder {
	b.audiences = []string{audience}
	return b
}

func (b *claimsBuilder) WithAudiences(audiences []string) ClaimsBuilder {
	b.audiences = audiences
	return b
}

func (b *claimsBuilder) WithServiceId(serviceId config.ServiceId) ClaimsBuilder {
	return b.WithAudience(string(serviceId))
}

func (b *claimsBuilder) WithServiceIds(serviceIds []config.ServiceId) ClaimsBuilder {
	return b.WithAudiences(util.Map(serviceIds, func(s config.ServiceId) string { return string(s) }))
}

func (b *claimsBuilder) WithExpiration(expiration time.Time) ClaimsBuilder {
	b.expiration = &expiration
	return b
}

func (b *claimsBuilder) WithExpiresIn(expiresIn time.Duration) ClaimsBuilder {
	b.expiresIn = &expiresIn
	return b
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

func (b *claimsBuilder) WithSelfSigned() ClaimsBuilder {
	b.selfSigned = true
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

func (b *claimsBuilder) WithActor(actor *Actor) ClaimsBuilder {
	b.actor = actor
	return b
}

func (b *claimsBuilder) WithNonce() ClaimsBuilder {
	u := uuid.New()
	b.nonce = &u
	return b
}

func (b *claimsBuilder) BuildCtx(ctx context.Context) (*AuthProxyClaims, error) {
	if util.CoerceBool(b.admin) && util.CoerceBool(b.superAdmin) {
		return nil, errors.New("cannot be both an admin and superadmin")
	}

	if util.CoerceBool(b.superAdmin) {
		b.id = util.ToPtr("superadmin/superadmin")
	}

	if b.actor != nil {
		if b.actor.ID != "" {
			b.id = util.ToPtr(b.actor.ID)
		}
	}

	if b.id == nil {
		return nil, errors.New("id is required")
	}

	if util.CoerceBool(b.admin) && !strings.HasPrefix(*b.id, "admin/") {
		b.id = util.ToPtr(fmt.Sprintf("admin/%s", *b.id))
		if b.actor != nil {
			b.actor.ID = *b.id
		}
	}

	if b.actor != nil {
		if b.email != nil {
			b.actor.Email = *b.email
		}

		if b.admin != nil {
			b.actor.Admin = *b.admin
		}
	}

	c := AuthProxyClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:  util.CoerceString(b.id),
			IssuedAt: &jwt.NumericDate{ctx.Clock().Now()},
			ID:       ctx.UuidGenerator().NewString(),
		},
		Actor:      b.actor,
		SelfSigned: b.selfSigned,
	}

	if b.issuer != nil {
		c.Issuer = *b.issuer
	}

	if len(b.audiences) > 0 {
		c.Audience = b.audiences
	}

	if b.expiresIn != nil {
		b.expiration = util.ToPtr(ctx.Clock().Now().Add(*b.expiresIn))
	}

	if b.expiration != nil {
		c.ExpiresAt = &jwt.NumericDate{*b.expiration}
	}

	if b.nonce != nil {
		if b.expiration == nil {
			return nil, errors.New("nonce requires an expiration")
		}

		c.Nonce = b.nonce
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

func NewClaimsBuilder() ClaimsBuilder {
	return &claimsBuilder{}
}
