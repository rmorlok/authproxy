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
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	namespaceschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	ratelimitschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
)

func coreReference(kind meta.Kind, id string) meta.ObjectReference {
	return meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       kind,
		ID:         id,
	}
}

func TestResolveObjectReferenceWrapsResources(t *testing.T) {
	ctx := context.Background()

	t.Run("actor", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		actor := &database.Actor{
			Id:         apid.New(apid.PrefixActor),
			Namespace:  "root.team",
			Name:       "operator",
			ExternalId: "operator@example.com",
		}
		ref := coreReference(actorschema.ActorKind, actor.Id.String())
		db.EXPECT().ResolveActorReference(ctx, ref).Return(actor, nil)

		resolved, err := s.ResolveActorReference(ctx, ref)
		require.NoError(t, err)
		require.Equal(t, actor.Id, resolved.GetId())
		require.Equal(t, actor.Name, resolved.GetName())
	})

	t.Run("key", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		key := &database.Key{
			Id:        apid.New(apid.PrefixKey),
			Namespace: "root.team",
			Name:      "primary",
		}
		ref := coreReference(keyschema.KeyKind, key.Id.String())
		db.EXPECT().ResolveKeyReference(ctx, ref).Return(key, nil)

		resolved, err := s.ResolveKeyReference(ctx, ref)
		require.NoError(t, err)
		require.Equal(t, key.Id, resolved.GetId())
		require.Equal(t, key.Name, resolved.GetName())
	})

	t.Run("namespace", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		namespace := &database.Namespace{Path: "root.team"}
		ref := coreReference(namespaceschema.NamespaceKind, namespace.Path)
		db.EXPECT().ResolveNamespaceReference(ctx, ref).Return(namespace, nil)

		resolved, err := s.ResolveNamespaceReference(ctx, ref)
		require.NoError(t, err)
		require.Equal(t, namespace.Path, resolved.GetPath())
		require.Equal(t, "team", string(resolved.GetName()))
	})

	t.Run("rate limit", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		rateLimit := &database.RateLimit{
			Id:        apid.New(apid.PrefixRateLimit),
			Namespace: "root.team",
			Name:      "api-budget",
		}
		ref := coreReference(ratelimitschema.RateLimitKind, rateLimit.Id.String())
		db.EXPECT().ResolveRateLimitReference(ctx, ref).Return(rateLimit, nil)

		resolved, err := s.ResolveRateLimitReference(ctx, ref)
		require.NoError(t, err)
		require.Equal(t, rateLimit.Id, resolved.GetId())
		require.Equal(t, rateLimit.Name, resolved.GetName())
	})
}

func TestResolveConnectorReferenceHydratesDefinitionVersion(t *testing.T) {
	ctx := context.Background()
	connectorID := apid.New(apid.PrefixConnector)
	logicalConnector := &database.Connector{
		Id:        connectorID,
		Namespace: "root.team",
		Name:      "billing",
	}
	definition := connectorschema.ConnectorDefinition{
		DisplayName: "Billing",
		Auth:        connectorschema.NewNoAuth(),
	}

	t.Run("explicit generation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, encrypt := FullMockService(t, ctrl)
		ref := coreReference(connectorschema.ConnectorKind, connectorID.String())
		ref.Generation = 2
		db.EXPECT().ResolveConnectorReference(ctx, ref).Return(logicalConnector, nil)
		mock.MockConnectorRetrival(
			ctx,
			db,
			encrypt,
			connectorResourceForMock(
				connectorID,
				2,
				database.ConnectorDefinitionVersionStateActive,
				nil,
				definition,
			),
		)

		resolved, err := s.ResolveConnectorReference(ctx, ref)
		require.NoError(t, err)
		require.Equal(t, uint64(2), resolved.GetVersion())
		require.Equal(t, "Billing", resolved.GetDefinition().DisplayName)
	})

	t.Run("omitted generation uses newest", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, encrypt := FullMockService(t, ctrl)
		ref := coreReference(connectorschema.ConnectorKind, connectorID.String())
		encryptedDefinition := encfield.EncryptedField{ID: "dek_test", Data: "encrypted"}
		definitionJSON, err := json.Marshal(definition)
		require.NoError(t, err)

		db.EXPECT().ResolveConnectorReference(ctx, ref).Return(logicalConnector, nil)
		db.EXPECT().NewestConnectorDefinitionVersionForId(ctx, connectorID).Return(
			&database.ConnectorWithDefinition{
				Id:                  connectorID,
				Namespace:           logicalConnector.Namespace,
				Name:                logicalConnector.Name,
				Version:             3,
				State:               database.ConnectorDefinitionVersionStateDraft,
				EncryptedDefinition: encryptedDefinition,
			},
			nil,
		)
		encrypt.EXPECT().DecryptString(gomock.Any(), encryptedDefinition).Return(string(definitionJSON), nil)

		resolved, err := s.ResolveConnectorReference(ctx, ref)
		require.NoError(t, err)
		require.Equal(t, uint64(3), resolved.GetVersion())
		require.Equal(t, logicalConnector.Name, resolved.GetName())
	})
}

