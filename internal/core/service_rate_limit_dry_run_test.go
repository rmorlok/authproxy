package core

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apasynq/mock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	hmock "github.com/rmorlok/authproxy/internal/httpf/mock"
	"github.com/rmorlok/authproxy/internal/ratelimit"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cfgschema "github.com/rmorlok/authproxy/internal/schema/config"
	rlschema "github.com/rmorlok/authproxy/internal/schema/rate_limit"
	"github.com/stretchr/testify/require"
)

// These tests exercise C.DryRunRateLimit directly. Route-level tests in
// internal/routes cover JSON shape + auth wiring; this file covers
// hydration, the namespace cascade, and the Peek-doesn't-mutate
// invariant at the layer that actually owns the logic.

func newDryRunService(t *testing.T) (iface.C, ratelimit.MutableCache, func()) {
	t.Helper()
	cfg := config.FromRoot(&cfgschema.Root{
		DevSettings: &cfgschema.DevSettings{
			Enabled:                  true,
			FakeEncryption:           true,
			FakeEncryptionSkipBase64: true,
		},
		Connectors: &cfgschema.Connectors{},
	})
	logger := slog.Default()
	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
	cfg, r := apredis.MustApplyTestConfig(cfg)
	e := encrypt.NewEncryptService(cfg, db, logger)

	ctrl := gomock.NewController(t)
	asynqClient := mock.NewMockClient(ctrl)
	h := hmock.NewMockF(ctrl)

	rlCache := ratelimit.NewCache()
	svc := NewCoreService(cfg, db, e, r, h, asynqClient, logger,
		WithRateLimitCache(rlCache))
	require.NoError(t, svc.Migrate(context.Background()))

	return svc, rlCache, ctrl.Finish
}

func installRule(t *testing.T, svc iface.C, rlCache ratelimit.MutableCache, namespace string, def rlschema.RateLimit) *database.RateLimit {
	t.Helper()
	created, err := svc.CreateRateLimit(context.Background(), namespace, def, nil, nil)
	require.NoError(t, err)
	// The cache the enforcer reads holds *database.RateLimit rows
	// (loaded by the refresher). Read straight back from the iface
	// rather than reaching into the db, then build a row shape the
	// cache accepts.
	row := &database.RateLimit{
		Id:         created.GetId(),
		Namespace:  created.GetNamespace(),
		Definition: created.GetDefinition(),
	}
	existing := rlCache.All()
	rlCache.Replace(append(existing, row), time.Now())
	return row
}

func freshTokenBucket() rlschema.RateLimit {
	return rlschema.RateLimit{
		Selector: rlschema.Selector{
			Methods:      []string{"POST"},
			RequestTypes: []common.RequestType{common.RequestTypeProxy},
		},
		Bucket: rlschema.Bucket{Dimensions: []string{rlschema.DimensionActor}},
		Algorithm: rlschema.Algorithm{
			TokenBucket: &rlschema.TokenBucket{Capacity: 3, RefillRate: 1.0},
		},
	}
}

// validBaseReq returns a dry-run input with the minimum fields needed
// to pass validation. Per-test edits override what they care about.
func validBaseReq() iface.DryRunRateLimitRequest {
	return iface.DryRunRateLimitRequest{
		Request: iface.ProxyRequest{
			Method: "POST",
			URL:    "https://api.example.com/v1/things",
		},
		RequestType: "proxy",
		Context: iface.DryRunRequestContext{
			Namespace: ptrStr("root"),
			ActorId:   ptrId("act_test"),
		},
	}
}

func TestDryRunRateLimit_ValidationErrors(t *testing.T) {
	svc, _, done := newDryRunService(t)
	defer done()

	t.Run("missing method", func(t *testing.T) {
		req := validBaseReq()
		req.Request.Method = ""
		_, err := svc.DryRunRateLimit(context.Background(), req)
		require.ErrorIs(t, err, ErrInvalidArgument)
	})

	t.Run("missing url", func(t *testing.T) {
		req := validBaseReq()
		req.Request.URL = ""
		_, err := svc.DryRunRateLimit(context.Background(), req)
		require.ErrorIs(t, err, ErrInvalidArgument)
	})

	t.Run("missing request type", func(t *testing.T) {
		req := validBaseReq()
		req.RequestType = ""
		_, err := svc.DryRunRateLimit(context.Background(), req)
		require.ErrorIs(t, err, ErrInvalidArgument)
	})

	t.Run("invalid request type", func(t *testing.T) {
		req := validBaseReq()
		req.RequestType = "bogus"
		_, err := svc.DryRunRateLimit(context.Background(), req)
		require.ErrorIs(t, err, ErrInvalidArgument)
	})

	t.Run("no connection or namespace", func(t *testing.T) {
		req := validBaseReq()
		req.Context = iface.DryRunRequestContext{}
		_, err := svc.DryRunRateLimit(context.Background(), req)
		require.ErrorIs(t, err, ErrInvalidArgument)
	})
}

