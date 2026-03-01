package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/rmorlok/authproxy/internal/util"
)

func (s *service) UpdateDraftConnectorVersion(ctx context.Context, id apid.ID, version uint64, definition *cschema.Connector, labels map[string]string) (iface.ConnectorVersion, error) {
	existing, err := s.db.GetConnectorVersion(ctx, id, version)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to get connector version")
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
		return nil, errors.Wrap(err, "failed to build connector version")
	}

	if labels != nil {
		cv.ConnectorVersion.Labels = labels
	} else {
		cv.ConnectorVersion.Labels = existing.Labels
	}

	if err := s.db.UpsertConnectorVersion(ctx, &cv.ConnectorVersion); err != nil {
		return nil, errors.Wrap(err, "failed to upsert connector version")
	}

	return s.getConnectorVersion(ctx, id, version)
}

func (s *service) GetOrCreateDraftConnectorVersion(ctx context.Context, id apid.ID) (iface.ConnectorVersion, error) {
	// Try to find an existing draft
	existingDraft, err := s.db.GetConnectorVersionForState(ctx, id, database.ConnectorVersionStateDraft)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, errors.Wrap(err, "failed to check for existing draft")
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
		return nil, errors.Wrap(err, "failed to get latest connector version")
	}

	// Decrypt and clone the latest definition
	wrapped := wrapConnectorVersion(*latest, s)
	latestDef, err := wrapped.getDefinition()
	if err != nil {
		return nil, errors.Wrap(err, "failed to decrypt latest version definition")
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
		return nil, errors.Wrap(err, "failed to build connector version")
	}

	cv.ConnectorVersion.Labels = latest.Labels

	if err := s.db.UpsertConnectorVersion(ctx, &cv.ConnectorVersion); err != nil {
		return nil, errors.Wrap(err, "failed to upsert connector version")
	}

	return s.getConnectorVersion(ctx, id, newVersion)
}
