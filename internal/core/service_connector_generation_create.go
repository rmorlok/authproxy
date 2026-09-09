package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

func (s *service) CreateConnector(
	ctx context.Context,
	resource *cschema.Connector,
) (iface.Connector, error) {
	if resource == nil {
		return nil, fmt.Errorf("connector cannot be nil")
	}

	if err := resource.ValidateFor(meta.ValidationModeCreate, nil); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}

	id := apctx.GetIdGenerator(ctx).New(apid.PrefixConnector)
	normalized := resource.ApplyAPICreateDefaults(id)
	state := database.ConnectorGenerationState(
		normalized.Spec.Release.DesiredState,
	)

	c, err := newConnectorBuilder(s).
		WithDefinition(&normalized.Spec.Definition).
		WithId(id).
		WithGeneration(1).
		WithState(state).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build connector generation: %w", err)
	}

	c.ConnectorWithDefinition.Labels = normalized.Metadata.Labels
	c.ConnectorWithDefinition.Annotations = normalized.Metadata.Annotations
	c.ConnectorWithDefinition.Name = normalized.Metadata.Name
	c.ConnectorWithDefinition.Namespace = normalized.Metadata.Namespace

	if err := s.db.UpsertConnectorGeneration(
		ctx,
		&c.ConnectorWithDefinition,
	); err != nil {
		return nil, fmt.Errorf("failed to upsert connector generation: %w", err)
	}

	return s.getConnectorGeneration(ctx, id, 1)
}

func (s *service) UpdateConnectorName(
	ctx context.Context,
	id apid.ID,
	name scommon.ResourceName,
) error {
	if err := s.db.UpdateConnectorName(ctx, id, name); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	s.enqueueConnectorLabelPropagation(ctx, id)
	return nil
}

func (s *service) CreateDraftConnectorGeneration(
	ctx context.Context,
	id apid.ID,
	definition *cschema.ConnectorDefinition,
	labels map[string]string,
	annotations map[string]string,
) (iface.Connector, error) {
	// Check for existing draft
	existingDraft, err := s.db.GetConnectorGenerationForState(
		ctx,
		id,
		database.ConnectorGenerationStateDraft,
	)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, fmt.Errorf("failed to check for existing draft: %w", err)
	}
	if existingDraft != nil {
		return nil, ErrDraftAlreadyExists
	}

	// Get the latest generation to determine the next generation number
	latest, err := s.db.NewestConnectorGenerationForId(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get latest connector generation: %w", err)
	}

	newGeneration := latest.Generation + 1

	// If definition is nil, clone from the latest generation
	var def *cschema.ConnectorDefinition
	if definition != nil {
		def = definition.Clone()
	} else {
		wrapped := wrapConnector(*latest, s)
		latestDef, err := wrapped.getDefinition()
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt latest generation definition: %w", err)
		}
		def = latestDef.Clone()
	}

	c, err := newConnectorBuilder(s).
		WithDefinition(def).
		WithId(id).
		WithGeneration(newGeneration).
		WithState(database.ConnectorGenerationStateDraft).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build connector generation: %w", err)
	}

	if labels != nil {
		c.ConnectorWithDefinition.Labels = labels
	} else {
		c.ConnectorWithDefinition.Labels = latest.Labels
	}

	if annotations != nil {
		c.ConnectorWithDefinition.Annotations = annotations
	} else {
		c.ConnectorWithDefinition.Annotations = latest.Annotations
	}
	c.ConnectorWithDefinition.Namespace = latest.Namespace
	c.ConnectorWithDefinition.Name = latest.Name

	if err := s.db.UpsertConnectorGeneration(
		ctx,
		&c.ConnectorWithDefinition,
	); err != nil {
		return nil, fmt.Errorf("failed to upsert connector generation: %w", err)
	}

	return s.getConnectorGeneration(ctx, id, newGeneration)
}

// CreateConnectorGeneration creates the next generation from a canonical
// Connector request. A nil request preserves the existing blank-POST behavior
// and clones the newest generation as a draft.
func (s *service) CreateConnectorGeneration(
	ctx context.Context,
	id apid.ID,
	resource *cschema.Connector,
) (iface.Connector, error) {
	if resource == nil {
		return s.CreateDraftConnectorGeneration(ctx, id, nil, nil, nil)
	}

	if err := resource.ValidateFor(meta.ValidationModeCreate, nil); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}

	currentPage := s.ListConnectorsBuilder().
		ForId(id).
		Limit(1).
		FetchPage(ctx)
	if currentPage.Error != nil {
		return nil, currentPage.Error
	}
	if len(currentPage.Results) == 0 {
		return nil, ErrNotFound
	}

	current := currentPage.Results[0]
	if resource.Metadata.Namespace != current.GetNamespace() {
		return nil, fmt.Errorf("%w: metadata.namespace must match the logical connector", ErrInvalidArgument)
	}

	if resource.Metadata.Name != "" &&
		resource.Metadata.Name != current.GetName() {
		return nil, fmt.Errorf("%w: metadata.name must match the logical connector", ErrInvalidArgument)
	}

	desiredState := resource.Spec.Release.DesiredState
	if desiredState == "" {
		desiredState = cschema.ConnectorReleaseStateDraft
	}

	created, err := s.CreateDraftConnectorGeneration(
		ctx,
		id,
		&resource.Spec.Definition,
		resource.Metadata.Labels,
		resource.Metadata.Annotations,
	)
	if err != nil {
		return nil, err
	}

	if desiredState == cschema.ConnectorReleaseStatePrimary {
		if err := created.SetState(
			ctx,
			database.ConnectorGenerationStatePrimary,
		); err != nil {
			return nil, err
		}

		return s.getConnectorGeneration(ctx, id, created.GetGeneration())
	}

	return created, nil
}
