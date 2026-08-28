package meta

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/go-multierror"
)

// Kubernetes-style label restrictions
const (
	// LabelKeyNameMaxLength is the maximum length for the name portion of a label key
	LabelKeyNameMaxLength = 63

	// LabelKeyPrefixMaxLength is the maximum length for the optional prefix portion of a label key
	LabelKeyPrefixMaxLength = 253

	// LabelValueMaxLength is the maximum length for a label value
	LabelValueMaxLength = 63

	// SystemLabelValueMaxLength is the maximum length for a label value stored
	// under an apxy/-prefixed key. System-managed labels such as
	// apxy/<rt>/-/ns can hold a namespace path that may exceed the standard
	// LabelValueMaxLength. User-supplied values are still capped at
	// LabelValueMaxLength via ValidateLabelValue.
	SystemLabelValueMaxLength = 253

	// AnnotationsTotalMaxSize is the maximum total size of all annotations (keys + values) in bytes.
	AnnotationsTotalMaxSize = 256 * 1024

	// SystemLabelPrefix is the reserved label-key prefix for system-managed
	// labels (implicit identifier labels and parent carry-forward labels).
	// User-supplied label keys may not begin with this prefix.
	SystemLabelPrefix = "apxy/"

	// SystemLabelSentinel is the segment used inside apxy/ keys to mark an
	// implicit identifier label, e.g. apxy/<rt>/-/id.
	SystemLabelSentinel = "-"
)

