package connectors

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/aplog"
	"github.com/rmorlok/authproxy/connectors/iface"
	"github.com/rmorlok/authproxy/database"
)

const taskTypeProbe = "connectors:probe"

func newProbeTask(connectionId uuid.UUID, probeId string) (*asynq.Task, error) {
	payload, err := json.Marshal(probeTaskPayload{ConnectionId: connectionId, ProbeId: probeId})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(taskTypeProbe, payload), nil
}

type probeTaskPayload struct {
	ConnectionId uuid.UUID `json:"connection_id"`
	ProbeId      string    `json:"probe_id"`
}

func skipTaskErrorIfProbeIsPeriodic(p iface.Probe, err error) error {
	if err == nil {
		return nil
	}

	if p.IsPeriodic() {
		return asynq.SkipRetry
	}

	return err
}

func (s *service) runProbeForConnection(ctx context.Context, t *asynq.Task) error {
	logger := aplog.NewBuilder(s.logger).
		WithTask(t).
		WithCtx(ctx).
		Build()
	logger.Debug("probe task started")
	defer logger.Debug("probe task completed")

	var p probeTaskPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("%s json.Unmarshal failed: %v: %w", taskTypeDisconnectConnection, err, asynq.SkipRetry)
	}

	if p.ConnectionId == uuid.Nil {
		return fmt.Errorf("%s connection id not specified: %w", taskTypeDisconnectConnection, asynq.SkipRetry)
	}

	logger = aplog.NewBuilder(logger).
		WithConnectionId(p.ConnectionId).
		With("probe_id", p.ProbeId).
		Build()

	logger.Debug("getting connection")
	conn, err := s.GetConnection(ctx, p.ConnectionId)
	if err != nil {
		if errors.Is(database.ErrNotFound, err) {
			logger.Error("connection not found", "error", err)
			return asynq.SkipRetry
		}

		return err
	}

	probe, err := conn.GetProbe(p.ProbeId)
	if err != nil {
		if errors.Is(ErrProbeNotFound, err) {
			logger.Error("probe not found", "error", err)
			return fmt.Errorf("%s probe not found: %w", taskTypeProbe, asynq.SkipRetry)
		}

		return skipTaskErrorIfProbeIsPeriodic(probe, err)
	}

	_, err = probe.Invoke(ctx)
	if err != nil {
		return skipTaskErrorIfProbeIsPeriodic(probe, err)
	}

	return nil
}
