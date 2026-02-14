package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/require"
)

func TestUpdateDraftConnectorVersion(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
		ctx := context.Background()

		// Existing draft version
		db.EXPECT().
			GetConnectorVersion(gomock.Any(), id, uint64(2)).
			Return(&database.ConnectorVersion{
				Id:        id,
				Version:   2,
				Namespace: "root",
				State:     database.ConnectorVersionStateDraft,
				Labels:    map[string]string{"env": "old"},
			}, nil)

		// Build encrypts
		e.EXPECT().
			EncryptStringForConnector(gomock.Any(), gomock.Any(), gomock.Any()).
			Return("encrypted-def", nil)

		// Upsert
		db.EXPECT().
			UpsertConnectorVersion(gomock.Any(), gomock.Any()).
			Return(nil)

		// Re-fetch
		newLabels := map[string]string{"env": "new"}
		mock.MockConnectorRetrival(ctx, db, e, &cschema.Connector{
			Id:          id,
			Version:     2,
			State:       string(database.ConnectorVersionStateDraft),
			DisplayName: "Updated",
			Labels:      newLabels,
		})

		definition := &cschema.Connector{
			DisplayName: "Updated",
		}

		result, err := s.UpdateDraftConnectorVersion(ctx, id, 2, definition, newLabels)
		require.NoError(t, err)
		require.Equal(t, id, result.GetId())
		require.Equal(t, uint64(2), result.GetVersion())
		require.Equal(t, database.ConnectorVersionStateDraft, result.GetState())
		require.Equal(t, "new", result.GetLabels()["env"])
	})

	t.Run("success keeps existing labels when nil", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
		ctx := context.Background()
		existingLabels := map[string]string{"env": "kept"}

		db.EXPECT().
			GetConnectorVersion(gomock.Any(), id, uint64(1)).
			Return(&database.ConnectorVersion{
				Id:        id,
				Version:   1,
				Namespace: "root",
				State:     database.ConnectorVersionStateDraft,
				Labels:    existingLabels,
			}, nil)

		e.EXPECT().
			EncryptStringForConnector(gomock.Any(), gomock.Any(), gomock.Any()).
			Return("encrypted-def", nil)

		db.EXPECT().
			UpsertConnectorVersion(gomock.Any(), gomock.Any()).
			Return(nil)

		mock.MockConnectorRetrival(ctx, db, e, &cschema.Connector{
			Id:          id,
			Version:     1,
			State:       string(database.ConnectorVersionStateDraft),
			DisplayName: "Test",
			Labels:      existingLabels,
		})

		definition := &cschema.Connector{
			DisplayName: "Test",
		}

		result, err := s.UpdateDraftConnectorVersion(ctx, id, 1, definition, nil)
		require.NoError(t, err)
		require.Equal(t, "kept", result.GetLabels()["env"])
	})

	t.Run("not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := uuid.New()
		ctx := context.Background()

		db.EXPECT().
			GetConnectorVersion(gomock.Any(), id, uint64(1)).
			Return(nil, database.ErrNotFound)

		_, err := s.UpdateDraftConnectorVersion(ctx, id, 1, &cschema.Connector{DisplayName: "Test"}, nil)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("db error getting version", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := uuid.New()
		ctx := context.Background()

		db.EXPECT().
			GetConnectorVersion(gomock.Any(), id, uint64(1)).
			Return(nil, errors.New("connection refused"))

		_, err := s.UpdateDraftConnectorVersion(ctx, id, 1, &cschema.Connector{DisplayName: "Test"}, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get connector version")
	})

	t.Run("not draft", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := uuid.New()
		ctx := context.Background()

		db.EXPECT().
			GetConnectorVersion(gomock.Any(), id, uint64(1)).
			Return(&database.ConnectorVersion{
				Id:      id,
				Version: 1,
				State:   database.ConnectorVersionStatePrimary,
			}, nil)

		_, err := s.UpdateDraftConnectorVersion(ctx, id, 1, &cschema.Connector{DisplayName: "Test"}, nil)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNotDraft)
	})

	t.Run("upsert error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := uuid.New()
		ctx := context.Background()

		db.EXPECT().
			GetConnectorVersion(gomock.Any(), id, uint64(1)).
			Return(&database.ConnectorVersion{
				Id:        id,
				Version:   1,
				Namespace: "root",
				State:     database.ConnectorVersionStateDraft,
			}, nil)

		e.EXPECT().
			EncryptStringForConnector(gomock.Any(), gomock.Any(), gomock.Any()).
			Return("encrypted", nil)

		db.EXPECT().
			UpsertConnectorVersion(gomock.Any(), gomock.Any()).
			Return(errors.New("write failed"))

		_, err := s.UpdateDraftConnectorVersion(ctx, id, 1, &cschema.Connector{DisplayName: "Test"}, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to upsert connector version")
	})
}

