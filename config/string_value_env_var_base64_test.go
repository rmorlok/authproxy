package config

import (
	"encoding/base64"
	"github.com/rmorlok/authproxy/context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringValueEnvBase64Var_GetData(t *testing.T) {
	tests := []struct {
		name          string
		envVar        string
		envValue      string
		expectedValue string
		expectedError bool
	}{
		{
			name:          "valid base64 value",
			envVar:        "TEST_ENV",
			envValue:      base64.StdEncoding.EncodeToString([]byte("test data")),
			expectedValue: "test data",
			expectedError: false,
		},
		{
			name:          "empty environment variable",
			envVar:        "TEST_ENV",
			envValue:      "",
			expectedValue: "",
			expectedError: true,
		},
		{
			name:          "environment variable not set",
			envVar:        "UNSET_ENV",
			envValue:      "",
			expectedValue: "",
			expectedError: true,
		},
		{
			name:          "invalid base64 value",
			envVar:        "TEST_ENV",
			envValue:      "invalid_base64",
			expectedValue: "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.envVar, tt.envValue)
			} else {
				os.Unsetenv(tt.envVar)
			}
			defer os.Unsetenv(tt.envVar)

			kev := StringValueEnvVarBase64{EnvVar: tt.envVar}
			data, err := kev.GetValue(context.Background())

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedValue, data)
		})
	}
}

func TestStringValueEnvBase64Var_HasData(t *testing.T) {
	tests := []struct {
		name     string
		envVar   string
		envValue string
		expected bool
	}{
		{
			name:     "environment variable set with value",
			envVar:   "TEST_ENV",
			envValue: "some_value",
			expected: true,
		},
		{
			name:     "environment variable set with empty value",
			envVar:   "TEST_ENV",
			envValue: "",
			expected: false,
		},
		{
			name:     "environment variable not set",
			envVar:   "UNSET_ENV",
			envValue: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.envVar, tt.envValue)
			} else {
				os.Unsetenv(tt.envVar)
			}
			defer os.Unsetenv(tt.envVar)

			kev := StringValueEnvVarBase64{EnvVar: tt.envVar}
			assert.Equal(t, tt.expected, kev.HasValue(context.Background()))
		})
	}
}
