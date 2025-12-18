package core

import (
	"context"
	"errors"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/config/common"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
)

func (s *service) CreateConnection(
	ctx context.Context,
	namespace string,
	cv iface.ConnectorVersion,
) (connection iface.Connection, err error) {
	logger := aplog.LoggerOrDefault(cv, s)
	logger.Info("creating new connection",
		"namespace", namespace,
		"connector_id", cv.GetID(),
		"connector_version", cv.GetVersion(),
	)

	if !common.NamespaceIsSameOrChild(cv.GetNamespace(), namespace) {
		return nil, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(errors.New("connections must be created in the same or child namespace of the connector")).
			Build()
	}

	id := apctx.GetUuidGenerator(ctx).New()
	now := apctx.GetClock(ctx).Now()

	dbConn := database.Connection{
		ID:               id,
		Namespace:        namespace,
		ConnectorId:      cv.GetID(),
		ConnectorVersion: cv.GetVersion(),
		CreatedAt:        now,
		UpdatedAt:        now,
		State:            database.ConnectionStateCreated,
	}

	err = s.db.CreateConnection(ctx, &dbConn)
	if err != nil {
		logger.Error("failed to create connection", "namespace", namespace, "error", err)
		return nil, err
	}

	rawCv := cv.(*ConnectorVersion)

	logger.Info("created new connection",
		"namespace", namespace,
		"connector_id", cv.GetID(),
		"connector_version", cv.GetVersion(),
		"connection_id", id)

	return wrapConnection(&dbConn, rawCv, s), nil
}
