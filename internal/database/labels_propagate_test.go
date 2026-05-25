package database

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

// minimalRateLimitDef returns a small but valid rate-limit definition so
// CreateRateLimit succeeds. The exact algorithm shape is not relevant to
// label-propagation tests.
func minimalRateLimitDef() rlschema.RateLimit {
	return rlschema.RateLimit{
		Selector: rlschema.Selector{},
		Bucket:   rlschema.Bucket{},
		Algorithm: rlschema.Algorithm{
			TokenBucket: &rlschema.TokenBucket{Capacity: 1, RefillRate: 1},
		},
	}
}

// TestRateLimitLabelChangePropagation drives RefreshNamespaceLabelsCarryForward
// directly to assert that a namespace label change at the top of a tree
// propagates all the way to a rate limit defined in a grandchild namespace —
// exercising the recursion through child namespaces and the
// refreshRateLimitsInNamespace step.
func TestRateLimitLabelChangePropagation(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	now := time.Date(2024, time.February, 1, 12, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	// Tree:
	//   root.rl                   labels: {team: platform}
	//   root.rl.child             labels: {env: prod}
	// One rate limit in each namespace.
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{
		Path: "root.rl", State: NamespaceStateActive, Labels: Labels{"team": "platform"},
	}))
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{
		Path: "root.rl.child", State: NamespaceStateActive, Labels: Labels{"env": "prod"},
	}))

	rlIn_root := apid.New(apid.PrefixRateLimit)
	require.NoError(t, db.CreateRateLimit(ctx, &RateLimit{
		Id: rlIn_root, Namespace: "root.rl", Definition: minimalRateLimitDef(),
	}))

	rlIn_child := apid.New(apid.PrefixRateLimit)
	require.NoError(t, db.CreateRateLimit(ctx, &RateLimit{
		Id: rlIn_child, Namespace: "root.rl.child", Definition: minimalRateLimitDef(),
	}))

	// Sanity: pre-update state.
	r, err := db.GetRateLimit(ctx, rlIn_root)
	require.NoError(t, err)
	require.Equal(t, "platform", r.Labels["apxy/ns/team"])

	r, err = db.GetRateLimit(ctx, rlIn_child)
	require.NoError(t, err)
	require.Equal(t, "platform", r.Labels["apxy/ns/team"])
	require.Equal(t, "prod", r.Labels["apxy/ns/env"])

	// Replace root.rl's labels — team goes away, region comes in.
	_, err = db.UpdateNamespaceLabels(ctx, "root.rl", map[string]string{"region": "us-east"})
	require.NoError(t, err)
	require.NoError(t, db.RefreshNamespaceLabelsCarryForward(ctx, "root.rl"))

	// Rate limit in root.rl: region in, team out.
	r, err = db.GetRateLimit(ctx, rlIn_root)
	require.NoError(t, err)
	require.Equal(t, "us-east", r.Labels["apxy/ns/region"])
	_, hasTeam := r.Labels["apxy/ns/team"]
	require.False(t, hasTeam, "stale apxy/ns/team should be gone")

	// Rate limit in root.rl.child: region propagates through root.rl.child's
	// chain, child's own env survives, team is gone.
	r, err = db.GetRateLimit(ctx, rlIn_child)
	require.NoError(t, err)
	require.Equal(t, "us-east", r.Labels["apxy/ns/region"])
	require.Equal(t, "prod", r.Labels["apxy/ns/env"])
	_, hasTeam = r.Labels["apxy/ns/team"]
	require.False(t, hasTeam)

	// User labels are still empty (we never set any) and identifier labels
	// survived.
	require.Equal(t, string(rlIn_child), r.Labels["apxy/rl/-/id"])
	require.Equal(t, "root.rl.child", r.Labels["apxy/rl/-/ns"])
}

