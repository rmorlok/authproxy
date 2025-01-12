package jwt

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	context2 "github.com/rmorlok/authproxy/context"
	"hash"
	"hash/crc64"
	"io"
	"regexp"
)

var reValidSha = regexp.MustCompile("^[a-fA-F0-9]{40}$")
var reValidCrc64 = regexp.MustCompile("^[a-fA-F0-9]{16}$")

const (
	adminAttr      = "admin"       // predefined attribute key for bool isAdmin status
	superAdminAttr = "super_admin" // predefined attribute key for bool isAdmin status
)

type contextKey string

const (
	actorContextKey contextKey = "actor"
)

// Actor is the information that identifies who is making a request. This can be a actor in the calling
// system, an admin from the calling system, a devops admin from the cli, etc.
type Actor struct {
	// set by service
	ID         string           `json:"id"`
	Audience   jwt.ClaimStrings `json:"aud,omitempty"`
	Admin      bool             `json:"admin,omitempty"`
	SuperAdmin bool             `json:"super_admin,omitempty"`
	// set by client
	IP         string                 `json:"ip,omitempty"`
	Email      string                 `json:"email,omitempty"`
	Attributes map[string]interface{} `json:"attrs,omitempty"`
	Role       string                 `json:"role,omitempty"`
}

// SetBoolAttr sets boolean attribute
func (a *Actor) SetBoolAttr(key string, val bool) {
	if a.Attributes == nil {
		a.Attributes = map[string]interface{}{}
	}
	a.Attributes[key] = val
}

// SetStrAttr sets string attribute
func (a *Actor) SetStrAttr(key, val string) {
	if a.Attributes == nil {
		a.Attributes = map[string]interface{}{}
	}
	a.Attributes[key] = val
}

// BoolAttr gets boolean attribute
func (a *Actor) BoolAttr(key string) bool {
	r, ok := a.Attributes[key].(bool)
	if !ok {
		return false
	}
	return r
}

// StrAttr gets string attribute
func (a *Actor) StrAttr(key string) string {
	r, ok := a.Attributes[key].(string)
	if !ok {
		return ""
	}
	return r
}

// IsAdmin is a helper to wrap the Admin attribute
func (a *Actor) IsAdmin() bool {
	if a == nil {
		return false
	}

	return a.Admin
}

// IsSuperAdmin is a helper to wrap the SuperAdmin attribute
func (a *Actor) IsSuperAdmin() bool {
	if a == nil {
		return false
	}

	return a.SuperAdmin
}

// IsNormalActor indicates that an actor is not an admin or superadmin
func (a *Actor) IsNormalActor() bool {
	if a == nil {
		// actors default to normal
		return true
	}

	return !a.IsSuperAdmin() && !a.IsAdmin()
}

// SliceAttr gets slice attribute
func (a *Actor) SliceAttr(key string) []string {
	r, ok := a.Attributes[key].([]string)
	if !ok {
		return []string{}
	}
	return r
}

// SetSliceAttr sets slice attribute for given key
func (a *Actor) SetSliceAttr(key string, val []string) {
	if a.Attributes == nil {
		a.Attributes = map[string]interface{}{}
	}
	a.Attributes[key] = val
}

// SetRole sets actor role for RBAC
func (a *Actor) SetRole(role string) {
	a.Role = role
}

// GetRole gets actor role
func (a *Actor) GetRole() string {
	return a.Role
}

// ContextWith sets actor in the context
func (a *Actor) ContextWith(ctx context.Context) context.Context {
	return context.WithValue(ctx, actorContextKey, a)
}

// MustGetActorFromContext always returns an actor, or panics if an actor is not present on the context.
func MustGetActorFromContext(ctx context2.Context) Actor {
	a := GetActorFromContext(ctx)
	if a == nil {
		panic("actor not present on context")
	}

	return *a
}

// SetActorInContext sets the actor on the context. This is just an alias for the context.With method.
func SetActorInContext(ctx context2.Context, actor *Actor) context2.Context {
	return ctx.With(actor)
}

// GetActorFromContext gets an actor from the context, or returns nil if one is not present
func GetActorFromContext(ctx context2.Context) *Actor {
	if a, ok := ctx.Value(actorContextKey).(*Actor); ok {
		return a
	}

	return nil
}

// HashID tries to hash val with hash.Hash and fallback to crc if needed
func HashID(h hash.Hash, val string) string {

	if reValidSha.MatchString(val) {
		return val // already hashed or empty
	}

	if _, err := io.WriteString(h, val); err != nil {
		// fail back to crc64
		if val == "" {
			val = "!empty string!"
		}
		if reValidCrc64.MatchString(val) {
			return val // already crced
		}
		return fmt.Sprintf("%x", crc64.Checksum([]byte(val), crc64.MakeTable(crc64.ECMA)))
	}
	return hex.EncodeToString(h.Sum(nil))
}
