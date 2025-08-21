package common

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"testing"
)

func TestHumanByteSizeSerialization(t *testing.T) {
	type TestStruct struct {
		Size HumanByteSize `json:"size" yaml:"size"`
	}

	// Example test duration
	size := TestStruct{
		Size: HumanByteSize{5 * 1024 * 1024},
	}

	t.Run("Test JSON Serialization", func(t *testing.T) {
		// Serialize to JSON
		jsonData, err := json.Marshal(size)
		if err != nil {
			t.Fatalf("Failed to serialize HumanByteSize to JSON: %v", err)
		}

		// Deserialize JSON to verify
		var deserialized TestStruct
		if err := json.Unmarshal(jsonData, &deserialized); err != nil {
			t.Fatalf("Failed to deserialize JSON back to HumanByteSize: %v", err)
		}

		// Verify the duration
		require.Equal(t, size.Size.uint64, deserialized.Size.uint64)
	})

	t.Run("Test YAML Serialization", func(t *testing.T) {
		// Serialize to YAML
		yamlData, err := yaml.Marshal(size)
		if err != nil {
			t.Fatalf("Failed to serialize HumanByteSize to YAML: %v", err)
		}

		// Deserialize YAML to verify
		var deserialized TestStruct
		if err := yaml.Unmarshal(yamlData, &deserialized); err != nil {
			t.Fatalf("Failed to deserialize YAML back to HumanByteSize: %v", err)
		}

		// Verify the duration
		require.Equal(t, size.Size.uint64, deserialized.Size.uint64)
	})

	t.Run("Test YAML Serialization", func(t *testing.T) {
		var deserialized TestStruct
		if err := yaml.Unmarshal([]byte("size: 10kb"), &deserialized); err != nil {
			t.Fatalf("Failed to deserialize YAML HumanByteSize: %v", err)
		}

		require.Equal(t, uint64(10*1024), deserialized.Size.uint64)
	})
}
