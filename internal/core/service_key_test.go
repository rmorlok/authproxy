package core

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
)

type asynqTaskTypeMatcher struct {
	taskType string
}

func (m asynqTaskTypeMatcher) Matches(x any) bool {
	task, ok := x.(*asynq.Task)
	return ok && task.Type() == m.taskType
}

func (m asynqTaskTypeMatcher) String() string {
	return fmt.Sprintf("asynq task type %q", m.taskType)
}

type keyReferenceListBuilder struct {
	database.ListKeysBuilder
	results          []database.Key
	limit            int32
	namespaceMatcher string
	name             scommon.ResourceName
}

func (b *keyReferenceListBuilder) FetchPage(context.Context) pagination.PageResult[database.Key] {
	return pagination.PageResult[database.Key]{Results: b.results}
}

func (b *keyReferenceListBuilder) Limit(limit int32) database.ListKeysBuilder {
	b.limit = limit
	return b
}

func (b *keyReferenceListBuilder) ForNamespaceMatcher(matcher string) database.ListKeysBuilder {
	b.namespaceMatcher = matcher
	return b
}

func (b *keyReferenceListBuilder) ForName(name scommon.ResourceName) database.ListKeysBuilder {
	b.name = name
	return b
}

func TestResolveKeyReference(t *testing.T) {
	ctx := context.Background()
	id := apid.ID("key_first550e8400abcd")
	otherID := apid.ID("key_other550e8400abcd")
	namedReference := meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       nschema.EncryptionKeyKind,
		Namespace:  "root.dev",
		Name:       "primary",
	}

	t.Run("invalid", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, _, _, _, _, _ := FullMockService(t, ctrl)

		key, err := s.ResolveKeyReference(ctx, meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1,
			Kind:       nschema.EncryptionKeyKind,
		})

		require.Nil(t, key)
		require.ErrorIs(t, err, ErrInvalidArgument)
	})

	t.Run("id", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		db.EXPECT().
			GetKey(ctx, id).
			Return(&database.Key{Id: id, Namespace: "root.dev", Name: "primary"}, nil)

		key, err := s.ResolveKeyReference(ctx, meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1,
			Kind:       nschema.EncryptionKeyKind,
			ID:         id.String(),
		})

		require.NoError(t, err)
		require.Equal(t, id, key.GetId())
	})

	t.Run("namespace and name", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		builder := &keyReferenceListBuilder{
			results: []database.Key{{Id: id, Namespace: "root.dev", Name: "primary"}},
		}
		db.EXPECT().ListKeysBuilder().Return(builder)

		key, err := s.ResolveKeyReference(ctx, namedReference)

		require.NoError(t, err)
		require.Equal(t, id, key.GetId())
		require.Equal(t, "root.dev", builder.namespaceMatcher)
		require.Equal(t, scommon.ResourceName("primary"), builder.name)
		require.Equal(t, int32(2), builder.limit)
	})

	t.Run("matching id, namespace, and name", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		db.EXPECT().
			GetKey(ctx, id).
			Return(&database.Key{Id: id, Namespace: "root.dev", Name: "primary"}, nil)
		db.EXPECT().
			ListKeysBuilder().
			Return(&keyReferenceListBuilder{
				results: []database.Key{{Id: id, Namespace: "root.dev", Name: "primary"}},
			})

		reference := namedReference
		reference.ID = id.String()
		key, err := s.ResolveKeyReference(ctx, reference)

		require.NoError(t, err)
		require.Equal(t, id, key.GetId())
	})

	t.Run("mismatched id, namespace, and name", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		db.EXPECT().
			GetKey(ctx, id).
			Return(&database.Key{Id: id, Namespace: "root.dev", Name: "primary"}, nil)
		db.EXPECT().
			ListKeysBuilder().
			Return(&keyReferenceListBuilder{
				results: []database.Key{{Id: otherID, Namespace: "root.dev", Name: "primary"}},
			})

		reference := namedReference
		reference.ID = id.String()
		key, err := s.ResolveKeyReference(ctx, reference)

		require.Nil(t, key)
		require.ErrorIs(t, err, ErrInvalidArgument)
		require.ErrorContains(t, err, "key reference id \"key_first550e8400abcd\" does not match key \"root.dev\"/\"primary\" with id \"key_other550e8400abcd\"")
	})

	t.Run("namespace and name not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		db.EXPECT().
			ListKeysBuilder().
			Return(&keyReferenceListBuilder{})

		key, err := s.ResolveKeyReference(ctx, namedReference)

		require.Nil(t, key)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("id not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		db.EXPECT().
			GetKey(ctx, id).
			Return(nil, database.ErrNotFound)

		key, err := s.ResolveKeyReference(ctx, meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1,
			Kind:       nschema.EncryptionKeyKind,
			ID:         id.String(),
		})

		require.Nil(t, key)
		require.ErrorIs(t, err, ErrNotFound)
	})
}

