package database

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLabels(t *testing.T) {
	t.Run("ValidateLabelKey", func(t *testing.T) {
		t.Run("valid keys", func(t *testing.T) {
			validKeys := []string{
				"a",
				"A",
				"0",
				"key",
				"Key",
				"KEY",
				"my-key",
				"my_key",
				"my.key",
				"my-key.name",
				"a1",
				"1a",
				"a-1",
				"a_1",
				"a.1",
				"app.kubernetes.io",
				"example.com/my-key",
				"app.kubernetes.io/name",
				"my-company.com/component",
				"a" + strings.Repeat("b", 61) + "c", // exactly 63 chars
			}

			for _, key := range validKeys {
				t.Run(key, func(t *testing.T) {
					err := ValidateLabelKey(key)
					require.NoError(t, err, "key %q should be valid", key)
				})
			}
		})

		t.Run("invalid keys", func(t *testing.T) {
			invalidKeys := []struct {
				key    string
				reason string
			}{
				{"", "empty key"},
				{"-key", "starts with hyphen"},
				{"key-", "ends with hyphen"},
				{"_key", "starts with underscore"},
				{"key_", "ends with underscore"},
				{".key", "starts with dot"},
				{"key.", "ends with dot"},
				{"my key", "contains space"},
				{"my@key", "contains invalid character"},
				{"my#key", "contains invalid character"},
				{"/name", "empty prefix"},
				{"example.com/", "empty name after prefix"},
				{strings.Repeat("a", 64), "name too long (64 chars)"},
				{strings.Repeat("a", 254) + "/name", "prefix too long"},
				{"invalid..prefix/name", "double dot in prefix"},
				{"-invalid.prefix/name", "prefix starts with hyphen"},
			}

			for _, tc := range invalidKeys {
				t.Run(tc.reason, func(t *testing.T) {
					err := ValidateLabelKey(tc.key)
					require.Error(t, err, "key %q should be invalid: %s", tc.key, tc.reason)
				})
			}
		})
	})

	t.Run("ValidateLabelValue", func(t *testing.T) {
		t.Run("valid values", func(t *testing.T) {
			validValues := []string{
				"", // empty is valid
				"a",
				"A",
				"0",
				"value",
				"Value",
				"VALUE",
				"my-value",
				"my_value",
				"my.value",
				"v1.2.3",
				"a1",
				"1a",
				"a" + strings.Repeat("b", 61) + "c", // exactly 63 chars
			}

			for _, value := range validValues {
				t.Run("value_"+value, func(t *testing.T) {
					err := ValidateLabelValue(value)
					require.NoError(t, err, "value %q should be valid", value)
				})
			}
		})

		t.Run("invalid values", func(t *testing.T) {
			invalidValues := []struct {
				value  string
				reason string
			}{
				{"-value", "starts with hyphen"},
				{"value-", "ends with hyphen"},
				{"_value", "starts with underscore"},
				{"value_", "ends with underscore"},
				{".value", "starts with dot"},
				{"value.", "ends with dot"},
				{"my value", "contains space"},
				{"my@value", "contains invalid character"},
				{strings.Repeat("a", 64), "value too long (64 chars)"},
			}

			for _, tc := range invalidValues {
				t.Run(tc.reason, func(t *testing.T) {
					err := ValidateLabelValue(tc.value)
					require.Error(t, err, "value %q should be invalid: %s", tc.value, tc.reason)
				})
			}
		})
	})

	t.Run("ValidateLabels", func(t *testing.T) {
		t.Run("valid labels", func(t *testing.T) {
			labels := Labels{
				"app":                      "myapp",
				"version":                  "v1.2.3",
				"app.kubernetes.io/name":   "myapp",
				"example.com/my-component": "frontend",
				"empty-value":              "",
			}
			require.NoError(t, ValidateLabels(labels))
		})
		t.Run("invalid value", func(t *testing.T) {
			labels := Labels{
				"app":                      "**bad**",
				"version":                  "v1.2.3",
				"app.kubernetes.io/name":   "myapp",
				"example.com/my-component": "frontend",
				"empty-value":              "",
			}
			require.Error(t, ValidateLabels(labels))
		})
		t.Run("invalid key", func(t *testing.T) {
			labels := Labels{
				"-bad":                     "myapp",
				"version":                  "v1.2.3",
				"app.kubernetes.io/name":   "myapp",
				"example.com/my-component": "frontend",
				"empty-value":              "",
			}
			require.Error(t, ValidateLabels(labels))
		})
	})

	t.Run("Labels.Validate", func(t *testing.T) {
		t.Run("valid labels", func(t *testing.T) {
			labels := Labels{
				"app":                      "myapp",
				"version":                  "v1.2.3",
				"app.kubernetes.io/name":   "myapp",
				"example.com/my-component": "frontend",
				"empty-value":              "",
			}
			require.NoError(t, labels.Validate())
		})

		t.Run("nil labels", func(t *testing.T) {
			var labels Labels
			require.NoError(t, labels.Validate())
		})

		t.Run("empty labels", func(t *testing.T) {
			labels := Labels{}
			require.NoError(t, labels.Validate())
		})

		t.Run("invalid key", func(t *testing.T) {
			labels := Labels{
				"valid-key":   "value",
				"invalid key": "value",
			}
			err := labels.Validate()
			require.Error(t, err)
			require.Contains(t, err.Error(), "invalid key")
		})

		t.Run("invalid value", func(t *testing.T) {
			labels := Labels{
				"valid-key": "invalid value",
			}
			err := labels.Validate()
			require.Error(t, err)
			require.Contains(t, err.Error(), "invalid label value")
		})

		t.Run("multiple errors", func(t *testing.T) {
			labels := Labels{
				"invalid key": "invalid value",
			}
			err := labels.Validate()
			require.Error(t, err)
			// Should have errors for both key and value
			require.Contains(t, err.Error(), "invalid label key")
			require.Contains(t, err.Error(), "invalid label value")
		})
	})

	t.Run("Labels.Value and Scan (serialization)", func(t *testing.T) {
		t.Run("non-empty labels", func(t *testing.T) {
			original := Labels{
				"app":     "myapp",
				"version": "v1.2.3",
			}

			// Serialize
			value, err := original.Value()
			require.NoError(t, err)
			require.NotNil(t, value)

			// Deserialize
			var scanned Labels
			err = scanned.Scan(value)
			require.NoError(t, err)
			require.Equal(t, original, scanned)
		})

		t.Run("empty labels", func(t *testing.T) {
			original := Labels{}

			// Serialize - empty should return nil
			value, err := original.Value()
			require.NoError(t, err)
			require.Nil(t, value)
		})

		t.Run("nil labels", func(t *testing.T) {
			var original Labels

			// Serialize - nil should return nil
			value, err := original.Value()
			require.NoError(t, err)
			require.Nil(t, value)
		})

		t.Run("scan nil", func(t *testing.T) {
			var labels Labels
			err := labels.Scan(nil)
			require.NoError(t, err)
			require.Nil(t, labels)
		})

		t.Run("scan string", func(t *testing.T) {
			var labels Labels
			err := labels.Scan(`{"app":"myapp"}`)
			require.NoError(t, err)
			require.Equal(t, Labels{"app": "myapp"}, labels)
		})

		t.Run("scan bytes", func(t *testing.T) {
			var labels Labels
			err := labels.Scan([]byte(`{"app":"myapp"}`))
			require.NoError(t, err)
			require.Equal(t, Labels{"app": "myapp"}, labels)
		})

		t.Run("scan empty string", func(t *testing.T) {
			var labels Labels
			err := labels.Scan("")
			require.NoError(t, err)
			require.Nil(t, labels)
		})

		t.Run("scan empty bytes", func(t *testing.T) {
			var labels Labels
			err := labels.Scan([]byte{})
			require.NoError(t, err)
			require.Nil(t, labels)
		})

		t.Run("scan invalid type", func(t *testing.T) {
			var labels Labels
			err := labels.Scan(123)
			require.Error(t, err)
			require.Contains(t, err.Error(), "cannot convert")
		})
	})

	t.Run("Labels.Get", func(t *testing.T) {
		labels := Labels{
			"app":     "myapp",
			"version": "v1.2.3",
		}

		value, ok := labels.Get("app")
		require.True(t, ok)
		require.Equal(t, "myapp", value)

		value, ok = labels.Get("nonexistent")
		require.False(t, ok)
		require.Empty(t, value)

		// Test nil labels
		var nilLabels Labels
		value, ok = nilLabels.Get("app")
		require.False(t, ok)
		require.Empty(t, value)
	})

	t.Run("Labels.Has", func(t *testing.T) {
		labels := Labels{
			"app":   "myapp",
			"empty": "",
		}

		require.True(t, labels.Has("app"))
		require.True(t, labels.Has("empty")) // has key even with empty value
		require.False(t, labels.Has("nonexistent"))

		// Test nil labels
		var nilLabels Labels
		require.False(t, nilLabels.Has("app"))
	})

	t.Run("Labels.Copy", func(t *testing.T) {
		original := Labels{
			"app":     "myapp",
			"version": "v1.2.3",
		}

		copied := original.Copy()
		require.Equal(t, original, copied)

		// Modify the copy and verify original is unchanged
		copied["app"] = "modified"
		require.Equal(t, "myapp", original["app"])
		require.Equal(t, "modified", copied["app"])

		// Test nil labels
		var nilLabels Labels
		require.Nil(t, nilLabels.Copy())
	})
}
