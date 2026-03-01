package jwt

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
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
	WithSystemSigned() ClaimsBuilder
	WithActorSigned() ClaimsBuilder
	WithActorExternalId(id string) ClaimsBuilder
	WithNamespace(namespace string) ClaimsBuilder
	WithActor(actor core.IActorData) ClaimsBuilder
	WithLabels(labels map[string]string) ClaimsBuilder
	WithLabel(key, value string) ClaimsBuilder
	WithNonce() ClaimsBuilder
	BuildCtx(context.Context) (*AuthProxyClaims, error)
	Build() (*AuthProxyClaims, error)
	MustBuild() AuthProxyClaims
	MustBuildCtx(context.Context) AuthProxyClaims
}

type claimsBuilder struct {
	issuer       *string
	audiences    []string
	expiresIn    *time.Duration
	expiration   *time.Time
	externalId   *string
	namespace    *string
	actor        *core.Actor
	labels       map[string]string
	systemSigned bool
	actorSigned  bool
	nonce        *apid.ID
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
	t := apctx.GetClock(ctx).Now().Add(expiresIn)
	b.expiration = &t
	return b
}

func (b *claimsBuilder) WithSystemSigned() ClaimsBuilder {
	b.systemSigned = true
	return b
}

func (b *claimsBuilder) WithActorSigned() ClaimsBuilder {
	b.actorSigned = true
	return b
}

func (b *claimsBuilder) WithActorExternalId(id string) ClaimsBuilder {
	b.externalId = &id
	return b
}

func (b *claimsBuilder) WithNamespace(namespace string) ClaimsBuilder {
	b.namespace = &namespace
	return b
}

func (b *claimsBuilder) WithActor(actor core.IActorData) ClaimsBuilder {
	b.actor = core.CreateActor(actor)
	return b
}

func (b *claimsBuilder) WithLabels(labels map[string]string) ClaimsBuilder {
	b.labels = labels
	return b
}

func (b *claimsBuilder) WithLabel(key, value string) ClaimsBuilder {
	if b.labels == nil {
		b.labels = make(map[string]string)
	}
	b.labels[key] = value
	return b
}

func (b *claimsBuilder) WithNonce() ClaimsBuilder {
	id := apid.New(apid.PrefixNonce)
	b.nonce = &id
	return b
}

func (b *claimsBuilder) BuildCtx(ctx context.Context) (*AuthProxyClaims, error) {
	if b.actor != nil {
		if b.actor.GetExternalId() != "" {
			b.externalId = util.ToPtr(b.actor.GetExternalId())
		}

		if b.actor.GetNamespace() != "" {
			b.namespace = util.ToPtr(b.actor.GetNamespace())
		}

		if b.namespace == nil {
			return nil, errors.New("namespace is required if specifying an actor")
		}

		if b.namespace != nil {
			b.actor.Namespace = *b.namespace
		}

		if b.externalId != nil {
			b.actor.ExternalId = *b.externalId
		}

		if len(b.labels) > 0 {
			if b.actor.Labels == nil {
				b.actor.Labels = make(map[string]string)
			}
			for k, v := range b.labels {
				b.actor.Labels[k] = v
			}
		}
	}

	if b.actor == nil && len(b.labels) > 0 {
		b.actor = &core.Actor{
			Labels: b.labels,
		}
	}

	if b.externalId == nil {
		return nil, errors.New("external_id is required")
	}

	c := AuthProxyClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:  util.CoerceString(b.externalId),
			IssuedAt: &jwt.NumericDate{apctx.GetClock(ctx).Now()},
			ID:       apctx.GetIdGenerator(ctx).NewString(apid.PrefixJwtId),
		},
		Actor:        b.actor,
		SystemSigned: b.systemSigned,
		ActorSigned:  b.actorSigned,
	}

	if b.namespace != nil {
		c.Namespace = *b.namespace
	}

	if b.issuer != nil {
		c.Issuer = *b.issuer
	}

	if len(b.audiences) > 0 {
		c.Audience = b.audiences
	}

	if b.expiresIn != nil {
		b.expiration = util.ToPtr(apctx.GetClock(ctx).Now().Add(*b.expiresIn))
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
