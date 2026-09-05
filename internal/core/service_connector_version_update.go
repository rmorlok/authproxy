package core

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	dbtasks "github.com/rmorlok/authproxy/internal/database/tasks"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// enqueueConnectorLabelPropagation enqueues an asynq task that
// refreshes the materialized apxy/cxr/* portion of every connection
// pointing at the logical connector. Failures to enqueue are logged but do not
// fail the originating request — the daily consistency checker (#198)
// covers any drift if the task is dropped.
func (s *service) enqueueConnectorLabelPropagation(
	ctx context.Context,
	id apid.ID,
) {
	task, err := dbtasks.NewPropagateConnectorLabelsTask(id)
	if err != nil {
		s.logger.Error("failed to build connector label propagation task", "id", id, "error", err)
		return
	}

	if _, err := s.ac.EnqueueContext(ctx, task); err != nil {
		s.logger.Error("failed to enqueue connector label propagation task", "id", id, "error", err)
	}
}

func (s *service) UpdateDraftConnectorVersion(
	ctx context.Context,
	id apid.ID,
	version uint64,
	definition *cschema.ConnectorDefinition,
	labels map[string]string,
	annotations map[string]string,
) (iface.Connector, error) {
	return s.updateDraftConnectorVersion(
		ctx,
		id,
		version,
		definition,
		labels,
		annotations,
		database.ConnectorDefinitionVersionStateDraft,
	)
}

func (s *service) updateDraftConnectorVersion(
	ctx context.Context,
	id apid.ID,
	version uint64,
	definition *cschema.ConnectorDefinition,
	labels map[string]string,
	annotations map[string]string,
	state database.ConnectorDefinitionVersionState,
) (iface.Connector, error) {
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

	c, err := newConnectorBuilder(s).
		WithDefinition(definition).
		WithId(id).
		WithVersion(version).
		WithState(state).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build connector version: %w", err)
	}

	if labels != nil {
		c.ConnectorWithDefinition.Labels = labels
	} else {
		c.ConnectorWithDefinition.Labels = existing.Labels
	}

	if annotations != nil {
		c.ConnectorWithDefinition.Annotations = annotations
	} else {
		c.ConnectorWithDefinition.Annotations = existing.Annotations
	}
	c.ConnectorWithDefinition.Namespace = existing.Namespace
	c.ConnectorWithDefinition.Name = existing.Name

	if err := s.db.UpsertConnectorDefinitionVersion(
		ctx,
		&c.ConnectorWithDefinition,
	); err != nil {
		return nil, fmt.Errorf("failed to upsert connector version: %w", err)
	}

	s.enqueueConnectorLabelPropagation(ctx, id)
	return s.getConnectorVersion(ctx, id, version)
}

