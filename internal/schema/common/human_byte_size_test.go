package common

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestHumanByteSizeSerialization(t *testing.T) {
	type TestStruct struct {
		Size HumanByteSize `json:"size" yaml:"size"`
	}
	var tests = []struct {
		input    string
		expected uint64
	}{
		{
			input:    "5b",
			expected: uint64(5),
		},
		{
			input:    "10kb",
			expected: uint64(10 * 1000),
		},
		{
			input:    "10kib",
			expected: uint64(10 * 1024),
		},
		{
			input:    "5mb",
			expected: uint64(5 * 1000 * 1000),
		},
		{
			input:    "5mib",
			expected: uint64(5 * 1024 * 1024),
		},
		{
			input:    "23gb",
			expected: uint64(23 * 1000 * 1000 * 1000),
		},
		{
			input:    "20gib",
			expected: uint64(20 * 1024 * 1024 * 1024),
		},
		{
			input:    "345tb",
			expected: uint64(345 * 1000 * 1000 * 1000 * 1000),
		},
		{
			input:    "300tib",
			expected: uint64(300 * 1024 * 1024 * 1024 * 1024),
		},
		{
			input:    "987pb",
			expected: uint64(987 * 1000 * 1000 * 1000 * 1000 * 1000),
		},
		{
			input:    "300pib",
			expected: uint64(300 * 1024 * 1024 * 1024 * 1024 * 1024),
		},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			t.Run("yaml", func(t *testing.T) {
				yamlVal := fmt.Sprintf("size: %s", test.input)
				var deserializedYaml TestStruct
				if err := yaml.Unmarshal([]byte(yamlVal), &deserializedYaml); err != nil {
					t.Fatalf("Failed to deserialize YAML HumanByteSize: %v", err)
				}

				require.Equal(t, int(test.expected), int(deserializedYaml.Size.uint64))

				backToYaml, err := yaml.Marshal(deserializedYaml)
				require.NoError(t, err)

				require.Equal(t, yamlVal, strings.TrimSpace(string(backToYaml)))

			})

			t.Run("json", func(t *testing.T) {
				jsonVal := fmt.Sprintf("{\"size\":\"%s\"}", test.input)
				var deserializedJson TestStruct
				if err := json.Unmarshal([]byte(jsonVal), &deserializedJson); err != nil {
					t.Fatalf("Failed to deserialize JSON HumanByteSize: %v", err)
				}

				require.Equal(t, test.expected, deserializedJson.Size.uint64)

				backToJson, err := json.Marshal(deserializedJson)
				require.NoError(t, err)

				require.Equal(t, jsonVal, string(backToJson))
			})
		})
	}
}
