package core

import (
	"context"
	"errors"

	apauth "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	ns "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

func (s *service) CreateConnection(
	ctx context.Context,
	namespace string,
	name scommon.ResourceName,
	c iface.Connector,
) (connection iface.Connection, err error) {
	logger := aplog.LoggerOrDefault(c, s)
	logger.Info("creating new connection",
		"namespace", namespace,
		"connector_id", c.GetId(),
		"connector_version", c.GetVersion(),
	)

	if !ns.IsSameOrChild(c.GetNamespace(), namespace) {
		return nil, httperr.BadRequest("connections must be created in the same or child namespace of the connector")
	}

	actor := apauth.ActorFromContext(ctx)
	if !ns.IsSameOrChild(actor.GetNamespace(), namespace) {
		return nil, httperr.Forbidden("connection namespace must be child of creating actor")
	}

	id := apctx.GetIdGenerator(ctx).New(apid.PrefixConnection)
	now := apctx.GetClock(ctx).Now()

	dbConn := database.Connection{
		Id:               id,
		Namespace:        namespace,
		Name:             name,
		ConnectorId:      c.GetId(),
		ConnectorVersion: c.GetVersion(),
		CreatedAt:        now,
		UpdatedAt:        now,
		State:            database.ConnectionStateSetup,
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
