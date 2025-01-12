package config

import (
	"fmt"
	"time"
)

type HumanDuration struct {
	time.Duration
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
