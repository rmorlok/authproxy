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
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
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

	created, err := s.CreateKey(ctx, "root.dev", keyData, map[string]string{"purpose": "test"})

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
		Return([]byte(`{"aws_kms_key_id":"alias/authproxy","aws_region":"us-east-1"}`), nil)

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
			require.Equal(t, "alias/authproxy-v2", decoded["aws_kms_key_id"])
			require.Equal(t, "us-east-1", decoded["aws_region"])
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
