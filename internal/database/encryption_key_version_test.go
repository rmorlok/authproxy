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

		ekId := apid.New(apid.PrefixEncryptionKey)
		ekv := &EncryptionKeyVersion{
			EncryptionKeyId: ekId,
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
		assert.Equal(t, ekId, got.EncryptionKeyId)
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
			EncryptionKeyId: apid.New(apid.PrefixEncryptionKey),
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

	t.Run("get current for encryption key", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ekId := apid.New(apid.PrefixEncryptionKey)

		// Create a non-current version
		err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
			EncryptionKeyId: ekId,
			Provider:        "local",
			ProviderID:      "key-old",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       false,
		})
		require.NoError(t, err)

		// Create the current version
		current := &EncryptionKeyVersion{
			EncryptionKeyId: ekId,
			Provider:        "local",
			ProviderID:      "key-new",
			ProviderVersion: "v2",
			OrderedVersion:  2,
			IsCurrent:       true,
		}
		err = db.CreateEncryptionKeyVersion(ctx, current)
		require.NoError(t, err)

		got, err := db.GetCurrentEncryptionKeyVersionForEncryptionKey(ctx, ekId)
		require.NoError(t, err)
		assert.Equal(t, current.Id, got.Id)
		assert.Equal(t, "key-new", got.ProviderID)
		assert.True(t, got.IsCurrent)
	})

	t.Run("get current for encryption key not found", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := apctx.NewBuilderBackground().Build()

		_, err := db.GetCurrentEncryptionKeyVersionForEncryptionKey(ctx, apid.New(apid.PrefixEncryptionKey))
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("list for encryption key ordered by version", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ekId := apid.New(apid.PrefixEncryptionKey)
		otherEkId := apid.New(apid.PrefixEncryptionKey)

		// Create versions out of order
		for _, v := range []int64{3, 1, 2} {
			err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
				EncryptionKeyId: ekId,
				Provider:        "local",
				ProviderID:      "key",
				ProviderVersion: "v1",
				OrderedVersion:  v,
				IsCurrent:       v == 3,
			})
			require.NoError(t, err)
		}

		// Create a version with a different encryption key
		err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
			EncryptionKeyId: otherEkId,
			Provider:        "local",
			ProviderID:      "key",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		})
		require.NoError(t, err)

		results, err := db.ListEncryptionKeyVersionsForEncryptionKey(ctx, ekId)
		require.NoError(t, err)
		require.Len(t, results, 3)
		assert.Equal(t, int64(1), results[0].OrderedVersion)
		assert.Equal(t, int64(2), results[1].OrderedVersion)
		assert.Equal(t, int64(3), results[2].OrderedVersion)
	})

	t.Run("list for encryption key empty", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := apctx.NewBuilderBackground().Build()

		results, err := db.ListEncryptionKeyVersionsForEncryptionKey(ctx, apid.New(apid.PrefixEncryptionKey))
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("get max ordered version", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ekId := apid.New(apid.PrefixEncryptionKey)

		// No versions yet
		maxVer, err := db.GetMaxOrderedVersionForEncryptionKey(ctx, ekId)
		require.NoError(t, err)
		assert.Equal(t, int64(0), maxVer)

		// Add some versions
		for _, v := range []int64{1, 5, 3} {
			err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
				EncryptionKeyId: ekId,
				Provider:        "local",
				ProviderID:      "key",
				ProviderVersion: "v1",
				OrderedVersion:  v,
				IsCurrent:       false,
			})
			require.NoError(t, err)
		}

		maxVer, err = db.GetMaxOrderedVersionForEncryptionKey(ctx, ekId)
		require.NoError(t, err)
		assert.Equal(t, int64(5), maxVer)
	})

	t.Run("clear current flag for encryption key", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ekId := apid.New(apid.PrefixEncryptionKey)
		otherEkId := apid.New(apid.PrefixEncryptionKey)

		// Create two current versions for same encryption key
		for i, id := range []string{"key-1", "key-2"} {
			err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
				EncryptionKeyId: ekId,
				Provider:        "local",
				ProviderID:      id,
				ProviderVersion: "v1",
				OrderedVersion:  int64(i + 1),
				IsCurrent:       true,
			})
			require.NoError(t, err)
		}

		// Create a current version with a different encryption key
		otherEk := &EncryptionKeyVersion{
			EncryptionKeyId: otherEkId,
			Provider:        "local",
			ProviderID:      "key-other",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		}
		err := db.CreateEncryptionKeyVersion(ctx, otherEk)
		require.NoError(t, err)

		err = db.ClearCurrentFlagForEncryptionKey(ctx, ekId)
		require.NoError(t, err)

		// No current version for this encryption key
		_, err = db.GetCurrentEncryptionKeyVersionForEncryptionKey(ctx, ekId)
		assert.ErrorIs(t, err, ErrNotFound)

		// Other encryption key unaffected
		got, err := db.GetCurrentEncryptionKeyVersionForEncryptionKey(ctx, otherEkId)
		require.NoError(t, err)
		assert.True(t, got.IsCurrent)
	})

	t.Run("delete soft deletes", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ekId := apid.New(apid.PrefixEncryptionKey)
		ekv := &EncryptionKeyVersion{
			EncryptionKeyId: ekId,
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
		results, err := db.ListEncryptionKeyVersionsForEncryptionKey(ctx, ekId)
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
			EncryptionKeyId: apid.New(apid.PrefixEncryptionKey),
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
			EncryptionKeyId: apid.New(apid.PrefixEncryptionKey),
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

	t.Run("validate", func(t *testing.T) {
		validEkv := func() *EncryptionKeyVersion {
			return &EncryptionKeyVersion{
				Id:              apid.New(apid.PrefixEncryptionKeyVersion),
				EncryptionKeyId: apid.New(apid.PrefixEncryptionKey),
				Provider:        "local",
				ProviderID:      "key-1",
				ProviderVersion: "v1",
				OrderedVersion:  1,
				IsCurrent:       true,
			}
		}

		t.Run("valid passes", func(t *testing.T) {
			assert.NoError(t, validEkv().Validate())
		})

		t.Run("missing id", func(t *testing.T) {
			ekv := validEkv()
			ekv.Id = apid.Nil
			assert.ErrorContains(t, ekv.Validate(), "id is required")
		})

		t.Run("wrong id prefix", func(t *testing.T) {
			ekv := validEkv()
			ekv.Id = apid.New(apid.PrefixActor)
			assert.Error(t, ekv.Validate())
		})

		t.Run("missing encryption key id", func(t *testing.T) {
			ekv := validEkv()
			ekv.EncryptionKeyId = apid.Nil
			assert.ErrorContains(t, ekv.Validate(), "encryption key id is required")
		})

		t.Run("wrong encryption key id prefix", func(t *testing.T) {
			ekv := validEkv()
			ekv.EncryptionKeyId = apid.New(apid.PrefixActor)
			assert.Error(t, ekv.Validate())
		})

		t.Run("missing provider", func(t *testing.T) {
			ekv := validEkv()
			ekv.Provider = ""
			assert.ErrorContains(t, ekv.Validate(), "provider is required")
		})

		t.Run("missing provider id", func(t *testing.T) {
			ekv := validEkv()
			ekv.ProviderID = ""
			assert.ErrorContains(t, ekv.Validate(), "provider id is required")
		})

		t.Run("missing provider version", func(t *testing.T) {
			ekv := validEkv()
			ekv.ProviderVersion = ""
			assert.ErrorContains(t, ekv.Validate(), "provider version is required")
		})

		t.Run("multiple errors accumulated", func(t *testing.T) {
			ekv := &EncryptionKeyVersion{}
			err := ekv.Validate()
			require.Error(t, err)
			assert.ErrorContains(t, err, "id is required")
			assert.ErrorContains(t, err, "encryption key id is required")
			assert.ErrorContains(t, err, "provider is required")
			assert.ErrorContains(t, err, "provider id is required")
			assert.ErrorContains(t, err, "provider version is required")
		})
	})

	t.Run("deleted versions excluded from max ordered version", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ekId := apid.New(apid.PrefixEncryptionKey)

		ekv := &EncryptionKeyVersion{
			EncryptionKeyId: ekId,
			Provider:        "local",
			ProviderID:      "key-1",
			ProviderVersion: "v1",
			OrderedVersion:  10,
			IsCurrent:       false,
		}
		err := db.CreateEncryptionKeyVersion(ctx, ekv)
		require.NoError(t, err)

		lower := &EncryptionKeyVersion{
			EncryptionKeyId: ekId,
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

		maxVer, err := db.GetMaxOrderedVersionForEncryptionKey(ctx, ekId)
		require.NoError(t, err)
		assert.Equal(t, int64(5), maxVer)
	})
}

func TestEncryptionKeyVersionForNamespace(t *testing.T) {
	t.Run("get current for namespace with encryption key", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ekId := apid.New(apid.PrefixEncryptionKey)

		// Create namespace with encryption_key_id
		err := db.CreateNamespace(ctx, &Namespace{
			Path:            "root.test",
			State:           NamespaceStateActive,
			EncryptionKeyId: &ekId,
		})
		require.NoError(t, err)

		// Create a current version for that encryption key
		current := &EncryptionKeyVersion{
			EncryptionKeyId: ekId,
			Provider:        "local",
			ProviderID:      "key-1",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		}
		err = db.CreateEncryptionKeyVersion(ctx, current)
		require.NoError(t, err)

		got, err := db.GetCurrentEncryptionKeyVersionForNamespace(ctx, "root.test")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, current.Id, got.Id)
		assert.True(t, got.IsCurrent)
	})

	t.Run("get current for namespace without encryption key returns nil", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// Create namespace without encryption_key_id
		err := db.CreateNamespace(ctx, &Namespace{
			Path:  "root.nokey",
			State: NamespaceStateActive,
		})
		require.NoError(t, err)

		got, err := db.GetCurrentEncryptionKeyVersionForNamespace(ctx, "root.nokey")
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("get current for nonexistent namespace returns error", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := apctx.NewBuilderBackground().Build()

		_, err := db.GetCurrentEncryptionKeyVersionForNamespace(ctx, "root.nonexistent")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("list for namespace with encryption key", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ekId := apid.New(apid.PrefixEncryptionKey)

		err := db.CreateNamespace(ctx, &Namespace{
			Path:            "root.listtest",
			State:           NamespaceStateActive,
			EncryptionKeyId: &ekId,
		})
		require.NoError(t, err)

		for _, v := range []int64{1, 2, 3} {
			err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
				EncryptionKeyId: ekId,
				Provider:        "local",
				ProviderID:      fmt.Sprintf("key-%d", v),
				ProviderVersion: "v1",
				OrderedVersion:  v,
				IsCurrent:       v == 3,
			})
			require.NoError(t, err)
		}

		results, err := db.ListEncryptionKeyVersionsForNamespace(ctx, "root.listtest")
		require.NoError(t, err)
		require.Len(t, results, 3)
		assert.Equal(t, int64(1), results[0].OrderedVersion)
		assert.Equal(t, int64(3), results[2].OrderedVersion)
	})

	t.Run("list for namespace without encryption key returns nil", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		err := db.CreateNamespace(ctx, &Namespace{
			Path:  "root.emptylist",
			State: NamespaceStateActive,
		})
		require.NoError(t, err)

		results, err := db.ListEncryptionKeyVersionsForNamespace(ctx, "root.emptylist")
		require.NoError(t, err)
		assert.Nil(t, results)
	})

	t.Run("get max ordered version for namespace", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ekId := apid.New(apid.PrefixEncryptionKey)

		err := db.CreateNamespace(ctx, &Namespace{
			Path:            "root.maxver",
			State:           NamespaceStateActive,
			EncryptionKeyId: &ekId,
		})
		require.NoError(t, err)

		for _, v := range []int64{1, 5, 3} {
			err := db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
				EncryptionKeyId: ekId,
				Provider:        "local",
				ProviderID:      fmt.Sprintf("key-%d", v),
				ProviderVersion: "v1",
				OrderedVersion:  v,
				IsCurrent:       false,
			})
			require.NoError(t, err)
		}

		maxVer, err := db.GetMaxOrderedVersionForNamespace(ctx, "root.maxver")
		require.NoError(t, err)
		assert.Equal(t, int64(5), maxVer)
	})

	t.Run("get max ordered version for namespace without key returns 0", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		err := db.CreateNamespace(ctx, &Namespace{
			Path:  "root.nomaxver",
			State: NamespaceStateActive,
		})
		require.NoError(t, err)

		maxVer, err := db.GetMaxOrderedVersionForNamespace(ctx, "root.nomaxver")
		require.NoError(t, err)
		assert.Equal(t, int64(0), maxVer)
	})

	t.Run("clear current flag for namespace", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ekId := apid.New(apid.PrefixEncryptionKey)

		err := db.CreateNamespace(ctx, &Namespace{
			Path:            "root.clearcurrent",
			State:           NamespaceStateActive,
			EncryptionKeyId: &ekId,
		})
		require.NoError(t, err)

		err = db.CreateEncryptionKeyVersion(ctx, &EncryptionKeyVersion{
			EncryptionKeyId: ekId,
			Provider:        "local",
			ProviderID:      "key-1",
			ProviderVersion: "v1",
			OrderedVersion:  1,
			IsCurrent:       true,
		})
		require.NoError(t, err)

		err = db.ClearCurrentFlagForNamespace(ctx, "root.clearcurrent")
		require.NoError(t, err)

		_, err = db.GetCurrentEncryptionKeyVersionForEncryptionKey(ctx, ekId)
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("clear current flag for namespace without key is no-op", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		err := db.CreateNamespace(ctx, &Namespace{
			Path:  "root.noclear",
			State: NamespaceStateActive,
		})
		require.NoError(t, err)

		err = db.ClearCurrentFlagForNamespace(ctx, "root.noclear")
		require.NoError(t, err)
	})
}
