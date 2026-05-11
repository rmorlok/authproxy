package request_log

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/require"
)

// --- bucket ---

func TestMarshalRateLimitBucket_NilProducesEmptyObject(t *testing.T) {
	out, err := marshalRateLimitBucket(nil)
	require.NoError(t, err)
	require.Equal(t, "{}", out, "nil bucket must serialize to valid JSON so the DB column NOT-NULL invariant holds")
}

func TestMarshalRateLimitBucket_EmptyProducesEmptyObject(t *testing.T) {
	out, err := marshalRateLimitBucket(map[string]string{})
	require.NoError(t, err)
	require.Equal(t, "{}", out)
}

func TestMarshalRateLimitBucket_PopulatedRoundTrips(t *testing.T) {
	in := map[string]string{"actor": "act_x", "labels/team": "alpha"}
	encoded, err := marshalRateLimitBucket(in)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	decoded, err := unmarshalRateLimitBucket([]byte(encoded))
	require.NoError(t, err)
	require.Equal(t, in, decoded)
}

func TestUnmarshalRateLimitBucket_NilOrEmptyYieldsNil(t *testing.T) {
	// nil/empty/{} all collapse to a nil map. Distinguishing "missing" from
	// "explicitly empty" is not useful for log readers — both render as no
	// bucket dimensions in the UI / API response.
	for _, tc := range []struct {
		name string
		in   []byte
	}{
		{"nil", nil},
		{"empty bytes", []byte("")},
		{"empty object", []byte("{}")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := unmarshalRateLimitBucket(tc.in)
			require.NoError(t, err)
			require.Nil(t, out)
		})
	}
}

func TestUnmarshalRateLimitBucket_BadJSONReturnsError(t *testing.T) {
	_, err := unmarshalRateLimitBucket([]byte("{not json"))
	require.Error(t, err)
}

// --- matched ---

func TestMarshalRateLimitMatched_NilProducesEmptyArray(t *testing.T) {
	out, err := marshalRateLimitMatched(nil)
	require.NoError(t, err)
	require.Equal(t, "[]", out)
}

func TestMarshalRateLimitMatched_EmptyProducesEmptyArray(t *testing.T) {
	out, err := marshalRateLimitMatched([]RateLimitMatch{})
	require.NoError(t, err)
	require.Equal(t, "[]", out)
}

func TestMarshalRateLimitMatched_PopulatedRoundTrips(t *testing.T) {
	in := []RateLimitMatch{
		{Id: apid.ID("rl_a"), Mode: "enforce", Bucket: map[string]string{"actor": "act_x"}},
		{Id: apid.ID("rl_b"), Mode: "observe", Bucket: map[string]string{"team": "alpha"}},
	}

	encoded, err := marshalRateLimitMatched(in)
	require.NoError(t, err)

	decoded, err := unmarshalRateLimitMatched([]byte(encoded))
	require.NoError(t, err)
	require.Equal(t, in, decoded)
}

func TestUnmarshalRateLimitMatched_NilOrEmptyYieldsNil(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []byte
	}{
		{"nil", nil},
		{"empty bytes", []byte("")},
		{"empty array", []byte("[]")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := unmarshalRateLimitMatched(tc.in)
			require.NoError(t, err)
			require.Nil(t, out)
		})
	}
}

func TestUnmarshalRateLimitMatched_BadJSONReturnsError(t *testing.T) {
	_, err := unmarshalRateLimitMatched([]byte("[not json"))
	require.Error(t, err)
}

// TestRateLimitMatchedJSON_PreservesOrder pins the property the firing-rule
// reporting depends on: when there are multiple matches, the order in the
// returned slice equals the order in which they were stored. Storing as a
// JSON array preserves order across encode/decode; this test is the
// regression guard.
func TestRateLimitMatchedJSON_PreservesOrder(t *testing.T) {
	in := []RateLimitMatch{
		{Id: apid.ID("rl_z"), Mode: "enforce"},
		{Id: apid.ID("rl_a"), Mode: "observe"},
		{Id: apid.ID("rl_m"), Mode: "observe"},
	}
	encoded, err := marshalRateLimitMatched(in)
	require.NoError(t, err)
	decoded, err := unmarshalRateLimitMatched([]byte(encoded))
	require.NoError(t, err)
	require.Equal(t, in, decoded)
}
