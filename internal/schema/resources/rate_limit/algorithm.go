package rate_limit

import (
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// SlidingWindowMode selects between exact (log) and approximate (counter)
// sliding-window evaluation.
type SlidingWindowMode string

const (
	SlidingWindowModeLog     SlidingWindowMode = "log"
	SlidingWindowModeCounter SlidingWindowMode = "counter"
)

// IsValidSlidingWindowMode reports whether m is a recognised sliding-window mode.
func IsValidSlidingWindowMode(m SlidingWindowMode) bool {
	switch m {
	case SlidingWindowModeLog, SlidingWindowModeCounter:
		return true
	default:
		return false
	}
}

// FixedWindow rejects once Limit requests have been counted within Window.
// The window resets at fixed boundaries derived from request time / Window.
type FixedWindow struct {
	Window common.HumanDuration `json:"window" yaml:"window"`
	Limit  int                  `json:"limit" yaml:"limit"`
}

// Validate ensures Window is positive and Limit is positive.
func (f *FixedWindow) Validate(vc *common.ValidationContext) error {
	if f == nil {
		return nil
	}
	result := &multierror.Error{}

	if f.Window.Duration <= 0 {
		result = multierror.Append(result, vc.NewErrorForField("window", "must be positive"))
	}
	if f.Limit <= 0 {
		result = multierror.Append(result, vc.NewErrorForField("limit", "must be positive"))
	}
	return result.ErrorOrNil()
}

// SlidingWindow rejects once Limit requests have been counted in the trailing
// Window relative to request time.
type SlidingWindow struct {
	Window common.HumanDuration `json:"window" yaml:"window"`
	Limit  int                  `json:"limit" yaml:"limit"`
	Mode   SlidingWindowMode    `json:"mode" yaml:"mode"`
}

// Validate ensures Window/Limit are positive and Mode is recognised.
func (s *SlidingWindow) Validate(vc *common.ValidationContext) error {
	if s == nil {
		return nil
	}
	result := &multierror.Error{}

	if s.Window.Duration <= 0 {
		result = multierror.Append(result, vc.NewErrorForField("window", "must be positive"))
	}
	if s.Limit <= 0 {
		result = multierror.Append(result, vc.NewErrorForField("limit", "must be positive"))
	}
	if !IsValidSlidingWindowMode(s.Mode) {
		result = multierror.Append(result, vc.NewErrorfForField("mode", "invalid mode %q (expected %q or %q)", string(s.Mode), string(SlidingWindowModeLog), string(SlidingWindowModeCounter)))
	}
	return result.ErrorOrNil()
}

// TokenBucket allows bursts up to Capacity, refilling at RefillRate tokens
// per second.
type TokenBucket struct {
	Capacity   int     `json:"capacity" yaml:"capacity"`
	RefillRate float64 `json:"refill_rate" yaml:"refill_rate"`
}

// Validate ensures Capacity and RefillRate are positive.
func (t *TokenBucket) Validate(vc *common.ValidationContext) error {
	if t == nil {
		return nil
	}
	result := &multierror.Error{}

	if t.Capacity <= 0 {
		result = multierror.Append(result, vc.NewErrorForField("capacity", "must be positive"))
	}
	if t.RefillRate <= 0 {
		result = multierror.Append(result, vc.NewErrorForField("refill_rate", "must be positive"))
	}
	return result.ErrorOrNil()
}

// Algorithm is a tagged union: exactly one of FixedWindow, SlidingWindow, or
// TokenBucket must be set.
type Algorithm struct {
	FixedWindow   *FixedWindow   `json:"fixed_window,omitempty" yaml:"fixed_window,omitempty"`
	SlidingWindow *SlidingWindow `json:"sliding_window,omitempty" yaml:"sliding_window,omitempty"`
	TokenBucket   *TokenBucket   `json:"token_bucket,omitempty" yaml:"token_bucket,omitempty"`
}

// Validate ensures exactly one variant is set and that the chosen variant
// is itself valid.
func (a *Algorithm) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}

	count := 0
	if a.FixedWindow != nil {
		count++
	}
	if a.SlidingWindow != nil {
		count++
	}
	if a.TokenBucket != nil {
		count++
	}

	switch count {
	case 0:
		result = multierror.Append(result, vc.NewError("exactly one of fixed_window, sliding_window, or token_bucket must be set"))
	case 1:
		// Validate the chosen variant.
		if err := a.FixedWindow.Validate(vc.PushField("fixed_window")); err != nil {
			result = multierror.Append(result, err)
		}
		if err := a.SlidingWindow.Validate(vc.PushField("sliding_window")); err != nil {
			result = multierror.Append(result, err)
		}
		if err := a.TokenBucket.Validate(vc.PushField("token_bucket")); err != nil {
			result = multierror.Append(result, err)
		}
	default:
		result = multierror.Append(result, vc.NewError("exactly one of fixed_window, sliding_window, or token_bucket must be set"))
	}

	return result.ErrorOrNil()
}
