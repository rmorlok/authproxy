package app_metrics

import (
	"context"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/require"
)

func TestIsValidResponseSource(t *testing.T) {
	require.True(t, IsValidResponseSource(ResponseSourceUpstream))
	require.True(t, IsValidResponseSource(ResponseSourceConnectorRateLimiter))
	require.True(t, IsValidResponseSource(ResponseSourceRateLimit))
	require.False(t, IsValidResponseSource(""))
	require.False(t, IsValidResponseSource("bogus"))
}

func TestAttributionContext_Roundtrip(t *testing.T) {
	ctx := context.Background()
	require.Nil(t, AttributionFromContext(ctx))

	attr := &Attribution{Source: ResponseSourceConnectorRateLimiter}
	ctx2 := ContextWithAttribution(ctx, attr)
	got := AttributionFromContext(ctx2)
	require.NotNil(t, got)
	require.Equal(t, ResponseSourceConnectorRateLimiter, got.Source)
}

func TestAttributionContext_NilAttrIsNoOp(t *testing.T) {
	ctx := context.Background()
	ctx2 := ContextWithAttribution(ctx, nil)
	require.Equal(t, ctx, ctx2)
	require.Nil(t, AttributionFromContext(ctx2))
}

func TestAttributionContext_PointerSharedAcrossLayers(t *testing.T) {
	// The attribution pointer is the only thing in the context — middlewares
	// mutate the struct it points to, and observers higher in the stack see
	// the change. This is the property the round-tripper chain relies on.
	attr := &Attribution{}
	ctx := ContextWithAttribution(context.Background(), attr)

	// Inner middleware writes…
	if a := AttributionFromContext(ctx); a != nil {
		a.Source = ResponseSourceRateLimit
		a.RateLimitId = apid.ID("rl_42")
	}

	// Outer middleware reads.
	got := AttributionFromContext(ctx)
	require.Equal(t, ResponseSourceRateLimit, got.Source)
	require.Equal(t, apid.ID("rl_42"), got.RateLimitId)

	// And the pointer the caller passed in is the same struct.
	require.Same(t, attr, got)
}

func TestApplyAttributionToLogRecord_NilContextDefaultsUpstream(t *testing.T) {
	er := &LogRecord{}
	ApplyAttributionToLogRecord(er, nil)
	require.Equal(t, ResponseSourceUpstream, er.ResponseSource)
}

func TestApplyAttributionToLogRecord_NoAttributionInContext(t *testing.T) {
	er := &LogRecord{}
	ApplyAttributionToLogRecord(er, context.Background())
	require.Equal(t, ResponseSourceUpstream, er.ResponseSource)
}

func TestApplyAttributionToLogRecord_StampsAllFields(t *testing.T) {
	er := &LogRecord{}
	attr := &Attribution{
		Source:          ResponseSourceRateLimit,
		RateLimitId:     apid.ID("rl_a"),
		RateLimitMode:   "enforce",
		RateLimitBucket: map[string]string{"actor": "act_x"},
		RateLimitMatched: []RateLimitMatch{
			{Id: apid.ID("rl_a"), Mode: "enforce", Bucket: map[string]string{"actor": "act_x"}},
		},
	}
	ctx := ContextWithAttribution(context.Background(), attr)

	ApplyAttributionToLogRecord(er, ctx)

	require.Equal(t, ResponseSourceRateLimit, er.ResponseSource)
	require.Equal(t, apid.ID("rl_a"), er.RateLimitId)
	require.Equal(t, "enforce", er.RateLimitMode)
	require.Equal(t, map[string]string{"actor": "act_x"}, er.RateLimitBucket)
	require.Len(t, er.RateLimitMatched, 1)
}

func TestApplyAttributionToLogRecord_KeepsExistingValuesWhenAttrIsBlank(t *testing.T) {
	// If a future code path pre-stamps the LogRecord then asks to layer
	// attribution on top, an empty attribution shouldn't clobber what's
	// already there (only non-zero fields overwrite).
	er := &LogRecord{
		ResponseSource: ResponseSourceConnectorRateLimiter,
		RateLimitId:    apid.ID("rl_existing"),
	}
	ctx := ContextWithAttribution(context.Background(), &Attribution{})
	ApplyAttributionToLogRecord(er, ctx)

	require.Equal(t, ResponseSourceConnectorRateLimiter, er.ResponseSource)
	require.Equal(t, apid.ID("rl_existing"), er.RateLimitId)
}

func TestRateLimitMatchJSON_RoundTrip(t *testing.T) {
	// The codec is exercised end-to-end in DB tests; this just pins the
	// public JSON shape used in API responses.
	m := []RateLimitMatch{{
		Id:     apid.ID("rl_alpha"),
		Mode:   "observe",
		Bucket: map[string]string{"team": "alpha"},
	}}
	encoded, err := marshalRateLimitMatched(m)
	require.NoError(t, err)
	decoded, err := unmarshalRateLimitMatched([]byte(encoded))
	require.NoError(t, err)
	require.Equal(t, m, decoded)
}
