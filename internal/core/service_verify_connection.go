package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apid"
)

// EnqueueVerifyConnection enqueues the verify_connection task for a connection. The task runs
// all probes in the background and advances the connection's setup step based on the outcome.
func (s *service) EnqueueVerifyConnection(ctx context.Context, id apid.ID) error {
	if id == apid.Nil {
		return errors.New("connection id is required")
	}

	t, err := newVerifyConnectionTask(id)
	if err != nil {
		return fmt.Errorf("failed to create verify connection task: %w", err)
	}

	if _, err := s.ac.EnqueueContext(ctx, t, asynq.Retention(10*time.Minute)); err != nil {
		return fmt.Errorf("failed to enqueue verify connection task: %w", err)
	}

	s.logger.Info("verify connection task enqueued", "id", id)
	return nil
}
