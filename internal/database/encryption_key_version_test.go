package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestEncryptionKeyVersion(t *testing.T) {
	t.Run("create and get", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ekv := &EncryptionKeyVersion{
			Scope:           "default",
			Provider:        "local",
			ProviderID:      "key-1",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		}

		err := db.CreateEncryptionKeyVersion(ctx, ekv)
		require.NoError(t, err)
		assert.False(t, ekv.Id.IsNil())
		assert.Equal(t, now, ekv.CreatedAt)
		assert.Equal(t, now, ekv.UpdatedAt)

		got, err := db.GetEncryptionKeyVersion(ctx, ekv.Id)
		require.NoError(t, err)
		assert.Equal(t, ekv.Id, got.Id)
		assert.Equal(t, "default", got.Scope)
		assert.Equal(t, "local", got.Provider)
		assert.Equal(t, "key-1", got.ProviderID)
		assert.Equal(t, "v1", got.ProviderVersion)
		assert.Equal(t, int64(1), got.OrderedVersion)
		assert.True(t, got.IsCurrent)
		assert.Nil(t, got.DeletedAt)
	})

	t.Run("create nil returns error", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		err := db.CreateEncryptionKeyVersion(ctx, nil)
		assert.Error(t, err)
	})

	t.Run("create with preset id preserves it", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		presetID := apid.New(apid.PrefixEncryptionKeyVersion)
		ekv := &EncryptionKeyVersion{
			Id:              presetID,
			Scope:           "default",
			Provider:        "local",
			ProviderID:      "key-1",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		}

		err := db.CreateEncryptionKeyVersion(ctx, ekv)
		require.NoError(t, err)
		assert.Equal(t, presetID, ekv.Id)
	})

	t.Run("get not found", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := apctx.NewBuilderBackground().Build()

		_, err := db.GetEncryptionKeyVersion(ctx, apid.New(apid.PrefixEncryptionKeyVersion))
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("get current for scope", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// Create a non-current version
		err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
			Scope:           "default",
			Provider:        "local",
			ProviderID:      "key-old",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       false,
		})
		require.NoError(t, err)

		// Create the current version
		current := &EncryptionKeyVersion{
			Scope:           "default",
			Provider:        "local",
			ProviderID:      "key-new",
			ProviderVersion: "v2",
			OrderedVersion:  2,
			IsCurrent:       true,
		}
		err = db.CreateEncryptionKeyVersion(ctx, current)
		require.NoError(t, err)

		got, err := db.GetCurrentEncryptionKeyVersionForScope(ctx, "default")
		require.NoError(t, err)
		assert.Equal(t, current.Id, got.Id)
		assert.Equal(t, "key-new", got.ProviderID)
		assert.True(t, got.IsCurrent)
	})

	t.Run("get current for scope not found", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := apctx.NewBuilderBackground().Build()

		_, err := db.GetCurrentEncryptionKeyVersionForScope(ctx, "nonexistent")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("list for scope ordered by version", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// Create versions out of order
		for _, v := range []int64{3, 1, 2} {
			err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
				Scope:           "default",
				Provider:        "local",
				ProviderID:      "key",
				ProviderVersion: "v1",
				OrderedVersion:  v,
				IsCurrent:       v == 3,
			})
			require.NoError(t, err)
		}

		// Create a version in a different scope
		err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
			Scope:           "other",
			Provider:        "local",
			ProviderID:      "key",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		})
		require.NoError(t, err)

		results, err := db.ListEncryptionKeyVersionsForScope(ctx, "default")
		require.NoError(t, err)
		require.Len(t, results, 3)
		assert.Equal(t, int64(1), results[0].OrderedVersion)
		assert.Equal(t, int64(2), results[1].OrderedVersion)
		assert.Equal(t, int64(3), results[2].OrderedVersion)
	})

	t.Run("list for scope empty", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := apctx.NewBuilderBackground().Build()

		results, err := db.ListEncryptionKeyVersionsForScope(ctx, "nonexistent")
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("get max ordered version", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// No versions yet
		maxVer, err := db.GetMaxOrderedVersionForScope(ctx, "default")
		require.NoError(t, err)
		assert.Equal(t, int64(0), maxVer)

		// Add some versions
		for _, v := range []int64{1, 5, 3} {
			err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
				Scope:           "default",
				Provider:        "local",
				ProviderID:      "key",
				ProviderVersion: "v1",
				OrderedVersion:  v,
				IsCurrent:       false,
			})
			require.NoError(t, err)
		}

		maxVer, err = db.GetMaxOrderedVersionForScope(ctx, "default")
		require.NoError(t, err)
		assert.Equal(t, int64(5), maxVer)
	})

	t.Run("clear current flag for scope", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// Create two current versions
		for i, id := range []string{"key-1", "key-2"} {
			err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
				Scope:           "default",
				Provider:        "local",
				ProviderID:      id,
				ProviderVersion: "v1",
				OrderedVersion:  int64(i + 1),
				IsCurrent:       true,
			})
			require.NoError(t, err)
		}

		// Create a current version in a different scope
		otherScope := &EncryptionKeyVersion{
			Scope:           "other",
			Provider:        "local",
			ProviderID:      "key-other",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		}
		err := db.CreateEncryptionKeyVersion(ctx, otherScope)
		require.NoError(t, err)

		err = db.ClearCurrentFlagForScope(ctx, "default")
		require.NoError(t, err)

		// No current version in default scope
		_, err = db.GetCurrentEncryptionKeyVersionForScope(ctx, "default")
		assert.ErrorIs(t, err, ErrNotFound)

		// Other scope unaffected
		got, err := db.GetCurrentEncryptionKeyVersionForScope(ctx, "other")
		require.NoError(t, err)
		assert.True(t, got.IsCurrent)
	})

	t.Run("delete soft deletes", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ekv := &EncryptionKeyVersion{
			Scope:           "default",
			Provider:        "local",
			ProviderID:      "key-1",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		}
		err := db.CreateEncryptionKeyVersion(ctx, ekv)
		require.NoError(t, err)

		err = db.DeleteEncryptionKeyVersion(ctx, ekv.Id)
		require.NoError(t, err)

		// Should not be findable via get
		_, err = db.GetEncryptionKeyVersion(ctx, ekv.Id)
		assert.ErrorIs(t, err, ErrNotFound)

		// Should not appear in list
		results, err := db.ListEncryptionKeyVersionsForScope(ctx, "default")
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("delete not found", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := apctx.NewBuilderBackground().Build()

		err := db.DeleteEncryptionKeyVersion(ctx, apid.New(apid.PrefixEncryptionKeyVersion))
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("set current flag", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ekv := &EncryptionKeyVersion{
			Scope:           "default",
			Provider:        "local",
			ProviderID:      "key-1",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       false,
		}
		err := db.CreateEncryptionKeyVersion(ctx, ekv)
		require.NoError(t, err)

		err = db.SetEncryptionKeyVersionCurrentFlag(ctx, ekv.Id, true)
		require.NoError(t, err)

		got, err := db.GetEncryptionKeyVersion(ctx, ekv.Id)
		require.NoError(t, err)
		assert.True(t, got.IsCurrent)
	})

	t.Run("set current flag not found for already current", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// Create a version that is already current
		ekv := &EncryptionKeyVersion{
			Scope:           "default",
			Provider:        "local",
			ProviderID:      "key-1",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		}
		err := db.CreateEncryptionKeyVersion(ctx, ekv)
		require.NoError(t, err)

		// Setting current on an already-current version returns ErrNotFound
		// because the WHERE clause requires is_current=false
		err = db.SetEncryptionKeyVersionCurrentFlag(ctx, ekv.Id, true)
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("set current flag not found for nonexistent id", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := apctx.NewBuilderBackground().Build()

		err := db.SetEncryptionKeyVersionCurrentFlag(ctx, apid.New(apid.PrefixEncryptionKeyVersion), true)
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("deleted versions excluded from max ordered version", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ekv := &EncryptionKeyVersion{
			Scope:           "default",
			Provider:        "local",
			ProviderID:      "key-1",
			ProviderVersion: "v1",
			OrderedVersion:  10,
			IsCurrent:       false,
		}
		err := db.CreateEncryptionKeyVersion(ctx, ekv)
		require.NoError(t, err)

		lower := &EncryptionKeyVersion{
			Scope:           "default",
			Provider:        "local",
			ProviderID:      "key-2",
			ProviderVersion: "v1",
			OrderedVersion:  5,
			IsCurrent:       false,
		}
		err = db.CreateEncryptionKeyVersion(ctx, lower)
		require.NoError(t, err)

		// Delete the higher version
		err = db.DeleteEncryptionKeyVersion(ctx, ekv.Id)
		require.NoError(t, err)

		maxVer, err := db.GetMaxOrderedVersionForScope(ctx, "default")
		require.NoError(t, err)
		assert.Equal(t, int64(5), maxVer)
	})
}

func TestEnumerateEncryptionKeyVersions(t *testing.T) {
	t.Run("empty database", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := apctx.NewBuilderBackground().Build()

		var callCount int
		err := db.EnumerateEncryptionKeyVersions(ctx, func(ekvs []*EncryptionKeyVersion, lastPage bool) (bool, error) {
			callCount++
			assert.Empty(t, ekvs)
			assert.True(t, lastPage)
			return false, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 1, callCount)
	})

	t.Run("fewer than page size", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		for i := 0; i < 5; i++ {
			err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
				Scope:           "default",
				Provider:        "local",
				ProviderID:      fmt.Sprintf("key-%d", i),
				ProviderVersion: "v1",
				OrderedVersion:  int64(i + 1),
				IsCurrent:       i == 4,
			})
			require.NoError(t, err)
		}

		var allResults []*EncryptionKeyVersion
		var callCount int
		err := db.EnumerateEncryptionKeyVersions(ctx, func(ekvs []*EncryptionKeyVersion, lastPage bool) (bool, error) {
			callCount++
			allResults = append(allResults, ekvs...)
			assert.True(t, lastPage)
			return false, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 1, callCount)
		assert.Len(t, allResults, 5)
	})

	t.Run("multiple pages", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		totalRecords := 250
		for i := 0; i < totalRecords; i++ {
			err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
				Scope:           fmt.Sprintf("scope-%d", i),
				Provider:        "local",
				ProviderID:      fmt.Sprintf("key-%d", i),
				ProviderVersion: "v1",
				OrderedVersion:  1,
				IsCurrent:       false,
			})
			require.NoError(t, err)
		}

		var allResults []*EncryptionKeyVersion
		var callCount int
		err := db.EnumerateEncryptionKeyVersions(ctx, func(ekvs []*EncryptionKeyVersion, lastPage bool) (bool, error) {
			callCount++
			allResults = append(allResults, ekvs...)
			if lastPage {
				assert.True(t, len(ekvs) <= 100)
			} else {
				assert.Len(t, ekvs, 100)
			}
			return false, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 3, callCount)
		assert.Len(t, allResults, totalRecords)
	})

	t.Run("excludes deleted records", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		kept := &EncryptionKeyVersion{
			Scope:           "default",
			Provider:        "local",
			ProviderID:      "key-keep",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		}
		err := db.CreateEncryptionKeyVersion(ctx, kept)
		require.NoError(t, err)

		deleted := &EncryptionKeyVersion{
			Scope:           "other",
			Provider:        "local",
			ProviderID:      "key-delete",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       false,
		}
		err = db.CreateEncryptionKeyVersion(ctx, deleted)
		require.NoError(t, err)

		err = db.DeleteEncryptionKeyVersion(ctx, deleted.Id)
		require.NoError(t, err)

		var allResults []*EncryptionKeyVersion
		err = db.EnumerateEncryptionKeyVersions(ctx, func(ekvs []*EncryptionKeyVersion, lastPage bool) (bool, error) {
			allResults = append(allResults, ekvs...)
			return false, nil
		})
		require.NoError(t, err)
		assert.Len(t, allResults, 1)
		assert.Equal(t, kept.Id, allResults[0].Id)
	})

	t.Run("early stop", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		for i := 0; i < 250; i++ {
			err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
				Scope:           fmt.Sprintf("scope-%d", i),
				Provider:        "local",
				ProviderID:      fmt.Sprintf("key-%d", i),
				ProviderVersion: "v1",
				OrderedVersion:  1,
				IsCurrent:       false,
			})
			require.NoError(t, err)
		}

		var callCount int
		err := db.EnumerateEncryptionKeyVersions(ctx, func(ekvs []*EncryptionKeyVersion, lastPage bool) (bool, error) {
			callCount++
			return true, nil // stop after first batch
		})
		require.NoError(t, err)
		assert.Equal(t, 1, callCount)
	})

	t.Run("callback error propagated", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
			Scope:           "default",
			Provider:        "local",
			ProviderID:      "key-1",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       false,
		})
		require.NoError(t, err)

		expectedErr := fmt.Errorf("callback error")
		err = db.EnumerateEncryptionKeyVersions(ctx, func(ekvs []*EncryptionKeyVersion, lastPage bool) (bool, error) {
			return false, expectedErr
		})
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("includes all scopes", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		for _, scope := range []string{"scope-a", "scope-b", "scope-c"} {
			err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
				Scope:           scope,
				Provider:        "local",
				ProviderID:      "key-1",
				ProviderVersion: "v1",
				OrderedVersion:  1,
				IsCurrent:       true,
			})
			require.NoError(t, err)
		}

		var allResults []*EncryptionKeyVersion
		err := db.EnumerateEncryptionKeyVersions(ctx, func(ekvs []*EncryptionKeyVersion, lastPage bool) (bool, error) {
			allResults = append(allResults, ekvs...)
			return false, nil
		})
		require.NoError(t, err)
		assert.Len(t, allResults, 3)
	})
}
