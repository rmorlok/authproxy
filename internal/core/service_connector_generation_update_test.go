package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/require"
)

func TestUpdateDraftConnectorGeneration(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, ac, e := FullMockService(t, ctrl)

		id := apid.MustParse("cxr_testaaaaaaaaaaaa")
		ctx := context.Background()

		// Successful update enqueues an asynq propagation task. The body
		// of the task is opaque to this test; just verify it's submitted.
		ac.EXPECT().EnqueueContext(gomock.Any(), gomock.Any()).Return(nil, nil)

		// Existing draft generation
		db.EXPECT().
			GetConnectorGeneration(gomock.Any(), id, uint64(2)).
			Return(&database.ConnectorWithDefinition{
				Id:         id,
				Generation: 2,
				Namespace:  "root",
				State:      database.ConnectorGenerationStateDraft,
				Labels:     map[string]string{"env": "old"},
			}, nil)

		// Build encrypts
		e.EXPECT().
			EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(encfield.EncryptedField{ID: "dek_test", Data: "encrypted-def"}, nil)

		// Upsert
		db.EXPECT().
			UpsertConnectorGeneration(gomock.Any(), gomock.Any()).
			Return(nil)

		// Re-fetch
		newLabels := map[string]string{"env": "new"}
		mock.MockConnectorRetrival(ctx, db, e, connectorResourceForMock(id, 2, database.ConnectorGenerationStateDraft, newLabels, cschema.ConnectorDefinition{
			DisplayName: "Updated",
		}))

		definition := &cschema.ConnectorDefinition{
			DisplayName: "Updated",
		}

		result, err := s.UpdateDraftConnectorGeneration(ctx, id, 2, definition, newLabels, nil)
		require.NoError(t, err)
		require.Equal(t, id, result.GetId())
		require.Equal(t, uint64(2), result.GetGeneration())
		require.Equal(t, database.ConnectorGenerationStateDraft, result.GetState())
		require.Equal(t, "new", result.GetLabels()["env"])
	})

	t.Run("success keeps existing labels when nil", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, ac, e := FullMockService(t, ctrl)

		id := apid.MustParse("cxr_testbbbbbbbbbbbb")
		ctx := context.Background()
		existingLabels := map[string]string{"env": "kept"}

		ac.EXPECT().EnqueueContext(gomock.Any(), gomock.Any()).Return(nil, nil)

		db.EXPECT().
			GetConnectorGeneration(gomock.Any(), id, uint64(1)).
			Return(&database.ConnectorWithDefinition{
				Id:         id,
				Generation: 1,
				Namespace:  "root",
				State:      database.ConnectorGenerationStateDraft,
				Labels:     existingLabels,
			}, nil)

		e.EXPECT().
			EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(encfield.EncryptedField{ID: "dek_test", Data: "encrypted-def"}, nil)

		db.EXPECT().
			UpsertConnectorGeneration(gomock.Any(), gomock.Any()).
			Return(nil)

		mock.MockConnectorRetrival(ctx, db, e, connectorResourceForMock(id, 1, database.ConnectorGenerationStateDraft, existingLabels, cschema.ConnectorDefinition{
			DisplayName: "Test",
		}))

		definition := &cschema.ConnectorDefinition{
			DisplayName: "Test",
		}

		result, err := s.UpdateDraftConnectorGeneration(ctx, id, 1, definition, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "kept", result.GetLabels()["env"])
	})

	t.Run("not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGeneration(gomock.Any(), id, uint64(1)).
			Return(nil, database.ErrNotFound)

		_, err := s.UpdateDraftConnectorGeneration(ctx, id, 1, &cschema.ConnectorDefinition{DisplayName: "Test"}, nil, nil)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("db error getting generation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGeneration(gomock.Any(), id, uint64(1)).
			Return(nil, errors.New("connection refused"))

		_, err := s.UpdateDraftConnectorGeneration(ctx, id, 1, &cschema.ConnectorDefinition{DisplayName: "Test"}, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get connector generation")
	})

	t.Run("not draft", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGeneration(gomock.Any(), id, uint64(1)).
			Return(&database.ConnectorWithDefinition{
				Id:         id,
				Generation: 1,
				State:      database.ConnectorGenerationStatePrimary,
			}, nil)

		_, err := s.UpdateDraftConnectorGeneration(ctx, id, 1, &cschema.ConnectorDefinition{DisplayName: "Test"}, nil, nil)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNotDraft)
	})

	t.Run("upsert error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGeneration(gomock.Any(), id, uint64(1)).
			Return(&database.ConnectorWithDefinition{
				Id:         id,
				Generation: 1,
				Namespace:  "root",
				State:      database.ConnectorGenerationStateDraft,
			}, nil)

		e.EXPECT().
			EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(encfield.EncryptedField{ID: "dek_test", Data: "encrypted"}, nil)

		db.EXPECT().
			UpsertConnectorGeneration(gomock.Any(), gomock.Any()).
			Return(errors.New("write failed"))

		_, err := s.UpdateDraftConnectorGeneration(ctx, id, 1, &cschema.ConnectorDefinition{DisplayName: "Test"}, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to upsert connector generation")
	})
}

