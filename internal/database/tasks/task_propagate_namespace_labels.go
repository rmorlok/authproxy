package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

const taskTypePropagateNamespaceLabels = "database:propagate_namespace_labels"

// PropagateNamespaceLabelsPayload is the asynq task payload for
// taskTypePropagateNamespaceLabels.
type PropagateNamespaceLabelsPayload struct {
	NamespacePath string `json:"namespace_path"`
}

// NewPropagateNamespaceLabelsTask returns an asynq task that, when
// processed, walks every resource and child namespace under
// namespacePath and re-derives the materialized apxy/ns/* carry-forward
// portion of each. Enqueue this task from the synchronous request handler
// after a namespace's user labels are updated.
func NewPropagateNamespaceLabelsTask(namespacePath string) (*asynq.Task, error) {
	payload, err := json.Marshal(PropagateNamespaceLabelsPayload{NamespacePath: namespacePath})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal propagate-namespace-labels payload: %w", err)
	}
	return asynq.NewTask(taskTypePropagateNamespaceLabels, payload), nil
}

func (th *taskHandler) propagateNamespaceLabels(ctx context.Context, t *asynq.Task) error {
	var payload PropagateNamespaceLabelsPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal propagate-namespace-labels payload: %w: %w", err, asynq.SkipRetry)
	}
	if payload.NamespacePath == "" {
		return fmt.Errorf("namespace_path is required: %w", asynq.SkipRetry)
	}

	th.logger.Info("propagating namespace label change", "namespace_path", payload.NamespacePath)
	if err := th.db.RefreshNamespaceLabelsCarryForward(ctx, payload.NamespacePath); err != nil {
		th.logger.Error("namespace label propagation failed", "namespace_path", payload.NamespacePath, "error", err)
		return err
	}
	th.logger.Info("namespace label propagation complete", "namespace_path", payload.NamespacePath)
	return nil
}
