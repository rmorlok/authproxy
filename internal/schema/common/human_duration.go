package common

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/invopop/jsonschema"
)

type HumanDuration struct {
	time.Duration
}

const HumanDurationRegex = "^([0-9]+d)?([0-9]+h)?([0-9]+m)?([0-9]+s)?([0-9]+ms)?$"

// JSONSchema customizes the JSON Schema to represent HumanDuration as a string like "60m", "10s", etc.
func (HumanDuration) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Pattern: HumanDurationRegex}
}

// MarshalJSON provides custom serialization of the duration to a human-readable string (e.g., "2m").
func (d HumanDuration) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%s\"", d.String())), nil // `time.Duration.String()` converts duration to "2m", "4h", etc.
}

func parseHumanDuration(s string) (time.Duration, error) {
	overallRegex := regexp.MustCompile(HumanDurationRegex)
	onlyDayRegex := regexp.MustCompile(`^(\d+d)+$`)
	startsWithDayRegex := regexp.MustCompile(`^(\d+d)(.+)`)

	matched := overallRegex.MatchString(s)
	if !matched {
		return time.Duration(0), fmt.Errorf("human duration string does not match required pattern: %s", s)
	}

	// Manually extract the day piece off the string
	if onlyDayRegex.MatchString(s) {
		days, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
		if err != nil {
			return time.Duration(0), fmt.Errorf("failed to parse days: %w", err)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	if startsWithDayRegex.MatchString(s) {
		additionalFromDays := 0 * time.Hour
		parsedDuration := 0 * time.Second

		parts := startsWithDayRegex.FindStringSubmatch(s)
		if len(parts) == 3 {
			// Found a day piece
			daysStr := parts[1]
			s = parts[2]
			days, err := strconv.ParseInt(daysStr[:len(daysStr)-1], 10, 64)
			if err != nil {
				return time.Duration(0), fmt.Errorf("failed to parse days: %w", err)
			}
			additionalFromDays = time.Duration(days) * 24 * time.Hour

			parsedDuration, err = time.ParseDuration(s)
			if err != nil {
				return time.Duration(0), fmt.Errorf("failed to parse duration: %w", err)
			}

			return parsedDuration + additionalFromDays, nil
		}
	}

	// Just a normal duration
	parsedDuration, err := time.ParseDuration(s)
	if err != nil {
		return time.Duration(0), fmt.Errorf("failed to parse duration: %w", err)
	}
	return parsedDuration, nil
}

// UnmarshalJSON parses a human-readable duration string back into `time.Duration`.
func (d *HumanDuration) UnmarshalJSON(data []byte) error {
	// Remove the surrounding quotes from the JSON string
	s := string(data)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return fmt.Errorf("invalid duration format: %s", s)
	}
	parsedDuration, err := parseHumanDuration(s[1 : len(s)-1])
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
