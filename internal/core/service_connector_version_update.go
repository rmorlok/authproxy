package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	dbtasks "github.com/rmorlok/authproxy/internal/database/tasks"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/util"
)

// enqueueConnectorLabelPropagation enqueues an asynq task that
// refreshes the materialized apxy/cxr/* portion of every connection
// pointing at the logical connector. Failures to enqueue are logged but do not
// fail the originating request — the daily consistency checker (#198)
// covers any drift if the task is dropped.
func (s *service) enqueueConnectorLabelPropagation(ctx context.Context, id apid.ID) {
	task, err := dbtasks.NewPropagateConnectorLabelsTask(id)
	if err != nil {
		s.logger.Error("failed to build connector label propagation task", "id", id, "error", err)
		return
	}
	if _, err := s.ac.EnqueueContext(ctx, task); err != nil {
		s.logger.Error("failed to enqueue connector label propagation task", "id", id, "error", err)
	}
}

func (s *service) UpdateDraftConnectorVersion(ctx context.Context, id apid.ID, version uint64, definition *cschema.Connector, labels map[string]string, annotations map[string]string) (iface.ConnectorVersion, error) {
	existing, err := s.db.GetConnectorDefinitionVersion(ctx, id, version)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get connector version: %w", err)
	}

	if existing.State != database.ConnectorDefinitionVersionStateDraft {
		return nil, ErrNotDraft
	}

	def := definition.Clone()
	def.Id = id
	def.Version = version
	def.Namespace = util.ToPtr(existing.Namespace)
	def.State = string(database.ConnectorDefinitionVersionStateDraft)

	cv, err := newConnectorVersionBuilder(s).
		WithConfig(def).
		WithId(id).
		WithVersion(version).
		WithState(database.ConnectorDefinitionVersionStateDraft).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build connector version: %w", err)
	}

	if labels != nil {
		cv.ConnectorWithDefinition.Labels = labels
	} else {
		cv.ConnectorWithDefinition.Labels = existing.Labels
	}

	if annotations != nil {
		cv.ConnectorWithDefinition.Annotations = annotations
	} else {
		cv.ConnectorWithDefinition.Annotations = existing.Annotations
	}

	if err := s.db.UpsertConnectorDefinitionVersion(ctx, &cv.ConnectorWithDefinition); err != nil {
		return nil, fmt.Errorf("failed to upsert connector version: %w", err)
	}

	s.enqueueConnectorLabelPropagation(ctx, id)
	return s.getConnectorVersion(ctx, id, version)
}

func (s *service) GetOrCreateDraftConnectorVersion(ctx context.Context, id apid.ID) (iface.ConnectorVersion, error) {
	// Try to find an existing draft
	existingDraft, err := s.db.GetConnectorDefinitionVersionForState(ctx, id, database.ConnectorDefinitionVersionStateDraft)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, fmt.Errorf("failed to check for existing draft: %w", err)
	}
	if existingDraft != nil {
		wrapped := wrapConnectorVersion(*existingDraft, s)
		// Verify we can load the definition
		if _, err := wrapped.getDefinition(); err != nil {
			return nil, err
		}
		return wrapped, nil
	}

	// No existing draft, get the latest version
	latest, err := s.db.NewestConnectorDefinitionVersionForId(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get latest connector version: %w", err)
	}

	// Decrypt and clone the latest definition
	wrapped := wrapConnectorVersion(*latest, s)
	latestDef, err := wrapped.getDefinition()
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt latest version definition: %w", err)
	}
	def := latestDef.Clone()

	newVersion := latest.Version + 1
	def.Id = id
	def.Version = newVersion
	def.Namespace = util.ToPtr(latest.Namespace)
	def.State = string(database.ConnectorDefinitionVersionStateDraft)

	cv, err := newConnectorVersionBuilder(s).
		WithConfig(def).
		WithId(id).
		WithVersion(newVersion).
		WithState(database.ConnectorDefinitionVersionStateDraft).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build connector version: %w", err)
	}

	cv.ConnectorWithDefinition.Labels = latest.Labels
	cv.ConnectorWithDefinition.Annotations = latest.Annotations

	if err := s.db.UpsertConnectorDefinitionVersion(ctx, &cv.ConnectorWithDefinition); err != nil {
		return nil, fmt.Errorf("failed to upsert connector version: %w", err)
	}

	return s.getConnectorVersion(ctx, id, newVersion)
}
