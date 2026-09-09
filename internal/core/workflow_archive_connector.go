package core

import (
	"context"
	"fmt"
	"time"

	"github.com/cschleiden/go-workflows/client"
	"github.com/cschleiden/go-workflows/registry"
	wflib "github.com/cschleiden/go-workflows/workflow"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
)

const (
	WorkflowNameArchiveConnectorV1 = "core.connector.archive.v1"

	ActivityNameArchiveConnectorPrepareGenerationsV1  = "core.connector.archive.prepare_generations.v1"
	ActivityNameArchiveConnectorFinalizeGenerationsV1 = "core.connector.archive.finalize_generations.v1"
)

type archiveConnectorWorkflowInputV1 struct {
	ConnectorID apid.ID       `json:"connectorId"` // ConnectorID is the durable identifier for the connector to archive.
	Timeout     time.Duration `json:"timeout"`     // Timeout is the maximum duration allowed for child disconnect workflows.
}

func (s *service) startArchiveConnectorWorkflow(
	ctx context.Context,
	connectorID apid.ID,
	opts iface.ConnectorLifecycleOptions,
) (*wflib.Instance, error) {
	return s.wc.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: archiveConnectorWorkflowInstanceID(connectorID),
		Queue:      apworkflows.DefaultQueue,
	}, WorkflowNameArchiveConnectorV1, archiveConnectorWorkflowInputV1{
		ConnectorID: connectorID,
		Timeout:     opts.Timeout,
	})
}

// archiveConnectorWorkflowInstanceID returns a standardized identifier for the workflow instance. This name
// guarantees that only one instance of the workflow runs at a time.
func archiveConnectorWorkflowInstanceID(connectorID apid.ID) string {
	return fmt.Sprintf("%s:%s", WorkflowNameArchiveConnectorV1, connectorID)
}

func archiveConnectorWorkflowV1(ctx wflib.Context, input archiveConnectorWorkflowInputV1) error {
	if _, err := wflib.ExecuteActivity[any](
		ctx,
		wflib.DefaultActivityOptions,
		ActivityNameArchiveConnectorPrepareGenerationsV1,
		input.ConnectorID,
	).Get(ctx); err != nil {
		return err
	}

	if _, err := wflib.CreateSubWorkflowInstance[any](
		ctx,
		wflib.SubWorkflowOptions{
			InstanceID: disconnectConnectorConnectionsWorkflowInstanceID(input.ConnectorID),
			Queue:      apworkflows.DefaultQueue,
		},
		WorkflowNameDisconnectConnectorConnectionsV1,
		disconnectConnectorConnectionsWorkflowInputV1{
			ConnectorID: input.ConnectorID,
			Timeout:     input.Timeout,
		},
	).Get(ctx); err != nil {
		return err
	}

	_, err := wflib.ExecuteActivity[any](
		ctx,
		wflib.DefaultActivityOptions,
		ActivityNameArchiveConnectorFinalizeGenerationsV1,
		input.ConnectorID,
	).Get(ctx)
	return err
}

func validateArchiveConnectorWorkflowConnectorID(connectorID apid.ID) error {
	if connectorID == apid.Nil {
		return fmt.Errorf("connector id not specified")
	}
	return connectorID.ValidatePrefix(apid.PrefixConnector)
}

// prepareArchiveConnectorGenerationsV1 is the activity that prepares the archive connector workflow by moving
// draft state connector generations to archived and primary generations to active. This prevents any future
// connections from being made while the existing connections are cleaned up.
func (s *service) prepareArchiveConnectorGenerationsV1(ctx context.Context, connectorID apid.ID) error {
	logger := s.logger.With(
		"workflow", WorkflowNameArchiveConnectorV1,
		"activity", ActivityNameArchiveConnectorPrepareGenerationsV1,
		"connector_id", connectorID,
	)
	logger.Info("prepare connector generations started")
	defer logger.Info("prepare connector generations completed")

	if err := validateArchiveConnectorWorkflowConnectorID(connectorID); err != nil {
		return err
	}

	found := false
	err := s.db.ListConnectorGenerationsBuilder().
		ForId(connectorID).
		Enumerate(ctx, func(page pagination.PageResult[database.ConnectorWithDefinition]) (pagination.KeepGoing, error) {
			for _, generation := range page.Results {
				found = true
				switch generation.State {
				case database.ConnectorGenerationStateDraft:
					logger.Info("archiving draft connector generation", "generation_id", generation.Id)
					if err := s.db.SetConnectorGenerationState(ctx, generation.Id, generation.Generation, database.ConnectorGenerationStateArchived); err != nil {
						logger.Info("failed archiving draft connector generation", "generation_id", generation.Id, "error", err)
						return pagination.Stop, err
					}
				case database.ConnectorGenerationStatePrimary:
					logger.Info("moving primary connector generation to active", "generation_id", generation.Id)
					if err := s.db.SetConnectorGenerationState(ctx, generation.Id, generation.Generation, database.ConnectorGenerationStateActive); err != nil {
						logger.Info("failed moving primary to active", "generation_id", generation.Id, "error", err)
						return pagination.Stop, err
					}
				}
			}
			return pagination.Continue, nil
		})
	if err != nil {
		return err
	}
	if !found {
		return database.ErrNotFound
	}
	return nil
}

// finalizeArchiveConnectorGenerationsV1 is the activity that runs after all connections have been cleaned up. It moves
// all generations of the connector to the archived state.
func (s *service) finalizeArchiveConnectorGenerationsV1(ctx context.Context, connectorID apid.ID) error {
	logger := s.logger.With(
		"workflow", WorkflowNameArchiveConnectorV1,
		"activity", ActivityNameArchiveConnectorFinalizeGenerationsV1,
		"connector_id", connectorID,
	)
	logger.Info("finalize connector generations started")
	defer logger.Info("finalize connector generations completed")

	if err := validateArchiveConnectorWorkflowConnectorID(connectorID); err != nil {
		return err
	}

	found := false
	err := s.db.ListConnectorGenerationsBuilder().
		ForId(connectorID).
		Enumerate(ctx, func(page pagination.PageResult[database.ConnectorWithDefinition]) (pagination.KeepGoing, error) {
			for _, generation := range page.Results {
				found = true
				if generation.State == database.ConnectorGenerationStateArchived {
					continue
				}

				logger.Info("archiving connector generation", "generation_id", generation.Id)
				if err := s.db.SetConnectorGenerationState(ctx, generation.Id, generation.Generation, database.ConnectorGenerationStateArchived); err != nil {
					logger.Info("failed archiving connector generation", "generation_id", generation.Id, "error", err)
					return pagination.Stop, err
				}
			}
			return pagination.Continue, nil
		})
	if err != nil {
		return err
	}
	if !found {
		return database.ErrNotFound
	}
	return nil
}

func (s *service) registerArchiveConnectorWorkflow(worker workflowRegistrar) error {
	if err := worker.RegisterWorkflow(
		archiveConnectorWorkflowV1,
		registry.WithName(WorkflowNameArchiveConnectorV1),
	); err != nil {
		return err
	}
	if err := worker.RegisterActivity(
		s.prepareArchiveConnectorGenerationsV1,
		registry.WithName(ActivityNameArchiveConnectorPrepareGenerationsV1),
	); err != nil {
		return err
	}
	return worker.RegisterActivity(
		s.finalizeArchiveConnectorGenerationsV1,
		registry.WithName(ActivityNameArchiveConnectorFinalizeGenerationsV1),
	)
}
