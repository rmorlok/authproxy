package database

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	rlschema "github.com/rmorlok/authproxy/internal/schema/rate_limit"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

// validDef returns a known-good definition for use across CRUD tests.
func validDef() rlschema.RateLimit {
	return rlschema.RateLimit{
		Mode: rlschema.ModeEnforce,
		Selector: rlschema.Selector{
			Methods: []string{"GET"},
			PathMatch: &rlschema.PathMatch{
				Kind:  rlschema.PathMatchKindPrefix,
				Value: "/v1/",
			},
			RequestTypes: []rlschema.RequestType{rlschema.RequestTypeProxy},
		},
		Bucket: rlschema.Bucket{Dimensions: []string{rlschema.DimensionActor}},
		Algorithm: rlschema.Algorithm{
			TokenBucket: &rlschema.TokenBucket{Capacity: 60, RefillRate: 1.0},
		},
	}
}

func TestRateLimit_CRUD(t *testing.T) {
	_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	// Create
	rl := &RateLimit{
		Id:         apid.New(apid.PrefixRateLimit),
		Namespace:  "root",
		Definition: validDef(),
		Labels:     Labels{"team": "platform"},
	}
	require.NoError(t, db.CreateRateLimit(ctx, rl))
	require.Equal(t, now, rl.CreatedAt)
	require.Equal(t, now, rl.UpdatedAt)

	// Get
	got, err := db.GetRateLimit(ctx, rl.Id)
	require.NoError(t, err)
	require.Equal(t, rl.Id, got.Id)
	require.Equal(t, "root", got.Namespace)
	require.Equal(t, rlschema.ModeEnforce, got.Definition.Mode)
	require.NotNil(t, got.Definition.Algorithm.TokenBucket)
	require.Equal(t, 60, got.Definition.Algorithm.TokenBucket.Capacity)

	// Implicit identifier labels are present.
	require.Equal(t, string(rl.Id), got.Labels["apxy/rl/-/id"])
	require.Equal(t, "root", got.Labels["apxy/rl/-/ns"])
	user, _ := SplitUserAndApxyLabels(got.Labels)
	require.Equal(t, Labels{"team": "platform"}, user)

	// UpdateRateLimitDefinition advances UpdatedAt and persists changes.
	later := now.Add(time.Hour)
	ctx = apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(later)).Build()
	newDef := validDef()
	newDef.Algorithm = rlschema.Algorithm{
		FixedWindow: &rlschema.FixedWindow{
			Window: common.HumanDuration{Duration: time.Minute},
			Limit:  10,
		},
		TokenBucket: nil,
	}
	updated, err := db.UpdateRateLimitDefinition(ctx, rl.Id, newDef)
	require.NoError(t, err)
	require.True(t, later.Equal(updated.UpdatedAt))
	require.Nil(t, updated.Definition.Algorithm.TokenBucket)
	require.NotNil(t, updated.Definition.Algorithm.FixedWindow)
	require.Equal(t, 10, updated.Definition.Algorithm.FixedWindow.Limit)

	// Get not found
	_, err = db.GetRateLimit(ctx, apid.New(apid.PrefixRateLimit))
	require.ErrorIs(t, err, ErrNotFound)

	// Delete (soft)
	require.NoError(t, db.DeleteRateLimit(ctx, rl.Id))
	_, err = db.GetRateLimit(ctx, rl.Id)
	require.ErrorIs(t, err, ErrNotFound)

	// Delete not found
	require.ErrorIs(t, db.DeleteRateLimit(ctx, apid.New(apid.PrefixRateLimit)), ErrNotFound)
}

func TestRateLimit_UpdateDefinition_RejectsInvalid(t *testing.T) {
	_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	rl := &RateLimit{
		Id:         apid.New(apid.PrefixRateLimit),
		Namespace:  "root",
		Definition: validDef(),
	}
	require.NoError(t, db.CreateRateLimit(ctx, rl))

	// Two algorithm variants set — should be rejected before any DB write.
	bad := validDef()
	bad.Algorithm.FixedWindow = &rlschema.FixedWindow{
		Window: common.HumanDuration{Duration: time.Minute}, Limit: 10,
	}
	_, err := db.UpdateRateLimitDefinition(ctx, rl.Id, bad)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exactly one of")
}

