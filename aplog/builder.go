package aplog

import (
	"context"
	"github.com/hibiken/asynq"
	"log/slog"
)

type Builder interface {
	WithService(serviceId string) Builder
	WithComponent(componentId string) Builder
	WithTask(t *asynq.Task) Builder
	WithCtx(ctx context.Context) Builder
	Build() *slog.Logger
}

type builder struct {
	l *slog.Logger
}

func (b *builder) WithService(serviceId string) Builder {
	return &builder{l: b.l.With("service", serviceId)}
}

func (b *builder) WithComponent(componentId string) Builder {
	return &builder{l: b.l.With("component", componentId)}
}

func (b *builder) WithTask(t *asynq.Task) Builder {
	return &builder{l: b.l.With(slog.Group("task",
		slog.String("id", t.ResultWriter().TaskID()),
		slog.String("type", t.Type()),
	))}
}

func (b *builder) WithCtx(ctx context.Context) Builder {
	// Nothing for now
	return b
}

func (b *builder) Build() *slog.Logger {
	return b.l
}

func NewBuilder(l *slog.Logger) Builder {
	if l == nil {
		panic("cannot create log builder with nil log")
	}
	
	return &builder{l: l}
}

var _ Builder = &builder{}
