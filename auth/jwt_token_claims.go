package auth

import (
	"encoding/json"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"strings"
)

// JwtTokenClaims is the struct that defines a JWT for the auth service. It contains information about the actor
// (user or system taking the action) as well as standard JWT information.
type JwtTokenClaims struct {
	jwt.RegisteredClaims
	Actor       *Actor `json:"actor,omitempty"`
	SessionOnly bool   `json:"sess_only,omitempty"`
}

func (tc *JwtTokenClaims) String() string {
	var tmp JwtTokenClaims
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
func (tc *JwtTokenClaims) AdminUsername() (string, error) {
	if !tc.IsAdmin() {
		return "", errors.New("not admin")
	}

	if tc.Subject != tc.Actor.ID {
		return "", errors.New("token subject and actor id do not match")
	}

	if !strings.HasPrefix(tc.Subject, "admin/") {
		return "", errors.New("admin username is not correctly formatted")
	}

	return strings.TrimPrefix(tc.Subject, "admin/"), nil
}

// IsAdmin checks if the actor represented by these claims is an admin
func (tc *JwtTokenClaims) IsAdmin() bool {
	if tc == nil {
		return false
	}

	return tc.Actor.IsAdmin()
}

// IsSuperAdmin checks if the actor represented by these claims is an admin
func (tc *JwtTokenClaims) IsSuperAdmin() bool {
	if tc == nil {
		return false
	}

	return tc.Actor.IsSuperAdmin()
}

// IsNormalActor checks if the actor represented by these claims is not an admin or superadmin
func (tc *JwtTokenClaims) IsNormalActor() bool {
	if tc == nil {
		// nil values default to normal actors to route to lower access paths
		return true
	}

	return tc.Actor.IsNormalActor()
}
