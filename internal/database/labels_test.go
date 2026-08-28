package database

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/require"
)

func TestLabels(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
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

		t.Run("delegates validation", func(t *testing.T) {
			labels := Labels{
				"invalid key": "invalid value",
			}
			err := labels.Validate()
			require.Error(t, err)
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
	require.Equal(t, "key", ApidPrefixToLabelToken(apid.PrefixKey))
	require.Equal(t, "", ApidPrefixToLabelToken(apid.Prefix("")))
}

func TestBuildImplicitResourceLabels(t *testing.T) {
	t.Run("builds id name and ns labels", func(t *testing.T) {
		id := apid.MustParse("cxn_test1234567890ab")
		labels := BuildImplicitResourceLabels(id, "billing", "root.foo.bar")
		require.Equal(t, Labels{
			"apxy/cxn/-/id":   "cxn_test1234567890ab",
			"apxy/cxn/-/name": "billing",
			"apxy/cxn/-/ns":   "root.foo.bar",
		}, labels)
		// Result must validate under the system rules.
		require.NoError(t, labels.Validate())
	})

	t.Run("nil id returns nil", func(t *testing.T) {
		require.Nil(t, BuildImplicitResourceLabels(apid.Nil, "billing", "/foo"))
	})

	t.Run("empty name defaults to id", func(t *testing.T) {
		id := apid.MustParse("act_test1234567890ab")
		labels := BuildImplicitResourceLabels(id, "", "root")
		require.Equal(t, id.String(), labels["apxy/act/-/name"])
	})

	t.Run("uses correct rt token per resource type", func(t *testing.T) {
		labels := BuildImplicitResourceLabels(apid.MustParse("cxr_test1234567890ab"), "connector", "root")
		_, ok := labels["apxy/cxr/-/id"]
		require.True(t, ok)
		_, ok = labels["apxy/cxr/-/name"]
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
			"pig":           "oink",
			"apxy/ns/dog":   "woof",
			"apxy/cxr/-/id": "cxr_abc",
			"apxy/cxr/-/ns": "/foo",
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
			"team":          "platform",
			"env":           "prod",
			"apxy/cxn/-/id": "cxn_abc",
			"apxy/cxr/type": "google_drive",
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
			"team":          "platform",
			"apxy/cxn/-/id": "cxn_abc",
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
			"team":            "platform",
			"apxy/cxr/source": "config",
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

func TestInjectSelfImplicitLabels(t *testing.T) {
	t.Run("adds id name and ns labels", func(t *testing.T) {
		id := apid.MustParse("cxn_test1234567890ab")
		out := InjectSelfImplicitLabels(id, "billing", "root.foo", Labels{"team": "platform"})
		require.Equal(t, "cxn_test1234567890ab", out["apxy/cxn/-/id"])
		require.Equal(t, "billing", out["apxy/cxn/-/name"])
		require.Equal(t, "root.foo", out["apxy/cxn/-/ns"])
		require.Equal(t, "platform", out["team"])
	})

	t.Run("nil input still produces implicit labels", func(t *testing.T) {
		id := apid.MustParse("cxn_test1234567890ab")
		out := InjectSelfImplicitLabels(id, "billing", "root", nil)
		require.Len(t, out, 3)
		require.Equal(t, "root", out["apxy/cxn/-/ns"])
	})

	t.Run("nil id is a no-op pass-through", func(t *testing.T) {
		out := InjectSelfImplicitLabels(apid.Nil, "billing", "root", Labels{"team": "platform"})
		require.Equal(t, Labels{"team": "platform"}, out)
	})
}

func TestInjectNamespaceSelfImplicitLabels(t *testing.T) {
	out := InjectNamespaceSelfImplicitLabels("root.foo.bar", Labels{"pig": "oink"})
	require.Equal(t, "root.foo.bar", out["apxy/ns/-/id"])
	require.Equal(t, "bar", out["apxy/ns/-/name"])
	require.Equal(t, "root.foo.bar", out["apxy/ns/-/ns"])
	require.Equal(t, "oink", out["pig"])
}

func TestApplyParentCarryForward(t *testing.T) {
	t.Run("merges user labels with parent carry-forward", func(t *testing.T) {
		parent := Labels{
			"type":         "google_drive",
			"apxy/ns/-/id": "root",
			"apxy/ns/-/ns": "root",
			"apxy/ns/dog":  "woof",
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