// UpdateConnector applies logical metadata changes without manufacturing a
// generation. Definition or release changes are applied to the existing draft,
// or to a newly cloned draft when the selected generation is published.
func (s *service) UpdateConnector(
	ctx context.Context,
	id apid.ID,
	patch *cschema.ConnectorPatch,
) (iface.Connector, error) {
	if patch == nil {
		return nil, fmt.Errorf("connector patch cannot be nil")
	}
	if patch.Metadata != nil && patch.Metadata.Generation != nil {
		return nil, fmt.Errorf("%w: metadata.generation can only be addressed through a connector version endpoint", ErrInvalidArgument)
	}

	page := s.ListConnectorsBuilder().ForId(id).Limit(1).FetchPage(ctx)
	if page.Error != nil {
		return nil, page.Error
	}
	if len(page.Results) == 0 {
		return nil, ErrNotFound
	}
	current := page.Results[0]
	desired, err := patch.ApplyTo(current.GetResource(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}

	if current.GetName() != desired.Metadata.Name {
		if err := s.UpdateConnectorName(ctx, id, desired.Metadata.Name); err != nil {
			return nil, err
		}
	}
	if !maps.Equal(current.GetLabels(), desired.Metadata.Labels) {
		if _, err := s.db.UpdateConnectorLabels(ctx, id, desired.Metadata.Labels); err != nil {
			return nil, mapDatabaseError(err)
		}
		s.enqueueConnectorLabelPropagation(ctx, id)
	}
	if !maps.Equal(current.GetAnnotations(), desired.Metadata.Annotations) {
		if _, err := s.db.UpdateConnectorAnnotations(ctx, id, desired.Metadata.Annotations); err != nil {
			return nil, mapDatabaseError(err)
		}
	}

	hasDefinition := patch.Spec != nil && patch.Spec.HasDefinition()
	hasDesiredState := patch.Spec != nil &&
		patch.Spec.Release != nil &&
		patch.Spec.Release.HasDesiredState()
	if !hasDefinition && !hasDesiredState {
		return s.getConnectorVersion(ctx, id, current.GetVersion())
	}
	if !hasDefinition && hasDesiredState &&
		desired.Spec.Release.DesiredState == cschema.ConnectorReleaseStatePrimary &&
		current.GetState() == database.ConnectorDefinitionVersionStatePrimary {
		return s.getConnectorVersion(ctx, id, current.GetVersion())
	}

	draft, err := s.GetOrCreateDraftConnectorVersion(ctx, id)
	if err != nil {
		return nil, err
	}
	desiredDraft, err := patch.ApplyTo(draft.GetResource(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}

	return s.updateDraftConnectorVersion(
		ctx,
		id,
		draft.GetVersion(),
		&desiredDraft.Spec.Definition,
		desired.Metadata.Labels,
		desired.Metadata.Annotations,
		database.ConnectorDefinitionVersionState(desiredDraft.Spec.Release.DesiredState),
	)
}

// UpdateConnectorVersion applies a canonical patch to one draft generation.
// Connector-level naming remains the responsibility of UpdateConnector.
func (s *service) UpdateConnectorVersion(
	ctx context.Context,
	id apid.ID,
	version uint64,
	patch *cschema.ConnectorPatch,
) (iface.Connector, error) {
	if patch == nil {
		return nil, fmt.Errorf("connector patch cannot be nil")
	}
	existing, err := s.GetConnectorVersion(ctx, id, version)
	if err != nil {
		return nil, err
	}
	if existing.GetState() != database.ConnectorDefinitionVersionStateDraft {
		return nil, ErrNotDraft
	}
	desired, err := patch.ApplyTo(existing.GetResource(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	if desired.Metadata.Name != existing.GetName() {
		return nil, fmt.Errorf("%w: connector names can only be changed through the connector-level update endpoint", ErrInvalidArgument)
	}

	return s.updateDraftConnectorVersion(
		ctx,
		id,
		version,
		&desired.Spec.Definition,
		desired.Metadata.Labels,
		desired.Metadata.Annotations,
		database.ConnectorDefinitionVersionState(desired.Spec.Release.DesiredState),
	)
}

func (s *service) GetOrCreateDraftConnectorVersion(
	ctx context.Context,
	id apid.ID,
) (iface.Connector, error) {
	// Try to find an existing draft
	existingDraft, err := s.db.GetConnectorDefinitionVersionForState(
		ctx,
		id,
		database.ConnectorDefinitionVersionStateDraft,
	)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, fmt.Errorf("failed to check for existing draft: %w", err)
	}

	if existingDraft != nil {
		wrapped := wrapConnector(*existingDraft, s)
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
	wrapped := wrapConnector(*latest, s)
	latestDef, err := wrapped.getDefinition()
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt latest version definition: %w", err)
	}
	def := latestDef.Clone()

	newVersion := latest.Version + 1
	c, err := newConnectorBuilder(s).
		WithDefinition(def).
		WithId(id).
		WithVersion(newVersion).
		WithState(database.ConnectorDefinitionVersionStateDraft).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build connector version: %w", err)
	}

	c.ConnectorWithDefinition.Labels = latest.Labels
	c.ConnectorWithDefinition.Annotations = latest.Annotations
	c.ConnectorWithDefinition.Namespace = latest.Namespace
	c.ConnectorWithDefinition.Name = latest.Name

	if err := s.db.UpsertConnectorDefinitionVersion(ctx, &c.ConnectorWithDefinition); err != nil {
		return nil, fmt.Errorf("failed to upsert connector version: %w", err)
	}

	return s.getConnectorVersion(ctx, id, newVersion)
}
