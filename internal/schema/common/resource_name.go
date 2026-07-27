package common

import (
	"errors"
	"fmt"
	"regexp"
)

const (
	// ResourceNameMaxLength keeps resource names compatible with Kubernetes
	// label values so names can be projected into AuthProxy's implicit labels.
	ResourceNameMaxLength = 63

	// ResourceNamePattern accepts the same non-empty character grammar as a
	// Kubernetes label value. AuthProxy IDs contain an underscore separator,
	// so the stricter Kubernetes DNS-name grammar cannot be used here.
	ResourceNamePattern = `^[A-Za-z0-9]([A-Za-z0-9._-]{0,61}[A-Za-z0-9])?$`
)

var resourceNameRegex = regexp.MustCompile(ResourceNamePattern)

// ResourceName is a mutable, human-meaningful resource identifier.
//
// Values are preserved exactly and compared case-sensitively. AuthProxy does
// not trim, case-fold, or otherwise normalize resource names.
type ResourceName string

// Validate checks that the resource name is non-empty and can be used as an
// implicit label value.
func (n ResourceName) Validate() error {
	if n == "" {
		return errors.New("resource name is required")
	}
	if len(n) > ResourceNameMaxLength {
		return fmt.Errorf("resource name exceeds maximum length of %d characters", ResourceNameMaxLength)
	}
	if !resourceNameRegex.MatchString(string(n)) {
		return errors.New("resource name must start and end with an alphanumeric character and contain only alphanumeric characters, '-', '_', or '.'")
	}
	return nil
}
