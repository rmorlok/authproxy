package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apid"
)

const taskTypePropagateConnectorLabels = "database:propagate_connector_labels"

// PropagateConnectorLabelsPayload is the asynq task payload for
// taskTypePropagateConnectorLabels.
type PropagateConnectorLabelsPayload struct {
	ConnectorId apid.ID `json:"connector_id"`
}

// NewPropagateConnectorLabelsTask returns an asynq task that, when
// processed, refreshes the materialized apxy/cxr/* carry-forward portion
// of every connection pointing at the logical connector.
func NewPropagateConnectorLabelsTask(id apid.ID) (*asynq.Task, error) {
	payload, err := json.Marshal(PropagateConnectorLabelsPayload{
		ConnectorId: id,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal propagate-connector-labels payload: %w", err)
	}
	return asynq.NewTask(taskTypePropagateConnectorLabels, payload), nil
}

func (th *taskHandler) propagateConnectorLabels(ctx context.Context, t *asynq.Task) error {
	var payload PropagateConnectorLabelsPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal propagate-connector-labels payload: %w: %w", err, asynq.SkipRetry)
	}
	if payload.ConnectorId.IsNil() {
		return fmt.Errorf("connector_id is required: %w", asynq.SkipRetry)
	}

	th.logger.Info(
		"propagating connector label change",
		"connector_id", payload.ConnectorId,
	)
	if err := th.db.RefreshConnectionsForConnector(ctx, payload.ConnectorId); err != nil {
		th.logger.Error(
			"connector label propagation failed",
			"connector_id", payload.ConnectorId,
			"error", err,
		)
		return err
	}
	th.logger.Info(
		"connector label propagation complete",
		"connector_id", payload.ConnectorId,
	)
	return nil
}
