package database

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestEncryptionKey(t *testing.T) {
	t.Run("CRUD", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// Create
		ek := &EncryptionKey{
			Id:        apid.New(apid.PrefixEncryptionKey),
			Namespace: "root",
			State:     EncryptionKeyStateActive,
			Labels:    Labels{"env": "test"},
		}

		err := db.CreateEncryptionKey(ctx, ek)
		require.NoError(t, err)
		require.Equal(t, now, ek.CreatedAt)
		require.Equal(t, now, ek.UpdatedAt)

		// Get
		got, err := db.GetEncryptionKey(ctx, ek.Id)
		require.NoError(t, err)
		require.Equal(t, ek.Id, got.Id)
		require.Equal(t, "root", got.Namespace)
		require.Equal(t, EncryptionKeyStateActive, got.State)
		gotUser, _ := SplitUserAndApxyLabels(got.Labels)
		require.Equal(t, Labels{"env": "test"}, gotUser)
		require.Equal(t, string(ek.Id), got.Labels["apxy/ek/-/id"])
		require.Equal(t, "root", got.Labels["apxy/ek/-/ns"])

		// Update state via UpdateEncryptionKey
		later := now.Add(time.Hour)
		ctx = apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(later)).Build()

		updated, err := db.UpdateEncryptionKey(ctx, ek.Id, map[string]interface{}{
			"state": EncryptionKeyStateDisabled,
		})
		require.NoError(t, err)
		require.True(t, later.Equal(updated.UpdatedAt), "UpdatedAt should match the clock time")
		require.Equal(t, EncryptionKeyStateDisabled, updated.State)

		// Revert state for subsequent tests
		_, err = db.UpdateEncryptionKey(ctx, ek.Id, map[string]interface{}{
			"state": EncryptionKeyStateActive,
		})
		require.NoError(t, err)

		// Get not found
		_, err = db.GetEncryptionKey(ctx, apid.New(apid.PrefixEncryptionKey))
		require.ErrorIs(t, err, ErrNotFound)

		// Delete (soft)
		err = db.DeleteEncryptionKey(ctx, ek.Id)
		require.NoError(t, err)

		_, err = db.GetEncryptionKey(ctx, ek.Id)
		require.ErrorIs(t, err, ErrNotFound)

		// Delete not found
		err = db.DeleteEncryptionKey(ctx, apid.New(apid.PrefixEncryptionKey))
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("DeleteGlobalKeyRejected", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		err := db.DeleteEncryptionKey(ctx, GlobalEncryptionKeyID)
		require.ErrorIs(t, err, ErrProtected)

		// Verify the global key still exists
		ek, err := db.GetEncryptionKey(ctx, GlobalEncryptionKeyID)
		require.NoError(t, err)
		require.Equal(t, GlobalEncryptionKeyID, ek.Id)
	})

	t.Run("SetState", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ek := &EncryptionKey{
			Id:        apid.New(apid.PrefixEncryptionKey),
			Namespace: "root",
			State:     EncryptionKeyStateActive,
		}
		require.NoError(t, db.CreateEncryptionKey(ctx, ek))

		err := db.SetEncryptionKeyState(ctx, ek.Id, EncryptionKeyStateDisabled)
		require.NoError(t, err)

		got, err := db.GetEncryptionKey(ctx, ek.Id)
		require.NoError(t, err)
		require.Equal(t, EncryptionKeyStateDisabled, got.State)
	})

	t.Run("Labels", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ek := &EncryptionKey{
			Id:        apid.New(apid.PrefixEncryptionKey),
			Namespace: "root",
			State:     EncryptionKeyStateActive,
			Labels:    Labels{"env": "test"},
		}
		require.NoError(t, db.CreateEncryptionKey(ctx, ek))

		// PutLabels
		later := now.Add(time.Hour)
		ctx = apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(later)).Build()
		updated, err := db.PutEncryptionKeyLabels(ctx, ek.Id, map[string]string{"region": "us-east"})
		require.NoError(t, err)
		require.Equal(t, "test", updated.Labels["env"])
		require.Equal(t, "us-east", updated.Labels["region"])

		// UpdateLabels (full replace) — apxy/ system labels are preserved.
		updated, err = db.UpdateEncryptionKeyLabels(ctx, ek.Id, map[string]string{"new-label": "value"})
		require.NoError(t, err)
		updatedUser, _ := SplitUserAndApxyLabels(updated.Labels)
		require.Equal(t, Labels{"new-label": "value"}, updatedUser)
		require.Equal(t, string(ek.Id), updated.Labels["apxy/ek/-/id"])

		// DeleteLabels
		updated, err = db.DeleteEncryptionKeyLabels(ctx, ek.Id, []string{"new-label"})
		require.NoError(t, err)
		updatedUser, _ = SplitUserAndApxyLabels(updated.Labels)
		require.Empty(t, updatedUser)
	})

	t.Run("Annotations", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ek := &EncryptionKey{
			Id:          apid.New(apid.PrefixEncryptionKey),
			Namespace:   "root",
			State:       EncryptionKeyStateActive,
			Annotations: Annotations{"description": "test key"},
		}
		require.NoError(t, db.CreateEncryptionKey(ctx, ek))

		// Verify annotations persisted on create
		got, err := db.GetEncryptionKey(ctx, ek.Id)
		require.NoError(t, err)
		require.Equal(t, Annotations{"description": "test key"}, got.Annotations)

		// PutAnnotations - merges with existing
		later := now.Add(time.Hour)
		ctx = apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(later)).Build()
		updated, err := db.PutEncryptionKeyAnnotations(ctx, ek.Id, map[string]string{"owner": "team-a"})
		require.NoError(t, err)
		require.Equal(t, "test key", updated.Annotations["description"])
		require.Equal(t, "team-a", updated.Annotations["owner"])

		// UpdateAnnotations (full replace)
		updated, err = db.UpdateEncryptionKeyAnnotations(ctx, ek.Id, map[string]string{"new-annotation": "value"})
		require.NoError(t, err)
		require.Equal(t, Annotations{"new-annotation": "value"}, updated.Annotations)

		// DeleteAnnotations
		updated, err = db.DeleteEncryptionKeyAnnotations(ctx, ek.Id, []string{"new-annotation"})
		require.NoError(t, err)
		require.Empty(t, updated.Annotations)

		// PutAnnotations on key with no annotations
		ek2 := &EncryptionKey{
			Id:        apid.New(apid.PrefixEncryptionKey),
			Namespace: "root",
			State:     EncryptionKeyStateActive,
		}
		require.NoError(t, db.CreateEncryptionKey(ctx, ek2))

		updated, err = db.PutEncryptionKeyAnnotations(ctx, ek2.Id, map[string]string{"note": "hello"})
		require.NoError(t, err)
		require.Equal(t, "hello", updated.Annotations["note"])

		// Not found errors
		fakeId := apid.New(apid.PrefixEncryptionKey)
		_, err = db.PutEncryptionKeyAnnotations(ctx, fakeId, map[string]string{"k": "v"})
		require.ErrorIs(t, err, ErrNotFound)

		_, err = db.UpdateEncryptionKeyAnnotations(ctx, fakeId, map[string]string{"k": "v"})
		require.ErrorIs(t, err, ErrNotFound)

		_, err = db.DeleteEncryptionKeyAnnotations(ctx, fakeId, []string{"k"})
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("ListBuilder", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// Create several keys
		for i := 0; i < 5; i++ {
			ek := &EncryptionKey{
				Id:        apid.New(apid.PrefixEncryptionKey),
				Namespace: "root",
				State:     EncryptionKeyStateActive,
			}
			if i == 3 {
				ek.State = EncryptionKeyStateDisabled
			}
			require.NoError(t, db.CreateEncryptionKey(ctx, ek))
		}

		// List all (5 created + 1 seeded ek_global from migration)
		result := db.ListEncryptionKeysBuilder().FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 6)

		// Filter by state (4 active created + 1 seeded ek_global)
		result = db.ListEncryptionKeysBuilder().ForState(EncryptionKeyStateActive).FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 5)

		result = db.ListEncryptionKeysBuilder().ForState(EncryptionKeyStateDisabled).FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 1)

		// Limit
		result = db.ListEncryptionKeysBuilder().Limit(2).FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 2)
		require.True(t, result.HasMore)

		// Namespace matcher (5 created + 1 seeded ek_global, all in "root")
		result = db.ListEncryptionKeysBuilder().ForNamespaceMatcher("root").FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 6)

		result = db.ListEncryptionKeysBuilder().ForNamespaceMatcher("root.nonexistent").FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 0)
	})

	t.Run("EncryptedKeyData", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ef := encfield.EncryptedField{
			ID:   apid.New(apid.PrefixEncryptionKeyVersion),
			Data: "encrypted-data-here",
		}

		ek := &EncryptionKey{
			Id:               apid.New(apid.PrefixEncryptionKey),
			Namespace:        "root",
			State:            EncryptionKeyStateActive,
			EncryptedKeyData: &ef,
		}
		require.NoError(t, db.CreateEncryptionKey(ctx, ek))

		got, err := db.GetEncryptionKey(ctx, ek.Id)
		require.NoError(t, err)
		require.NotNil(t, got.EncryptedKeyData)
		require.Equal(t, ef.ID, got.EncryptedKeyData.ID)
		require.Equal(t, ef.Data, got.EncryptedKeyData.Data)
	})

	t.Run("EnumerateInDependencyOrder/single_root", func(t *testing.T) {
		// The seeded ek_global is the only key; it should appear at depth 0.
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		var levels []struct {
			ids   []apid.ID
			depth int
		}

		orphans, err := db.EnumerateEncryptionKeysInDependencyOrder(ctx, func(keys []*EncryptionKey, depth int) (pagination.KeepGoing, error) {
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
		require.Equal(t, apid.ID("ek_global"), levels[0].ids[0])
	})

	t.Run("EnumerateInDependencyOrder/three_levels", func(t *testing.T) {
		// Build a 3-level tree:
		//   ek_global (root, seeded)
		//     ├── childA (depth 1)
		//     └── childB (depth 1)
		//           └── grandchild (depth 2)
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// Create an encryption key version for ek_global so children can reference it
		ekvGlobal := &EncryptionKeyVersion{
			Id:              apid.New(apid.PrefixEncryptionKeyVersion),
			EncryptionKeyId: "ek_global",
			Provider:        "local",
			ProviderID:      "global-key",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		}
		require.NoError(t, db.CreateEncryptionKeyVersion(ctx, ekvGlobal))

		// Create childA encrypted by ek_global's version
		childA := &EncryptionKey{
			Id:        apid.New(apid.PrefixEncryptionKey),
			Namespace: "root",
			State:     EncryptionKeyStateActive,
			EncryptedKeyData: &encfield.EncryptedField{
				ID:   ekvGlobal.Id,
				Data: "childA-encrypted-data",
			},
		}
		require.NoError(t, db.CreateEncryptionKey(ctx, childA))

		// Create childB encrypted by ek_global's version
		childB := &EncryptionKey{
			Id:        apid.New(apid.PrefixEncryptionKey),
			Namespace: "root",
			State:     EncryptionKeyStateActive,
			EncryptedKeyData: &encfield.EncryptedField{
				ID:   ekvGlobal.Id,
				Data: "childB-encrypted-data",
			},
		}
		require.NoError(t, db.CreateEncryptionKey(ctx, childB))

		// Create an encryption key version for childB so grandchild can reference it
		ekvChildB := &EncryptionKeyVersion{
			Id:              apid.New(apid.PrefixEncryptionKeyVersion),
			EncryptionKeyId: childB.Id,
			Provider:        "local",
			ProviderID:      "childB-key",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		}
		require.NoError(t, db.CreateEncryptionKeyVersion(ctx, ekvChildB))

		// Create grandchild encrypted by childB's version
		grandchild := &EncryptionKey{
			Id:        apid.New(apid.PrefixEncryptionKey),
			Namespace: "root",
			State:     EncryptionKeyStateActive,
			EncryptedKeyData: &encfield.EncryptedField{
				ID:   ekvChildB.Id,
				Data: "grandchild-encrypted-data",
			},
		}
		require.NoError(t, db.CreateEncryptionKey(ctx, grandchild))

		// Enumerate and collect results
		type level struct {
			ids   map[apid.ID]bool
			depth int
		}
		var levels []level

		orphans, err := db.EnumerateEncryptionKeysInDependencyOrder(ctx, func(keys []*EncryptionKey, depth int) (pagination.KeepGoing, error) {
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
		require.True(t, levels[0].ids[apid.ID("ek_global")])

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

		// Create a child so there would be depth 1
		ekvGlobal := &EncryptionKeyVersion{
			Id:              apid.New(apid.PrefixEncryptionKeyVersion),
			EncryptionKeyId: "ek_global",
			Provider:        "local",
			ProviderID:      "global-key",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		}
		require.NoError(t, db.CreateEncryptionKeyVersion(ctx, ekvGlobal))

		child := &EncryptionKey{
			Id:        apid.New(apid.PrefixEncryptionKey),
			Namespace: "root",
			State:     EncryptionKeyStateActive,
			EncryptedKeyData: &encfield.EncryptedField{
				ID:   ekvGlobal.Id,
				Data: "child-data",
			},
		}
		require.NoError(t, db.CreateEncryptionKey(ctx, child))

		callCount := 0
		_, err := db.EnumerateEncryptionKeysInDependencyOrder(ctx, func(keys []*EncryptionKey, depth int) (pagination.KeepGoing, error) {
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

		ekvGlobal := &EncryptionKeyVersion{
			Id:              apid.New(apid.PrefixEncryptionKeyVersion),
			EncryptionKeyId: "ek_global",
			Provider:        "local",
			ProviderID:      "global-key",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		}
		require.NoError(t, db.CreateEncryptionKeyVersion(ctx, ekvGlobal))

		child := &EncryptionKey{
			Id:        apid.New(apid.PrefixEncryptionKey),
			Namespace: "root",
			State:     EncryptionKeyStateActive,
			EncryptedKeyData: &encfield.EncryptedField{
				ID:   ekvGlobal.Id,
				Data: "child-data",
			},
		}
		require.NoError(t, db.CreateEncryptionKey(ctx, child))

		// Soft-delete the child
		require.NoError(t, db.DeleteEncryptionKey(ctx, child.Id))

		var allIDs []apid.ID
		orphans, err := db.EnumerateEncryptionKeysInDependencyOrder(ctx, func(keys []*EncryptionKey, depth int) (pagination.KeepGoing, error) {
			for _, k := range keys {
				allIDs = append(allIDs, k.Id)
			}
			return pagination.Continue, nil
		})
		require.NoError(t, err)
		require.Empty(t, orphans)
		// Only the root should remain
		require.Equal(t, []apid.ID{"ek_global"}, allIDs)
	})

	t.Run("EnumerateInDependencyOrder/orphaned_key_returned", func(t *testing.T) {
		// A key whose EncryptedKeyData references a non-existent ekv should be returned as an orphan.
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		orphan := &EncryptionKey{
			Id:        apid.New(apid.PrefixEncryptionKey),
			Namespace: "root",
			State:     EncryptionKeyStateActive,
			EncryptedKeyData: &encfield.EncryptedField{
				ID:   apid.New(apid.PrefixEncryptionKeyVersion), // non-existent version
				Data: "orphan-data",
			},
		}
		require.NoError(t, db.CreateEncryptionKey(ctx, orphan))

		var allIDs []apid.ID
		orphans, err := db.EnumerateEncryptionKeysInDependencyOrder(ctx, func(keys []*EncryptionKey, depth int) (pagination.KeepGoing, error) {
			for _, k := range keys {
				allIDs = append(allIDs, k.Id)
			}
			return pagination.Continue, nil
		})
		require.NoError(t, err)
		// Only root in the walk; orphan is not walked
		require.Equal(t, []apid.ID{"ek_global"}, allIDs)
		// Orphan is returned separately
		require.Len(t, orphans, 1)
		require.Equal(t, orphan.Id, orphans[0].Id)
	})

	t.Run("DeleteCascadesVersions", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ek := &EncryptionKey{
			Id:        apid.New(apid.PrefixEncryptionKey),
			Namespace: "root",
			State:     EncryptionKeyStateActive,
		}
		require.NoError(t, db.CreateEncryptionKey(ctx, ek))

		// Create multiple versions for this key
		var versionIds []apid.ID
		for i := int64(1); i <= 3; i++ {
			ekv := &EncryptionKeyVersion{
				EncryptionKeyId: ek.Id,
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
		otherEk := &EncryptionKey{
			Id:        apid.New(apid.PrefixEncryptionKey),
			Namespace: "root",
			State:     EncryptionKeyStateActive,
		}
		require.NoError(t, db.CreateEncryptionKey(ctx, otherEk))

		otherEkv := &EncryptionKeyVersion{
			EncryptionKeyId: otherEk.Id,
			Provider:        "local",
			ProviderID:      "other-key",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		}
		require.NoError(t, db.CreateEncryptionKeyVersion(ctx, otherEkv))

		// Verify versions exist before delete
		versions, err := db.ListEncryptionKeyVersionsForEncryptionKey(ctx, ek.Id)
		require.NoError(t, err)
		require.Len(t, versions, 3)

		// Delete the encryption key
		err = db.DeleteEncryptionKey(ctx, ek.Id)
		require.NoError(t, err)

		// The key should be gone
		_, err = db.GetEncryptionKey(ctx, ek.Id)
		require.ErrorIs(t, err, ErrNotFound)

		// All versions for the deleted key should be gone
		versions, err = db.ListEncryptionKeyVersionsForEncryptionKey(ctx, ek.Id)
		require.NoError(t, err)
		require.Empty(t, versions)

		// Each version should individually be not found
		for _, vid := range versionIds {
			_, err = db.GetEncryptionKeyVersion(ctx, vid)
			require.ErrorIs(t, err, ErrNotFound)
		}

		// The other key's version should be unaffected
		otherVersions, err := db.ListEncryptionKeyVersionsForEncryptionKey(ctx, otherEk.Id)
		require.NoError(t, err)
		require.Len(t, otherVersions, 1)
	})

	t.Run("DeleteCascadesNoVersions", func(t *testing.T) {
		// Deleting a key with no versions should succeed without error
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ek := &EncryptionKey{
			Id:        apid.New(apid.PrefixEncryptionKey),
			Namespace: "root",
			State:     EncryptionKeyStateActive,
		}
		require.NoError(t, db.CreateEncryptionKey(ctx, ek))

		err := db.DeleteEncryptionKey(ctx, ek.Id)
		require.NoError(t, err)

		_, err = db.GetEncryptionKey(ctx, ek.Id)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("Validation", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// Missing id
		ek := &EncryptionKey{
			Namespace: "root",
			State:     EncryptionKeyStateActive,
		}
		err := db.CreateEncryptionKey(ctx, ek)
		require.Error(t, err)

		// Missing namespace
		ek = &EncryptionKey{
			Id:    apid.New(apid.PrefixEncryptionKey),
			State: EncryptionKeyStateActive,
		}
		err = db.CreateEncryptionKey(ctx, ek)
		require.Error(t, err)
	})
}
