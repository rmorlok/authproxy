package apasynq

import (
	"context"

	"github.com/hibiken/asynq"
)

type wrappedClient struct {
	inner    Client
	defaults []asynq.Option
}

func (w *wrappedClient) Close() error {
	return w.inner.Close()
}

func (w *wrappedClient) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	return w.inner.Enqueue(task, append(w.defaults, opts...)...)
}

func (w *wrappedClient) EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	return w.inner.EnqueueContext(ctx, task, append(w.defaults, opts...)...)
}

func (w *wrappedClient) Ping() error {
	return w.inner.Ping()
}

func WrapClientWithDefaultOptions(c Client, defaultOpts []asynq.Option) Client {
	return &wrappedClient{
		inner:    c,
		defaults: defaultOpts,
	}
}