func TestResolveConnectionReferenceHydratesPinnedConnector(t *testing.T) {
	ctrl := gomock.NewController(t)
	s, db, _, _, _, encrypt := FullMockService(t, ctrl)
	ctx := context.Background()
	connectorID := apid.New(apid.PrefixConnector)
	connection := &database.Connection{
		Id:               apid.New(apid.PrefixConnection),
		Namespace:        "root.team",
		Name:             "production",
		ConnectorId:      connectorID,
		ConnectorVersion: 4,
	}
	ref := coreReference(connectionschema.ConnectionKind, connection.Id.String())
	db.EXPECT().ResolveConnectionReference(ctx, ref).Return(connection, nil)
	mock.MockConnectorRetrival(
		ctx,
		db,
		encrypt,
		connectorResourceForMock(
			connectorID,
			4,
			database.ConnectorDefinitionVersionStatePrimary,
			nil,
			connectorschema.ConnectorDefinition{
				DisplayName: "Billing",
				Auth:        connectorschema.NewNoAuth(),
			},
		),
	)

	resolved, err := s.ResolveConnectionReference(ctx, ref)
	require.NoError(t, err)
	require.Equal(t, connection.Id, resolved.GetId())
	require.Equal(t, connectorID, resolved.GetConnector().GetId())
	require.Equal(t, uint64(4), resolved.GetConnector().GetVersion())
}

func TestResolveObjectReferenceNormalizesDatabaseErrors(t *testing.T) {
	ctx := context.Background()
	ref := coreReference(keyschema.KeyKind, apid.New(apid.PrefixKey).String())

	t.Run("not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		db.EXPECT().ResolveKeyReference(ctx, ref).Return(nil, database.ErrNotFound)

		resolved, err := s.ResolveKeyReference(ctx, ref)
		require.Nil(t, resolved)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("invalid reference", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		db.EXPECT().ResolveKeyReference(ctx, ref).Return(nil, database.ErrInvalidReference)

		resolved, err := s.ResolveKeyReference(ctx, ref)
		require.Nil(t, resolved)
		require.ErrorIs(t, err, ErrInvalidArgument)
	})
}

func TestResolveObjectReferencePropagatesUnexpectedDatabaseErrors(t *testing.T) {
	ctx := context.Background()
	databaseError := errors.New("database unavailable")

	t.Run("actor", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		ref := coreReference(actorschema.ActorKind, apid.New(apid.PrefixActor).String())
		db.EXPECT().ResolveActorReference(ctx, ref).Return(nil, databaseError)

		resolved, err := s.ResolveActorReference(ctx, ref)
		require.Nil(t, resolved)
		require.ErrorIs(t, err, databaseError)
	})

	t.Run("connection", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		ref := coreReference(connectionschema.ConnectionKind, apid.New(apid.PrefixConnection).String())
		db.EXPECT().ResolveConnectionReference(ctx, ref).Return(nil, databaseError)

		resolved, err := s.ResolveConnectionReference(ctx, ref)
		require.Nil(t, resolved)
		require.ErrorIs(t, err, databaseError)
	})

	t.Run("connector", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		ref := coreReference(connectorschema.ConnectorKind, apid.New(apid.PrefixConnector).String())
		db.EXPECT().ResolveConnectorReference(ctx, ref).Return(nil, databaseError)

		resolved, err := s.ResolveConnectorReference(ctx, ref)
		require.Nil(t, resolved)
		require.ErrorIs(t, err, databaseError)
	})

	t.Run("namespace", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		ref := coreReference(namespaceschema.NamespaceKind, "root.team")
		db.EXPECT().ResolveNamespaceReference(ctx, ref).Return(nil, databaseError)

		resolved, err := s.ResolveNamespaceReference(ctx, ref)
		require.Nil(t, resolved)
		require.ErrorIs(t, err, databaseError)
	})

	t.Run("rate limit", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		ref := coreReference(ratelimitschema.RateLimitKind, apid.New(apid.PrefixRateLimit).String())
		db.EXPECT().ResolveRateLimitReference(ctx, ref).Return(nil, databaseError)

		resolved, err := s.ResolveRateLimitReference(ctx, ref)
		require.Nil(t, resolved)
		require.ErrorIs(t, err, databaseError)
	})
}

func TestResolveConnectorReferenceReturnsNotFoundWithoutDefinitionVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	s, db, _, _, _, _ := FullMockService(t, ctrl)
	ctx := context.Background()
	connector := &database.Connector{Id: apid.New(apid.PrefixConnector)}
	ref := coreReference(connectorschema.ConnectorKind, connector.Id.String())
	db.EXPECT().ResolveConnectorReference(ctx, ref).Return(connector, nil)
	db.EXPECT().NewestConnectorDefinitionVersionForId(ctx, connector.Id).Return(nil, database.ErrNotFound)

	resolved, err := s.ResolveConnectorReference(ctx, ref)
	require.Nil(t, resolved)
	require.ErrorIs(t, err, ErrNotFound)
}
