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
	"github.com/rmorlok/authproxy/internal/util"
)

func (s *service) CreateConnectorVersion(ctx context.Context, namespace string, name scommon.ResourceName, definition *cschema.Connector, labels map[string]string, annotations map[string]string) (iface.Connector, error) {
	id := apctx.GetIdGenerator(ctx).New(apid.PrefixConnectorVersion)

	def := definition.Clone()
	def.Id = id
	def.Version = 1
	def.Namespace = util.ToPtr(namespace)
	def.State = string(database.ConnectorDefinitionVersionStateDraft)

	c, err := newConnectorBuilder(s).
		WithConfig(def).
		WithId(id).
		WithVersion(1).
		WithState(database.ConnectorDefinitionVersionStateDraft).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build connector version: %w", err)
	}

	c.ConnectorWithDefinition.Labels = labels
	c.ConnectorWithDefinition.Annotations = annotations
	c.ConnectorWithDefinition.Name = name

	if err := s.db.UpsertConnectorDefinitionVersion(ctx, &c.ConnectorWithDefinition); err != nil {
		return nil, fmt.Errorf("failed to upsert connector version: %w", err)
	}

	return s.getConnectorVersion(ctx, id, 1)
}

func (s *service) UpdateConnectorName(ctx context.Context, id apid.ID, name scommon.ResourceName) error {
	if err := s.db.UpdateConnectorName(ctx, id, name); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *service) CreateDraftConnectorVersion(ctx context.Context, id apid.ID, definition *cschema.Connector, labels map[string]string, annotations map[string]string) (iface.Connector, error) {
	// Check for existing draft
	existingDraft, err := s.db.GetConnectorDefinitionVersionForState(ctx, id, database.ConnectorDefinitionVersionStateDraft)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, fmt.Errorf("failed to check for existing draft: %w", err)
	}
	if existingDraft != nil {
		return nil, ErrDraftAlreadyExists
	}

	// Get the latest version to determine the next version number
	latest, err := s.db.NewestConnectorDefinitionVersionForId(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get latest connector version: %w", err)
	}

	newVersion := latest.Version + 1

	// If definition is nil, clone from the latest version
	var def *cschema.Connector
	if definition != nil {
		def = definition.Clone()
	} else {
		wrapped := wrapConnector(*latest, s)
		latestDef, err := wrapped.getDefinition()
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt latest version definition: %w", err)
		}
		def = latestDef.Clone()
	}

	def.Id = id
	def.Version = newVersion
	def.Namespace = util.ToPtr(latest.Namespace)
	def.State = string(database.ConnectorDefinitionVersionStateDraft)

	c, err := newConnectorBuilder(s).
		WithConfig(def).
		WithId(id).
		WithVersion(newVersion).
		WithState(database.ConnectorDefinitionVersionStateDraft).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build connector version: %w", err)
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

	if err := s.db.UpsertConnectorDefinitionVersion(ctx, &c.ConnectorWithDefinition); err != nil {
		return nil, fmt.Errorf("failed to upsert connector version: %w", err)
	}

	return s.getConnectorVersion(ctx, id, newVersion)
}
