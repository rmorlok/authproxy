package rate_limit

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

// vc returns a fresh root validation context for direct Validate-method tests.
func vc() *common.ValidationContext { return &common.ValidationContext{} }

// validRateLimit returns a fully-populated, valid RateLimit for use as a
// starting point in negative tests.
func validRateLimit() *RateLimit {
	return &RateLimit{
		Mode: ModeEnforce,
		Selector: Selector{
			LabelSelector: "apxy/connector/-/id=salesforce",
			Methods:       []string{"GET", "POST"},
			PathMatch: &PathMatch{
				Kind:  PathMatchKindPrefix,
				Value: "/services/data/",
			},
			RequestTypes: []common.RequestType{common.RequestTypeProxy},
		},
		Bucket: Bucket{
			Dimensions: []string{DimensionActor, "labels/team"},
		},
		Algorithm: Algorithm{
			TokenBucket: &TokenBucket{Capacity: 60, RefillRate: 1.0},
		},
	}
}

func TestRateLimit_Validate_HappyPath(t *testing.T) {
	require.NoError(t, validRateLimit().Validate())
}

func TestRateLimit_EffectiveMode(t *testing.T) {
	require.Equal(t, ModeEnforce, (&RateLimit{}).EffectiveMode())
	require.Equal(t, ModeObserve, (&RateLimit{Mode: ModeObserve}).EffectiveMode())

	// Method on a nil receiver still returns the default — handy for code
	// paths that may receive a missing definition.
	var nilRl *RateLimit
	require.Equal(t, ModeEnforce, nilRl.EffectiveMode())
}

func TestRateLimit_Validate_Mode(t *testing.T) {
	rl := validRateLimit()
	rl.Mode = "audit" // not a recognised mode
	err := rl.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "mode")

	// Empty mode is allowed (uses default).
	rl.Mode = ""
	require.NoError(t, rl.Validate())
}

func TestSelector_Validate_Methods(t *testing.T) {
	rl := validRateLimit()
	rl.Selector.Methods = []string{"GET", "FROBNICATE"}
	err := rl.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "FROBNICATE")
}

func TestSelector_Validate_RequestTypes_DefaultWhenNil(t *testing.T) {
	rl := validRateLimit()
	rl.Selector.RequestTypes = nil
	require.NoError(t, rl.Validate())
	require.Equal(t, []common.RequestType{common.RequestTypeProxy, common.RequestTypeProbe}, rl.Selector.EffectiveRequestTypes())
}

func TestSelector_Validate_RequestTypes_RejectExplicitEmpty(t *testing.T) {
	rl := validRateLimit()
	rl.Selector.RequestTypes = []common.RequestType{}
	err := rl.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "request_types")
	require.Contains(t, err.Error(), "must not be an empty list")
}

func TestSelector_Validate_RequestTypes_RejectUnknown(t *testing.T) {
	rl := validRateLimit()
	rl.Selector.RequestTypes = []common.RequestType{common.RequestTypeProxy, "bogus"}
	err := rl.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "bogus")
}

func TestSelector_Validate_RequestTypes_AcceptsAllKnown(t *testing.T) {
	rl := validRateLimit()
	rl.Selector.RequestTypes = []common.RequestType{
		common.RequestTypeProxy,
		common.RequestTypeProbe,
		common.RequestTypeOAuth,
	}
	require.NoError(t, rl.Validate())
}

func TestSelector_EffectiveRequestTypes_NilReceiver(t *testing.T) {
	var s *Selector
	require.Equal(t, []common.RequestType{common.RequestTypeProxy, common.RequestTypeProbe}, s.EffectiveRequestTypes())
}

func TestPathMatch_Validate(t *testing.T) {
	cases := []struct {
		name    string
		pm      PathMatch
		wantErr bool
		errPart string
	}{
		{"prefix-ok", PathMatch{Kind: PathMatchKindPrefix, Value: "/x"}, false, ""},
		{"glob-ok", PathMatch{Kind: PathMatchKindGlob, Value: "/x/*"}, false, ""},
		{"regex-ok", PathMatch{Kind: PathMatchKindRegex, Value: `^/x/[0-9]+$`}, false, ""},
		{"empty-value", PathMatch{Kind: PathMatchKindPrefix, Value: ""}, true, "must not be empty"},
		{"bad-kind", PathMatch{Kind: "weird", Value: "/x"}, true, "invalid kind"},
		{"bad-regex", PathMatch{Kind: PathMatchKindRegex, Value: "[unterminated"}, true, "invalid regex"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.pm.Validate(&common.ValidationContext{})
			if tc.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errPart)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBucket_Validate_ReservedAndLabel(t *testing.T) {
	b := Bucket{
		Dimensions: []string{DimensionActor, DimensionConnection, "labels/team", "labels/region"},
	}
	require.NoError(t, b.Validate(&common.ValidationContext{}))
}

func TestBucket_Validate_Empty(t *testing.T) {
	// An empty dimensions list is valid — it means a single global bucket.
	require.NoError(t, (&Bucket{}).Validate(&common.ValidationContext{}))
}

func TestBucket_Validate_Errors(t *testing.T) {
	cases := []struct {
		name    string
		dims    []string
		errPart string
	}{
		{"empty-string", []string{""}, "must not be empty"},
		{"unknown-name", []string{"team"}, "must be a reserved name"},
		{"missing-label-key", []string{"labels/"}, "missing label key"},
		{"duplicate", []string{DimensionActor, DimensionActor}, "duplicate dimension"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := (&Bucket{Dimensions: tc.dims}).Validate(&common.ValidationContext{})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errPart)
		})
	}
}

