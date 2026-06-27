package database

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestDataEncryptionKey(t *testing.T) {
	t.Run("create get and list", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ekID := apid.New(apid.PrefixKey)
		dek := &DataEncryptionKey{
			KeyId:           ekID,
			Provider:        "mock_kms",
			ProviderID:      "provider-key",
			ProviderVersion: "v1",
			ProviderMetadata: DataEncryptionKeyProviderMetadata{
				"rotation": "r1",
			},
			ProtectedData: &sconfig.KeyVersionProtectedData{
				Type:        "mock_kms",
				WrappedData: "wrapped",
				Metadata:    map[string]string{"k": "v"},
			},
			IsCurrent: true,
		}

		require.NoError(t, db.CreateDataEncryptionKey(ctx, dek))
		require.True(t, dek.Id.HasPrefix(apid.PrefixDataEncryptionKey))
		require.True(t, now.Equal(dek.CreatedAt))
		require.True(t, now.Equal(dek.UpdatedAt))

		got, err := db.GetDataEncryptionKey(ctx, dek.Id)
		require.NoError(t, err)
		require.Equal(t, dek.Id, got.Id)
		require.Equal(t, ekID, got.KeyId)
		require.Equal(t, "mock_kms", got.Provider)
		require.Equal(t, "provider-key", got.ProviderID)
		require.Equal(t, "v1", got.ProviderVersion)
		require.Equal(t, "r1", got.ProviderMetadata["rotation"])
		require.True(t, got.IsCurrent)
		require.NotNil(t, got.ProtectedData)
		require.Equal(t, "wrapped", got.ProtectedData.WrappedData)
		require.Equal(t, "v", got.ProtectedData.Metadata["k"])

		listed, err := db.ListDataEncryptionKeysForKey(ctx, ekID)
		require.NoError(t, err)
		require.Len(t, listed, 1)
		require.Equal(t, dek.Id, listed[0].Id)

		current, err := db.GetCurrentDataEncryptionKeyForKey(ctx, ekID)
		require.NoError(t, err)
		require.Equal(t, dek.Id, current.Id)
	})

	t.Run("enumerate by key", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := apctx.NewBuilderBackground().Build()
		ekID := apid.New(apid.PrefixKey)
		otherKeyID := apid.New(apid.PrefixKey)

		first := validTestDataEncryptionKey(ekID, "v1", false)
		require.NoError(t, db.CreateDataEncryptionKey(ctx, first))
		second := validTestDataEncryptionKey(ekID, "v2", false)
		require.NoError(t, db.CreateDataEncryptionKey(ctx, second))
		require.NoError(t, db.CreateDataEncryptionKey(ctx, validTestDataEncryptionKey(otherKeyID, "v1", false)))

		var ids []apid.ID
		var sawLastPage bool
		err := db.EnumerateDataEncryptionKeysForKey(ctx, ekID, func(deks []*DataEncryptionKey, lastPage bool) (pagination.KeepGoing, error) {
			sawLastPage = lastPage
			for _, dek := range deks {
				ids = append(ids, dek.Id)
				require.Equal(t, ekID, dek.KeyId)
			}
			return pagination.Continue, nil
		})
		require.NoError(t, err)
		require.True(t, sawLastPage)
		require.ElementsMatch(t, []apid.ID{first.Id, second.Id}, ids)
	})

	t.Run("new current clears previous current", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := apctx.NewBuilderBackground().Build()
		ekID := apid.New(apid.PrefixKey)

		first := validTestDataEncryptionKey(ekID, "v1", true)
		require.NoError(t, db.CreateDataEncryptionKey(ctx, first))

		second := validTestDataEncryptionKey(ekID, "v2", true)
		require.NoError(t, db.CreateDataEncryptionKey(ctx, second))

		gotFirst, err := db.GetDataEncryptionKey(ctx, first.Id)
		require.NoError(t, err)
		require.False(t, gotFirst.IsCurrent)

		gotSecond, err := db.GetDataEncryptionKey(ctx, second.Id)
		require.NoError(t, err)
		require.True(t, gotSecond.IsCurrent)

		current, err := db.GetCurrentDataEncryptionKeyForKey(ctx, ekID)
		require.NoError(t, err)
		require.Equal(t, second.Id, current.Id)
	})

	t.Run("current flag updates", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1985, time.October, 26, 1, 20, 0, 0, time.UTC)
		fakeClock := clock.NewFakeClock(now)
		ctx := apctx.NewBuilderBackground().WithClock(fakeClock).Build()

		ekID := apid.New(apid.PrefixKey)
		otherKeyID := apid.New(apid.PrefixKey)

		first := validTestDataEncryptionKey(ekID, "v1", true)
		require.NoError(t, db.CreateDataEncryptionKey(ctx, first))

		second := validTestDataEncryptionKey(ekID, "v2", false)
		require.NoError(t, db.CreateDataEncryptionKey(ctx, second))

		other := validTestDataEncryptionKey(otherKeyID, "v1", true)
		require.NoError(t, db.CreateDataEncryptionKey(ctx, other))

		current, err := db.GetCurrentDataEncryptionKeyForKey(ctx, ekID)
		require.NoError(t, err)
		require.Equal(t, first.Id, current.Id)

		fakeClock.Step(time.Hour)
		require.NoError(t, db.SetDataEncryptionKeyCurrentFlag(ctx, second.Id, true))

		gotFirst, err := db.GetDataEncryptionKey(ctx, first.Id)
		require.NoError(t, err)
		require.False(t, gotFirst.IsCurrent)

		gotSecond, err := db.GetDataEncryptionKey(ctx, second.Id)
		require.NoError(t, err)
		require.True(t, gotSecond.IsCurrent)
		require.True(t, now.Add(time.Hour).Equal(gotSecond.UpdatedAt))

		current, err = db.GetCurrentDataEncryptionKeyForKey(ctx, ekID)
		require.NoError(t, err)
		require.Equal(t, second.Id, current.Id)

		otherCurrent, err := db.GetCurrentDataEncryptionKeyForKey(ctx, otherKeyID)
		require.NoError(t, err)
		require.Equal(t, other.Id, otherCurrent.Id)

		fakeClock.Step(time.Hour)
		require.NoError(t, db.ClearCurrentDataEncryptionKeyFlagForKey(ctx, ekID))

		_, err = db.GetCurrentDataEncryptionKeyForKey(ctx, ekID)
		require.ErrorIs(t, err, ErrNotFound)

		otherCurrent, err = db.GetCurrentDataEncryptionKeyForKey(ctx, otherKeyID)
		require.NoError(t, err)
		require.Equal(t, other.Id, otherCurrent.Id)
	})

	t.Run("current flag update missing id", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := apctx.NewBuilderBackground().Build()

		err := db.SetDataEncryptionKeyCurrentFlag(ctx, apid.New(apid.PrefixDataEncryptionKey), true)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("validation", func(t *testing.T) {
		ekID := apid.New(apid.PrefixKey)
		require.NoError(t, validTestDataEncryptionKey(ekID, "v1", true).Validate())

		missingProtected := validTestDataEncryptionKey(ekID, "v1", true)
		missingProtected.ProtectedData = nil
		require.ErrorContains(t, missingProtected.Validate(), "protected data is required")

		wrongPrefix := validTestDataEncryptionKey(ekID, "v1", true)
		wrongPrefix.Id = apid.New(apid.PrefixActor)
		require.Error(t, wrongPrefix.Validate())
	})
}

func validTestDataEncryptionKey(ekID apid.ID, version string, isCurrent bool) *DataEncryptionKey {
	return &DataEncryptionKey{
		Id:              apid.New(apid.PrefixDataEncryptionKey),
		KeyId:           ekID,
		Provider:        "mock_kms",
		ProviderID:      "provider-key",
		ProviderVersion: version,
		ProtectedData: &sconfig.KeyVersionProtectedData{
			Type:        "mock_kms",
			WrappedData: "wrapped-" + version,
		},
		IsCurrent: isCurrent,
	}
}
