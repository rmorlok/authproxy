package jwt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apctx"
)

// AuthProxyClaims is the struct that defines a JWT for the auth service. It contains information about the actor
// (user or system taking the action) as well as standard JWT information.
type AuthProxyClaims struct {
	jwt.RegisteredClaims

	// Actor is the entity taking the action. Specifying the full actor here (versus just the ID in the subject)
	// implies that the actor should be upserted into the system as specified versus only working against a previous
	// actor configured in the system.
	Actor *core.Actor `json:"actor,omitempty"`

	// SelfSigned indicates this token is signed with the GlobalAESKey. This mean that that AuthProxy has signed
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

// AdminUsername retrieves the username of an admin actor. Admin actors must have their id and token subject formatted
// in the form admin/username. If token subject and actor id do not match, or they are not correctly formatted, this
// method will return an error.
func (tc *AuthProxyClaims) AdminUsername() (string, error) {
	if !tc.IsAdmin() {
		return "", errors.New("not admin")
	}

	if tc.Actor != nil && tc.Subject != tc.Actor.GetExternalId() {
		return "", errors.New("token subject and actor id do not match")
	}

	if !strings.HasPrefix(tc.Subject, "admin/") {
		return "", errors.New("admin username is not correctly formatted")
	}

	return strings.TrimPrefix(tc.Subject, "admin/"), nil
}

// IsAdmin checks if the actor represented by these claims is an admin
func (tc *AuthProxyClaims) IsAdmin() bool {
	if tc == nil {
		return false
	}

	return strings.HasPrefix(tc.Subject, "admin/") && (tc.Actor == nil || tc.Actor.IsAdmin())
}

// IsSuperAdmin checks if the actor represented by these claims is an admin
func (tc *AuthProxyClaims) IsSuperAdmin() bool {
	if tc == nil {
		return false
	}

	return tc.Actor.IsSuperAdmin()
}

// IsNormalActor checks if the actor represented by these claims is not an admin or superadmin
func (tc *AuthProxyClaims) IsNormalActor() bool {
	if tc == nil {
		// nil values default to normal actors to route to lower access paths
		return true
	}

	return tc.Actor.IsNormalActor()
}

// IsExpired returns true if claims expired
func (tc *AuthProxyClaims) IsExpired(ctx context.Context) bool {
	if tc == nil {
		return true
	}

	return tc.ExpiresAt != nil && tc.ExpiresAt.Before(apctx.GetClock(ctx).Now())
}