func TestAlgorithm_Validate_ExactlyOne(t *testing.T) {
	none := Algorithm{}
	err := none.Validate(&common.ValidationContext{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "exactly one of")

	two := Algorithm{
		FixedWindow: &FixedWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 10},
		TokenBucket: &TokenBucket{Capacity: 10, RefillRate: 1},
	}
	err = two.Validate(&common.ValidationContext{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "exactly one of")

	all := Algorithm{
		FixedWindow:   &FixedWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 10},
		SlidingWindow: &SlidingWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 10, Mode: SlidingWindowModeLog},
		TokenBucket:   &TokenBucket{Capacity: 10, RefillRate: 1},
	}
	err = all.Validate(&common.ValidationContext{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "exactly one of")
}

func TestAlgorithm_Validate_FixedWindow(t *testing.T) {
	cases := []struct {
		name    string
		fw      FixedWindow
		errPart string
	}{
		{"ok", FixedWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 10}, ""},
		{"zero-window", FixedWindow{Window: common.HumanDuration{Duration: 0}, Limit: 10}, "window"},
		{"negative-limit", FixedWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: -1}, "limit"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := Algorithm{FixedWindow: &tc.fw}
			err := a.Validate(&common.ValidationContext{})
			if tc.errPart == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errPart)
			}
		})
	}
}

func TestAlgorithm_Validate_SlidingWindow(t *testing.T) {
	cases := []struct {
		name    string
		sw      SlidingWindow
		errPart string
	}{
		{"ok-log", SlidingWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 10, Mode: SlidingWindowModeLog}, ""},
		{"ok-counter", SlidingWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 10, Mode: SlidingWindowModeCounter}, ""},
		{"missing-mode", SlidingWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 10}, "mode"},
		{"bad-mode", SlidingWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 10, Mode: "exact"}, "mode"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := Algorithm{SlidingWindow: &tc.sw}
			err := a.Validate(&common.ValidationContext{})
			if tc.errPart == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errPart)
			}
		})
	}
}

func TestAlgorithm_Validate_TokenBucket(t *testing.T) {
	cases := []struct {
		name    string
		tb      TokenBucket
		errPart string
	}{
		{"ok", TokenBucket{Capacity: 10, RefillRate: 1.0}, ""},
		{"zero-capacity", TokenBucket{Capacity: 0, RefillRate: 1.0}, "capacity"},
		{"zero-rate", TokenBucket{Capacity: 10, RefillRate: 0}, "refill_rate"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := Algorithm{TokenBucket: &tc.tb}
			err := a.Validate(&common.ValidationContext{})
			if tc.errPart == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errPart)
			}
		})
	}
}

// --- Direct tests for leaf Validate methods ---
//
// The earlier table tests exercise each algorithm variant via the parent
// Algorithm.Validate, which is the call site that matters in production.
// These tests pin the leaf types' contracts directly so a regression in
// FixedWindow.Validate / SlidingWindow.Validate / TokenBucket.Validate is
// caught even if the parent dispatch logic also breaks.

func TestFixedWindow_Validate_Direct(t *testing.T) {
	require.NoError(t, (*FixedWindow)(nil).Validate(vc()))

	ok := &FixedWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 5}
	require.NoError(t, ok.Validate(vc()))

	zeroWindow := &FixedWindow{Window: common.HumanDuration{Duration: 0}, Limit: 5}
	err := zeroWindow.Validate(vc())
	require.Error(t, err)
	require.Contains(t, err.Error(), "window")

	zeroLimit := &FixedWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 0}
	err = zeroLimit.Validate(vc())
	require.Error(t, err)
	require.Contains(t, err.Error(), "limit")

	negativeWindow := &FixedWindow{Window: common.HumanDuration{Duration: -1 * time.Second}, Limit: 5}
	err = negativeWindow.Validate(vc())
	require.Error(t, err)
	require.Contains(t, err.Error(), "window")
}

