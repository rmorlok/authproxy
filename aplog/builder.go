package aplog

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type Builder interface {
	WithService(serviceId string) Builder
	WithComponent(componentId string) Builder
	WithTask(t *asynq.Task) Builder
	WithCtx(ctx context.Context) Builder
	WithConnectionId(connectionId uuid.UUID) Builder
	WithConnectorId(connectionId uuid.UUID) Builder
	With(args ...any) Builder
	Build() *slog.Logger
}

type builder struct {
	l *slog.Logger
}

func (b *builder) With(args ...any) Builder {
	return &builder{l: b.l.With(args...)}
}

func (b *builder) WithService(serviceId string) Builder {
	return &builder{l: b.l.With("service", serviceId)}
}

func (b *builder) WithComponent(componentId string) Builder {
	return &builder{l: b.l.With("component", componentId)}
}

func (b *builder) WithTask(t *asynq.Task) Builder {
	attrs := []any{
		slog.String("type", t.Type()),
	}

	// This is because the writer isn't present in tests
	w := t.ResultWriter()
	if w != nil {
		attrs = append(attrs, slog.String("id", w.TaskID()))
	}

	return &builder{l: b.l.With(slog.Group("task", attrs...))}
}

func (b *builder) WithCtx(ctx context.Context) Builder {
	// Nothing for now
	return b
}

func (b *builder) WithConnectionId(connectionId uuid.UUID) Builder {
	return &builder{l: b.l.With("connection_id", connectionId.String())}
}

func (b *builder) WithConnectorId(connectorId uuid.UUID) Builder {
	return &builder{l: b.l.With("connector_id", connectorId.String())}
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
