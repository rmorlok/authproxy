package tasks

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
)

// TrackedVia allows for multiple backends for tracking tasks. For now, just asynq.
type TrackedVia string

const (
	TrackedViaAsynq = "asynq"
)

type TaskInfo struct {
	TrackedVia TrackedVia `json:"tracked_via"`
	ActorId    apid.ID    `json:"actor_id,omitempty"`
	AsynqId    string     `json:"asynq_id,omitempty"`
	AsynqQueue string     `json:"asynq_queue,omitempty"`
	AsynqType  string     `json:"asynq_type,omitempty"`
}

type Actor interface {
	GetId() apid.ID
}

func (ti *TaskInfo) BindToActor(actor Actor) *TaskInfo {
	ti.ActorId = actor.GetId()
	return ti
}

type Encrypt interface {
	EncryptGlobal(ctx context.Context, data []byte) (encfield.EncryptedField, error)
	Decrypt(ctx context.Context, ef encfield.EncryptedField) ([]byte, error)
}

func (ti *TaskInfo) ToSecureEncryptedString(ctx context.Context, e Encrypt) (string, error) {
	jsonData, err := json.Marshal(ti)
	if err != nil {
		return "", err
	}

	ef, err := e.EncryptGlobal(ctx, jsonData)
	if err != nil {
		return "", err
	}

	efJSON, err := json.Marshal(ef)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(efJSON), nil
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
	efJSON, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}

	var ef encfield.EncryptedField
	err = json.Unmarshal(efJSON, &ef)
	if err != nil {
		return nil, err
	}

	decryptedData, err := e.Decrypt(ctx, ef)
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