func TestSlidingWindow_Validate_Direct(t *testing.T) {
	require.NoError(t, (*SlidingWindow)(nil).Validate(vc()))

	for _, mode := range []SlidingWindowMode{SlidingWindowModeLog, SlidingWindowModeCounter} {
		sw := &SlidingWindow{
			Window: common.HumanDuration{Duration: time.Minute},
			Limit:  10,
			Mode:   mode,
		}
		require.NoError(t, sw.Validate(vc()), "mode %s should be valid", mode)
	}

	missingMode := &SlidingWindow{
		Window: common.HumanDuration{Duration: time.Minute}, Limit: 10,
	}
	err := missingMode.Validate(vc())
	require.Error(t, err)
	require.Contains(t, err.Error(), "mode")

	badMode := &SlidingWindow{
		Window: common.HumanDuration{Duration: time.Minute}, Limit: 10, Mode: "exact",
	}
	err = badMode.Validate(vc())
	require.Error(t, err)
	require.Contains(t, err.Error(), "mode")

	zeroWindow := &SlidingWindow{Mode: SlidingWindowModeLog, Limit: 10}
	err = zeroWindow.Validate(vc())
	require.Error(t, err)
	require.Contains(t, err.Error(), "window")

	zeroLimit := &SlidingWindow{
		Window: common.HumanDuration{Duration: time.Minute}, Mode: SlidingWindowModeLog,
	}
	err = zeroLimit.Validate(vc())
	require.Error(t, err)
	require.Contains(t, err.Error(), "limit")
}

func TestTokenBucket_Validate_Direct(t *testing.T) {
	require.NoError(t, (*TokenBucket)(nil).Validate(vc()))

	ok := &TokenBucket{Capacity: 10, RefillRate: 1.5}
	require.NoError(t, ok.Validate(vc()))

	zeroCap := &TokenBucket{Capacity: 0, RefillRate: 1.0}
	err := zeroCap.Validate(vc())
	require.Error(t, err)
	require.Contains(t, err.Error(), "capacity")

	zeroRate := &TokenBucket{Capacity: 10, RefillRate: 0}
	err = zeroRate.Validate(vc())
	require.Error(t, err)
	require.Contains(t, err.Error(), "refill_rate")

	negativeCap := &TokenBucket{Capacity: -1, RefillRate: 1.0}
	err = negativeCap.Validate(vc())
	require.Error(t, err)
	require.Contains(t, err.Error(), "capacity")

	negativeRate := &TokenBucket{Capacity: 10, RefillRate: -0.5}
	err = negativeRate.Validate(vc())
	require.Error(t, err)
	require.Contains(t, err.Error(), "refill_rate")
}

func TestSelector_Validate_Direct(t *testing.T) {
	// Empty selector validates — every clause is optional.
	require.NoError(t, (&Selector{}).Validate(vc()))

	// Methods accept canonical HTTP verbs.
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"} {
		s := &Selector{Methods: []string{m}}
		require.NoError(t, s.Validate(vc()), "method %s should be valid", m)
	}

	// Lowercase is rejected so a typo in the rule can't silently match
	// nothing.
	bad := &Selector{Methods: []string{"get"}}
	err := bad.Validate(vc())
	require.Error(t, err)
	require.Contains(t, err.Error(), "get")

	// PathMatch errors propagate with a path prefix.
	withBadPath := &Selector{
		PathMatch: &PathMatch{Kind: PathMatchKindRegex, Value: "[unterminated"},
	}
	err = withBadPath.Validate(vc())
	require.Error(t, err)
	require.Contains(t, err.Error(), "path_match")
	require.Contains(t, err.Error(), "regex")

	// Explicit empty request_types is rejected; nil is fine.
	empty := &Selector{RequestTypes: []common.RequestType{}}
	err = empty.Validate(vc())
	require.Error(t, err)
	require.Contains(t, err.Error(), "request_types")

	// Nil request_types is valid (default is materialised by EffectiveRequestTypes).
	require.NoError(t, (&Selector{}).Validate(vc()))

	// Validation errors carry the field name passed in via vc.
	vcWithRoot := (&common.ValidationContext{}).PushField("selector")
	err = (&Selector{Methods: []string{"WAT"}}).Validate(vcWithRoot)
	require.Error(t, err)
	require.Contains(t, err.Error(), "selector.methods")
}

func TestRateLimit_Validate_ErrorPathPrefix(t *testing.T) {
	// Errors from nested validation should carry the field path so the user
	// knows which field tripped the rule.
	rl := validRateLimit()
	rl.Selector.Methods = []string{"WAT"}
	err := rl.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "selector.methods")
}