func TestDryRunRateLimit_PeekDoesNotMutate(t *testing.T) {
	svc, rlCache, done := newDryRunService(t)
	defer done()

	rule := installRule(t, svc, rlCache, "root", freshTokenBucket())

	req := validBaseReq()

	// 5 dry-runs in a row should each see capacity-1 remaining.
	for i := 0; i < 5; i++ {
		res, err := svc.DryRunRateLimit(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, res.Matched, 1, "iteration %d", i+1)
		require.Equal(t, rule.Id, res.Matched[0].RateLimitId)
		require.True(t, res.Matched[0].WouldAllow, "iteration %d", i+1)
		require.Equal(t, 2, res.Matched[0].Remaining, "iteration %d", i+1)
	}
}

func TestDryRunRateLimit_NamespaceCascade(t *testing.T) {
	svc, rlCache, done := newDryRunService(t)
	defer done()

	// Rule at root should apply to a request in root.child; a rule at
	// root.other should not.
	rootRule := installRule(t, svc, rlCache, "root", freshTokenBucket())
	_, err := svc.CreateNamespace(context.Background(), "root.child", nil)
	require.NoError(t, err)
	_, err = svc.CreateNamespace(context.Background(), "root.other", nil)
	require.NoError(t, err)
	otherRule := installRule(t, svc, rlCache, "root.other", freshTokenBucket())

	req := validBaseReq()
	req.Context.Namespace = ptrStr("root.child")
	res, err := svc.DryRunRateLimit(context.Background(), req)
	require.NoError(t, err)

	require.Equal(t, "root.child", res.Namespace)
	require.Len(t, res.Matched, 1, "ancestor rule applies, sibling does not")
	require.Equal(t, rootRule.Id, res.Matched[0].RateLimitId)
	require.Empty(t, res.NotMatched, "the sibling-namespace rule should be filtered out before the matcher runs")
	_ = otherRule
}

func TestDryRunRateLimit_RequestLabelsOverride(t *testing.T) {
	svc, rlCache, done := newDryRunService(t)
	defer done()

	installRule(t, svc, rlCache, "root", rlschema.RateLimit{
		Selector: rlschema.Selector{
			LabelSelector: "team=acme",
			Methods:       []string{"POST"},
			RequestTypes:  []common.RequestType{common.RequestTypeProxy},
		},
		Bucket: rlschema.Bucket{Dimensions: []string{rlschema.DimensionActor}},
		Algorithm: rlschema.Algorithm{
			TokenBucket: &rlschema.TokenBucket{Capacity: 5, RefillRate: 1.0},
		},
	})

	req := validBaseReq()
	req.Request.Labels = map[string]string{"team": "acme"}
	res, err := svc.DryRunRateLimit(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, res.Matched, 1)
	require.Equal(t, "acme", res.RequestLabelSnapshot["team"])
}

func TestDryRunRateLimit_MissReasonReported(t *testing.T) {
	svc, rlCache, done := newDryRunService(t)
	defer done()

	rule := installRule(t, svc, rlCache, "root", rlschema.RateLimit{
		Selector: rlschema.Selector{
			Methods:      []string{"DELETE"},
			RequestTypes: []common.RequestType{common.RequestTypeProxy},
		},
		Bucket:    rlschema.Bucket{},
		Algorithm: rlschema.Algorithm{TokenBucket: &rlschema.TokenBucket{Capacity: 1, RefillRate: 1.0}},
	})

	res, err := svc.DryRunRateLimit(context.Background(), validBaseReq())
	require.NoError(t, err)
	require.Empty(t, res.Matched)
	require.Len(t, res.NotMatched, 1)
	require.Equal(t, rule.Id, res.NotMatched[0].RateLimitId)
	require.Contains(t, res.NotMatched[0].Reason, "method")
}

func TestDryRunRateLimit_NoCache(t *testing.T) {
	// A core service without WithRateLimitCache should still accept
	// dry-run requests and return empty results rather than 500ing.
	cfg := config.FromRoot(&cfgschema.Root{
		DevSettings: &cfgschema.DevSettings{Enabled: true, FakeEncryption: true, FakeEncryptionSkipBase64: true},
		Connectors:  &cfgschema.Connectors{},
	})
	logger := slog.Default()
	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
	cfg, r := apredis.MustApplyTestConfig(cfg)
	e := encrypt.NewEncryptService(cfg, db, logger)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := NewCoreService(cfg, db, e, r, hmock.NewMockF(ctrl), mock.NewMockClient(ctrl), logger)
	require.NoError(t, svc.Migrate(context.Background()))

	res, err := svc.DryRunRateLimit(context.Background(), validBaseReq())
	require.NoError(t, err)
	require.Empty(t, res.Matched)
	require.Empty(t, res.NotMatched)
}

func ptrId(s string) *apid.ID {
	id := apid.ID(s)
	return &id
}
