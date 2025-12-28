package tasks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	mockEncrypt "github.com/rmorlok/authproxy/internal/encrypt/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockActor is a simple mock implementation of the Actor interface
type MockActor struct {
	id uuid.UUID
}

func (m *MockActor) GetId() uuid.UUID {
	return m.id
}

func TestBindToActor(t *testing.T) {
	// Create a TaskInfo instance
	taskInfo := &TaskInfo{
		TrackedVia: TrackedViaAsynq,
		AsynqId:    "test-id",
		AsynqQueue: "test-queue",
		AsynqType:  "test-type",
	}

	// Create a mock actor
	actorId := uuid.New()
	mockActor := &MockActor{id: actorId}

	// Call BindToActor
	result := taskInfo.BindToActor(mockActor)

	// Verify the result
	assert.Equal(t, actorId, result.ActorId)
	assert.Equal(t, taskInfo, result) // Should return the same instance
}

func TestToSecureEncryptedString(t *testing.T) {
	ctx := context.Background()

	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockE := mockEncrypt.NewMockE(ctrl)

	taskInfo := &TaskInfo{
		TrackedVia: TrackedViaAsynq,
		ActorId:    uuid.New(),
		AsynqId:    "test-id",
		AsynqQueue: "test-queue",
		AsynqType:  "test-type",
	}

	// Expected JSON data
	expectedJSON, err := json.Marshal(taskInfo)
	require.NoError(t, err)

	// Mock encrypted data
	mockEncryptedData := []byte("encrypted-data")

	// Expected base64 encoded result
	expectedResult := base64.RawURLEncoding.EncodeToString(mockEncryptedData)

	t.Run("successful encryption", func(t *testing.T) {
		// Setup expectations
		mockE.EXPECT().
			EncryptGlobal(gomock.Any(), expectedJSON).
			Return(mockEncryptedData, nil)

		// Call the method
		result, err := taskInfo.ToSecureEncryptedString(ctx, mockE)

		// Verify results
		assert.NoError(t, err)
		assert.Equal(t, expectedResult, result)
	})

	t.Run("encryption error", func(t *testing.T) {
		// Setup expectations for encryption error
		mockE.EXPECT().
			EncryptGlobal(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("encryption error"))

		// Call the method
		result, err := taskInfo.ToSecureEncryptedString(ctx, mockE)

		// Verify results
		assert.Error(t, err)
		assert.Equal(t, "", result)
	})
}

func TestFromAsynqTask(t *testing.T) {
	t.Run("with valid task", func(t *testing.T) {
		// Create a mock asynq.TaskInfo
		asynqTask := &asynq.TaskInfo{
			ID:    "test-id",
			Queue: "test-queue",
			Type:  "test-type",
		}

		// Call FromAsynqTask
		result := FromAsynqTask(asynqTask)

		// Verify the result
		assert.NotNil(t, result)
		assert.Equal(t, string(TrackedViaAsynq), string(result.TrackedVia))
		assert.Equal(t, "test-id", result.AsynqId)
		assert.Equal(t, "test-queue", result.AsynqQueue)
		assert.Equal(t, "test-type", result.AsynqType)
	})

	t.Run("with nil task", func(t *testing.T) {
		// Call FromAsynqTask with nil
		result := FromAsynqTask(nil)

		// Verify the result is nil
		assert.Nil(t, result)
	})
}

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	e := encrypt.NewFakeEncryptService(false)
	taskInfo := &TaskInfo{
		TrackedVia: TrackedViaAsynq,
		ActorId:    uuid.New(),
		AsynqId:    "test-id",
		AsynqQueue: "test-queue",
		AsynqType:  "test-type",
	}

	encryptedString, err := taskInfo.ToSecureEncryptedString(ctx, e)
	require.NoError(t, err)

	decryptedTaskInfo, err := FromSecureEncryptedString(ctx, e, encryptedString)
	require.NoError(t, err)

	assert.Equal(t, taskInfo, decryptedTaskInfo)
}
