package connectors

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestProbe_EffectiveThresholds_DefaultsWhenUnset(t *testing.T) {
	p := &Probe{Id: "ping"}
	require.Equal(t, DefaultProbeFailureThreshold, p.EffectiveFailureThreshold())
	require.Equal(t, DefaultProbeRecoveryThreshold, p.EffectiveRecoveryThreshold())
}

func TestProbe_EffectiveThresholds_UseExplicitValuesWhenSet(t *testing.T) {
	p := &Probe{
		Id:                "ping",
		FailureThreshold:  util.ToPtr(5),
		RecoveryThreshold: util.ToPtr(2),
	}
	require.Equal(t, 5, p.EffectiveFailureThreshold())
	require.Equal(t, 2, p.EffectiveRecoveryThreshold())
}

func TestProbe_EffectiveThresholds_NilReceiverReturnsDefaults(t *testing.T) {
	// Defensive: callers iterating Probes that find a nil entry shouldn't
	// crash. The methods short-circuit to defaults.
	var p *Probe
	require.Equal(t, DefaultProbeFailureThreshold, p.EffectiveFailureThreshold())
	require.Equal(t, DefaultProbeRecoveryThreshold, p.EffectiveRecoveryThreshold())
}

func TestProbe_Validate_AcceptsValidThresholds(t *testing.T) {
	cases := []struct {
		name string
		f, r *int
	}{
		{"both unset", nil, nil},
		{"failure set", util.ToPtr(3), nil},
		{"recovery set", nil, util.ToPtr(1)},
		{"both set, both 1", util.ToPtr(1), util.ToPtr(1)},
		{"both set, large", util.ToPtr(100), util.ToPtr(50)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Probe{
				Id:                "ping",
				FailureThreshold:  tc.f,
				RecoveryThreshold: tc.r,
				Http:              &ProbeHttp{Method: "GET", URL: "https://example.com"},
			}
			require.NoError(t, p.Validate(&common.ValidationContext{}))
		})
	}
}

func TestProbe_Validate_RejectsZero(t *testing.T) {
	t.Run("failure_threshold = 0", func(t *testing.T) {
		p := &Probe{
			Id:               "ping",
			FailureThreshold: util.ToPtr(0),
			Http:             &ProbeHttp{Method: "GET", URL: "https://example.com"},
		}
		err := p.Validate(&common.ValidationContext{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "failure_threshold")
	})

	t.Run("recovery_threshold = 0", func(t *testing.T) {
		p := &Probe{
			Id:                "ping",
			RecoveryThreshold: util.ToPtr(0),
			Http:              &ProbeHttp{Method: "GET", URL: "https://example.com"},
		}
		err := p.Validate(&common.ValidationContext{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "recovery_threshold")
	})
}

func TestProbe_Validate_RejectsNegative(t *testing.T) {
	p := &Probe{
		Id:                "ping",
		FailureThreshold:  util.ToPtr(-1),
		RecoveryThreshold: util.ToPtr(-5),
		Http:              &ProbeHttp{Method: "GET", URL: "https://example.com"},
	}
	err := p.Validate(&common.ValidationContext{})
	require.Error(t, err)
	// Both errors should surface.
	require.Contains(t, err.Error(), "failure_threshold")
	require.Contains(t, err.Error(), "recovery_threshold")
}

func TestProbe_YAMLRoundTrip_WithThresholds(t *testing.T) {
	original := &Probe{
		Id:                "ping",
		Cron:              util.ToPtr("*/5 * * * *"),
		FailureThreshold:  util.ToPtr(5),
		RecoveryThreshold: util.ToPtr(2),
		Http:              &ProbeHttp{Method: "GET", URL: "https://example.com/ping"},
	}

	t.Run("yaml", func(t *testing.T) {
		data, err := yaml.Marshal(original)
		require.NoError(t, err)
		assert.Contains(t, string(data), "failure_threshold: 5")
		assert.Contains(t, string(data), "recovery_threshold: 2")

		var back Probe
		require.NoError(t, yaml.Unmarshal(data, &back))
		require.Equal(t, original, &back)
	})

	t.Run("json", func(t *testing.T) {
		data, err := json.Marshal(original)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"failure_threshold":5`)
		assert.Contains(t, string(data), `"recovery_threshold":2`)

		var back Probe
		require.NoError(t, json.Unmarshal(data, &back))
		require.Equal(t, original, &back)
	})
}

func TestProbe_YAMLRoundTrip_OmitsUnsetThresholds(t *testing.T) {
	// omitempty: unset thresholds shouldn't appear in serialized output,
	// so YAML written by AuthProxy doesn't accidentally pin the system
	// default as an explicit value (which would survive a default change).
	p := &Probe{
		Id:   "ping",
		Cron: util.ToPtr("*/5 * * * *"),
		Http: &ProbeHttp{Method: "GET", URL: "https://example.com/ping"},
	}

	yamlData, err := yaml.Marshal(p)
	require.NoError(t, err)
	assert.NotContains(t, string(yamlData), "failure_threshold")
	assert.NotContains(t, string(yamlData), "recovery_threshold")

	jsonData, err := json.Marshal(p)
	require.NoError(t, err)
	assert.NotContains(t, string(jsonData), "failure_threshold")
	assert.NotContains(t, string(jsonData), "recovery_threshold")
}
