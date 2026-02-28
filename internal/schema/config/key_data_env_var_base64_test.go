package config

import (
	"context"
	"encoding/base64"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeyDataEnvBase64Var_GetCurrentVersion(t *testing.T) {
	tests := []struct {
		name          string
		envVar        string
		envValue      string
		expectedValue []byte
		expectedError bool
	}{
		{
			name:          "valid base64 value",
			envVar:        "TEST_ENV",
			envValue:      base64.StdEncoding.EncodeToString([]byte("test data")),
			expectedValue: []byte("test data"),
			expectedError: false,
		},
		{
			name:          "empty environment variable",
			envVar:        "TEST_ENV",
			envValue:      "",
			expectedValue: nil,
			expectedError: true,
		},
		{
			name:          "environment variable not set",
			envVar:        "UNSET_ENV",
			envValue:      "",
			expectedValue: nil,
			expectedError: true,
		},
		{
			name:          "invalid base64 value",
			envVar:        "TEST_ENV",
			envValue:      "invalid_base64",
			expectedValue: nil,
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

			kev := KeyDataEnvBase64Var{EnvVar: tt.envVar}
			ver, err := kev.GetCurrentVersion(context.Background())

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedValue, ver.Data)
				assert.Equal(t, ProviderTypeEnvVarBase64, ver.Provider)
				assert.Equal(t, tt.envVar, ver.ProviderID)
				assert.True(t, ver.IsCurrent)
			}
		})
	}
}

func TestKeyDataEnvBase64Var_GetCurrentVersion_HasData(t *testing.T) {
	tests := []struct {
		name        string
		envVar      string
		envValue    string
		expectError bool
	}{
		{
			name:        "environment variable set with value",
			envVar:      "TEST_ENV",
			envValue:    "some_value",
			expectError: true, // "some_value" is not valid base64, but has data
		},
		{
			name:        "environment variable set with empty value",
			envVar:      "TEST_ENV",
			envValue:    "",
			expectError: true,
		},
		{
			name:        "environment variable not set",
			envVar:      "UNSET_ENV",
			envValue:    "",
			expectError: true,
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

			kev := KeyDataEnvBase64Var{EnvVar: tt.envVar}
			_, err := kev.GetCurrentVersion(context.Background())
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
