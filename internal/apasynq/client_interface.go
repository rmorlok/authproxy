package apasynq

import (
	"context"

	"github.com/hibiken/asynq"
)

//go:generate mockgen -source=client_interface.go -destination=./mock/mock_client_interface.go -package=mock
type Client interface {
	Close() error
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
	Ping() error
}

var _ Client = (*asynq.Client)(nil)
