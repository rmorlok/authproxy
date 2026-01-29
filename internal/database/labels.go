package database

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
)

// Kubernetes-style label restrictions
const (
	// LabelKeyNameMaxLength is the maximum length for the name portion of a label key
	LabelKeyNameMaxLength = 63

	// LabelKeyPrefixMaxLength is the maximum length for the optional prefix portion of a label key
	LabelKeyPrefixMaxLength = 253

	// LabelValueMaxLength is the maximum length for a label value
	LabelValueMaxLength = 63
)

var (
	// labelKeyNameRegex validates the name portion of a label key:
	// - 1-63 characters
	// - must start and end with alphanumeric
	// - may contain alphanumeric, '-', '_', '.'
	labelKeyNameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]{0,61}[a-zA-Z0-9])?$|^[a-zA-Z0-9]$`)

	// labelKeyPrefixRegex validates the prefix portion of a label key (DNS subdomain):
	// - max 253 characters
	// - one or more DNS labels separated by '.'
	// - each label: starts/ends with alphanumeric, may contain alphanumeric and '-'
	labelKeyPrefixRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

	// labelValueRegex validates a label value:
	// - 0-63 characters (can be empty)
	// - if non-empty: must start and end with alphanumeric, may contain alphanumeric, '-', '_', '.'
	labelValueRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9._-]{0,61}[a-zA-Z0-9])?)?$|^[a-zA-Z0-9]?$`)
)

// Labels is a map of key-value pairs following Kubernetes label restrictions.
// Keys follow the format [prefix/]name where:
// - prefix (optional): valid DNS subdomain, max 253 characters
// - name (required): 1-63 characters, alphanumeric start/end, may contain '-', '_', '.'
// Values: 0-63 characters, if non-empty must start/end with alphanumeric
type Labels map[string]string

// Value implements the driver.Valuer interface for Labels
func (l Labels) Value() (driver.Value, error) {
	if len(l) == 0 {
		return nil, nil
	}

	return json.Marshal(l)
}

// Scan implements the sql.Scanner interface for Labels
func (l *Labels) Scan(value interface{}) error {
	if value == nil {
		*l = nil
		return nil
	}

	switch v := value.(type) {
	case string:
		if v == "" {
			*l = nil
			return nil
		}
		return json.Unmarshal([]byte(v), l)
	case []byte:
		if len(v) == 0 {
			*l = nil
			return nil
		}
		return json.Unmarshal(v, l)
	default:
		return fmt.Errorf("cannot convert %T to Labels", value)
	}
}

// ValidateLabelKey validates a single label key according to Kubernetes restrictions.
// Format: [prefix/]name
// - prefix (optional): valid DNS subdomain, max 253 characters
// - name (required): 1-63 characters, must start/end with alphanumeric, may contain '-', '_', '.'
func ValidateLabelKey(key string) error {
	if key == "" {
		return errors.New("label key cannot be empty")
	}

	var prefix, name string
	if idx := strings.LastIndex(key, "/"); idx != -1 {
		prefix = key[:idx]
		name = key[idx+1:]

		// If there's a slash, the prefix must not be empty
		if prefix == "" {
			return errors.New("label key prefix cannot be empty when slash is present")
		}
	} else {
		name = key
	}

	// Validate prefix if present
	if prefix != "" {
		if len(prefix) > LabelKeyPrefixMaxLength {
			return errors.Errorf("label key prefix exceeds maximum length of %d characters", LabelKeyPrefixMaxLength)
		}
		if !labelKeyPrefixRegex.MatchString(prefix) {
			return errors.Errorf("label key prefix %q is not a valid DNS subdomain", prefix)
		}
	}

	// Validate name
	if name == "" {
		return errors.New("label key name cannot be empty")
	}
	if len(name) > LabelKeyNameMaxLength {
		return errors.Errorf("label key name exceeds maximum length of %d characters", LabelKeyNameMaxLength)
	}
	if !labelKeyNameRegex.MatchString(name) {
		return errors.Errorf("label key name %q must start and end with alphanumeric and contain only alphanumeric, '-', '_', or '.'", name)
	}

	return nil
}

// ValidateLabelValue validates a single label value according to Kubernetes restrictions.
// - 0-63 characters (can be empty)
// - if non-empty: must start and end with alphanumeric, may contain alphanumeric, '-', '_', '.'
func ValidateLabelValue(value string) error {
	if len(value) > LabelValueMaxLength {
		return errors.Errorf("label value exceeds maximum length of %d characters", LabelValueMaxLength)
	}

	if value != "" && !labelValueRegex.MatchString(value) {
		return errors.Errorf("label value %q must start and end with alphanumeric and contain only alphanumeric, '-', '_', or '.'", value)
	}

	return nil
}

// Validate validates all labels according to Kubernetes restrictions.
func (l Labels) Validate() error {
	if l == nil {
		return nil
	}

	result := &multierror.Error{}

	for key, value := range l {
		if err := ValidateLabelKey(key); err != nil {
			result = multierror.Append(result, errors.Wrapf(err, "invalid label key %q", key))
		}
		if err := ValidateLabelValue(value); err != nil {
			result = multierror.Append(result, errors.Wrapf(err, "invalid label value for key %q", key))
		}
	}

	return result.ErrorOrNil()
}

// Get returns the value for a label key, and whether the key exists.
func (l Labels) Get(key string) (string, bool) {
	if l == nil {
		return "", false
	}
	v, ok := l[key]
	return v, ok
}

// Has returns true if the label key exists.
func (l Labels) Has(key string) bool {
	if l == nil {
		return false
	}
	_, ok := l[key]
	return ok
}

// Copy returns a deep copy of the labels.
func (l Labels) Copy() Labels {
	if l == nil {
		return nil
	}
	copy := make(Labels, len(l))
	for k, v := range l {
		copy[k] = v
	}
	return copy
}
