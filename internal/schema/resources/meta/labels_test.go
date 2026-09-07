package meta

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLabelValidation(t *testing.T) {
	t.Run("label keys", func(t *testing.T) {
		validKeys := []string{
			"a",
			"A",
			"0",
			"my-key",
			"my_key",
			"my.key",
			"app.kubernetes.io/name",
			"a" + strings.Repeat("b", 61) + "c",
			"apxy/cxr/-/id",
			"apxy/cxn/-/ns",
			"apxy/cxr/cxn/userkey",
		}
		for _, key := range validKeys {
			t.Run("valid "+key, func(t *testing.T) {
				require.NoError(t, ValidateLabelKey(key))
			})
		}

		invalidKeys := []string{
			"",
			"-key",
			"key-",
			"_key",
			"key_",
			".key",
			"key.",
			"my key",
			"my@key",
			"/name",
			"example.com/",
			strings.Repeat("a", 64),
			strings.Repeat("a", 254) + "/name",
			"invalid..prefix/name",
			"-invalid.prefix/name",
			"apxy/",
			"apxy//id",
			"apxy/cxr//id",
			"apxy/cxr/-/",
			"apxy/cx@r/id",
			"apxy/cxr/-/-bad",
		}
		for _, key := range invalidKeys {
			t.Run("invalid "+key, func(t *testing.T) {
				require.Error(t, ValidateLabelKey(key))
			})
		}
	})

	t.Run("user label keys", func(t *testing.T) {
		require.NoError(t, ValidateUserLabelKey("my-key"))
		require.NoError(t, ValidateUserLabelKey("example.com/key"))
		require.Error(t, ValidateUserLabelKey("-bad"))
		require.Error(t, ValidateUserLabelKey(""))

		err := ValidateUserLabelKey("apxy/cxr/type")
		require.Error(t, err)
		require.ErrorContains(t, err, "reserved")
	})

	t.Run("user label maps", func(t *testing.T) {
		require.NoError(t, ValidateUserLabels(map[string]string{
			"team": "platform",
			"env":  "prod",
		}))

		err := ValidateUserLabels(map[string]string{
			"good":         "ok",
			"apxy/cxr/bad": "nope",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "reserved")

		require.NoError(t, ValidateUserLabelDeletionKeys([]string{"team", "env"}))
		err = ValidateUserLabelDeletionKeys([]string{"team", "apxy/cxr/type"})
		require.Error(t, err)
		require.ErrorContains(t, err, "reserved")
	})

	t.Run("label values", func(t *testing.T) {
		validValues := []string{
			"",
			"a",
			"A",
			"0",
			"my-value",
			"my_value",
			"v1.2.3",
			"a" + strings.Repeat("b", 61) + "c",
		}
		for _, value := range validValues {
			require.NoError(t, ValidateLabelValue(value), "value %q should be valid", value)
		}

		invalidValues := []string{
			"-value",
			"value-",
			"_value",
			"value_",
			".value",
			"value.",
			"my value",
			"my@value",
			strings.Repeat("a", 64),
		}
		for _, value := range invalidValues {
			require.Error(t, ValidateLabelValue(value), "value %q should be invalid", value)
		}
	})

	t.Run("system label values", func(t *testing.T) {
		longPath := "root." + strings.Repeat("a", 60) + ".more"
		require.Greater(t, len(longPath), LabelValueMaxLength)
		require.Error(t, ValidateLabelValue(longPath))
		require.NoError(t, ValidateSystemLabelValue(longPath))
		require.NoError(t, ValidateSystemLabelValue("_billing-"))
		require.Error(t, ValidateLabelValue("_billing-"))

		tooLong := "a" + strings.Repeat("b", 252) + "c"
		require.Equal(t, SystemLabelValueMaxLength+1, len(tooLong))
		require.Error(t, ValidateSystemLabelValue(tooLong))

		require.NoError(t, ValidateLabelValueForKey("apxy/cxn/-/ns", longPath))
		require.Error(t, ValidateLabelValueForKey("team", longPath))
	})

	t.Run("label maps", func(t *testing.T) {
		require.NoError(t, ValidateLabels(map[string]string{
			"app":                    "myapp",
			"app.kubernetes.io/name": "myapp",
			"empty-value":            "",
		}))
		require.Error(t, ValidateLabels(map[string]string{"app": "**bad**"}))
		require.Error(t, ValidateLabels(map[string]string{"-bad": "myapp"}))

		longPath := "root." + strings.Repeat("a", 60) + ".more"
		require.NoError(t, ValidateLabels(map[string]string{"apxy/cxn/-/ns": longPath}))
		require.Error(t, ValidateLabels(map[string]string{"team": longPath}))
	})
}

func TestAnnotationValidation(t *testing.T) {
	t.Run("annotation keys", func(t *testing.T) {
		for _, key := range []string{"a", "my-key", "my_key", "my.key", "example.com/my-key"} {
			require.NoError(t, ValidateAnnotationKey(key))
		}
		for _, key := range []string{"", "-key", "my key", "/name", strings.Repeat("a", 64)} {
			require.Error(t, ValidateAnnotationKey(key))
		}
	})

	t.Run("annotation values", func(t *testing.T) {
		for _, value := range []string{
			"",
			"simple",
			"has spaces",
			"has-special@chars#!",
			strings.Repeat("a", 1000),
			"multi\nline\nvalue",
		} {
			require.NoError(t, ValidateAnnotationValue(value))
		}
	})

	t.Run("annotation maps", func(t *testing.T) {
		require.NoError(t, ValidateAnnotations(map[string]string{
			"app":                "my application description",
			"example.com/config": `{"key": "value"}`,
		}))
		require.Error(t, ValidateAnnotations(map[string]string{"-bad-key": "value"}))

		err := ValidateAnnotations(map[string]string{
			"key": strings.Repeat("x", AnnotationsTotalMaxSize),
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "exceeds maximum")
	})
}
