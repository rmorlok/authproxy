package common

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"testing"
	"time"
)

func TestHumanDurationSerialization(t *testing.T) {
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

	t.Run("Test YAML Serialization", func(t *testing.T) {
		var deserialized TestStruct
		if err := yaml.Unmarshal([]byte("duration: 6m"), &deserialized); err != nil {
			t.Fatalf("Failed to deserialize YAML HumanDuration: %v", err)
		}

		require.Equal(t, time.Minute*6, deserialized.Duration.Duration)
	})
}
