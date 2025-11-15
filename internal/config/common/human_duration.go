package common

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/invopop/jsonschema"
)

type HumanDuration struct {
	time.Duration
}

// JSONSchema customizes the JSON Schema to represent HumanDuration as a string like "60m", "10s", etc.
func (HumanDuration) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Pattern: "^[0-9]+(ns|us|µs|ms|s|m|h)$"}
}

// MarshalJSON provides custom serialization of the duration to a human-readable string (e.g., "2m").
func (d HumanDuration) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%s\"", d.String())), nil // `time.Duration.String()` converts duration to "2m", "4h", etc.
}

// UnmarshalJSON parses a human-readable duration string back into `time.Duration`.
func (d *HumanDuration) UnmarshalJSON(data []byte) error {
	// Remove the surrounding quotes from the JSON string
	s := string(data)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return fmt.Errorf("invalid duration format: %s", s)
	}
	parsedDuration, err := time.ParseDuration(s[1 : len(s)-1])
	if err != nil {
		return fmt.Errorf("failed to parse duration: %w", err)
	}
	d.Duration = parsedDuration
	return nil
}

// MarshalYAML provides custom serialization of the duration to a human-readable string (e.g., "2m").
func (d HumanDuration) MarshalYAML() (interface{}, error) {
	return d.String(), nil // `time.Duration.String()` converts duration to "2m", "4h", etc.
}

// UnmarshalYAML parses a human-readable duration string back into `time.Duration`.
func (d *HumanDuration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	parsedDuration, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("failed to parse duration: %w", err)
	}
	d.Duration = parsedDuration
	return nil
}

// HumanDurationFor returns a HumanDuration for the given string. Used for testing. Will panic if the string is invalid.
func HumanDurationFor(h string) *HumanDuration {
	var d HumanDuration
	err := json.Unmarshal([]byte(fmt.Sprintf("\"%s\"", h)), &d)
	if err != nil {
		panic(err)
	}
	return &d
}
