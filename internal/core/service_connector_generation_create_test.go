package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

func connectorResourceForMock(id apid.ID, generation uint64, state database.ConnectorGenerationState, labels map[string]string, definition cschema.ConnectorDefinition) *cschema.Connector {
	return &cschema.Connector{
		TypeMeta: meta.NewTypeMeta(cschema.ConnectorKind),
		Metadata: meta.ObjectMeta{
			ID:         id.String(),
			Namespace:  "root",
			Generation: generation,
			Labels:     labels,
		},
		Spec: cschema.ConnectorSpec{
			Release:    cschema.ConnectorReleaseSpec{DesiredState: cschema.ConnectorReleaseState(state)},
			Definition: definition,
		},
	}
}

func TestCreateConnector(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		fixedId := apid.MustParse("cxr_testaaaaaaaaaaaa")
		ctx := apctx.WithFixedIdGenerator(context.Background(), fixedId)

		definition := &cschema.ConnectorDefinition{
			DisplayName: "New Connector",
			Description: "A new connector",
		}
		labels := map[string]string{"env": "test"}

		// Build step encrypts the definition
		e.EXPECT().
			EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(encfield.EncryptedField{ID: "dek_test", Data: "encrypted-def"}, nil)

		// Upsert is called
		db.EXPECT().
			UpsertConnectorGeneration(gomock.Any(), gomock.Any()).
			Return(nil)

		// Re-fetch after upsert
		mock.MockConnectorRetrival(ctx, db, e, connectorResourceForMock(fixedId, 1, database.ConnectorGenerationStateDraft, labels, cschema.ConnectorDefinition{
			DisplayName: "New Connector",
			Description: "A new connector",
		}))

		resource := cschema.NewConnector()
		resource.Metadata.Namespace = "root"
		resource.Metadata.Labels = labels
		resource.Spec.Definition = *definition

		result, err := s.CreateConnector(ctx, resource)
		require.NoError(t, err)
		require.Equal(t, fixedId, result.GetId())
		require.Equal(t, uint64(1), result.GetGeneration())
		require.Equal(t, database.ConnectorGenerationStateDraft, result.GetState())
		require.Equal(t, "test", result.GetLabels()["env"])
	})

	t.Run("upsert error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		fixedId := apid.MustParse("cxr_testbbbbbbbbbbbb")
		ctx := apctx.WithFixedIdGenerator(context.Background(), fixedId)

		definition := &cschema.ConnectorDefinition{
			DisplayName: "Test",
		}

		e.EXPECT().
			EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(encfield.EncryptedField{ID: "dek_test", Data: "encrypted-def"}, nil)

		db.EXPECT().
			UpsertConnectorGeneration(gomock.Any(), gomock.Any()).
			Return(errors.New("db write failed"))

		resource := cschema.NewConnector()
		resource.Metadata.Namespace = "root"
		resource.Spec.Definition = *definition

		_, err := s.CreateConnector(ctx, resource)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to upsert connector generation")
	})
}

func TestUpdateConnectorName(t *testing.T) {
	t.Run("enqueues connection label propagation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, ac, _ := FullMockService(t, ctrl)
		id := apid.MustParse("cxr_testrename000001")

		db.EXPECT().UpdateConnectorName(gomock.Any(), id, scommon.ResourceName("renamed")).Return(nil)
		ac.EXPECT().EnqueueContext(gomock.Any(), gomock.Any()).Return(nil, nil)

		require.NoError(t, s.UpdateConnectorName(context.Background(), id, "renamed"))
	})

	t.Run("does not enqueue after database failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		id := apid.MustParse("cxr_testrename000002")

		db.EXPECT().UpdateConnectorName(gomock.Any(), id, scommon.ResourceName("renamed")).Return(database.ErrNotFound)

		require.ErrorIs(t, s.UpdateConnectorName(context.Background(), id, "renamed"), ErrNotFound)
	})
}

