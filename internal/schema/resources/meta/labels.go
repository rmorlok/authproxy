package meta

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/go-multierror"
)

const (
	LabelKeyNameMaxLength     = 63
	LabelKeyPrefixMaxLength   = 253
	LabelValueMaxLength       = 63
	SystemLabelValueMaxLength = 253
	AnnotationsTotalMaxSize   = 256 * 1024

	SystemLabelPrefix   = "apxy/"
	SystemLabelSentinel = "-"
)

var (
	labelKeyNamePattern      = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]{0,61}[a-zA-Z0-9])?$|^[a-zA-Z0-9]$`)
	labelKeyPrefixPattern    = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
	labelValuePattern        = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9._-]{0,61}[a-zA-Z0-9])?)?$|^[a-zA-Z0-9]?$`)
	systemPathSegmentPattern = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?|-)$`)
	systemLabelValuePattern  = regexp.MustCompile(`^[a-zA-Z0-9._-]{0,253}$`)
)

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

func ValidateUserLabelKey(key string) error {
	if strings.HasPrefix(key, SystemLabelPrefix) {
		return fmt.Errorf("label key %q is in the reserved %q namespace and cannot be set by users", key, SystemLabelPrefix)
	}
	return ValidateLabelKey(key)
}

func ValidateLabelValue(value string) error {
	if len(value) > LabelValueMaxLength {
		return fmt.Errorf("label value exceeds maximum length of %d characters", LabelValueMaxLength)
	}
	if value != "" && !labelValuePattern.MatchString(value) {
		return fmt.Errorf("label value %q must start and end with alphanumeric and contain only alphanumeric, '-', '_', or '.'", value)
	}
	return nil
}

func ValidateSystemLabelValue(value string) error {
	if len(value) > SystemLabelValueMaxLength {
		return fmt.Errorf("system label value exceeds maximum length of %d characters", SystemLabelValueMaxLength)
	}
	if value != "" && !systemLabelValuePattern.MatchString(value) {
		return fmt.Errorf("system label value %q may contain only alphanumeric characters, '-', '_', or '.'", value)
	}
	return nil
}

func ValidateLabels(labels map[string]string) error {
	var result *multierror.Error
	for _, key := range sortedMapKeys(labels) {
		value := labels[key]
		if err := ValidateLabelKey(key); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label key %q: %w", key, err))
		}
		valueErr := ValidateLabelValue(value)
		if strings.HasPrefix(key, SystemLabelPrefix) {
			valueErr = ValidateSystemLabelValue(value)
		}
		if valueErr != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label value for key %q: %w", key, valueErr))
		}
	}
	return result.ErrorOrNil()
}

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

func ValidateUserLabelDeletionKeys(keys []string) error {
	var result *multierror.Error
	for _, key := range keys {
		if err := ValidateUserLabelKey(key); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label key %q: %w", key, err))
		}
	}
	return result.ErrorOrNil()
}

func ValidateAnnotationKey(key string) error { return ValidateLabelKey(key) }
func ValidateAnnotationValue(_ string) error { return nil }

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
