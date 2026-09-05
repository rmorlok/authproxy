package jwt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

var ErrInvalidClaims = errors.New("invalid jwt claims")

// AuthProxyClaims is the struct that defines a JWT for the auth service. It contains information about the actor
// (user or system taking the action) as well as standard JWT information.
type AuthProxyClaims struct {
	jwt.RegisteredClaims

	// Namespace is the namespace of the actor. This is used to identify valid signing keys for the request, as well
	// as where to look up the actor in the database. The value of subject must be unique within a given namespace. If
	// omitted, Namespace is assumed to be root. If Actor is provided, the value of namespace must be the same as
	// the value of the actor's namespace.
	Namespace string `json:"namespace,omitempty"`

	// Actor is the entity taking the action, represented as a restricted
	// authproxy.net/v1alpha1 Actor resource. Specifying it implies that the
	// actor should be upserted into the system. Database identity, lifecycle
	// status, timestamps, and signing material are forbidden in this claim.
	Actor *ActorClaim `json:"actor,omitempty"`

	// Permissions optionally restrict what this specific token can do. Every
	// permission must be contained by the authenticated actor's permissions, and
	// the two sets are intersected during authorization.
	Permissions []aschema.Permission `json:"permissions,omitempty"`

	// SystemSigned indicates this token was signed by an AuthProxy service using the GlobalAESKey (HMAC).
	// This is used for internal auth transfer between services, OAuth redirects, etc.
	SystemSigned bool `json:"systemSigned,omitempty"`

	// ActorSigned indicates this token was signed by an actor using their own private key (asymmetric).
	// This is used by the CLI and other external callers that have actor credentials.
	ActorSigned bool `json:"actorSigned,omitempty"`

	// Nonce is a one-time-use value. Adding a nonce to the JWT make it a one-time-use for auth purposes. If you use
	// a nonce, the JWT must also have an expiry so that tracking of the nonce values do not need to be kept forever.
	Nonce *apid.ID `json:"nonce,omitempty"`
}

func (tc *AuthProxyClaims) String() string {
	var tmp AuthProxyClaims
	if tc != nil {
		tmp = *tc
	}

	b, err := json.Marshal(tmp)
	if err != nil {
		return fmt.Sprintf("%+v %+v", tmp.RegisteredClaims, tmp.Actor)
	}
	return string(b)
}

func (tc *AuthProxyClaims) GetNamespace() string {
	if tc.Namespace == "" {
		return "root"
	}

	return tc.Namespace
}

func (tc *AuthProxyClaims) Validate(v *jwt.Validator) error {
	result := &multierror.Error{}

	if err := v.Validate(*tc); err != nil {
		result = multierror.Append(result, err)
	}

	if tc.Actor != nil {
		if err := tc.Actor.Validate(); err != nil {
			result = multierror.Append(result, err)
		}

		if tc.Subject != tc.Actor.Spec.ExternalId {
			result = multierror.Append(result, errors.New("token subject and actor spec.externalId do not match"))
		}

		if tc.GetNamespace() != tc.Actor.Metadata.Namespace {
			result = multierror.Append(result, errors.New("token namespace and actor metadata.namespace do not match"))
		}
	}

	for _, permission := range tc.Permissions {
		if err := permission.Validate(); err != nil {
			result = multierror.Append(result, err)
		}
	}

	if tc.Actor != nil {
		if err := core.ValidatePermissionRestrictions(
			tc.Actor.ToCoreActor(),
			tc.Actor.Spec.Permissions,
			tc.Permissions,
		); err != nil {
			result = multierror.Append(result, err)
		}
	}

	if result.ErrorOrNil() != nil {
		result = multierror.Append(result, ErrInvalidClaims)
	}

	return result.ErrorOrNil()
}

// IsExpired returns true if claims expired
func (tc *AuthProxyClaims) IsExpired(ctx context.Context) bool {
	if tc == nil {
		return true
	}

	return tc.ExpiresAt != nil && tc.ExpiresAt.Before(apctx.GetClock(ctx).Now())
}
