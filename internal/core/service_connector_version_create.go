package core

import (
	"context"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/rmorlok/authproxy/internal/util"
)

func (s *service) CreateConnectorVersion(ctx context.Context, namespace string, definition *cschema.Connector, labels map[string]string) (iface.ConnectorVersion, error) {
	id := apctx.GetUuidGenerator(ctx).New()

	def := definition.Clone()
	def.Id = id
	def.Version = 1
	def.Namespace = util.ToPtr(namespace)
	def.State = string(database.ConnectorVersionStateDraft)

	cv, err := newConnectorVersionBuilder(s).
		WithConfig(def).
		WithId(id).
		WithVersion(1).
		WithState(database.ConnectorVersionStateDraft).
		Build()
	if err != nil {
		return nil, errors.Wrap(err, "failed to build connector version")
	}

	cv.ConnectorVersion.Labels = labels

	if err := s.db.UpsertConnectorVersion(ctx, &cv.ConnectorVersion); err != nil {
		return nil, errors.Wrap(err, "failed to upsert connector version")
	}

	return s.getConnectorVersion(ctx, id, 1)
}

func (s *service) CreateDraftConnectorVersion(ctx context.Context, id uuid.UUID, definition *cschema.Connector, labels map[string]string) (iface.ConnectorVersion, error) {
	// Check for existing draft
	existingDraft, err := s.db.GetConnectorVersionForState(ctx, id, database.ConnectorVersionStateDraft)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, errors.Wrap(err, "failed to check for existing draft")
	}
	if existingDraft != nil {
		return nil, ErrDraftAlreadyExists
	}

	// Get the latest version to determine the next version number
	latest, err := s.db.NewestConnectorVersionForId(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to get latest connector version")
	}

	newVersion := latest.Version + 1

	// If definition is nil, clone from the latest version
	var def *cschema.Connector
	if definition != nil {
		def = definition.Clone()
	} else {
		wrapped := wrapConnectorVersion(*latest, s)
		latestDef, err := wrapped.getDefinition()
		if err != nil {
			return nil, errors.Wrap(err, "failed to decrypt latest version definition")
		}
		def = latestDef.Clone()
	}

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

	if labels != nil {
		cv.ConnectorVersion.Labels = labels
	} else {
		cv.ConnectorVersion.Labels = latest.Labels
	}

	if err := s.db.UpsertConnectorVersion(ctx, &cv.ConnectorVersion); err != nil {
		return nil, errors.Wrap(err, "failed to upsert connector version")
	}

	return s.getConnectorVersion(ctx, id, newVersion)
}
