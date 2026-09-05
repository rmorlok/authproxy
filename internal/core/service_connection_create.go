package core

import (
	"context"
	"errors"
	"fmt"

	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	ns "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

func (s *service) CreateConnection(
	ctx context.Context,
	namespace string,
	name scommon.ResourceName,
	labels map[string]string,
	annotations map[string]string,
	c iface.Connector,
) (connection iface.Connection, err error) {
	logger := aplog.LoggerOrDefault(c, s)
	logger.Info("creating new connection",
		"namespace", namespace,
		"connector_id", c.GetId(),
		"connector_version", c.GetVersion(),
	)

	if !ns.IsSameOrChild(c.GetNamespace(), namespace) {
		return nil, httperr.BadRequestErr(errors.New("connections must be created in the same or child namespace of the connector"))
	}

	id := apctx.GetIdGenerator(ctx).New(apid.PrefixConnection)
	now := apctx.GetClock(ctx).Now()

	dbConn := database.Connection{
		Id:               id,
		Namespace:        namespace,
		Name:             name,
		ConnectorId:      c.GetId(),
		ConnectorVersion: c.GetVersion(),
		Labels:           database.Labels(labels),
		Annotations:      database.Annotations(annotations),
		CreatedAt:        now,
		UpdatedAt:        now,
		State:            database.ConnectionStateSetup,
	}
	if actor := authcore.ActorFromContext(ctx); actor != nil && !actor.GetId().IsNil() {
		actorID := actor.GetId()
		dbConn.ActorId = &actorID
	}

	err = s.db.CreateConnection(ctx, &dbConn)
	if err != nil {
		logger.Error("failed to create connection", "namespace", namespace, "error", err)
		return nil, err
	}

	connector := c.(*Connector)

	logger.Info("created new connection",
		"namespace", namespace,
		"connector_id", c.GetId(),
		"connector_version", c.GetVersion(),
		"connection_id", id)

	return wrapConnection(&dbConn, connector, s), nil
}

func (s *service) UpdateConnectionName(ctx context.Context, id apid.ID, name scommon.ResourceName) (iface.Connection, error) {
	dbConn, err := s.db.UpdateConnectionName(ctx, id, name)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	c, err := s.getConnectorVersion(ctx, dbConn.ConnectorId, dbConn.ConnectorVersion)
	if err != nil {
		return nil, err
	}
	return wrapConnection(dbConn, c, s), nil
}

func (s *service) UpdateConnection(
	ctx context.Context,
	id apid.ID,
	patch *connectionschema.ConnectionPatch,
) (iface.Connection, error) {
	current, err := s.getConnection(ctx, id)
	if err != nil {
		return nil, err
	}

	currentResource, err := current.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	updated, err := currentResource.ApplyUpdate(patch)
	if err != nil {
		return nil, fmt.Errorf("invalid connection update: %w", err)
	}

	if updated.Metadata.Name != current.GetName() {
		if _, err := s.db.UpdateConnectionName(ctx, id, updated.Metadata.Name); err != nil {
			if errors.Is(err, database.ErrNotFound) {
				return nil, ErrNotFound
			}
			return nil, err
		}
	}
	if patch.Metadata.Labels != nil {
		if _, err := s.db.UpdateConnectionLabels(ctx, id, updated.Metadata.Labels); err != nil {
			return nil, err
		}
	}
	if patch.Metadata.Annotations != nil {
		if _, err := s.db.UpdateConnectionAnnotations(ctx, id, updated.Metadata.Annotations); err != nil {
			return nil, err
		}
	}

	return s.GetConnection(ctx, id)
}
