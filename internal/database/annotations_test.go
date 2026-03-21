package database

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnnotations(t *testing.T) {
	t.Run("ValidateAnnotationKey", func(t *testing.T) {
		t.Run("valid keys", func(t *testing.T) {
			validKeys := []string{
				"a",
				"key",
				"my-key",
				"my_key",
				"my.key",
				"example.com/my-key",
				"app.kubernetes.io/name",
			}

			for _, key := range validKeys {
				t.Run(key, func(t *testing.T) {
					err := ValidateAnnotationKey(key)
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
				{"my key", "contains space"},
				{"/name", "empty prefix"},
				{strings.Repeat("a", 64), "name too long"},
			}

			for _, tc := range invalidKeys {
				t.Run(tc.reason, func(t *testing.T) {
					err := ValidateAnnotationKey(tc.key)
					require.Error(t, err, "key %q should be invalid: %s", tc.key, tc.reason)
				})
			}
		})
	})

	t.Run("ValidateAnnotationValue", func(t *testing.T) {
		// Annotation values have no format restriction
		t.Run("any string is valid", func(t *testing.T) {
			validValues := []string{
				"",
				"simple",
				"has spaces",
				"has-special@chars#!",
				strings.Repeat("a", 1000), // long values are fine
				"multi\nline\nvalue",
			}
			for _, v := range validValues {
				require.NoError(t, ValidateAnnotationValue(v))
			}
		})
	})

	t.Run("ValidateAnnotations", func(t *testing.T) {
		t.Run("valid annotations", func(t *testing.T) {
			annotations := Annotations{
				"app":                    "my application description",
				"example.com/note":       "this can be any string value with spaces & symbols!",
				"example.com/config":     `{"key": "value"}`,
				"example.com/empty":      "",
			}
			require.NoError(t, ValidateAnnotations(annotations))
		})

		t.Run("invalid key", func(t *testing.T) {
			annotations := Annotations{
				"-bad-key": "value",
			}
			require.Error(t, ValidateAnnotations(annotations))
		})

		t.Run("total size exceeds limit", func(t *testing.T) {
			annotations := Annotations{
				"key": strings.Repeat("x", AnnotationsTotalMaxSize),
			}
			err := ValidateAnnotations(annotations)
			require.Error(t, err)
			require.Contains(t, err.Error(), "exceeds maximum")
		})
	})

	t.Run("Annotations.Validate", func(t *testing.T) {
		t.Run("valid annotations", func(t *testing.T) {
			annotations := Annotations{
				"app":  "my app",
				"note": "some note with special chars!",
			}
			require.NoError(t, annotations.Validate())
		})

		t.Run("nil annotations", func(t *testing.T) {
			var annotations Annotations
			require.NoError(t, annotations.Validate())
		})

		t.Run("empty annotations", func(t *testing.T) {
			annotations := Annotations{}
			require.NoError(t, annotations.Validate())
		})

		t.Run("invalid key", func(t *testing.T) {
			annotations := Annotations{
				"valid-key":   "value",
				"invalid key": "value",
			}
			err := annotations.Validate()
			require.Error(t, err)
		})
	})

	t.Run("Annotations.Value and Scan (serialization)", func(t *testing.T) {
		t.Run("non-empty annotations", func(t *testing.T) {
			original := Annotations{
				"app":  "my app",
				"note": "some note",
			}

			value, err := original.Value()
			require.NoError(t, err)
			require.NotNil(t, value)

			var scanned Annotations
			err = scanned.Scan(value)
			require.NoError(t, err)
			require.Equal(t, original, scanned)
		})

		t.Run("empty annotations", func(t *testing.T) {
			original := Annotations{}

			value, err := original.Value()
			require.NoError(t, err)
			require.Nil(t, value)
		})

		t.Run("nil annotations", func(t *testing.T) {
			var original Annotations

			value, err := original.Value()
			require.NoError(t, err)
			require.Nil(t, value)
		})

		t.Run("scan nil", func(t *testing.T) {
			var annotations Annotations
			err := annotations.Scan(nil)
			require.NoError(t, err)
			require.Nil(t, annotations)
		})

		t.Run("scan string", func(t *testing.T) {
			var annotations Annotations
			err := annotations.Scan(`{"app":"my app"}`)
			require.NoError(t, err)
			require.Equal(t, Annotations{"app": "my app"}, annotations)
		})

		t.Run("scan bytes", func(t *testing.T) {
			var annotations Annotations
			err := annotations.Scan([]byte(`{"app":"my app"}`))
			require.NoError(t, err)
			require.Equal(t, Annotations{"app": "my app"}, annotations)
		})

		t.Run("scan empty string", func(t *testing.T) {
			var annotations Annotations
			err := annotations.Scan("")
			require.NoError(t, err)
			require.Nil(t, annotations)
		})

		t.Run("scan empty bytes", func(t *testing.T) {
			var annotations Annotations
			err := annotations.Scan([]byte{})
			require.NoError(t, err)
			require.Nil(t, annotations)
		})

		t.Run("scan invalid type", func(t *testing.T) {
			var annotations Annotations
			err := annotations.Scan(123)
			require.Error(t, err)
			require.Contains(t, err.Error(), "cannot convert")
		})
	})

	t.Run("Annotations.Get", func(t *testing.T) {
		annotations := Annotations{
			"app":  "my app",
			"note": "some note",
		}

		value, ok := annotations.Get("app")
		require.True(t, ok)
		require.Equal(t, "my app", value)

		value, ok = annotations.Get("nonexistent")
		require.False(t, ok)
		require.Empty(t, value)

		var nilAnnotations Annotations
		value, ok = nilAnnotations.Get("app")
		require.False(t, ok)
		require.Empty(t, value)
	})

	t.Run("Annotations.Has", func(t *testing.T) {
		annotations := Annotations{
			"app":   "my app",
			"empty": "",
		}

		require.True(t, annotations.Has("app"))
		require.True(t, annotations.Has("empty"))
		require.False(t, annotations.Has("nonexistent"))

		var nilAnnotations Annotations
		require.False(t, nilAnnotations.Has("app"))
	})

	t.Run("Annotations.Copy", func(t *testing.T) {
		original := Annotations{
			"app":  "my app",
			"note": "some note",
		}

		copied := original.Copy()
		require.Equal(t, original, copied)

		copied["app"] = "modified"
		require.Equal(t, "my app", original["app"])
		require.Equal(t, "modified", copied["app"])

		var nilAnnotations Annotations
		require.Nil(t, nilAnnotations.Copy())
	})
}
