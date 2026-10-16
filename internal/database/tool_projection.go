package database

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// validateToolDefinitionProjection checks the plaintext fields needed for search
// and permission candidate filtering. It follows ToolDefinition's literal verb
// rules; it does not authorize a request or compare against encrypted source.
func validateToolDefinitionProjection(description string, verbs []string) error {
	if strings.TrimSpace(description) == "" {
		return errors.New("tool description is required")
	}
	if len(verbs) == 0 {
		return errors.New("tool verbs must contain at least one permission verb")
	}

	seen := make(map[string]bool, len(verbs))
	for _, verb := range verbs {
		if strings.TrimSpace(verb) == "" || strings.TrimSpace(verb) != verb {
			return errors.New("tool verbs must be nonempty without surrounding whitespace")
		}
		if seen[verb] {
			return fmt.Errorf("duplicate tool verb %q", verb)
		}
		seen[verb] = true
	}
	return nil
}

// toolVerbsDB adds SQL encoding to Tool.Verbs without changing its slice type.
// Values are exact permission aliases; JSON escaping preserves punctuation and
// case. NULL represents a legacy Tool whose projection has not been prepared,
// never a wildcard grant. New definition writes require a nonempty list.
type toolVerbsDB []string

// Value stores a verb list as JSON text for SQLite or PostgreSQL's jsonb column.
func (v toolVerbsDB) Value() (driver.Value, error) {
	if len(v) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(v)
	return string(data), err
}

// Scan decodes either provider's JSON representation into a detached slice.
// A failed decode leaves the receiver unchanged; NULL clears a prior value.
func (v *toolVerbsDB) Scan(src any) error {
	if src == nil {
		*v = nil
		return nil
	}
	var data []byte
	switch value := src.(type) {
	case string:
		data = []byte(value)
	case []byte:
		data = value
	default:
		return fmt.Errorf("toolVerbsDB: cannot scan %T", src)
	}
	var verbs []string
	if err := json.Unmarshal(data, &verbs); err != nil {
		return err
	}
	*v = verbs
	return nil
}
