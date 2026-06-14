package database

import (
	"context"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func createDependencyDEK(t *testing.T, ctx context.Context, db DB, keyID apid.ID) apid.ID {
	t.Helper()

	dekID := apid.New(apid.PrefixDataEncryptionKey)
	require.NoError(t, db.CreateDataEncryptionKey(ctx, &DataEncryptionKey{
		Id:              dekID,
		KeyId:           keyID,
		Provider:        "test",
		ProviderID:      string(dekID),
		ProviderVersion: "v1",
		ProtectedData: &sconfig.KeyVersionProtectedData{
			Type:        "test",
			WrappedData: "wrapped",
		},
	}))

	return dekID
}

func TestKey(t *testing.T) {
	t.Run("CRUD", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// Create
		ek := &Key{
			Id:        apid.New(apid.PrefixKey),
			Namespace: "root",
			State:     KeyStateActive,
			Labels:    Labels{"env": "test"},
		}

		err := db.CreateKey(ctx, ek)
		require.NoError(t, err)
		require.Equal(t, now, ek.CreatedAt)
		require.Equal(t, now, ek.UpdatedAt)

		// Get
		got, err := db.GetKey(ctx, ek.Id)
		require.NoError(t, err)
		require.Equal(t, ek.Id, got.Id)
		require.Equal(t, "root", got.Namespace)
		require.Equal(t, KeyStateActive, got.State)
		gotUser, _ := SplitUserAndApxyLabels(got.Labels)
		require.Equal(t, Labels{"env": "test"}, gotUser)
		require.Equal(t, string(ek.Id), got.Labels["apxy/key/-/id"])
		require.Equal(t, "root", got.Labels["apxy/key/-/ns"])

		// Update state via UpdateKey
		later := now.Add(time.Hour)
		ctx = apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(later)).Build()

		updated, err := db.UpdateKey(ctx, ek.Id, map[string]interface{}{
			"state": KeyStateDisabled,
		})
		require.NoError(t, err)
		require.True(t, later.Equal(updated.UpdatedAt), "UpdatedAt should match the clock time")
		require.Equal(t, KeyStateDisabled, updated.State)

		// Revert state for subsequent tests
		_, err = db.UpdateKey(ctx, ek.Id, map[string]interface{}{
			"state": KeyStateActive,
		})
		require.NoError(t, err)

		// Get not found
		_, err = db.GetKey(ctx, apid.New(apid.PrefixKey))
		require.ErrorIs(t, err, ErrNotFound)

		// Delete (soft)
		err = db.DeleteKey(ctx, ek.Id)
		require.NoError(t, err)

		_, err = db.GetKey(ctx, ek.Id)
		require.ErrorIs(t, err, ErrNotFound)

		// Delete not found
		err = db.DeleteKey(ctx, apid.New(apid.PrefixKey))
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("DeleteGlobalKeyRejected", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		err := db.DeleteKey(ctx, GlobalKeyID)
		require.ErrorIs(t, err, ErrProtected)

		// Verify the global key still exists
		ek, err := db.GetKey(ctx, GlobalKeyID)
		require.NoError(t, err)
		require.Equal(t, GlobalKeyID, ek.Id)
	})

	t.Run("SetState", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ek := &Key{
			Id:        apid.New(apid.PrefixKey),
			Namespace: "root",
			State:     KeyStateActive,
		}
		require.NoError(t, db.CreateKey(ctx, ek))

		err := db.SetKeyState(ctx, ek.Id, KeyStateDisabled)
		require.NoError(t, err)

		got, err := db.GetKey(ctx, ek.Id)
		require.NoError(t, err)
		require.Equal(t, KeyStateDisabled, got.State)
	})

	t.Run("Labels", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ek := &Key{
			Id:        apid.New(apid.PrefixKey),
			Namespace: "root",
			State:     KeyStateActive,
			Labels:    Labels{"env": "test"},
		}
		require.NoError(t, db.CreateKey(ctx, ek))

		// PutLabels
		later := now.Add(time.Hour)
		ctx = apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(later)).Build()
		updated, err := db.PutKeyLabels(ctx, ek.Id, map[string]string{"region": "us-east"})
		require.NoError(t, err)
		require.Equal(t, "test", updated.Labels["env"])
		require.Equal(t, "us-east", updated.Labels["region"])

		// UpdateLabels (full replace) — apxy/ system labels are preserved.
		updated, err = db.UpdateKeyLabels(ctx, ek.Id, map[string]string{"new-label": "value"})
		require.NoError(t, err)
		updatedUser, _ := SplitUserAndApxyLabels(updated.Labels)
		require.Equal(t, Labels{"new-label": "value"}, updatedUser)
		require.Equal(t, string(ek.Id), updated.Labels["apxy/key/-/id"])

		// DeleteLabels
		updated, err = db.DeleteKeyLabels(ctx, ek.Id, []string{"new-label"})
		require.NoError(t, err)
		updatedUser, _ = SplitUserAndApxyLabels(updated.Labels)
		require.Empty(t, updatedUser)
	})

	t.Run("Annotations", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ek := &Key{
			Id:          apid.New(apid.PrefixKey),
			Namespace:   "root",
			State:       KeyStateActive,
			Annotations: Annotations{"description": "test key"},
		}
		require.NoError(t, db.CreateKey(ctx, ek))

		// Verify annotations persisted on create
		got, err := db.GetKey(ctx, ek.Id)
		require.NoError(t, err)
		require.Equal(t, Annotations{"description": "test key"}, got.Annotations)

		// PutAnnotations - merges with existing
		later := now.Add(time.Hour)
		ctx = apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(later)).Build()
		updated, err := db.PutKeyAnnotations(ctx, ek.Id, map[string]string{"owner": "team-a"})
		require.NoError(t, err)
		require.Equal(t, "test key", updated.Annotations["description"])
		require.Equal(t, "team-a", updated.Annotations["owner"])

		// UpdateAnnotations (full replace)
		updated, err = db.UpdateKeyAnnotations(ctx, ek.Id, map[string]string{"new-annotation": "value"})
		require.NoError(t, err)
		require.Equal(t, Annotations{"new-annotation": "value"}, updated.Annotations)

		// DeleteAnnotations
		updated, err = db.DeleteKeyAnnotations(ctx, ek.Id, []string{"new-annotation"})
		require.NoError(t, err)
		require.Empty(t, updated.Annotations)

		// PutAnnotations on key with no annotations
		ek2 := &Key{
			Id:        apid.New(apid.PrefixKey),
			Namespace: "root",
			State:     KeyStateActive,
		}
		require.NoError(t, db.CreateKey(ctx, ek2))

		updated, err = db.PutKeyAnnotations(ctx, ek2.Id, map[string]string{"note": "hello"})
		require.NoError(t, err)
		require.Equal(t, "hello", updated.Annotations["note"])

		// Not found errors
		fakeId := apid.New(apid.PrefixKey)
		_, err = db.PutKeyAnnotations(ctx, fakeId, map[string]string{"k": "v"})
		require.ErrorIs(t, err, ErrNotFound)

		_, err = db.UpdateKeyAnnotations(ctx, fakeId, map[string]string{"k": "v"})
		require.ErrorIs(t, err, ErrNotFound)

		_, err = db.DeleteKeyAnnotations(ctx, fakeId, []string{"k"})
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("ListBuilder", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// Create several keys
		for i := 0; i < 5; i++ {
			ek := &Key{
				Id:        apid.New(apid.PrefixKey),
				Namespace: "root",
				State:     KeyStateActive,
			}
			if i == 3 {
				ek.State = KeyStateDisabled
			}
			require.NoError(t, db.CreateKey(ctx, ek))
		}

		// List all (5 created + 1 seeded key_global from migration)
		result := db.ListKeysBuilder().FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 6)

		// Filter by state (4 active created + 1 seeded key_global)
		result = db.ListKeysBuilder().ForState(KeyStateActive).FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 5)

		result = db.ListKeysBuilder().ForState(KeyStateDisabled).FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 1)

		// Limit
		result = db.ListKeysBuilder().Limit(2).FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 2)
		require.True(t, result.HasMore)

		// Namespace matcher (5 created + 1 seeded key_global, all in "root")
		result = db.ListKeysBuilder().ForNamespaceMatcher("root").FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 6)

		result = db.ListKeysBuilder().ForNamespaceMatcher("root.nonexistent").FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 0)
	})

	t.Run("EncryptedKeyData", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ef := encfield.EncryptedField{
			ID:   apid.New(apid.PrefixDataEncryptionKey),
			Data: "encrypted-data-here",
		}

		ek := &Key{
			Id:               apid.New(apid.PrefixKey),
			Namespace:        "root",
			State:            KeyStateActive,
			EncryptedKeyData: &ef,
		}
		require.NoError(t, db.CreateKey(ctx, ek))

		got, err := db.GetKey(ctx, ek.Id)
		require.NoError(t, err)
		require.NotNil(t, got.EncryptedKeyData)
		require.Equal(t, ef.ID, got.EncryptedKeyData.ID)
		require.Equal(t, ef.Data, got.EncryptedKeyData.Data)
	})

	t.Run("EnumerateInDependencyOrder/single_root", func(t *testing.T) {
		// The seeded key_global is the only key; it should appear at depth 0.
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		var levels []struct {
			ids   []apid.ID
			depth int
		}

		orphans, err := db.EnumerateKeysInDependencyOrder(ctx, func(keys []*Key, depth int) (pagination.KeepGoing, error) {
			var ids []apid.ID
			for _, k := range keys {
				ids = append(ids, k.Id)
			}
			levels = append(levels, struct {
				ids   []apid.ID
				depth int
			}{ids, depth})
			return pagination.Continue, nil
		})
		require.NoError(t, err)
		require.Empty(t, orphans)
		require.Len(t, levels, 1)
		require.Equal(t, 0, levels[0].depth)
		require.Len(t, levels[0].ids, 1)
		require.Equal(t, apid.ID("key_global"), levels[0].ids[0])
	})

	t.Run("EnumerateInDependencyOrder/three_levels", func(t *testing.T) {
		// Build a 3-level tree:
		//   key_global (root, seeded)
		//     ├── childA (depth 1)
		//     └── childB (depth 1)
		//           └── grandchild (depth 2)
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// Create a DEK owned by key_global so children can reference it.
		globalDEKID := createDependencyDEK(t, ctx, db, GlobalKeyID)

		// Create childA encrypted by key_global's DEK.
		childA := &Key{
			Id:        apid.New(apid.PrefixKey),
			Namespace: "root",
			State:     KeyStateActive,
			EncryptedKeyData: &encfield.EncryptedField{
				ID:   globalDEKID,
				Data: "childA-encrypted-data",
			},
		}
		require.NoError(t, db.CreateKey(ctx, childA))

		// Create childB encrypted by key_global's DEK.
		childB := &Key{
			Id:        apid.New(apid.PrefixKey),
			Namespace: "root",
			State:     KeyStateActive,
			EncryptedKeyData: &encfield.EncryptedField{
				ID:   globalDEKID,
				Data: "childB-encrypted-data",
			},
		}
		require.NoError(t, db.CreateKey(ctx, childB))

		// Create a DEK owned by childB so grandchild can reference it.
		childBDEKID := createDependencyDEK(t, ctx, db, childB.Id)

		// Create grandchild encrypted by childB's DEK.
		grandchild := &Key{
			Id:        apid.New(apid.PrefixKey),
			Namespace: "root",
			State:     KeyStateActive,
			EncryptedKeyData: &encfield.EncryptedField{
				ID:   childBDEKID,
				Data: "grandchild-encrypted-data",
			},
		}
		require.NoError(t, db.CreateKey(ctx, grandchild))

		// Enumerate and collect results
		type level struct {
			ids   map[apid.ID]bool
			depth int
		}
		var levels []level

		orphans, err := db.EnumerateKeysInDependencyOrder(ctx, func(keys []*Key, depth int) (pagination.KeepGoing, error) {
			ids := make(map[apid.ID]bool)
			for _, k := range keys {
				ids[k.Id] = true
			}
			levels = append(levels, level{ids, depth})
			return pagination.Continue, nil
		})
		require.NoError(t, err)
		require.Empty(t, orphans)

		require.Len(t, levels, 3)

		// Depth 0: root only
		require.Equal(t, 0, levels[0].depth)
		require.Len(t, levels[0].ids, 1)
		require.True(t, levels[0].ids[apid.ID("key_global")])

		// Depth 1: childA and childB
		require.Equal(t, 1, levels[1].depth)
		require.Len(t, levels[1].ids, 2)
		require.True(t, levels[1].ids[childA.Id])
		require.True(t, levels[1].ids[childB.Id])

		// Depth 2: grandchild
		require.Equal(t, 2, levels[2].depth)
		require.Len(t, levels[2].ids, 1)
		require.True(t, levels[2].ids[grandchild.Id])
	})

	t.Run("EnumerateInDependencyOrder/early_stop", func(t *testing.T) {
		// Verify that returning keepGoing=false halts after that level.
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// Create a child so there would be depth 1.
		globalDEKID := createDependencyDEK(t, ctx, db, GlobalKeyID)

		child := &Key{
			Id:        apid.New(apid.PrefixKey),
			Namespace: "root",
			State:     KeyStateActive,
			EncryptedKeyData: &encfield.EncryptedField{
				ID:   globalDEKID,
				Data: "child-data",
			},
		}
		require.NoError(t, db.CreateKey(ctx, child))

		callCount := 0
		_, err := db.EnumerateKeysInDependencyOrder(ctx, func(keys []*Key, depth int) (pagination.KeepGoing, error) {
			callCount++
			return pagination.Stop, nil // stop immediately after first level
		})
		require.NoError(t, err)
		require.Equal(t, 1, callCount)
	})

	t.Run("EnumerateInDependencyOrder/skips_deleted", func(t *testing.T) {
		// Deleted keys should not appear in the enumeration.
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		globalDEKID := createDependencyDEK(t, ctx, db, GlobalKeyID)

		child := &Key{
			Id:        apid.New(apid.PrefixKey),
			Namespace: "root",
			State:     KeyStateActive,
			EncryptedKeyData: &encfield.EncryptedField{
				ID:   globalDEKID,
				Data: "child-data",
			},
		}
		require.NoError(t, db.CreateKey(ctx, child))

		// Soft-delete the child
		require.NoError(t, db.DeleteKey(ctx, child.Id))

		var allIDs []apid.ID
		orphans, err := db.EnumerateKeysInDependencyOrder(ctx, func(keys []*Key, depth int) (pagination.KeepGoing, error) {
			for _, k := range keys {
				allIDs = append(allIDs, k.Id)
			}
			return pagination.Continue, nil
		})
		require.NoError(t, err)
		require.Empty(t, orphans)
		// Only the root should remain
		require.Equal(t, []apid.ID{"key_global"}, allIDs)
	})

	t.Run("EnumerateInDependencyOrder/orphaned_key_returned", func(t *testing.T) {
		// A key whose EncryptedKeyData references a non-existent DEK should be returned as an orphan.
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		orphan := &Key{
			Id:        apid.New(apid.PrefixKey),
			Namespace: "root",
			State:     KeyStateActive,
			EncryptedKeyData: &encfield.EncryptedField{
				ID:   apid.New(apid.PrefixDataEncryptionKey), // non-existent DEK
				Data: "orphan-data",
			},
		}
		require.NoError(t, db.CreateKey(ctx, orphan))

		var allIDs []apid.ID
		orphans, err := db.EnumerateKeysInDependencyOrder(ctx, func(keys []*Key, depth int) (pagination.KeepGoing, error) {
			for _, k := range keys {
				allIDs = append(allIDs, k.Id)
			}
			return pagination.Continue, nil
		})
		require.NoError(t, err)
		// Only root in the walk; orphan is not walked
		require.Equal(t, []apid.ID{"key_global"}, allIDs)
		// Orphan is returned separately
		require.Len(t, orphans, 1)
		require.Equal(t, orphan.Id, orphans[0].Id)
	})

	t.Run("DeleteCascadesVersions", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ek := &Key{
			Id:        apid.New(apid.PrefixKey),
			Namespace: "root",
			State:     KeyStateActive,
		}
		require.NoError(t, db.CreateKey(ctx, ek))

		// Create multiple versions for this key
		var versionIds []apid.ID
		for i := int64(1); i <= 3; i++ {
			ekv := &EncryptionKeyVersion{
				KeyId:           ek.Id,
				Provider:        "local",
				ProviderID:      "key",
				ProviderVersion: "v1",
				OrderedVersion:  i,
				IsCurrent:       i == 3,
			}
			require.NoError(t, db.CreateEncryptionKeyVersion(ctx, ekv))
			versionIds = append(versionIds, ekv.Id)
		}

		// Create a version for a different key to ensure it's not affected
		otherEk := &Key{
			Id:        apid.New(apid.PrefixKey),
			Namespace: "root",
			State:     KeyStateActive,
		}
		require.NoError(t, db.CreateKey(ctx, otherEk))

		otherEkv := &EncryptionKeyVersion{
			KeyId:           otherEk.Id,
			Provider:        "local",
			ProviderID:      "other-key",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		}
		require.NoError(t, db.CreateEncryptionKeyVersion(ctx, otherEkv))

		// Verify versions exist before delete
		versions, err := db.ListEncryptionKeyVersionsForKey(ctx, ek.Id)
		require.NoError(t, err)
		require.Len(t, versions, 3)

		// Delete the encryption key
		err = db.DeleteKey(ctx, ek.Id)
		require.NoError(t, err)

		// The key should be gone
		_, err = db.GetKey(ctx, ek.Id)
		require.ErrorIs(t, err, ErrNotFound)

		// All versions for the deleted key should be gone
		versions, err = db.ListEncryptionKeyVersionsForKey(ctx, ek.Id)
		require.NoError(t, err)
		require.Empty(t, versions)

		// Each version should individually be not found
		for _, vid := range versionIds {
			_, err = db.GetEncryptionKeyVersion(ctx, vid)
			require.ErrorIs(t, err, ErrNotFound)
		}

		// The other key's version should be unaffected
		otherVersions, err := db.ListEncryptionKeyVersionsForKey(ctx, otherEk.Id)
		require.NoError(t, err)
		require.Len(t, otherVersions, 1)
	})

	t.Run("DeleteCascadesNoVersions", func(t *testing.T) {
		// Deleting a key with no versions should succeed without error
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ek := &Key{
			Id:        apid.New(apid.PrefixKey),
			Namespace: "root",
			State:     KeyStateActive,
		}
		require.NoError(t, db.CreateKey(ctx, ek))

		err := db.DeleteKey(ctx, ek.Id)
		require.NoError(t, err)

		_, err = db.GetKey(ctx, ek.Id)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("Validation", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// Missing id
		ek := &Key{
			Namespace: "root",
			State:     KeyStateActive,
		}
		err := db.CreateKey(ctx, ek)
		require.Error(t, err)

		// Missing namespace
		ek = &Key{
			Id:    apid.New(apid.PrefixKey),
			State: KeyStateActive,
		}
		err = db.CreateKey(ctx, ek)
		require.Error(t, err)
	})
}
