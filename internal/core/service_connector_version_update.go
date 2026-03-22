package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/rmorlok/authproxy/internal/util"
)

func (s *service) UpdateDraftConnectorVersion(ctx context.Context, id apid.ID, version uint64, definition *cschema.Connector, labels map[string]string, annotations map[string]string) (iface.ConnectorVersion, error) {
	existing, err := s.db.GetConnectorVersion(ctx, id, version)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get connector version: %w", err)
	}

	if existing.State != database.ConnectorVersionStateDraft {
		return nil, ErrNotDraft
	}

	def := definition.Clone()
	def.Id = id
	def.Version = version
	def.Namespace = util.ToPtr(existing.Namespace)
	def.State = string(database.ConnectorVersionStateDraft)

	cv, err := newConnectorVersionBuilder(s).
		WithConfig(def).
		WithId(id).
		WithVersion(version).
		WithState(database.ConnectorVersionStateDraft).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build connector version: %w", err)
	}

	if labels != nil {
		cv.ConnectorVersion.Labels = labels
	} else {
		cv.ConnectorVersion.Labels = existing.Labels
	}

	if annotations != nil {
		cv.ConnectorVersion.Annotations = annotations
	} else {
		cv.ConnectorVersion.Annotations = existing.Annotations
	}

	if err := s.db.UpsertConnectorVersion(ctx, &cv.ConnectorVersion); err != nil {
		return nil, fmt.Errorf("failed to upsert connector version: %w", err)
	}

	return s.getConnectorVersion(ctx, id, version)
}

func (s *service) GetOrCreateDraftConnectorVersion(ctx context.Context, id apid.ID) (iface.ConnectorVersion, error) {
	// Try to find an existing draft
	existingDraft, err := s.db.GetConnectorVersionForState(ctx, id, database.ConnectorVersionStateDraft)
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
	latest, err := s.db.NewestConnectorVersionForId(ctx, id)
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
	def.State = string(database.ConnectorVersionStateDraft)

	cv, err := newConnectorVersionBuilder(s).
		WithConfig(def).
		WithId(id).
		WithVersion(newVersion).
		WithState(database.ConnectorVersionStateDraft).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build connector version: %w", err)
	}

	cv.ConnectorVersion.Labels = latest.Labels
	cv.ConnectorVersion.Annotations = latest.Annotations

	if err := s.db.UpsertConnectorVersion(ctx, &cv.ConnectorVersion); err != nil {
		return nil, fmt.Errorf("failed to upsert connector version: %w", err)
	}

	return s.getConnectorVersion(ctx, id, newVersion)
}
