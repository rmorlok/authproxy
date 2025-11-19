package common

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestHumanDuration(t *testing.T) {
	type TestStruct struct {
		Duration HumanDuration `json:"duration" yaml:"duration"`
	}

	// Example test duration
	duration := TestStruct{
		Duration: HumanDuration{Duration: 5 * time.Minute},
	}

	t.Run("Test JSON Serialization", func(t *testing.T) {
		// Serialize to JSON
		jsonData, err := json.Marshal(duration)
		if err != nil {
			t.Fatalf("Failed to serialize HumanDuration to JSON: %v", err)
		}

		// Deserialize JSON to verify
		var deserialized TestStruct
		if err := json.Unmarshal(jsonData, &deserialized); err != nil {
			t.Fatalf("Failed to deserialize JSON back to HumanDuration: %v", err)
		}

		// Verify the duration
		require.Equal(t, duration.Duration, deserialized.Duration)
	})

	t.Run("Test YAML Serialization", func(t *testing.T) {
		// Serialize to YAML
		yamlData, err := yaml.Marshal(duration)
		if err != nil {
			t.Fatalf("Failed to serialize HumanDuration to YAML: %v", err)
		}

		// Deserialize YAML to verify
		var deserialized TestStruct
		if err := yaml.Unmarshal(yamlData, &deserialized); err != nil {
			t.Fatalf("Failed to deserialize YAML back to HumanDuration: %v", err)
		}

		// Verify the duration
		require.Equal(t, duration.Duration, deserialized.Duration)
	})

	t.Run("Test YAML Deserialization", func(t *testing.T) {
		var deserialized TestStruct
		if err := yaml.Unmarshal([]byte("duration: 6m"), &deserialized); err != nil {
			t.Fatalf("Failed to deserialize YAML HumanDuration: %v", err)
		}

		require.Equal(t, time.Minute*6, deserialized.Duration.Duration)
	})

	t.Run("Test Parse", func(t *testing.T) {
		tests := []struct {
			input     string
			expected  time.Duration
			expectErr bool
		}{
			{
				input:    "1ms",
				expected: time.Millisecond,
			},
			{
				input:    "1s",
				expected: time.Second,
			},
			{
				input:    "1m",
				expected: time.Minute,
			},
			{
				input:    "1h",
				expected: time.Hour,
			},
			{
				input:    "1d",
				expected: 24 * time.Hour,
			},
			{
				input:    "1h30m",
				expected: time.Hour + 30*time.Minute,
			},
			{
				input:    "1d2h30m",
				expected: 26*time.Hour + 30*time.Minute,
			},
			{
				input:     "bad",
				expectErr: true,
			},
			{
				input:     "2d1d",
				expectErr: true,
			},
			{
				input:     "2s1m",
				expectErr: true,
			},
		}
		for _, test := range tests {
			dur, err := parseHumanDuration(test.input)
			if test.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expected, dur, "Failed to parse duration %s", test.input)
			}
		}
	})
}
