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

	ActivityNameArchiveConnectorPrepareVersionsV1  = "core.connector.archive.prepare_versions.v1"
	ActivityNameArchiveConnectorFinalizeVersionsV1 = "core.connector.archive.finalize_versions.v1"
)

type archiveConnectorWorkflowInputV1 struct {
	ConnectorID apid.ID       `json:"connector_id"` // ConnectorID is the durable identifier for the connector to archive.
	Timeout     time.Duration `json:"timeout"`      // Timeout is the maximum duration allowed for child disconnect workflows.
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
		ActivityNameArchiveConnectorPrepareVersionsV1,
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
		ActivityNameArchiveConnectorFinalizeVersionsV1,
		input.ConnectorID,
	).Get(ctx)
	return err
}

func validateArchiveConnectorWorkflowConnectorID(connectorID apid.ID) error {
	if connectorID == apid.Nil {
		return fmt.Errorf("connector id not specified")
	}
	return connectorID.ValidatePrefix(apid.PrefixConnectorVersion)
}

// prepareArchiveConnectorVersionsV1 is the activity that prepares the archive connector workflow by moving
// draft state connector versions to archived and primary versions to active. This prevents any future
// connections from being made while the existing connections are cleaned up.
func (s *service) prepareArchiveConnectorVersionsV1(ctx context.Context, connectorID apid.ID) error {
	logger := s.logger.With(
		"workflow", WorkflowNameDisconnectConnectionV1,
		"activity", ActivityNameDisconnectConnectionRevokeCredentialsV1,
		"connector_id", connectorID,
	)
	logger.Info("prepare connector versions started")
	defer logger.Info("prepare connector versions completed")

	if err := validateArchiveConnectorWorkflowConnectorID(connectorID); err != nil {
		return err
	}

	found := false
	err := s.db.ListConnectorVersionsBuilder().
		ForId(connectorID).
		Enumerate(ctx, func(page pagination.PageResult[database.ConnectorVersion]) (pagination.KeepGoing, error) {
			for _, version := range page.Results {
				found = true
				switch version.State {
				case database.ConnectorVersionStateDraft:
					logger.Info("archiving draft connector version", "version_id", version.Id)
					if err := s.db.SetConnectorVersionState(ctx, version.Id, version.Version, database.ConnectorVersionStateArchived); err != nil {
						logger.Info("failed archiving draft connector version", "version_id", version.Id, "error", err)
						return pagination.Stop, err
					}
				case database.ConnectorVersionStatePrimary:
					logger.Info("moving version to primary", "version_id", version.Id)
					if err := s.db.SetConnectorVersionState(ctx, version.Id, version.Version, database.ConnectorVersionStateActive); err != nil {
						logger.Info("failed moving primary to active", "version_id", version.Id, "error", err)
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

// finalizeArchiveConnectorVersionsV1 is the activity that runs after all connections have been cleand up. It moves
// all versions of the connector to the archived state.
func (s *service) finalizeArchiveConnectorVersionsV1(ctx context.Context, connectorID apid.ID) error {
	logger := s.logger.With(
		"workflow", WorkflowNameDisconnectConnectionV1,
		"activity", ActivityNameDisconnectConnectionRevokeCredentialsV1,
		"connector_id", connectorID,
	)
	logger.Info("finalize connector versions started")
	defer logger.Info("finalize connector versions completed")

	if err := validateArchiveConnectorWorkflowConnectorID(connectorID); err != nil {
		return err
	}

	found := false
	err := s.db.ListConnectorVersionsBuilder().
		ForId(connectorID).
		Enumerate(ctx, func(page pagination.PageResult[database.ConnectorVersion]) (pagination.KeepGoing, error) {
			for _, version := range page.Results {
				found = true
				if version.State == database.ConnectorVersionStateArchived {
					continue
				}

				logger.Info("archiving connector version", "version_id", version.Id)
				if err := s.db.SetConnectorVersionState(ctx, version.Id, version.Version, database.ConnectorVersionStateArchived); err != nil {
					logger.Info("failed archiving connector version", "version_id", version.Id, "error", err)
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
		s.prepareArchiveConnectorVersionsV1,
		registry.WithName(ActivityNameArchiveConnectorPrepareVersionsV1),
	); err != nil {
		return err
	}
	return worker.RegisterActivity(
		s.finalizeArchiveConnectorVersionsV1,
		registry.WithName(ActivityNameArchiveConnectorFinalizeVersionsV1),
	)
}
