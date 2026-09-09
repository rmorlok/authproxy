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

func (s *service) MigrateConnectionGeneration(
	ctx context.Context,
	id apid.ID,
	opts iface.ConnectionMigrationOptions,
) (*iface.ConnectionMigrationTask, error) {
	if id == apid.Nil {
		return nil, httperr.BadRequest("connection id is required")
	}
	if opts.TargetGeneration == 0 {
		return nil, httperr.BadRequest("target_generation is required")
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
	if conn.ConnectorGeneration == opts.TargetGeneration {
		return nil, httperr.BadRequest("connection is already on target generation")
	}

	target, err := s.getConnectorGeneration(ctx, conn.ConnectorId, opts.TargetGeneration)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, database.ErrNotFound) {
			return nil, httperr.NotFound("target connector generation not found", httperr.WithInternalErr(err))
		}
		return nil, err
	}
	if target.State != database.ConnectorGenerationStatePrimary && target.State != database.ConnectorGenerationStateActive {
		return nil, httperr.BadRequest("target connector generation must be primary or active")
	}

	instance, err := s.startMigrateConnectionGenerationWorkflow(ctx, id, opts)
	if err != nil {
		return nil, err
	}

	return &iface.ConnectionMigrationTask{
		TaskInfo:         tasks.FromWorkflowInstance(instance, WorkflowNameMigrateConnectionGenerationV1, string(apworkflows.DefaultQueue)),
		ConnectionID:     id,
		SourceGeneration: conn.ConnectorGeneration,
		TargetGeneration: opts.TargetGeneration,
	}, nil
}