func TestRateLimit_Validation(t *testing.T) {
	_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	// Missing namespace
	rl := &RateLimit{
		Id:         apid.New(apid.PrefixRateLimit),
		Definition: validDef(),
	}
	require.Error(t, db.CreateRateLimit(ctx, rl))

	// Missing id
	rl = &RateLimit{
		Namespace:  "root",
		Definition: validDef(),
	}
	require.Error(t, db.CreateRateLimit(ctx, rl))

	// Wrong prefix on id
	rl = &RateLimit{
		Id:         apid.New(apid.PrefixActor),
		Namespace:  "root",
		Definition: validDef(),
	}
	err := db.CreateRateLimit(ctx, rl)
	require.Error(t, err)
	require.Contains(t, err.Error(), "prefix")

	// Definition validation surfaces (empty algorithm).
	rl = &RateLimit{
		Id:        apid.New(apid.PrefixRateLimit),
		Namespace: "root",
		Definition: rlschema.RateLimit{
			Selector: rlschema.Selector{Methods: []string{"GET"}},
			Bucket:   rlschema.Bucket{},
			// Algorithm intentionally empty.
		},
	}
	err = db.CreateRateLimit(ctx, rl)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exactly one of")
}

func TestRateLimit_Labels(t *testing.T) {
	_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	rl := &RateLimit{
		Id:         apid.New(apid.PrefixRateLimit),
		Namespace:  "root",
		Definition: validDef(),
		Labels:     Labels{"team": "platform"},
	}
	require.NoError(t, db.CreateRateLimit(ctx, rl))

	// PutLabels merges
	later := now.Add(time.Hour)
	ctx = apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(later)).Build()
	updated, err := db.PutRateLimitLabels(ctx, rl.Id, map[string]string{"region": "us-east"})
	require.NoError(t, err)
	require.Equal(t, "platform", updated.Labels["team"])
	require.Equal(t, "us-east", updated.Labels["region"])

	// UpdateLabels (full replace) — apxy/* survive.
	updated, err = db.UpdateRateLimitLabels(ctx, rl.Id, map[string]string{"only-this": "now"})
	require.NoError(t, err)
	user, _ := SplitUserAndApxyLabels(updated.Labels)
	require.Equal(t, Labels{"only-this": "now"}, user)
	require.Equal(t, string(rl.Id), updated.Labels["apxy/rl/-/id"])

	// DeleteLabels
	updated, err = db.DeleteRateLimitLabels(ctx, rl.Id, []string{"only-this"})
	require.NoError(t, err)
	user, _ = SplitUserAndApxyLabels(updated.Labels)
	require.Empty(t, user)

	// Not found cases
	fake := apid.New(apid.PrefixRateLimit)
	_, err = db.PutRateLimitLabels(ctx, fake, map[string]string{"k": "v"})
	require.ErrorIs(t, err, ErrNotFound)
	_, err = db.UpdateRateLimitLabels(ctx, fake, map[string]string{"k": "v"})
	require.ErrorIs(t, err, ErrNotFound)
	_, err = db.DeleteRateLimitLabels(ctx, fake, []string{"k"})
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRateLimit_Annotations(t *testing.T) {
	_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	rl := &RateLimit{
		Id:          apid.New(apid.PrefixRateLimit),
		Namespace:   "root",
		Definition:  validDef(),
		Annotations: Annotations{"description": "service throttle"},
	}
	require.NoError(t, db.CreateRateLimit(ctx, rl))

	got, err := db.GetRateLimit(ctx, rl.Id)
	require.NoError(t, err)
	require.Equal(t, Annotations{"description": "service throttle"}, got.Annotations)

	updated, err := db.PutRateLimitAnnotations(ctx, rl.Id, map[string]string{"owner": "platform@example.com"})
	require.NoError(t, err)
	require.Equal(t, "service throttle", updated.Annotations["description"])
	require.Equal(t, "platform@example.com", updated.Annotations["owner"])

	updated, err = db.UpdateRateLimitAnnotations(ctx, rl.Id, map[string]string{"only": "this"})
	require.NoError(t, err)
	require.Equal(t, Annotations{"only": "this"}, updated.Annotations)

	updated, err = db.DeleteRateLimitAnnotations(ctx, rl.Id, []string{"only"})
	require.NoError(t, err)
	require.Empty(t, updated.Annotations)
}