func TestGetOrCreateDraftConnectorVersion(t *testing.T) {
	t.Run("returns existing draft", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")
		ctx := context.Background()
		encryptedDef := "existing-encrypted-def"

		existingDef := &cschema.Connector{
			Id:          id,
			Version:     2,
			DisplayName: "Existing Draft",
		}
		defJson, _ := json.Marshal(existingDef)

		db.EXPECT().
			GetConnectorVersionForState(gomock.Any(), id, database.ConnectorVersionStateDraft).
			Return(&database.ConnectorVersion{
				Id:                  id,
				Version:             2,
				Namespace:           "root",
				State:               database.ConnectorVersionStateDraft,
				EncryptedDefinition: encryptedDef,
			}, nil)

		// Decrypt to verify definition loads
		e.EXPECT().
			DecryptStringForConnector(gomock.Any(), gomock.Any(), encryptedDef).
			Return(string(defJson), nil)

		result, err := s.GetOrCreateDraftConnectorVersion(ctx, id)
		require.NoError(t, err)
		require.Equal(t, id, result.GetId())
		require.Equal(t, uint64(2), result.GetVersion())
		require.Equal(t, database.ConnectorVersionStateDraft, result.GetState())
	})

	t.Run("creates new draft from latest", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd")
		ctx := context.Background()
		encryptedDef := "latest-encrypted-def"

		latestDef := &cschema.Connector{
			Id:          id,
			Version:     3,
			DisplayName: "Latest Version",
		}
		latestDefJson, _ := json.Marshal(latestDef)

		// No existing draft
		db.EXPECT().
			GetConnectorVersionForState(gomock.Any(), id, database.ConnectorVersionStateDraft).
			Return(nil, database.ErrNotFound)

		// Latest version
		db.EXPECT().
			NewestConnectorVersionForId(gomock.Any(), id).
			Return(&database.ConnectorVersion{
				Id:                  id,
				Version:             3,
				Namespace:           "root",
				State:               database.ConnectorVersionStatePrimary,
				Labels:              map[string]string{"env": "prod"},
				EncryptedDefinition: encryptedDef,
			}, nil)

		// Decrypt latest definition
		e.EXPECT().
			DecryptStringForConnector(gomock.Any(), gomock.Any(), encryptedDef).
			Return(string(latestDefJson), nil)

		// Build encrypts the new version
		e.EXPECT().
			EncryptStringForConnector(gomock.Any(), gomock.Any(), gomock.Any()).
			Return("new-encrypted-def", nil)

		// Upsert
		db.EXPECT().
			UpsertConnectorVersion(gomock.Any(), gomock.Any()).
			Return(nil)

		// Re-fetch
		mock.MockConnectorRetrival(ctx, db, e, &cschema.Connector{
			Id:          id,
			Version:     4,
			State:       string(database.ConnectorVersionStateDraft),
			DisplayName: "Latest Version",
			Labels:      map[string]string{"env": "prod"},
		})

		result, err := s.GetOrCreateDraftConnectorVersion(ctx, id)
		require.NoError(t, err)
		require.Equal(t, id, result.GetId())
		require.Equal(t, uint64(4), result.GetVersion())
		require.Equal(t, database.ConnectorVersionStateDraft, result.GetState())
		require.Equal(t, "prod", result.GetLabels()["env"])
	})

	t.Run("db error checking for draft", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := uuid.New()
		ctx := context.Background()

		db.EXPECT().
			GetConnectorVersionForState(gomock.Any(), id, database.ConnectorVersionStateDraft).
			Return(nil, errors.New("connection refused"))

		_, err := s.GetOrCreateDraftConnectorVersion(ctx, id)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to check for existing draft")
	})

	t.Run("connector not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := uuid.New()
		ctx := context.Background()

		db.EXPECT().
			GetConnectorVersionForState(gomock.Any(), id, database.ConnectorVersionStateDraft).
			Return(nil, database.ErrNotFound)

		db.EXPECT().
			NewestConnectorVersionForId(gomock.Any(), id).
			Return(nil, database.ErrNotFound)

		_, err := s.GetOrCreateDraftConnectorVersion(ctx, id)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("db error getting newest version", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := uuid.New()
		ctx := context.Background()

		db.EXPECT().
			GetConnectorVersionForState(gomock.Any(), id, database.ConnectorVersionStateDraft).
			Return(nil, database.ErrNotFound)

		db.EXPECT().
			NewestConnectorVersionForId(gomock.Any(), id).
			Return(nil, errors.New("timeout"))

		_, err := s.GetOrCreateDraftConnectorVersion(ctx, id)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get latest connector version")
	})

	t.Run("upsert error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := uuid.New()
		ctx := context.Background()
		encryptedDef := "latest-encrypted-def"

		latestDef := &cschema.Connector{
			Id:          id,
			Version:     1,
			DisplayName: "Test",
		}
		latestDefJson, _ := json.Marshal(latestDef)

		db.EXPECT().
			GetConnectorVersionForState(gomock.Any(), id, database.ConnectorVersionStateDraft).
			Return(nil, database.ErrNotFound)

		db.EXPECT().
			NewestConnectorVersionForId(gomock.Any(), id).
			Return(&database.ConnectorVersion{
				Id:                  id,
				Version:             1,
				Namespace:           "root",
				State:               database.ConnectorVersionStatePrimary,
				EncryptedDefinition: encryptedDef,
			}, nil)

		e.EXPECT().
			DecryptStringForConnector(gomock.Any(), gomock.Any(), encryptedDef).
			Return(string(latestDefJson), nil)

		e.EXPECT().
			EncryptStringForConnector(gomock.Any(), gomock.Any(), gomock.Any()).
			Return("new-encrypted", nil)

		db.EXPECT().
			UpsertConnectorVersion(gomock.Any(), gomock.Any()).
			Return(errors.New("constraint violation"))

		_, err := s.GetOrCreateDraftConnectorVersion(ctx, id)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to upsert connector version")
	})
}