func TestCreateDraftConnectorGeneration(t *testing.T) {
	t.Run("success with provided definition", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := apid.MustParse("cxr_testcccccccccccc")
		ctx := context.Background()

		// No existing draft
		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(nil, database.ErrNotFound)

		// Latest generation is generation 1
		db.EXPECT().
			NewestConnectorGenerationForId(gomock.Any(), id).
			Return(&database.ConnectorWithDefinition{
				Id:         id,
				Generation: 1,
				Namespace:  "root",
				State:      database.ConnectorGenerationStatePrimary,
				Labels:     map[string]string{"type": "test"},
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
		newLabels := map[string]string{"env": "staging"}
		mock.MockConnectorRetrival(ctx, db, e, connectorResourceForMock(id, 2, database.ConnectorGenerationStateDraft, newLabels, cschema.ConnectorDefinition{
			DisplayName: "Updated Def",
		}))

		definition := &cschema.ConnectorDefinition{
			DisplayName: "Updated Def",
		}

		result, err := s.CreateDraftConnectorGeneration(ctx, id, definition, newLabels, nil)
		require.NoError(t, err)
		require.Equal(t, id, result.GetId())
		require.Equal(t, uint64(2), result.GetGeneration())
		require.Equal(t, database.ConnectorGenerationStateDraft, result.GetState())
	})

	t.Run("success with nil definition clones latest", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := apid.MustParse("cxr_testdddddddddddd")
		ctx := context.Background()
		encryptedDef := encfield.EncryptedField{ID: "dek_test", Data: "latest-encrypted-def"}

		// No existing draft
		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(nil, database.ErrNotFound)

		latestDef := &cschema.ConnectorDefinition{
			DisplayName: "Latest Connector",
		}

		// Latest generation
		db.EXPECT().
			NewestConnectorGenerationForId(gomock.Any(), id).
			Return(&database.ConnectorWithDefinition{
				Id:                  id,
				Generation:          3,
				Namespace:           "root.child",
				State:               database.ConnectorGenerationStatePrimary,
				Labels:              map[string]string{"type": "original"},
				EncryptedDefinition: encryptedDef,
			}, nil)

		// Decrypt latest definition
		latestDefJson, _ := json.Marshal(latestDef)
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
		mock.MockConnectorRetrival(ctx, db, e, connectorResourceForMock(id, 4, database.ConnectorGenerationStateDraft, map[string]string{"type": "original"}, cschema.ConnectorDefinition{
			DisplayName: "Latest Connector",
		}))

		result, err := s.CreateDraftConnectorGeneration(ctx, id, nil, nil, nil)
		require.NoError(t, err)
		require.Equal(t, id, result.GetId())
		require.Equal(t, uint64(4), result.GetGeneration())
		require.Equal(t, "Latest Connector", result.GetDefinition().DisplayName)
		require.Equal(t, "original", result.GetLabels()["type"])
	})

	t.Run("draft already exists", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.MustParse("cxr_testeeeeeeeeeeee")
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(&database.ConnectorWithDefinition{
				Id:         id,
				Generation: 2,
				State:      database.ConnectorGenerationStateDraft,
			}, nil)

		_, err := s.CreateDraftConnectorGeneration(ctx, id, nil, nil, nil)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrDraftAlreadyExists)
	})

	t.Run("connector not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.MustParse("cxr_testffffffffffff")
		ctx := context.Background()

		// No draft
		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(nil, database.ErrNotFound)

		// No generations at all
		db.EXPECT().
			NewestConnectorGenerationForId(gomock.Any(), id).
			Return(nil, database.ErrNotFound)

		_, err := s.CreateDraftConnectorGeneration(ctx, id, nil, nil, nil)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("db error checking for draft", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(nil, errors.New("connection refused"))

		_, err := s.CreateDraftConnectorGeneration(ctx, id, nil, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to check for existing draft")
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

		_, err := s.CreateDraftConnectorGeneration(ctx, id, nil, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get latest connector generation")
	})

	t.Run("upsert error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(nil, database.ErrNotFound)

		db.EXPECT().
			NewestConnectorGenerationForId(gomock.Any(), id).
			Return(&database.ConnectorWithDefinition{
				Id:         id,
				Generation: 1,
				Namespace:  "root",
				State:      database.ConnectorGenerationStatePrimary,
			}, nil)

		e.EXPECT().
			EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(encfield.EncryptedField{ID: "dek_test", Data: "encrypted"}, nil)

		db.EXPECT().
			UpsertConnectorGeneration(gomock.Any(), gomock.Any()).
			Return(errors.New("constraint violation"))

		_, err := s.CreateDraftConnectorGeneration(ctx, id, &cschema.ConnectorDefinition{DisplayName: "Test"}, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to upsert connector generation")
	})
}