func TestCreateKeyEnqueuesDEKGeneration(t *testing.T) {
	ctrl := gomock.NewController(t)
	s, db, redisClient, _, asynqClient, enc := FullMockService(t, ctrl)
	ctx := context.Background()

	keyData := &sconfig.KeyData{
		InnerVal: &sconfig.KeyDataRawVal{Raw: []byte("01234567890123456789012345678901")},
	}

	enc.EXPECT().
		EncryptKeyForNamespace(gomock.Any(), "root.dev", gomock.Any()).
		Return(encfield.EncryptedField{ID: "dek_parent", Data: "encrypted-key-data"}, nil)

	db.EXPECT().
		CreateKey(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, key *database.Key) error {
			require.True(t, key.Id.HasPrefix(apid.PrefixKey))
			require.Equal(t, "root.dev", key.Namespace)
			require.Equal(t, database.KeyStateActive, key.State)
			require.Equal(t, database.Labels{"purpose": "test"}, key.Labels)
			require.Equal(t, encfield.EncryptedField{ID: "dek_parent", Data: "encrypted-key-data"}, *key.EncryptedKeyData)
			return nil
		})

	gomock.InOrder(
		asynqClient.EXPECT().
			EnqueueContext(gomock.Any(), asynqTaskTypeMatcher{taskType: encrypt.TaskTypeGenerateDataEncryptionKeys}).
			Return(nil, nil),
		redisClient.EXPECT().
			Del(gomock.Any(), gomock.Any()).
			Return(redis.NewIntCmd(ctx)),
		asynqClient.EXPECT().
			EnqueueContext(gomock.Any(), asynqTaskTypeMatcher{taskType: encrypt.TaskTypeSyncKeysToDatabase}).
			Return(nil, nil),
	)

	created, err := s.CreateKey(ctx, "root.dev", "", keyData, map[string]string{"purpose": "test"})

	require.NoError(t, err)
	require.Equal(t, "root.dev", created.GetNamespace())
	require.Equal(t, database.KeyStateActive, created.GetState())
}

func TestGetKeyDataDecryptsProviderConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	s, db, _, _, _, enc := FullMockService(t, ctrl)
	ctx := context.Background()
	keyID := apid.ID("key_test")
	encrypted := encfield.EncryptedField{ID: "dek_parent", Data: "encrypted-key-data"}

	db.EXPECT().
		GetKey(gomock.Any(), keyID).
		Return(&database.Key{
			Id:               keyID,
			Namespace:        "root.dev",
			State:            database.KeyStateActive,
			EncryptedKeyData: &encrypted,
		}, nil)

	enc.EXPECT().
		Decrypt(gomock.Any(), encrypted).
		Return([]byte(`{"awsKmsKeyId":"alias/authproxy","awsRegion":"us-east-1"}`), nil)

	keyData, err := s.GetKeyData(ctx, keyID)

	require.NoError(t, err)
	awsKMS, ok := keyData.InnerVal.(*sconfig.KeyDataAwsKMS)
	require.True(t, ok)
	require.Equal(t, "alias/authproxy", awsKMS.AwsKMSKeyID)
	require.Equal(t, "us-east-1", awsKMS.AwsRegion)
}

func TestUpdateKeyDataEncryptsAndEnqueuesReconciliation(t *testing.T) {
	ctrl := gomock.NewController(t)
	s, db, redisClient, _, asynqClient, enc := FullMockService(t, ctrl)
	ctx := context.Background()
	keyID := apid.ID("key_test")
	updatedEncrypted := encfield.EncryptedField{ID: "dek_parent", Data: "updated-key-data"}

	keyData := &sconfig.KeyData{
		InnerVal: &sconfig.KeyDataAwsKMS{
			AwsKMSKeyID: "alias/authproxy-v2",
			AwsRegion:   "us-east-1",
		},
	}

	db.EXPECT().
		GetKey(gomock.Any(), keyID).
		Return(&database.Key{
			Id:        keyID,
			Namespace: "root.dev",
			State:     database.KeyStateActive,
		}, nil)

	enc.EXPECT().
		EncryptKeyForNamespace(gomock.Any(), "root.dev", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, data []byte) (encfield.EncryptedField, error) {
			var decoded map[string]interface{}
			require.NoError(t, json.Unmarshal(data, &decoded))
			require.Equal(t, "alias/authproxy-v2", decoded["awsKmsKeyId"])
			require.Equal(t, "us-east-1", decoded["awsRegion"])
			return updatedEncrypted, nil
		})

	db.EXPECT().
		UpdateKey(gomock.Any(), keyID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ apid.ID, updates map[string]interface{}) (*database.Key, error) {
			require.Equal(t, updatedEncrypted, updates["encrypted_key_data"])
			return &database.Key{
				Id:               keyID,
				Namespace:        "root.dev",
				State:            database.KeyStateActive,
				EncryptedKeyData: &updatedEncrypted,
			}, nil
		})

	gomock.InOrder(
		asynqClient.EXPECT().
			EnqueueContext(gomock.Any(), asynqTaskTypeMatcher{taskType: encrypt.TaskTypeGenerateDataEncryptionKeys}).
			Return(nil, nil),
		redisClient.EXPECT().
			Del(gomock.Any(), gomock.Any()).
			Return(redis.NewIntCmd(ctx)),
		asynqClient.EXPECT().
			EnqueueContext(gomock.Any(), asynqTaskTypeMatcher{taskType: encrypt.TaskTypeSyncKeysToDatabase}).
			Return(nil, nil),
	)

	updated, err := s.UpdateKeyData(ctx, keyID, keyData)

	require.NoError(t, err)
	require.Equal(t, keyID, updated.GetId())
	require.Equal(t, "root.dev", updated.GetNamespace())
}
