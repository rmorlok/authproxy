package core

import (
	"context"
	"fmt"
	"time"

	"github.com/cschleiden/go-workflows/client"
	"github.com/cschleiden/go-workflows/registry"
	wflib "github.com/cschleiden/go-workflows/workflow"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util/retry"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
)

const (
	WorkflowNameDisconnectConnectionV1 = "core.connection.disconnect.v1"

	ActivityNameDisconnectConnectionRevokeCredentialsV1 = "core.connection.disconnect.revoke_credentials.v1"
	ActivityNameDisconnectConnectionFinalizeV1          = "core.connection.disconnect.finalize.v1"
)

// maxRevokeAttempts caps the number of times a revoke operation is retried
// inside a single disconnect task invocation. After exhausting attempts, the
// disconnect proceeds so a connection cannot get stuck in `disconnecting`
// because a 3rd-party revoke endpoint is misbehaving.
const maxRevokeAttempts = 3

func disconnectConnectionWorkflowV1(ctx wflib.Context, connectionId string) error {
	if _, err := wflib.ExecuteActivity[any](
		ctx,
		disconnectConnectionRevokeActivityOptions(),
		ActivityNameDisconnectConnectionRevokeCredentialsV1,
		connectionId,
	).Get(ctx); err != nil {
		return err
	}

	_, err := wflib.ExecuteActivity[any](
		ctx,
		wflib.DefaultActivityOptions,
		ActivityNameDisconnectConnectionFinalizeV1,
		connectionId,
	).Get(ctx)
	return err
}

func disconnectConnectionRevokeActivityOptions() wflib.ActivityOptions {
	opts := wflib.DefaultActivityOptions
	opts.RetryOptions = wflib.RetryOptions{
		MaxAttempts:        maxRevokeAttempts,
		FirstRetryInterval: 1 * time.Second,
		BackoffCoefficient: 1,
	}
	return opts
}

func (s *service) registerDisconnectConnectionWorkflow(worker *apworkflows.Worker) error {
	if err := worker.RegisterWorkflow(
		disconnectConnectionWorkflowV1,
		registry.WithName(WorkflowNameDisconnectConnectionV1),
	); err != nil {
		return err
	}
	if err := worker.RegisterActivity(
		s.revokeDisconnectConnectionCredentialsV1,
		registry.WithName(ActivityNameDisconnectConnectionRevokeCredentialsV1),
	); err != nil {
		return err
	}
	return worker.RegisterActivity(
		s.finalizeDisconnectConnectionV1,
		registry.WithName(ActivityNameDisconnectConnectionFinalizeV1),
	)
}

func (s *service) RegisterWorkflows(worker *apworkflows.Worker) error {
	return s.registerDisconnectConnectionWorkflow(worker)
}

func disconnectConnectionWorkflowInstanceID(connectionId apid.ID) string {
	return fmt.Sprintf("%s:%s", WorkflowNameDisconnectConnectionV1, connectionId)
}

func (s *service) startDisconnectConnectionWorkflow(ctx context.Context, connectionId apid.ID) (*wflib.Instance, error) {
	if s.wc == nil {
		return nil, fmt.Errorf("workflow client is not configured")
	}
	return s.wc.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: disconnectConnectionWorkflowInstanceID(connectionId),
		Queue:      apworkflows.DefaultQueue,
	}, WorkflowNameDisconnectConnectionV1, connectionId.String())
}

func parseDisconnectConnectionWorkflowConnectionID(connectionId string) (apid.ID, error) {
	id, err := apid.Parse(connectionId)
	if err != nil {
		return apid.Nil, fmt.Errorf("invalid connection id: %w", err)
	}
	if id == apid.Nil {
		return apid.Nil, fmt.Errorf("connection id not specified")
	}
	if err := id.ValidatePrefix(apid.PrefixConnection); err != nil {
		return apid.Nil, err
	}
	return id, nil
}

func (s *service) revokeDisconnectConnectionCredentialsV1(ctx context.Context, connectionId string) error {
	id, err := parseDisconnectConnectionWorkflowConnectionID(connectionId)
	if err != nil {
		return err
	}

	logger := s.logger.With(
		"workflow", WorkflowNameDisconnectConnectionV1,
		"activity", ActivityNameDisconnectConnectionRevokeCredentialsV1,
		"connection_id", id,
	)
	logger.Info("disconnect connection revoke activity started")
	defer logger.Info("disconnect connection revoke activity completed")

	conn, err := s.getConnection(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get connection to disconnect connection: %w", err)
	}

	revokeOps := conn.getRevokeCredentialsOperations()
	if len(revokeOps) > 0 {
		logger.Info("revoking credentials")
		for _, op := range revokeOps {
			// The Warn is emitted inside the op so every failing attempt
			// (including the terminal one) gets a log line — matching the
			// pre-consolidation behavior here, which differs from the OAuth
			// callsites that only log on retries.
			attempt := 0
			res, err := retry.Do(ctx, retry.Options[struct{}]{
				MaxAttempts: maxRevokeAttempts,
				Backoff:     &retry.LinearBackOff{Step: 1 * time.Second},
			}, func(ctx context.Context) (struct{}, error) {
				attempt++
				err := op(ctx)
				if err != nil {
					logger.Warn(
						"revoke attempt failed",
						"error", err,
						"attempt", attempt,
						"max_attempts", maxRevokeAttempts,
					)
				}
				return struct{}{}, err
			})
			if err != nil {
				// ctx errors are terminal — surface them so the task can be
				// retried by asynq rather than swallowed under a "proceeding
				// with disconnect" log line that doesn't apply.
				if ctx.Err() != nil {
					return ctx.Err()
				}
				// Otherwise: proceed with the rest of the disconnect so the
				// connection does not stay stuck in `disconnecting` forever.
				logger.Error(
					"revocation failed after max attempts; proceeding with disconnect",
					"error", err,
					"attempts", res.Attempts,
				)
			}
		}
	}

	return nil
}

func (s *service) finalizeDisconnectConnectionV1(ctx context.Context, connectionId string) error {
	id, err := parseDisconnectConnectionWorkflowConnectionID(connectionId)
	if err != nil {
		return err
	}

	logger := s.logger.With("workflow", WorkflowNameDisconnectConnectionV1, "activity", ActivityNameDisconnectConnectionFinalizeV1, "connection_id", id)
	logger.Info("disconnect connection finalize activity started")
	defer logger.Info("disconnect connection finalize activity completed")

	conn, err := s.getConnection(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get connection to disconnect connection: %w", err)
	}

	logger.Debug("marking connection as disconnected")
	err = conn.SetState(ctx, database.ConnectionStateDisconnected)
	if err != nil {
		logger.Error("failed to mark connection as disconnected", "error", err)
		return err
	}

	logger.Debug("deleting connection")
	err = s.db.DeleteConnection(ctx, id)
	if err != nil {
		logger.Error("failed to delete connection", "error", err)
		return err
	}

	return nil
}
