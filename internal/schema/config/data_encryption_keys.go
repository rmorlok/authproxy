package config

import (
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

const defaultDataEncryptionKeyRotationInterval = 90 * 24 * time.Hour

// DataEncryptionKeys configures lifecycle management for generated DEKs used by
// KMS-backed namespace encryption keys.
type DataEncryptionKeys struct {
	// RotationInterval is how old the current DEK can be before a new current DEK
	// is generated. Defaults to 90 days.
	RotationInterval *HumanDuration `json:"rotation_interval,omitempty" yaml:"rotation_interval,omitempty"`

	// EnsureCurrent controls whether the DEK generator creates a current DEK
	// when a KMS-backed encryption key has none. Defaults to true.
	EnsureCurrent *bool `json:"ensure_current,omitempty" yaml:"ensure_current,omitempty"`
}

func (d *DataEncryptionKeys) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}

	if d == nil {
		return nil
	}

	if d.RotationInterval != nil && d.RotationInterval.Duration < 0 {
		result = multierror.Append(result, vc.PushField("rotation_interval").NewErrorf(
			"must be greater than or equal to 0, got %s",
			d.RotationInterval.Duration,
		))
	}

	return result.ErrorOrNil()
}

func (d *DataEncryptionKeys) GetRotationInterval() time.Duration {
	if d == nil || d.RotationInterval == nil {
		return defaultDataEncryptionKeyRotationInterval
	}

	return d.RotationInterval.Duration
}

func (d *DataEncryptionKeys) ShouldEnsureCurrent() bool {
	if d == nil || d.EnsureCurrent == nil {
		return true
	}

	return *d.EnsureCurrent
}

func (d *DataEncryptionKeys) ShouldRotate(now time.Time, currentCreatedAt time.Time) bool {
	interval := d.GetRotationInterval()
	if interval == 0 {
		return false
	}

	return !currentCreatedAt.Add(interval).After(now)
}
