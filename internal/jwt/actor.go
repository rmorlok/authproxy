package jwt

import (
	"encoding/hex"
	"fmt"
	"hash"
	"hash/crc64"
	"io"
	"regexp"
)

var reValidSha = regexp.MustCompile("^[a-fA-F0-9]{40}$")
var reValidCrc64 = regexp.MustCompile("^[a-fA-F0-9]{16}$")

// Actor is the information that identifies who is making a request. This can be a actor in the calling
// system, an admin from the calling system, a devops admin from the cli, etc.
type Actor struct {
	ID          string   `json:"id"`
	Permissions []string `json:"permissions"`
	Admin       bool     `json:"admin,omitempty"`
	SuperAdmin  bool     `json:"super_admin,omitempty"`
	Email       string   `json:"email,omitempty"`
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
