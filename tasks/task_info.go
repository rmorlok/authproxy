package tasks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// TrackedVia allows for multiple backends for tracking tasks. For now, just asynq.
type TrackedVia string

const (
	TrackedViaAsynq = "asynq"
)

type TaskInfo struct {
	TrackedVia TrackedVia `json:"tracked_via"`
	ActorId    uuid.UUID  `json:"actor_id,omitempty"`
	AsynqId    string     `json:"asynq_id,omitempty"`
	AsynqQueue string     `json:"asynq_queue,omitempty"`
	AsynqType  string     `json:"asynq_type,omitempty"`
}

type Actor interface {
	GetID() uuid.UUID
}

func (ti *TaskInfo) BindToActor(actor Actor) *TaskInfo {
	ti.ActorId = actor.GetID()
	return ti
}

type Encrypt interface {
	EncryptGlobal(ctx context.Context, data []byte) ([]byte, error)
	DecryptGlobal(ctx context.Context, data []byte) ([]byte, error)
}

func (ti *TaskInfo) ToSecureEncryptedString(ctx context.Context, e Encrypt) (string, error) {
	jsonData, err := json.Marshal(ti)
	if err != nil {
		return "", err
	}

	encryptedData, err := e.EncryptGlobal(ctx, jsonData)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(encryptedData), nil
}

func FromAsynqTask(task *asynq.TaskInfo) *TaskInfo {
	if task == nil {
		return nil
	}

	return &TaskInfo{
		TrackedVia: TrackedViaAsynq,
		AsynqId:    task.ID,
		AsynqQueue: task.Queue,
		AsynqType:  task.Type,
	}
}

func FromSecureEncryptedString(ctx context.Context, e Encrypt, s string) (*TaskInfo, error) {
	encryptedData, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}

	decryptedData, err := e.DecryptGlobal(ctx, encryptedData)
	if err != nil {
		return nil, err
	}

	var taskInfo TaskInfo
	err = json.Unmarshal(decryptedData, &taskInfo)
	if err != nil {
		return nil, err
	}

	return &taskInfo, nil
}