var (
	labelKeyNamePattern      = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]{0,61}[a-zA-Z0-9])?$|^[a-zA-Z0-9]$`)
	labelKeyPrefixPattern    = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
	labelValuePattern        = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9._-]{0,61}[a-zA-Z0-9])?)?$|^[a-zA-Z0-9]?$`)
	systemPathSegmentPattern = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?|-)$`)
	systemLabelValuePattern  = regexp.MustCompile(`^[a-zA-Z0-9._-]{0,253}$`)
)

// ValidateLabelKey validates a single label key.
//
// Two grammars are accepted:
//
//  1. Standard Kubernetes-style key: [prefix/]name
//     - prefix (optional): valid DNS subdomain, max 253 characters
//     - name (required): 1-63 characters, must start/end with alphanumeric,
//     may contain '-', '_', '.'
//
//  2. Reserved apxy/ multi-segment key: apxy/<seg>(/<seg>)*/<name>
//     - each <seg> is a DNS-label-like token or the literal "-" sentinel
//     - <name> follows the standard name rule above
//     - total prefix portion (everything before the final '/') still capped
//     at LabelKeyPrefixMaxLength characters
//
// This function accepts apxy/ keys; user-input call sites should use
// ValidateUserLabelKey to additionally reject the reserved namespace.
func ValidateLabelKey(key string) error {
	if key == "" {
		return errors.New("label key cannot be empty")
	}
	if strings.HasPrefix(key, SystemLabelPrefix) {
		return validateSystemLabelKey(key)
	}

	prefix, name, err := splitQualifiedKey(key)
	if err != nil {
		return err
	}
	if prefix != "" {
		if len(prefix) > LabelKeyPrefixMaxLength {
			return fmt.Errorf("label key prefix exceeds maximum length of %d characters", LabelKeyPrefixMaxLength)
		}
		if !labelKeyPrefixPattern.MatchString(prefix) {
			return fmt.Errorf("label key prefix %q is not a valid DNS subdomain", prefix)
		}
	}
	return validateLabelKeyName(name)
}

func splitQualifiedKey(key string) (string, string, error) {
	if idx := strings.LastIndex(key, "/"); idx >= 0 {
		if idx == 0 {
			return "", "", errors.New("label key prefix cannot be empty when slash is present")
		}
		return key[:idx], key[idx+1:], nil
	}
	return "", key, nil
}

func validateLabelKeyName(name string) error {
	if name == "" {
		return errors.New("label key name cannot be empty")
	}
	if len(name) > LabelKeyNameMaxLength {
		return fmt.Errorf("label key name exceeds maximum length of %d characters", LabelKeyNameMaxLength)
	}
	if !labelKeyNamePattern.MatchString(name) {
		return fmt.Errorf("label key name %q must start and end with alphanumeric and contain only alphanumeric, '-', '_', or '.'", name)
	}
	return nil
}

func validateSystemLabelKey(key string) error {
	prefix, name, err := splitQualifiedKey(key)
	if err != nil {
		return err
	}
	if len(prefix) > LabelKeyPrefixMaxLength {
		return fmt.Errorf("label key prefix exceeds maximum length of %d characters", LabelKeyPrefixMaxLength)
	}
	innerPath := strings.TrimPrefix(prefix, strings.TrimSuffix(SystemLabelPrefix, "/"))
	if innerPath == "" {
		return fmt.Errorf("%s label key requires at least one segment after %s", SystemLabelPrefix, SystemLabelPrefix)
	}
	for _, segment := range strings.Split(strings.TrimPrefix(innerPath, "/"), "/") {
		if segment == "" {
			return fmt.Errorf("%s label key has empty path segment", SystemLabelPrefix)
		}
		if !systemPathSegmentPattern.MatchString(segment) {
			return fmt.Errorf("%s label key segment %q must be a DNS label or the %q sentinel", SystemLabelPrefix, segment, SystemLabelSentinel)
		}
	}
	return validateLabelKeyName(name)
}

// ValidateUserLabelKey validates a label key supplied directly by an end user.
// In addition to the rules of ValidateLabelKey, it rejects any key in the
// reserved apxy/ namespace — those keys are managed by the system and may not
// be set, modified, or deleted through user-input endpoints.
func ValidateUserLabelKey(key string) error {
	if strings.HasPrefix(key, SystemLabelPrefix) {
		return fmt.Errorf("label key %q is in the reserved %q namespace and cannot be set by users", key, SystemLabelPrefix)
	}
	return ValidateLabelKey(key)
}

// ValidateLabelValue validates a single label value according to Kubernetes restrictions.
// - 0-63 characters (can be empty)
// - if non-empty: must start and end with alphanumeric, may contain alphanumeric, '-', '_', '.'
func ValidateLabelValue(value string) error {
	if len(value) > LabelValueMaxLength {
		return fmt.Errorf("label value exceeds maximum length of %d characters", LabelValueMaxLength)
	}
	if value != "" && !labelValuePattern.MatchString(value) {
		return fmt.Errorf("label value %q must start and end with alphanumeric and contain only alphanumeric, '-', '_', or '.'", value)
	}
	return nil
}

// ValidateSystemLabelValue validates a label value stored under an apxy/-prefixed
// key. It allows up to SystemLabelValueMaxLength characters so namespace paths
// (e.g. root.foo.bar.baz...) can fit, including the leading underscores and
// trailing hyphens accepted in namespace path segments.
func ValidateSystemLabelValue(value string) error {
	if len(value) > SystemLabelValueMaxLength {
		return fmt.Errorf("system label value exceeds maximum length of %d characters", SystemLabelValueMaxLength)
	}
	if value != "" && !systemLabelValuePattern.MatchString(value) {
		return fmt.Errorf("system label value %q may contain only alphanumeric characters, '-', '_', or '.'", value)
	}
	return nil
}

// ValidateLabelValueForKey validates a label value using the user or system limit implied by its key.
func ValidateLabelValueForKey(key, value string) error {
	if strings.HasPrefix(key, SystemLabelPrefix) {
		return ValidateSystemLabelValue(value)
	}
	return ValidateLabelValue(value)
}

// ValidateLabels validates all labels in a map. apxy/-prefixed keys are
// accepted (use ValidateUserLabels at user-input boundaries instead) and
// values stored under apxy/ keys are validated against the longer
// SystemLabelValueMaxLength cap.
func ValidateLabels(labels map[string]string) error {
	var result *multierror.Error
	for _, key := range sortedMapKeys(labels) {
		value := labels[key]
		if err := ValidateLabelKey(key); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label key %q: %w", key, err))
		}
		if err := ValidateLabelValueForKey(key, value); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label value for key %q: %w", key, err))
		}
	}
	return result.ErrorOrNil()
}

// ValidateUserLabels validates a labels map supplied by a user. It applies
// the same key/value rules as ValidateLabels but rejects any key in the
// reserved apxy/ namespace.
func ValidateUserLabels(labels map[string]string) error {
	var result *multierror.Error
	for _, key := range sortedMapKeys(labels) {
		value := labels[key]
		if err := ValidateUserLabelKey(key); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label key %q: %w", key, err))
		}
		if err := ValidateLabelValue(value); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label value for key %q: %w", key, err))
		}
	}
	return result.ErrorOrNil()
}

// ValidateUserLabelDeletionKeys validates a list of keys passed to a
// user-facing label-deletion endpoint. Keys must be well-formed and must not
// reference the reserved apxy/ namespace.
func ValidateUserLabelDeletionKeys(keys []string) error {
	var result *multierror.Error
	for _, key := range keys {
		if err := ValidateUserLabelKey(key); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label key %q: %w", key, err))
		}
	}
	return result.ErrorOrNil()
}

// ValidateAnnotationKey validates a single annotation key.
// Annotation keys follow the same format as label keys.
func ValidateAnnotationKey(key string) error { return ValidateLabelKey(key) }

// ValidateAnnotationValue validates a single annotation value.
// Annotation values have no format restriction — any string is allowed.
// Individual value size is not restricted; only the total annotations size is checked.
func ValidateAnnotationValue(_ string) error { return nil }

// ValidateAnnotations validates all annotations in a map.
func ValidateAnnotations(annotations map[string]string) error {
	var result *multierror.Error
	totalSize := 0
	for _, key := range sortedMapKeys(annotations) {
		value := annotations[key]
		if err := ValidateAnnotationKey(key); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid annotation key %q: %w", key, err))
		}
		totalSize += len(key) + len(value)
	}
	if totalSize > AnnotationsTotalMaxSize {
		result = multierror.Append(result, fmt.Errorf("total annotations size %d exceeds maximum of %d bytes", totalSize, AnnotationsTotalMaxSize))
	}
	return result.ErrorOrNil()
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
