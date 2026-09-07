package core

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	authschema "github.com/rmorlok/authproxy/internal/schema/auth"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

func actorServiceTestResource() *actorschema.Actor {
	resource := actorschema.NewActor()
	resource.Metadata.Namespace = "root.acme"
	resource.Metadata.Name = "billing"
	resource.Metadata.Labels = map[string]string{"team": "platform"}
	resource.Metadata.Annotations = map[string]string{"owner": "alice"}
	resource.Spec.ExternalId = "user-123"
	resource.Spec.Permissions = []authschema.Permission{{
		Namespace: "root.acme.**",
		Resources: []string{"connections"},
		Verbs:     []string{"read"},
	}}
	return resource
}

func actorServiceTestDatabaseActor() *database.Actor {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	return &database.Actor{
		Id:          apid.New(apid.PrefixActor),
		Namespace:   "root.acme",
		Name:        "billing",
		ExternalId:  "user-123",
		Permissions: database.Permissions(actorServiceTestResource().Spec.Permissions),
		Labels:      database.Labels{"team": "platform"},
		Annotations: database.Annotations{"owner": "alice"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestActorResourceFromDatabaseRedactsSigningKey(t *testing.T) {
	actor := actorServiceTestDatabaseActor()
	actor.EncryptedKey = &encfield.EncryptedField{
		ID:   apid.New(apid.PrefixDataEncryptionKey),
		Data: "ciphertext",
	}

	resource := actorResourceFromDatabase(*actor)
	require.Equal(t, meta.APIVersionV1Alpha1, resource.APIVersion)
	require.Equal(t, actorschema.ActorKind, resource.Kind)
	require.Equal(t, actor.Id.String(), resource.Metadata.ID)
	require.Equal(t, actor.ExternalId, resource.Spec.ExternalId)
	require.Nil(t, resource.Spec.SigningKey)
	require.True(t, resource.Status.SigningKeyConfigured)
	require.NoError(t, resource.ValidateFor(meta.ValidationModeResponse, nil))

	resource.Metadata.Labels["team"] = "changed"
	resource.Spec.Permissions[0].Verbs[0] = "delete"
	require.Equal(t, "platform", actor.Labels["team"])
	require.Equal(t, "read", actor.Permissions[0].Verbs[0])
}

func TestDatabaseActorFromResourceExcludesSystemLabels(t *testing.T) {
	resource := actorServiceTestResource()
	resource.Metadata.Labels["apxy/act/-/name"] = "stale-name"

	actor, err := databaseActorFromResource(resource, apid.New(apid.PrefixActor), nil)
	require.NoError(t, err)
	require.Equal(t, database.Labels{"team": "platform"}, actor.Labels)
}

func TestCreateActorEncryptsWriteOnlySigningKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	service, db, _, _, _, encryption := FullMockService(t, ctrl)
	ctx := context.Background()
	resource := actorServiceTestResource()
	resource.Spec.SigningKey = &keyschema.SigningKey{InnerVal: &keyschema.KeyShared{
		SharedKey: &keyschema.KeyData{InnerVal: &keyschema.KeyDataValue{Value: "secret"}},
	}}
	encrypted := encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "ciphertext"}

	encryption.EXPECT().EncryptStringForNamespace(ctx, resource.Metadata.Namespace, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, data string) (encfield.EncryptedField, error) {
			var decoded keyschema.SigningKey
			require.NoError(t, json.Unmarshal([]byte(data), &decoded))
			require.True(t, decoded.CanSign())
			return encrypted, nil
		},
	)
	var stored *database.Actor
	db.EXPECT().CreateActor(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, actor *database.Actor) error {
		stored = actor
		return nil
	})
	db.EXPECT().GetActor(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, id apid.ID) (*database.Actor, error) {
		result := *stored
		result.Id = id
		result.CreatedAt = time.Now().UTC()
		result.UpdatedAt = result.CreatedAt
		return &result, nil
	})

	created, err := service.CreateActor(ctx, resource)
	require.NoError(t, err)
	require.Equal(t, encrypted, *stored.EncryptedKey)
	require.Nil(t, created.GetResource().Spec.SigningKey)
	require.True(t, created.GetResource().Status.SigningKeyConfigured)
	require.NotEqual(t, resource, created.GetResource())
}

