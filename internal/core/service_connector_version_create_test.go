package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/require"
)

func TestCreateConnectorVersion(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		fixedId := apid.MustParse("cxr_testaaaaaaaaaaaa")
		ctx := apctx.WithFixedIdGenerator(context.Background(), fixedId)

		definition := &cschema.Connector{
			DisplayName: "New Connector",
			Description: "A new connector",
		}
		labels := map[string]string{"env": "test"}

		// Build step encrypts the definition
		e.EXPECT().
			EncryptStringForConnector(gomock.Any(), gomock.Any(), gomock.Any()).
			Return("encrypted-def", nil)

		// Upsert is called
		db.EXPECT().
			UpsertConnectorVersion(gomock.Any(), gomock.Any()).
			Return(nil)

		// Re-fetch after upsert
		mock.MockConnectorRetrival(ctx, db, e, &cschema.Connector{
			Id:          fixedId,
			Version:     1,
			State:       string(database.ConnectorVersionStateDraft),
			DisplayName: "New Connector",
			Description: "A new connector",
			Labels:      labels,
		})

		result, err := s.CreateConnectorVersion(ctx, "root", definition, labels)
		require.NoError(t, err)
		require.Equal(t, fixedId, result.GetId())
		require.Equal(t, uint64(1), result.GetVersion())
		require.Equal(t, database.ConnectorVersionStateDraft, result.GetState())
		require.Equal(t, "test", result.GetLabels()["env"])
	})

	t.Run("upsert error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		fixedId := apid.MustParse("cxr_testbbbbbbbbbbbb")
		ctx := apctx.WithFixedIdGenerator(context.Background(), fixedId)

		definition := &cschema.Connector{
			DisplayName: "Test",
		}

		e.EXPECT().
			EncryptStringForConnector(gomock.Any(), gomock.Any(), gomock.Any()).
			Return("encrypted-def", nil)

		db.EXPECT().
			UpsertConnectorVersion(gomock.Any(), gomock.Any()).
			Return(errors.New("db write failed"))

		_, err := s.CreateConnectorVersion(ctx, "root", definition, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to upsert connector version")
	})
}

func TestCreateDraftConnectorVersion(t *testing.T) {
	t.Run("success with provided definition", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := apid.MustParse("cxr_testcccccccccccc")
		ctx := context.Background()

		// No existing draft
		db.EXPECT().
			GetConnectorVersionForState(gomock.Any(), id, database.ConnectorVersionStateDraft).
			Return(nil, database.ErrNotFound)

		// Latest version is version 1
		db.EXPECT().
			NewestConnectorVersionForId(gomock.Any(), id).
			Return(&database.ConnectorVersion{
				Id:        id,
				Version:   1,
				Namespace: "root",
				State:     database.ConnectorVersionStatePrimary,
				Labels:    map[string]string{"type": "test"},
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
		newLabels := map[string]string{"env": "staging"}
		mock.MockConnectorRetrival(ctx, db, e, &cschema.Connector{
			Id:          id,
			Version:     2,
			State:       string(database.ConnectorVersionStateDraft),
			DisplayName: "Updated Def",
			Labels:      newLabels,
		})

		definition := &cschema.Connector{
			DisplayName: "Updated Def",
		}

		result, err := s.CreateDraftConnectorVersion(ctx, id, definition, newLabels)
		require.NoError(t, err)
		require.Equal(t, id, result.GetId())
		require.Equal(t, uint64(2), result.GetVersion())
		require.Equal(t, database.ConnectorVersionStateDraft, result.GetState())
	})

	t.Run("success with nil definition clones latest", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := apid.MustParse("cxr_testdddddddddddd")
		ctx := context.Background()
		encryptedDef := "latest-encrypted-def"

		// No existing draft
		db.EXPECT().
			GetConnectorVersionForState(gomock.Any(), id, database.ConnectorVersionStateDraft).
			Return(nil, database.ErrNotFound)

		latestDef := &cschema.Connector{
			Id:          id,
			Version:     3,
			DisplayName: "Latest Connector",
			Labels:      map[string]string{"type": "original"},
		}

		// Latest version
		db.EXPECT().
			NewestConnectorVersionForId(gomock.Any(), id).
			Return(&database.ConnectorVersion{
				Id:                  id,
				Version:             3,
				Namespace:           "root.child",
				State:               database.ConnectorVersionStatePrimary,
				Labels:              map[string]string{"type": "original"},
				EncryptedDefinition: encryptedDef,
			}, nil)

		// Decrypt latest definition
		latestDefJson, _ := json.Marshal(latestDef)
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
			DisplayName: "Latest Connector",
			Labels:      map[string]string{"type": "original"},
		})

		result, err := s.CreateDraftConnectorVersion(ctx, id, nil, nil)
		require.NoError(t, err)
		require.Equal(t, id, result.GetId())
		require.Equal(t, uint64(4), result.GetVersion())
		require.Equal(t, "Latest Connector", result.GetDefinition().DisplayName)
		require.Equal(t, "original", result.GetLabels()["type"])
	})

	t.Run("draft already exists", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.MustParse("cxr_testeeeeeeeeeeee")
		ctx := context.Background()

		db.EXPECT().
			GetConnectorVersionForState(gomock.Any(), id, database.ConnectorVersionStateDraft).
			Return(&database.ConnectorVersion{
				Id:      id,
				Version: 2,
				State:   database.ConnectorVersionStateDraft,
			}, nil)

		_, err := s.CreateDraftConnectorVersion(ctx, id, nil, nil)
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
			GetConnectorVersionForState(gomock.Any(), id, database.ConnectorVersionStateDraft).
			Return(nil, database.ErrNotFound)

		// No versions at all
		db.EXPECT().
			NewestConnectorVersionForId(gomock.Any(), id).
			Return(nil, database.ErrNotFound)

		_, err := s.CreateDraftConnectorVersion(ctx, id, nil, nil)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("db error checking for draft", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorVersionForState(gomock.Any(), id, database.ConnectorVersionStateDraft).
			Return(nil, errors.New("connection refused"))

		_, err := s.CreateDraftConnectorVersion(ctx, id, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to check for existing draft")
	})

	t.Run("db error getting newest version", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorVersionForState(gomock.Any(), id, database.ConnectorVersionStateDraft).
			Return(nil, database.ErrNotFound)

		db.EXPECT().
			NewestConnectorVersionForId(gomock.Any(), id).
			Return(nil, errors.New("timeout"))

		_, err := s.CreateDraftConnectorVersion(ctx, id, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get latest connector version")
	})

	t.Run("upsert error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorVersionForState(gomock.Any(), id, database.ConnectorVersionStateDraft).
			Return(nil, database.ErrNotFound)

		db.EXPECT().
			NewestConnectorVersionForId(gomock.Any(), id).
			Return(&database.ConnectorVersion{
				Id:        id,
				Version:   1,
				Namespace: "root",
				State:     database.ConnectorVersionStatePrimary,
			}, nil)

		e.EXPECT().
			EncryptStringForConnector(gomock.Any(), gomock.Any(), gomock.Any()).
			Return("encrypted", nil)

		db.EXPECT().
			UpsertConnectorVersion(gomock.Any(), gomock.Any()).
			Return(errors.New("constraint violation"))

		_, err := s.CreateDraftConnectorVersion(ctx, id, &cschema.Connector{DisplayName: "Test"}, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to upsert connector version")
	})
}
