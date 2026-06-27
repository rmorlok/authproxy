package core

import (
	"context"
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