func TestRateLimit_ListBuilder(t *testing.T) {
	_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	for i := 0; i < 5; i++ {
		rl := &RateLimit{
			Id:         apid.New(apid.PrefixRateLimit),
			Namespace:  "root",
			Definition: validDef(),
		}
		require.NoError(t, db.CreateRateLimit(ctx, rl))
	}

	page := db.ListRateLimitsBuilder().FetchPage(ctx)
	require.NoError(t, page.Error)
	require.Len(t, page.Results, 5)

	page = db.ListRateLimitsBuilder().Limit(2).FetchPage(ctx)
	require.NoError(t, page.Error)
	require.Len(t, page.Results, 2)
	require.True(t, page.HasMore)

	page = db.ListRateLimitsBuilder().ForNamespaceMatcher("root").FetchPage(ctx)
	require.NoError(t, page.Error)
	require.Len(t, page.Results, 5)

	page = db.ListRateLimitsBuilder().ForNamespaceMatcher("root.nonexistent").FetchPage(ctx)
	require.NoError(t, page.Error)
	require.Len(t, page.Results, 0)
}

// TestRateLimit_CarryForward verifies that rate limits inherit their parent
// namespace's user labels at creation, and that a subsequent namespace label
// change propagates to the rate limit via the same refresh hook used by other
// resource types.
func TestRateLimit_CarryForward(t *testing.T) {
	_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	// Set a user label on root namespace.
	_, err := db.PutNamespaceLabels(ctx, "root", map[string]string{"env": "prod"})
	require.NoError(t, err)

	// Create a rate limit in root and confirm the namespace's user label
	// gets carried-forward as apxy/ns/env=prod on the rate limit.
	rl := &RateLimit{
		Id:         apid.New(apid.PrefixRateLimit),
		Namespace:  "root",
		Definition: validDef(),
	}
	require.NoError(t, db.CreateRateLimit(ctx, rl))

	got, err := db.GetRateLimit(ctx, rl.Id)
	require.NoError(t, err)
	require.Equal(t, "prod", got.Labels["apxy/ns/env"])

	// Change the namespace's user label and trigger refresh.
	later := now.Add(time.Hour)
	ctx = apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(later)).Build()
	_, err = db.UpdateNamespaceLabels(ctx, "root", map[string]string{"env": "staging"})
	require.NoError(t, err)
	require.NoError(t, db.RefreshNamespaceLabelsCarryForward(ctx, "root"))

	got, err = db.GetRateLimit(ctx, rl.Id)
	require.NoError(t, err)
	require.Equal(t, "staging", got.Labels["apxy/ns/env"])

	// User-supplied labels and identifier labels still present after refresh.
	require.Equal(t, string(rl.Id), got.Labels["apxy/rl/-/id"])
}

// TestRateLimit_DefinitionRoundTripJSON ensures the JSON-encoded definition
// column survives a full round-trip including each algorithm variant.
func TestRateLimit_DefinitionRoundTripJSON(t *testing.T) {
	_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	cases := []struct {
		name string
		def  rlschema.RateLimit
	}{
		{
			name: "fixed_window",
			def: rlschema.RateLimit{
				Mode:     rlschema.ModeEnforce,
				Selector: rlschema.Selector{Methods: []string{"GET"}, RequestTypes: []rlschema.RequestType{rlschema.RequestTypeProxy}},
				Bucket:   rlschema.Bucket{Dimensions: []string{rlschema.DimensionActor}},
				Algorithm: rlschema.Algorithm{
					FixedWindow: &rlschema.FixedWindow{
						Window: common.HumanDuration{Duration: time.Minute}, Limit: 100,
					},
				},
			},
		},
		{
			name: "sliding_window_log",
			def: rlschema.RateLimit{
				Selector: rlschema.Selector{Methods: []string{"POST"}, RequestTypes: []rlschema.RequestType{rlschema.RequestTypeProxy}},
				Bucket:   rlschema.Bucket{Dimensions: []string{rlschema.DimensionConnection}},
				Algorithm: rlschema.Algorithm{
					SlidingWindow: &rlschema.SlidingWindow{
						Window: common.HumanDuration{Duration: 5 * time.Minute}, Limit: 50, Mode: rlschema.SlidingWindowModeLog,
					},
				},
			},
		},
		{
			name: "token_bucket_observe",
			def: rlschema.RateLimit{
				Mode:     rlschema.ModeObserve,
				Selector: rlschema.Selector{},
				Bucket:   rlschema.Bucket{},
				Algorithm: rlschema.Algorithm{
					TokenBucket: &rlschema.TokenBucket{Capacity: 200, RefillRate: 5.5},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rl := &RateLimit{
				Id:         apid.New(apid.PrefixRateLimit),
				Namespace:  "root",
				Definition: tc.def,
			}
			require.NoError(t, db.CreateRateLimit(ctx, rl))
			got, err := db.GetRateLimit(ctx, rl.Id)
			require.NoError(t, err)
			require.Equal(t, tc.def, got.Definition)
		})
	}
}