// TestRateLimitDriftReconciliation simulates label-column drift on a
// rate_limits row (e.g., from a botched manual SQL fix) and runs the daily
// reconciler. The drifted row should be the only one corrected.
func TestRateLimitDriftReconciliation(t *testing.T) {
	_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
	defer rawDb.Close()
	now := time.Date(2024, time.March, 1, 12, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	require.NoError(t, db.CreateNamespace(ctx, &Namespace{
		Path: "root.drift", State: NamespaceStateActive, Labels: Labels{"team": "platform"},
	}))

	rlID := apid.New(apid.PrefixRateLimit)
	require.NoError(t, db.CreateRateLimit(ctx, &RateLimit{
		Id: rlID, Namespace: "root.drift", Definition: minimalRateLimitDef(),
	}))

	// Drain pre-existing drift on the seeded root namespace.
	_, err := db.ReconcileCarryForwardLabels(ctx, 100, nil)
	require.NoError(t, err)

	// Now everything is clean; a second run finds nothing to do.
	fixed, err := db.ReconcileCarryForwardLabels(ctx, 100, nil)
	require.NoError(t, err)
	require.Equal(t, int64(0), fixed, "no drift expected after the first sweep")

	// Corrupt the rate limit's apxy/ns/team label by going around the DB
	// write paths. The reconciler should put it back.
	_, err = rawDb.ExecContext(ctx, `UPDATE rate_limits SET labels = $1 WHERE id = $2`,
		`{"apxy/ns/team":"stale","apxy/rl/-/id":"`+string(rlID)+`","apxy/rl/-/ns":"root.drift"}`, rlID)
	require.NoError(t, err)

	fixed, err = db.ReconcileCarryForwardLabels(ctx, 100, nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), fixed, "exactly the drifted rate-limit row should be corrected")

	got, err := db.GetRateLimit(ctx, rlID)
	require.NoError(t, err)
	require.Equal(t, "platform", got.Labels["apxy/ns/team"], "carry-forward should overwrite the stale value")
}

// TestRateLimitRecomputeIdempotent confirms recomputeRateLimitLabelsTx returns
// false when the on-disk labels already match the canonical computation —
// the property the reconciler relies on to skip writes for clean rows.
func TestRateLimitRecomputeIdempotent(t *testing.T) {
	cfg, db := MustApplyBlankTestDbConfig(t, nil)
	now := time.Date(2024, time.March, 1, 12, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	require.NoError(t, db.CreateNamespace(ctx, &Namespace{
		Path: "root.idempotent", State: NamespaceStateActive, Labels: Labels{"team": "x"},
	}))

	rl := &RateLimit{
		Id:         apid.New(apid.PrefixRateLimit),
		Namespace:  "root.idempotent",
		Definition: minimalRateLimitDef(),
	}
	require.NoError(t, db.CreateRateLimit(ctx, rl))

	// Cast through the *service so we can call the unexported recompute
	// helper directly. The DB interface returned by MustApplyBlankTestDbConfig
	// is satisfied by *service.
	_ = cfg
	svc := db.(*service)

	corrected, err := svc.recomputeRateLimitLabelsTx(ctx, rl.Id)
	require.NoError(t, err)
	require.False(t, corrected, "recompute on a freshly-created row should report no drift")

	// And again, just to be sure.
	corrected, err = svc.recomputeRateLimitLabelsTx(ctx, rl.Id)
	require.NoError(t, err)
	require.False(t, corrected)
}

// TestRateLimitRecomputeOnDeletedRow ensures the recompute helper treats a
// missing row as a no-op (returns nil error, drift=false) rather than failing.
// This matches the behaviour of the other recompute*Tx helpers and is the
// behaviour the reconciler relies on when a row is deleted between the list
// query and the per-row recompute.
func TestRateLimitRecomputeOnDeletedRow(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	now := time.Date(2024, time.March, 1, 12, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	require.NoError(t, db.CreateNamespace(ctx, &Namespace{
		Path: "root.gone", State: NamespaceStateActive,
	}))
	rl := &RateLimit{
		Id: apid.New(apid.PrefixRateLimit), Namespace: "root.gone", Definition: minimalRateLimitDef(),
	}
	require.NoError(t, db.CreateRateLimit(ctx, rl))
	require.NoError(t, db.DeleteRateLimit(ctx, rl.Id))

	svc := db.(*service)
	corrected, err := svc.recomputeRateLimitLabelsTx(ctx, rl.Id)
	require.NoError(t, err)
	require.False(t, corrected)
}

// TestLabelsEqual covers the small helper used by writeRecomputedLabels to
// short-circuit no-op writes. It's a pure function so a focused test is
// cheap insurance against a regression that would silently turn the
// reconciler into a write storm.
func TestLabelsEqual(t *testing.T) {
	cases := []struct {
		name     string
		a, b     Labels
		expected bool
	}{
		{"both nil", nil, nil, true},
		{"nil and empty", nil, Labels{}, true},
		{"identical", Labels{"k": "v"}, Labels{"k": "v"}, true},
		{"different value", Labels{"k": "v"}, Labels{"k": "w"}, false},
		{"different size", Labels{"k": "v"}, Labels{"k": "v", "x": "y"}, false},
		{"different keys", Labels{"k": "v"}, Labels{"j": "v"}, false},
		{"order-insensitive", Labels{"a": "1", "b": "2"}, Labels{"b": "2", "a": "1"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, labelsEqual(tc.a, tc.b))
			// labelsEqual must be symmetric.
			require.Equal(t, tc.expected, labelsEqual(tc.b, tc.a))
		})
	}
}
