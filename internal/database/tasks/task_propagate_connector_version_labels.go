package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apid"
)

const taskTypePropagateConnectorVersionLabels = "database:propagate_connector_version_labels"

// PropagateConnectorVersionLabelsPayload is the asynq task payload for
// taskTypePropagateConnectorVersionLabels.
type PropagateConnectorVersionLabelsPayload struct {
	ConnectorVersionId apid.ID `json:"connector_version_id"`
	Version            uint64  `json:"version"`
}

// NewPropagateConnectorVersionLabelsTask returns an asynq task that, when
// processed, refreshes the materialized apxy/cxr/* carry-forward portion
// of every connection pointing at the given (id, version). Enqueue from
// the synchronous request handler after a connector version's user labels
// change. Only meaningful for draft connector versions; primary and
// active versions are immutable so the propagation is a no-op for them.
func NewPropagateConnectorVersionLabelsTask(id apid.ID, version uint64) (*asynq.Task, error) {
	payload, err := json.Marshal(PropagateConnectorVersionLabelsPayload{
		ConnectorVersionId: id,
		Version:            version,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal propagate-connector-version-labels payload: %w", err)
	}
	return asynq.NewTask(taskTypePropagateConnectorVersionLabels, payload), nil
}

func (th *taskHandler) propagateConnectorVersionLabels(ctx context.Context, t *asynq.Task) error {
	var payload PropagateConnectorVersionLabelsPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal propagate-connector-version-labels payload: %w: %w", err, asynq.SkipRetry)
	}
	if payload.ConnectorVersionId.IsNil() {
		return fmt.Errorf("connector_version_id is required: %w", asynq.SkipRetry)
	}
	if payload.Version == 0 {
		return fmt.Errorf("version is required: %w", asynq.SkipRetry)
	}

	th.logger.Info(
		"propagating connector version label change",
		"connector_version_id", payload.ConnectorVersionId,
		"version", payload.Version,
	)
	if err := th.db.RefreshConnectionsForConnectorVersion(ctx, payload.ConnectorVersionId, payload.Version); err != nil {
		th.logger.Error(
			"connector version label propagation failed",
			"connector_version_id", payload.ConnectorVersionId,
			"version", payload.Version,
			"error", err,
		)
		return err
	}
	th.logger.Info(
		"connector version label propagation complete",
		"connector_version_id", payload.ConnectorVersionId,
		"version", payload.Version,
	)
	return nil
}
