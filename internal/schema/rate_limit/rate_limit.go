// Package rate_limit defines the schema and validation rules for the
// RateLimit resource. Types here describe the JSON-serialized "definition"
// portion of a RateLimit; the database envelope (id, namespace, labels,
// annotations, timestamps) lives in internal/database.
package rate_limit

import (
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// Mode controls whether a matching rule rejects requests or only observes them.
type Mode string

const (
	ModeEnforce Mode = "enforce"
	ModeObserve Mode = "observe"
)

// DefaultMode is used when a RateLimit's Mode is unset.
const DefaultMode = ModeEnforce

// IsValidMode reports whether m is a recognised mode value.
func IsValidMode(m Mode) bool {
	switch m {
	case ModeEnforce, ModeObserve:
		return true
	default:
		return false
	}
}

// RateLimit is the configuration payload for a RateLimit resource — the
// JSON-serialised "definition" stored in the database.
type RateLimit struct {
	Mode      Mode      `json:"mode,omitempty" yaml:"mode,omitempty"`
	Selector  Selector  `json:"selector" yaml:"selector"`
	Bucket    Bucket    `json:"bucket" yaml:"bucket"`
	Algorithm Algorithm `json:"algorithm" yaml:"algorithm"`
}

// EffectiveMode returns Mode, falling back to DefaultMode when unset.
func (r *RateLimit) EffectiveMode() Mode {
	if r == nil || r.Mode == "" {
		return DefaultMode
	}
	return r.Mode
}

// Validate runs the full set of validation rules against r.
func (r *RateLimit) Validate() error {
	vc := &common.ValidationContext{}
	return r.validate(vc)
}

func (r *RateLimit) validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}

	if r.Mode != "" && !IsValidMode(r.Mode) {
		result = multierror.Append(result, vc.NewErrorfForField("mode", "invalid mode %q", string(r.Mode)))
	}

	if err := r.Selector.Validate(vc.PushField("selector")); err != nil {
		result = multierror.Append(result, err)
	}

	if err := r.Bucket.Validate(vc.PushField("bucket")); err != nil {
		result = multierror.Append(result, err)
	}

	if err := r.Algorithm.Validate(vc.PushField("algorithm")); err != nil {
		result = multierror.Append(result, err)
	}

	return result.ErrorOrNil()
}
