package ratelimit

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
)

func TestBucketKey_String_Global(t *testing.T) {
	k := BucketKey{}
	require.True(t, k.IsGlobal())
	require.Equal(t, "*", k.String())
}

func TestBucketKey_String_Stable(t *testing.T) {
	// Two identical inputs must produce the same string. Tests deterministic
	// serialization across invocations — important because the string is
	// used as a Redis sub-key.
	k1 := BucketKey{Components: []BucketKeyComponent{
		{Name: "actor", Value: "act_a"},
		{Name: "labels/team", Value: "alpha"},
	}}
	k2 := BucketKey{Components: []BucketKeyComponent{
		{Name: "actor", Value: "act_a"},
		{Name: "labels/team", Value: "alpha"},
	}}
	require.Equal(t, k1.String(), k2.String())
	require.Equal(t, "actor=act_a|labels/team=alpha", k1.String())
}

func TestBucketKey_String_OrderMatters(t *testing.T) {
	// Component order is preserved as-is so two rules with the same
	// dimension *set* but different *order* produce different keys
	// (matching the rule's own dimension list, which is the source of
	// truth).
	k1 := BucketKey{Components: []BucketKeyComponent{
		{Name: "actor", Value: "act_a"},
		{Name: "namespace", Value: "root"},
	}}
	k2 := BucketKey{Components: []BucketKeyComponent{
		{Name: "namespace", Value: "root"},
		{Name: "actor", Value: "act_a"},
	}}
	require.NotEqual(t, k1.String(), k2.String())
}

func TestBucketKey_AsMap_Global(t *testing.T) {
	// IsGlobal() keys have no components — AsMap should return nil so
	// downstream JSON serialisation produces `null` (or omits the field
	// entirely when annotated `omitempty`) rather than `{}`.
	require.Nil(t, BucketKey{}.AsMap())
}

func TestBucketKey_AsMap_PopulatedRoundTrip(t *testing.T) {
	k := BucketKey{Components: []BucketKeyComponent{
		{Name: "actor", Value: "act_a"},
		{Name: "labels/team", Value: "alpha"},
	}}
	got := k.AsMap()
	require.Equal(t, map[string]string{
		"actor":       "act_a",
		"labels/team": "alpha",
	}, got)
}

func TestBucketKey_AsMap_EmptyValuesPreserved(t *testing.T) {
	// Missing dimensions resolve to "" — those should still appear in the
	// map so log-side filtering can distinguish "actor=" (unauthenticated
	// traffic) from a populated value.
	k := BucketKey{Components: []BucketKeyComponent{
		{Name: "actor", Value: ""},
	}}
	require.Equal(t, map[string]string{"actor": ""}, k.AsMap())
}

func TestBucketKey_String_Escapes(t *testing.T) {
	// Values containing the field/separator characters must be escaped so
	// they don't produce ambiguous parses or collisions.
	k := BucketKey{Components: []BucketKeyComponent{
		{Name: "labels/raw", Value: "a|b=c%d"},
	}}
	require.Equal(t, "labels/raw=a%7Cb%3Dc%25d", k.String())
}

func TestResolveBucketKey_Empty(t *testing.T) {
	// Empty dimensions list = single global bucket per rule.
	rule := rlschema.RateLimit{Bucket: rlschema.Bucket{}}
	ctx := &RequestContext{ActorID: apid.ID("act_x")}
	k := ResolveBucketKey(rule, ctx)
	require.True(t, k.IsGlobal())
}

func TestResolveBucketKey_ReservedDimensions(t *testing.T) {
	rule := rlschema.RateLimit{Bucket: rlschema.Bucket{
		Dimensions: []string{
			rlschema.DimensionActor,
			rlschema.DimensionConnection,
			rlschema.DimensionConnector,
			rlschema.DimensionConnectorVersion,
			rlschema.DimensionNamespace,
			rlschema.DimensionMethod,
		},
	}}
	ctx := &RequestContext{
		ActorID:          apid.ID("act_a"),
		ConnectionID:     apid.ID("cxn_1"),
		ConnectorID:      apid.ID("cxr_1"),
		ConnectorVersion: 7,
		Namespace:        "root.team-x",
		Method:           "POST",
	}
	k := ResolveBucketKey(rule, ctx)
	require.Equal(t,
		"actor=act_a|connection=cxn_1|connector=cxr_1|connector_version=7|namespace=root.team-x|method=POST",
		k.String(),
	)
}

func TestResolveBucketKey_LabelDimensions(t *testing.T) {
	rule := rlschema.RateLimit{Bucket: rlschema.Bucket{
		Dimensions: []string{"labels/team", "labels/region"},
	}}
	ctx := &RequestContext{Labels: map[string]string{
		"team":   "alpha",
		"region": "us-east",
	}}
	k := ResolveBucketKey(rule, ctx)
	require.Equal(t, "labels/team=alpha|labels/region=us-east", k.String())
}

func TestResolveBucketKey_MissingValuesResolveEmpty(t *testing.T) {
	// Missing reserved + missing label both resolve to "" — distinct
	// from a present-but-empty value only in semantics. The downstream
	// counter sees the resolved tuple as opaque.
	rule := rlschema.RateLimit{Bucket: rlschema.Bucket{
		Dimensions: []string{
			rlschema.DimensionActor,
			"labels/missing",
		},
	}}
	ctx := &RequestContext{} // nothing populated
	k := ResolveBucketKey(rule, ctx)
	require.Equal(t, "actor=|labels/missing=", k.String())
}

func TestResolveBucketKey_NilContext(t *testing.T) {
	rule := rlschema.RateLimit{Bucket: rlschema.Bucket{
		Dimensions: []string{rlschema.DimensionActor},
	}}
	k := ResolveBucketKey(rule, nil)
	require.Equal(t, "actor=", k.String())
}

func TestResolveBucketKey_ZeroConnectorVersionResolvesEmpty(t *testing.T) {
	// 0 means "no connector version" (unset) — distinct from version 0,
	// which is impossible per apid conventions. Render as "" so it
	// doesn't bucket alongside other small versions.
	rule := rlschema.RateLimit{Bucket: rlschema.Bucket{
		Dimensions: []string{rlschema.DimensionConnectorVersion},
	}}
	ctx := &RequestContext{ConnectorVersion: 0}
	k := ResolveBucketKey(rule, ctx)
	require.Equal(t, "connector_version=", k.String())
}

func TestResolveBucketKey_UnknownDimensionResolvesEmpty(t *testing.T) {
	// Schema validation should prevent unknown dimensions; if one slips
	// through we still render the key deterministically rather than
	// panicking or producing a malformed string.
	rule := rlschema.RateLimit{Bucket: rlschema.Bucket{
		Dimensions: []string{"made_up_thing"},
	}}
	ctx := &RequestContext{}
	k := ResolveBucketKey(rule, ctx)
	require.Equal(t, "made_up_thing=", k.String())
}