func TestUpdateActorSigningKeyPresence(t *testing.T) {
	ctx := context.Background()

	t.Run("omitted preserves encrypted key", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		service, db, _, _, _, _ := FullMockService(t, ctrl)
		existing := actorServiceTestDatabaseActor()
		existing.EncryptedKey = &encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "old"}
		patch := actorschema.NewActorPatch()
		labels := map[string]string{"team": "security"}
		patch.Metadata.Labels = &labels

		db.EXPECT().GetActor(ctx, existing.Id).Return(existing, nil)
		db.EXPECT().UpsertActor(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, data database.IActorData) (*database.Actor, error) {
			updated := data.(*database.Actor)
			require.Equal(t, existing.EncryptedKey, updated.EncryptedKey)
			require.Equal(t, "security", updated.Labels["team"])
			updated.CreatedAt = existing.CreatedAt
			updated.UpdatedAt = existing.UpdatedAt.Add(time.Second)
			return updated, nil
		})

		updated, err := service.UpdateActor(ctx, existing.Id, patch)
		require.NoError(t, err)
		require.True(t, updated.GetResource().Status.SigningKeyConfigured)
	})

	t.Run("explicit null clears encrypted key", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		service, db, _, _, _, _ := FullMockService(t, ctrl)
		existing := actorServiceTestDatabaseActor()
		existing.EncryptedKey = &encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "old"}
		patch := actorschema.NewActorPatch()
		patch.Spec.SetSigningKey(nil)

		db.EXPECT().GetActor(ctx, existing.Id).Return(existing, nil)
		db.EXPECT().UpsertActor(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, data database.IActorData) (*database.Actor, error) {
			updated := data.(*database.Actor)
			require.Nil(t, updated.EncryptedKey)
			updated.CreatedAt = existing.CreatedAt
			updated.UpdatedAt = existing.UpdatedAt.Add(time.Second)
			return updated, nil
		})

		updated, err := service.UpdateActor(ctx, existing.Id, patch)
		require.NoError(t, err)
		require.False(t, updated.GetResource().Status.SigningKeyConfigured)
	})

	t.Run("replacement uses the actor namespace key", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		service, db, _, _, _, encryption := FullMockService(t, ctrl)
		existing := actorServiceTestDatabaseActor()
		existing.EncryptedKey = &encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "old"}
		patch := actorschema.NewActorPatch()
		patch.Spec.SetSigningKey(&keyschema.SigningKey{InnerVal: &keyschema.KeyShared{
			SharedKey: &keyschema.KeyData{InnerVal: &keyschema.KeyDataValue{Value: "replacement"}},
		}})
		replacement := encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "new"}

		db.EXPECT().GetActor(ctx, existing.Id).Return(existing, nil)
		encryption.EXPECT().EncryptStringForNamespace(ctx, existing.Namespace, gomock.Any()).Return(replacement, nil)
		db.EXPECT().UpsertActor(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, data database.IActorData) (*database.Actor, error) {
			updated := data.(*database.Actor)
			require.Equal(t, &replacement, updated.EncryptedKey)
			updated.CreatedAt = existing.CreatedAt
			updated.UpdatedAt = existing.UpdatedAt.Add(time.Second)
			return updated, nil
		})

		updated, err := service.UpdateActor(ctx, existing.Id, patch)
		require.NoError(t, err)
		require.True(t, updated.GetResource().Status.SigningKeyConfigured)
	})
}

func TestUpdateActorRejectsImmutableExternalID(t *testing.T) {
	ctrl := gomock.NewController(t)
	service, db, _, _, _, _ := FullMockService(t, ctrl)
	ctx := context.Background()
	existing := actorServiceTestDatabaseActor()
	patch := actorschema.NewActorPatch()
	different := "different"
	patch.Spec.ExternalId = &different

	db.EXPECT().GetActor(ctx, existing.Id).Return(existing, nil)
	_, err := service.UpdateActor(ctx, existing.Id, patch)
	require.ErrorIs(t, err, ErrInvalidArgument)
	require.ErrorContains(t, err, "spec.externalId")
}
