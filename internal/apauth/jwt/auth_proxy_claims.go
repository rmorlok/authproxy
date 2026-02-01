package jwt

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apctx"
)

var ErrInvalidClaims = errors.New("invalid jwt claims")

// AuthProxyClaims is the struct that defines a JWT for the auth service. It contains information about the actor
// (user or system taking the action) as well as standard JWT information.
type AuthProxyClaims struct {
	jwt.RegisteredClaims

	// Namespace is the namespace of the actor. This is used to identify valid signing keys for the request, as well
	// where to lookup the actor in th database. THe value of subject must be unique within a given namespace. If
	// omitted, Namespace is assumed to be root. If Actor is provided, the value of namespace must be the same as
	// the value of the actor's namespace.
	Namespace string `json:"namespace,omitempty"`

	// Actor is the entity taking the action. Specifying the full actor here (versus just the ID in the subject)
	// implies that the actor should be upserted into the system as specified versus only working against a previous
	// actor configured in the system. If Actor is specified, the value of ExternalId must be the same as sub in the
	// base claims.
	Actor *core.Actor `json:"actor,omitempty"`

	// SelfSigned indicates this token is signed with the GlobalAESKey. This means that AuthProxy has signed
	// this token to itself for auth transfer between services, etc.
	SelfSigned bool `json:"self_signed,omitempty"`

	// Nonce is a one-time-use value. Adding a nonce to the JWT make it a one-time-use for auth purposes. If you use
	// a nonce, the JWT must also have an expiry so that tracking of the nonce values do not need to be kept forever.
	Nonce *uuid.UUID `json:"nonce,omitempty"`
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
		if tc.Subject != tc.Actor.GetExternalId() {
			result = multierror.Append(result, errors.New("token subject and actor id do not match"))
		}

		if tc.GetNamespace() != tc.Actor.GetNamespace() {
			result = multierror.Append(result, errors.New("token namespace and actor namespace do not match"))
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