func TestGetOrCreateDraftConnectorGeneration(t *testing.T) {
	t.Run("returns existing draft", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := apid.MustParse("cxr_testcccccccccccc")
		ctx := context.Background()
		encryptedDef := encfield.EncryptedField{ID: "dek_test", Data: "existing-encrypted-def"}

		existingDef := &cschema.ConnectorDefinition{
			DisplayName: "Existing Draft",
		}
		defJson, _ := json.Marshal(existingDef)

		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(&database.ConnectorWithDefinition{
				Id:                  id,
				Generation:          2,
				Namespace:           "root",
				State:               database.ConnectorGenerationStateDraft,
				EncryptedDefinition: encryptedDef,
			}, nil)

		// Decrypt to verify definition loads
		e.EXPECT().
			DecryptString(gomock.Any(), encryptedDef).
			Return(string(defJson), nil)

		result, err := s.GetOrCreateDraftConnectorGeneration(ctx, id)
		require.NoError(t, err)
		require.Equal(t, id, result.GetId())
		require.Equal(t, uint64(2), result.GetGeneration())
		require.Equal(t, database.ConnectorGenerationStateDraft, result.GetState())
	})

	t.Run("creates new draft from latest", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := apid.MustParse("cxr_testdddddddddddd")
		ctx := context.Background()
		encryptedDef := encfield.EncryptedField{ID: "dek_test", Data: "latest-encrypted-def"}

		latestDef := &cschema.ConnectorDefinition{
			DisplayName: "Latest Generation",
		}
		latestDefJson, _ := json.Marshal(latestDef)

		// No existing draft
		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(nil, database.ErrNotFound)

		// Latest generation
		db.EXPECT().
			NewestConnectorGenerationForId(gomock.Any(), id).
			Return(&database.ConnectorWithDefinition{
				Id:                  id,
				Generation:          3,
				Namespace:           "root",
				State:               database.ConnectorGenerationStatePrimary,
				Labels:              map[string]string{"env": "prod"},
				EncryptedDefinition: encryptedDef,
			}, nil)

		// Decrypt latest definition
		e.EXPECT().
			DecryptString(gomock.Any(), encryptedDef).
			Return(string(latestDefJson), nil)

		// Build encrypts the new generation
		e.EXPECT().
			EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(encfield.EncryptedField{ID: "dek_test", Data: "new-encrypted-def"}, nil)

		// Upsert
		db.EXPECT().
			UpsertConnectorGeneration(gomock.Any(), gomock.Any()).
			Return(nil)

		// Re-fetch
		mock.MockConnectorRetrival(ctx, db, e, connectorResourceForMock(id, 4, database.ConnectorGenerationStateDraft, map[string]string{"env": "prod"}, cschema.ConnectorDefinition{
			DisplayName: "Latest Generation",
		}))

		result, err := s.GetOrCreateDraftConnectorGeneration(ctx, id)
		require.NoError(t, err)
		require.Equal(t, id, result.GetId())
		require.Equal(t, uint64(4), result.GetGeneration())
		require.Equal(t, database.ConnectorGenerationStateDraft, result.GetState())
		require.Equal(t, "prod", result.GetLabels()["env"])
	})

	t.Run("db error checking for draft", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(nil, errors.New("connection refused"))

		_, err := s.GetOrCreateDraftConnectorGeneration(ctx, id)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to check for existing draft")
	})

	t.Run("connector not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(nil, database.ErrNotFound)

		db.EXPECT().
			NewestConnectorGenerationForId(gomock.Any(), id).
			Return(nil, database.ErrNotFound)

		_, err := s.GetOrCreateDraftConnectorGeneration(ctx, id)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("db error getting newest generation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(nil, database.ErrNotFound)

		db.EXPECT().
			NewestConnectorGenerationForId(gomock.Any(), id).
			Return(nil, errors.New("timeout"))

		_, err := s.GetOrCreateDraftConnectorGeneration(ctx, id)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get latest connector generation")
	})

	t.Run("upsert error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()
		encryptedDef := encfield.EncryptedField{ID: "dek_test", Data: "latest-encrypted-def"}

		latestDef := &cschema.ConnectorDefinition{
			DisplayName: "Test",
		}
		latestDefJson, _ := json.Marshal(latestDef)

		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(nil, database.ErrNotFound)

		db.EXPECT().
			NewestConnectorGenerationForId(gomock.Any(), id).
			Return(&database.ConnectorWithDefinition{
				Id:                  id,
				Generation:          1,
				Namespace:           "root",
				State:               database.ConnectorGenerationStatePrimary,
				EncryptedDefinition: encryptedDef,
			}, nil)

		e.EXPECT().
			DecryptString(gomock.Any(), encryptedDef).
			Return(string(latestDefJson), nil)

		e.EXPECT().
			EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(encfield.EncryptedField{ID: "dek_test", Data: "new-encrypted"}, nil)

		db.EXPECT().
			UpsertConnectorGeneration(gomock.Any(), gomock.Any()).
			Return(errors.New("constraint violation"))

		_, err := s.GetOrCreateDraftConnectorGeneration(ctx, id)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to upsert connector generation")
	})
}
