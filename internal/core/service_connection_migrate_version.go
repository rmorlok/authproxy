package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/tasks"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
)

func (s *service) MigrateConnectionVersion(
	ctx context.Context,
	id apid.ID,
	opts iface.ConnectionMigrationOptions,
) (*iface.ConnectionMigrationTask, error) {
	if id == apid.Nil {
		return nil, httperr.BadRequest("connection id is required")
	}
	if opts.TargetVersion == 0 {
		return nil, httperr.BadRequest("target_version is required")
	}
	if s.wc == nil {
		return nil, fmt.Errorf("workflow client is not configured")
	}

	conn, err := s.getConnection(ctx, id)
	if err != nil {
		if errors.Is(err, iface.ErrConnectionNotFound) {
			return nil, httperr.NotFound("connection not found", httperr.WithInternalErr(err))
		}
		return nil, err
	}
	if conn.ConnectorVersion == opts.TargetVersion {
		return nil, httperr.BadRequest("connection is already on target version")
	}

	target, err := s.getConnectorVersion(ctx, conn.ConnectorId, opts.TargetVersion)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, database.ErrNotFound) {
			return nil, httperr.NotFound("target connector version not found", httperr.WithInternalErr(err))
		}
		return nil, err
	}
	if target.State != database.ConnectorVersionStatePrimary && target.State != database.ConnectorVersionStateActive {
		return nil, httperr.BadRequest("target connector version must be primary or active")
	}

	instance, err := s.startMigrateConnectionVersionWorkflow(ctx, id, opts)
	if err != nil {
		return nil, err
	}

	return &iface.ConnectionMigrationTask{
		TaskInfo:      tasks.FromWorkflowInstance(instance, WorkflowNameMigrateConnectionVersionV1, string(apworkflows.DefaultQueue)),
		ConnectionID:  id,
		SourceVersion: conn.ConnectorVersion,
		TargetVersion: opts.TargetVersion,
	}, nil
}
