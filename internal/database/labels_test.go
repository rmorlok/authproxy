package database

import (
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
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

		t.Run("apxy/ multi-segment keys", func(t *testing.T) {
			t.Run("valid", func(t *testing.T) {
				validKeys := []string{
					"apxy/cxr/-/id",
					"apxy/cxr/-/ns",
					"apxy/cxn/-/id",
					"apxy/ns/-/id",
					"apxy/cxr/type",
					"apxy/cxr/my-key",
					"apxy/cxr/my.key",
					"apxy/cxr/my_key",
					"apxy/ns/dog",
					"apxy/cxr/cxn/userkey",
				}
				for _, key := range validKeys {
					t.Run(key, func(t *testing.T) {
						require.NoError(t, ValidateLabelKey(key), "key %q should be valid", key)
					})
				}
			})

			t.Run("invalid", func(t *testing.T) {
				invalidKeys := []struct {
					key    string
					reason string
				}{
					{"apxy/", "no segments after apxy/"},
					{"apxy//id", "empty intermediate segment"},
					{"apxy/cxr//id", "empty intermediate segment in middle"},
					{"apxy/cxr/", "empty name"},
					{"apxy/cxr/-/", "empty name after sentinel"},
					{"apxy/cx@r/id", "invalid character in segment"},
					{"apxy/cxr/-/-bad", "name starts with hyphen"},
				}
				for _, tc := range invalidKeys {
					t.Run(tc.reason, func(t *testing.T) {
						err := ValidateLabelKey(tc.key)
						require.Error(t, err, "key %q should be invalid: %s", tc.key, tc.reason)
					})
				}
			})
		})
	})

	t.Run("ValidateUserLabelKey", func(t *testing.T) {
		t.Run("rejects apxy/ prefix", func(t *testing.T) {
			err := ValidateUserLabelKey("apxy/cxr/type")
			require.Error(t, err)
			require.Contains(t, err.Error(), "reserved")
		})

		t.Run("accepts non-apxy keys", func(t *testing.T) {
			require.NoError(t, ValidateUserLabelKey("my-key"))
			require.NoError(t, ValidateUserLabelKey("example.com/key"))
		})

		t.Run("still rejects invalid keys", func(t *testing.T) {
			require.Error(t, ValidateUserLabelKey("-bad"))
			require.Error(t, ValidateUserLabelKey(""))
		})
	})

	t.Run("ValidateUserLabels", func(t *testing.T) {
		t.Run("rejects map containing apxy/ key", func(t *testing.T) {
			err := ValidateUserLabels(map[string]string{
				"good":         "ok",
				"apxy/cxr/bad": "nope",
			})
			require.Error(t, err)
			require.Contains(t, err.Error(), "reserved")
		})

		t.Run("accepts pure user labels", func(t *testing.T) {
			require.NoError(t, ValidateUserLabels(map[string]string{
				"team": "platform",
				"env":  "prod",
			}))
		})
	})

	t.Run("ValidateUserLabelDeletionKeys", func(t *testing.T) {
		t.Run("rejects apxy/ keys", func(t *testing.T) {
			err := ValidateUserLabelDeletionKeys([]string{"team", "apxy/cxr/type"})
			require.Error(t, err)
			require.Contains(t, err.Error(), "reserved")
		})

		t.Run("accepts user keys", func(t *testing.T) {
			require.NoError(t, ValidateUserLabelDeletionKeys([]string{"team", "env"}))
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

func TestApidPrefixToLabelToken(t *testing.T) {
	require.Equal(t, "cxr", ApidPrefixToLabelToken(apid.PrefixConnectorVersion))
	require.Equal(t, "cxn", ApidPrefixToLabelToken(apid.PrefixConnection))
	require.Equal(t, "act", ApidPrefixToLabelToken(apid.PrefixActor))
	require.Equal(t, "ek", ApidPrefixToLabelToken(apid.PrefixEncryptionKey))
	require.Equal(t, "", ApidPrefixToLabelToken(apid.Prefix("")))
}

func TestBuildImplicitIdentifierLabels(t *testing.T) {
	t.Run("builds id and ns labels", func(t *testing.T) {
		id := apid.MustParse("cxn_test1234567890ab")
		labels := BuildImplicitIdentifierLabels(id, "root.foo.bar")
		require.Equal(t, Labels{
			"apxy/cxn/-/id": "cxn_test1234567890ab",
			"apxy/cxn/-/ns": "root.foo.bar",
		}, labels)
		// Result must validate under the system rules.
		require.NoError(t, labels.Validate())
	})

	t.Run("nil id returns nil", func(t *testing.T) {
		require.Nil(t, BuildImplicitIdentifierLabels(apid.Nil, "/foo"))
	})

	t.Run("uses correct rt token per resource type", func(t *testing.T) {
		labels := BuildImplicitIdentifierLabels(apid.MustParse("cxr_test1234567890ab"), "root")
		_, ok := labels["apxy/cxr/-/id"]
		require.True(t, ok)
		_, ok = labels["apxy/cxr/-/ns"]
		require.True(t, ok)
	})
}

func TestBuildCarriedLabels(t *testing.T) {
	t.Run("re-keys user labels under parent rt", func(t *testing.T) {
		parent := Labels{
			"type":    "google_drive",
			"variant": "shared",
		}
		out := BuildCarriedLabels("cxr", parent)
		require.Equal(t, Labels{
			"apxy/cxr/type":    "google_drive",
			"apxy/cxr/variant": "shared",
		}, out)
		require.NoError(t, out.Validate())
	})

	t.Run("forwards apxy/ keys as-is", func(t *testing.T) {
		parent := Labels{
			"pig":            "oink",
			"apxy/ns/dog":    "woof",
			"apxy/cxr/-/id":  "cxr_abc",
			"apxy/cxr/-/ns":  "/foo",
		}
		out := BuildCarriedLabels("cxr", parent)
		require.Equal(t, Labels{
			"apxy/cxr/pig":  "oink",
			"apxy/ns/dog":   "woof",
			"apxy/cxr/-/id": "cxr_abc",
			"apxy/cxr/-/ns": "/foo",
		}, out)
	})

	t.Run("empty parent labels returns nil", func(t *testing.T) {
		require.Nil(t, BuildCarriedLabels("cxr", nil))
		require.Nil(t, BuildCarriedLabels("cxr", Labels{}))
	})

	t.Run("empty parent rt returns nil", func(t *testing.T) {
		require.Nil(t, BuildCarriedLabels("", Labels{"type": "google_drive"}))
	})
}

func TestSplitAndMergeUserAndApxyLabels(t *testing.T) {
	t.Run("split partitions by prefix", func(t *testing.T) {
		all := Labels{
			"team":           "platform",
			"env":            "prod",
			"apxy/cxn/-/id":  "cxn_abc",
			"apxy/cxr/type":  "google_drive",
		}
		user, apxy := SplitUserAndApxyLabels(all)
		require.Equal(t, Labels{"team": "platform", "env": "prod"}, user)
		require.Equal(t, Labels{"apxy/cxn/-/id": "cxn_abc", "apxy/cxr/type": "google_drive"}, apxy)
	})

	t.Run("split returns nil for empty halves", func(t *testing.T) {
		user, apxy := SplitUserAndApxyLabels(Labels{"team": "platform"})
		require.Equal(t, Labels{"team": "platform"}, user)
		require.Nil(t, apxy)

		user, apxy = SplitUserAndApxyLabels(Labels{"apxy/cxn/-/id": "cxn_abc"})
		require.Nil(t, user)
		require.Equal(t, Labels{"apxy/cxn/-/id": "cxn_abc"}, apxy)
	})

	t.Run("merge round-trips", func(t *testing.T) {
		all := Labels{
			"team":           "platform",
			"apxy/cxn/-/id":  "cxn_abc",
		}
		user, apxy := SplitUserAndApxyLabels(all)
		merged := MergeApxyAndUserLabels(user, apxy)
		require.Equal(t, all, merged)
	})

	t.Run("merge returns nil for empty inputs", func(t *testing.T) {
		require.Nil(t, MergeApxyAndUserLabels(nil, nil))
		require.Nil(t, MergeApxyAndUserLabels(Labels{}, Labels{}))
	})
}

func TestMergeUpsertLabels(t *testing.T) {
	t.Run("user portion fully comes from caller, dropping stored user labels", func(t *testing.T) {
		caller := Labels{"team": "platform", "env": "staging"}
		existing := Labels{"team": "old-team", "owner": "alice"}

		merged := MergeUpsertLabels(caller, existing)

		require.Equal(t, "platform", merged["team"])
		require.Equal(t, "staging", merged["env"])
		_, hasOwner := merged["owner"]
		require.False(t, hasOwner, "stored user-only labels should not survive an upsert")
	})

	t.Run("stored apxy labels are preserved when caller does not pass them", func(t *testing.T) {
		caller := Labels{"team": "platform"}
		existing := Labels{
			"apxy/cxr/-/id": "cxr_keep",
			"apxy/cxr/-/ns": "root",
		}

		merged := MergeUpsertLabels(caller, existing)

		require.Equal(t, "cxr_keep", merged["apxy/cxr/-/id"])
		require.Equal(t, "root", merged["apxy/cxr/-/ns"])
		require.Equal(t, "platform", merged["team"])
	})

	t.Run("caller apxy labels override stored apxy labels for the same key", func(t *testing.T) {
		caller := Labels{
			"team":             "platform",
			"apxy/cxr/source":  "config",
		}
		existing := Labels{
			"apxy/cxr/source": "api",
			"apxy/cxr/-/id":   "cxr_keep",
		}

		merged := MergeUpsertLabels(caller, existing)

		require.Equal(t, "config", merged["apxy/cxr/source"], "caller's apxy value must win")
		require.Equal(t, "cxr_keep", merged["apxy/cxr/-/id"], "stored apxy keys not in caller stay intact")
	})

	t.Run("returns nil when both inputs are empty", func(t *testing.T) {
		require.Nil(t, MergeUpsertLabels(nil, nil))
		require.Nil(t, MergeUpsertLabels(Labels{}, Labels{}))
	})
}

func TestApxyLabelValueValidation(t *testing.T) {
	// A namespace path that exceeds the standard 63-char user-value cap.
	longPath := "root." + strings.Repeat("a", 60) + ".more"
	require.Greater(t, len(longPath), LabelValueMaxLength)

	t.Run("user-mode rejects long values", func(t *testing.T) {
		require.Error(t, ValidateLabelValue(longPath))
	})

	t.Run("apxy mode accepts long namespace path", func(t *testing.T) {
		require.NoError(t, ValidateApxyLabelValue(longPath))
	})

	t.Run("apxy mode rejects values exceeding apxy cap", func(t *testing.T) {
		// 254 chars: starts/ends alphanumeric so the regex is the only constraint.
		tooLong := "a" + strings.Repeat("b", 252) + "c"
		require.Equal(t, ApxyLabelValueMaxLength+1, len(tooLong))
		require.Error(t, ValidateApxyLabelValue(tooLong))
	})

	t.Run("ValidateLabels uses apxy cap for apxy keys only", func(t *testing.T) {
		// Long value under an apxy/ key is valid.
		require.NoError(t, ValidateLabels(map[string]string{
			"apxy/cxn/-/ns": longPath,
		}))
		// Same long value under a user key is rejected.
		require.Error(t, ValidateLabels(map[string]string{
			"team": longPath,
		}))
	})
}

func TestInjectSelfImplicitLabels(t *testing.T) {
	t.Run("adds id and ns labels", func(t *testing.T) {
		id := apid.MustParse("cxn_test1234567890ab")
		out := InjectSelfImplicitLabels(id, "root.foo", Labels{"team": "platform"})
		require.Equal(t, "cxn_test1234567890ab", out["apxy/cxn/-/id"])
		require.Equal(t, "root.foo", out["apxy/cxn/-/ns"])
		require.Equal(t, "platform", out["team"])
	})

	t.Run("nil input still produces implicit labels", func(t *testing.T) {
		id := apid.MustParse("cxn_test1234567890ab")
		out := InjectSelfImplicitLabels(id, "root", nil)
		require.Len(t, out, 2)
		require.Equal(t, "root", out["apxy/cxn/-/ns"])
	})

	t.Run("nil id is a no-op pass-through", func(t *testing.T) {
		out := InjectSelfImplicitLabels(apid.Nil, "root", Labels{"team": "platform"})
		require.Equal(t, Labels{"team": "platform"}, out)
	})
}

func TestInjectNamespaceSelfImplicitLabels(t *testing.T) {
	out := InjectNamespaceSelfImplicitLabels("root.foo.bar", Labels{"pig": "oink"})
	require.Equal(t, "root.foo.bar", out["apxy/ns/-/id"])
	require.Equal(t, "root.foo.bar", out["apxy/ns/-/ns"])
	require.Equal(t, "oink", out["pig"])
}

func TestApplyParentCarryForward(t *testing.T) {
	t.Run("merges user labels with parent carry-forward", func(t *testing.T) {
		parent := Labels{
			"type":          "google_drive",
			"apxy/ns/-/id":  "root",
			"apxy/ns/-/ns":  "root",
			"apxy/ns/dog":   "woof",
		}
		out := ApplyParentCarryForward(
			Labels{"subscription_level": "pro"},
			ParentCarryForward{Rt: "cxr", Labels: parent},
		)
		require.Equal(t, "pro", out["subscription_level"])
		require.Equal(t, "google_drive", out["apxy/cxr/type"])
		require.Equal(t, "root", out["apxy/ns/-/id"])
		require.Equal(t, "woof", out["apxy/ns/dog"])
	})

	t.Run("later parent overrides earlier on apxy/ collisions", func(t *testing.T) {
		// Two parents both forwarding apxy/ns/-/ns. The "more direct" parent
		// (listed last) wins.
		cv := Labels{"apxy/ns/-/id": "root", "apxy/ns/-/ns": "root"}
		ns := Labels{"apxy/ns/-/id": "root.foo", "apxy/ns/-/ns": "root.foo"}
		out := ApplyParentCarryForward(
			nil,
			ParentCarryForward{Rt: "cxr", Labels: cv},
			ParentCarryForward{Rt: "ns", Labels: ns},
		)
		require.Equal(t, "root.foo", out["apxy/ns/-/id"])
		require.Equal(t, "root.foo", out["apxy/ns/-/ns"])
	})

	t.Run("returns nil when nothing to apply", func(t *testing.T) {
		require.Nil(t, ApplyParentCarryForward(nil))
		require.Nil(t, ApplyParentCarryForward(nil, ParentCarryForward{Rt: "cxr", Labels: nil}))
	})

	t.Run("user labels survive when no parent labels", func(t *testing.T) {
		out := ApplyParentCarryForward(
			Labels{"team": "platform"},
			ParentCarryForward{Rt: "cxr", Labels: nil},
		)
		require.Equal(t, Labels{"team": "platform"}, out)
	})
}
