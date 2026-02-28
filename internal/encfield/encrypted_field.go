package encfield

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rmorlok/authproxy/internal/apid"
)

// EncryptedField represents an encrypted value stored as JSON {"id":"<ekv_id>","d":"<base64>"}.
// It implements driver.Valuer and sql.Scanner for seamless database integration.
type EncryptedField struct {
	ID   apid.ID `json:"id"`
	Data string  `json:"d"`
}

// IsZero returns true if the field has no data.
func (ef EncryptedField) IsZero() bool {
	return ef.ID == apid.Nil && ef.Data == ""
}

// IsEncryptedWithKey returns true if this field was encrypted with the given key ID.
func (ef EncryptedField) IsEncryptedWithKey(keyID apid.ID) bool {
	return ef.ID == keyID
}

// ToInlineString returns the field in the "<ekv_id>:<base64>" format.
func (ef EncryptedField) ToInlineString() string {
	return fmt.Sprintf("%s:%s", ef.ID, ef.Data)
}

func (ef *EncryptedField) Equal(other *EncryptedField) bool {
	if ef == nil {
		return other == nil
	}

	if other == nil {
		return false
	}

	return ef.ID == other.ID && ef.Data == other.Data
}

// Value implements driver.Valuer, marshaling to a JSON string for database storage.
// Returns nil for zero-value fields.
func (ef EncryptedField) Value() (driver.Value, error) {
	if ef.IsZero() {
		return nil, nil
	}
	b, err := json.Marshal(ef)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal EncryptedField: %w", err)
	}
	return string(b), nil
}

// Scan implements sql.Scanner, reading a JSON string from the database.
func (ef *EncryptedField) Scan(value interface{}) error {
	if value == nil {
		*ef = EncryptedField{}
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case string:
		if v == "" {
			*ef = EncryptedField{}
			return nil
		}
		data = []byte(v)
	case []byte:
		if len(v) == 0 {
			*ef = EncryptedField{}
			return nil
		}
		data = v
	default:
		return fmt.Errorf("cannot scan %T into EncryptedField", value)
	}

	return json.Unmarshal(data, ef)
}

// ParseInlineString parses the "<ekv_id>:<base64>" format into an EncryptedField.
// This is for use in non-DB contexts (e.g. cursor encryption) and is NOT used during Scan.
func ParseInlineString(s string) (EncryptedField, error) {
	if !strings.HasPrefix(s, string(apid.PrefixEncryptionKeyVersion)) {
		return EncryptedField{}, fmt.Errorf("invalid legacy format: missing ekv_ prefix")
	}

	colonIdx := strings.Index(s, ":")
	if colonIdx < 0 {
		return EncryptedField{}, fmt.Errorf("invalid legacy format: missing colon separator")
	}

	return EncryptedField{
		ID:   apid.ID(s[:colonIdx]),
		Data: s[colonIdx+1:],
	}, nil
}
