package apid

import (
	"crypto/rand"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

const (
	// base62Chars is the alphabet used for generating random suffixes.
	base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	// suffixLen is the length of the random suffix after the prefix.
	suffixLen = 16
)

// Prefix represents an entity type prefix for IDs.
type Prefix string

const (
	PrefixActor            Prefix = "act_"
	PrefixConnection       Prefix = "cxn_"
	PrefixConnectorVersion Prefix = "cxr_"
	PrefixOAuth2Token      Prefix = "tok_"
	PrefixOauth2State      Prefix = "oas_"
	PrefixNonce            Prefix = "non_"
	PrefixRequestLog       Prefix = "req_"
	PrefixCorrelation      Prefix = "cor_"
	PrefixJwtId            Prefix = "jti_"

	PrefixSession Prefix = "sess_"
)

// validPrefixes is the set of all known prefixes for validation.
var validPrefixes = map[Prefix]bool{
	PrefixActor:            true,
	PrefixConnection:       true,
	PrefixConnectorVersion: true,
	PrefixOAuth2Token:      true,
	PrefixNonce:            true,
	PrefixRequestLog:       true,
	PrefixCorrelation:      true,
	PrefixJwtId:            true,
	PrefixOauth2State:      true,
	PrefixSession:          true,
}

// ID is a prefixed identifier string. The zero value is Nil (empty string).
type ID string

// Nil is the zero-value ID (empty string), analogous to uuid.Nil.
const Nil ID = ""

// New generates a new ID with the given prefix and a random base62 suffix.
func New(prefix Prefix) ID {
	suffix := make([]byte, suffixLen)
	max := big.NewInt(int64(len(base62Chars)))
	for i := range suffix {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic(fmt.Sprintf("apid: crypto/rand failed: %v", err))
		}
		suffix[i] = base62Chars[n.Int64()]
	}
	return ID(string(prefix) + string(suffix))
}

// Parse validates and returns an ID from a string. Returns an error if the
// string does not start with a known prefix.
func Parse(s string) (ID, error) {
	if s == "" {
		return Nil, nil
	}
	for p := range validPrefixes {
		if strings.HasPrefix(s, string(p)) {
			return ID(s), nil
		}
	}
	return Nil, fmt.Errorf("apid: unknown prefix in %q", s)
}

// MustParse is like Parse but panics on error.
func MustParse(s string) ID {
	id, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}

// String returns the string representation of the ID.
func (id ID) String() string {
	return string(id)
}

// IsNil returns true if the ID is the zero value.
func (id ID) IsNil() bool {
	return id == Nil
}

// Prefix returns the prefix portion of the ID, or empty string if nil.
func (id ID) Prefix() Prefix {
	s := string(id)
	idx := strings.Index(s, "_")
	if idx < 0 {
		return ""
	}
	return Prefix(s[:idx+1])
}

// HasPrefix returns true if the ID starts with the given prefix.
func (id ID) HasPrefix(p Prefix) bool {
	return strings.HasPrefix(string(id), string(p))
}

// ValidatePrefix returns an error if the ID is non-nil and does not have the expected prefix.
func (id ID) ValidatePrefix(expected Prefix) error {
	if id.IsNil() {
		return nil
	}
	if !id.HasPrefix(expected) {
		return fmt.Errorf("expected prefix %q but got %q", expected, id.Prefix())
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (id ID) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(id))
}

// UnmarshalJSON implements json.Unmarshaler.
func (id *ID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*id = ID(s)
	return nil
}

// Scan implements sql.Scanner for reading from the database.
func (id *ID) Scan(src interface{}) error {
	if src == nil {
		*id = Nil
		return nil
	}
	switch v := src.(type) {
	case string:
		*id = ID(v)
	case []byte:
		*id = ID(string(v))
	default:
		return fmt.Errorf("apid: cannot scan %T into ID", src)
	}
	return nil
}

// Value implements driver.Valuer for writing to the database.
func (id ID) Value() (driver.Value, error) {
	if id.IsNil() {
		return nil, nil
	}
	return string(id), nil
}

// MarshalYAML implements yaml.Marshaler.
func (id ID) MarshalYAML() (interface{}, error) {
	return string(id), nil
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (id *ID) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	*id = ID(s)
	return nil
}
