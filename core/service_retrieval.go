package core

import (
	"context"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/aplog"
	"github.com/rmorlok/authproxy/core/iface"
	"github.com/rmorlok/authproxy/database"
)

func (s *service) GetConnectorVersion(ctx context.Context, id uuid.UUID, version uint64) (iface.ConnectorVersion, error) {
	return s.getConnectorVersion(ctx, id, version)
}

func (s *service) getConnectorVersion(ctx context.Context, id uuid.UUID, version uint64) (*ConnectorVersion, error) {
	cv, err := s.db.GetConnectorVersion(ctx, id, version)
	if err != nil {
		return nil, err
	}

	if cv == nil {
		return nil, nil
	}

	wrapped := wrapConnectorVersion(*cv, s)

	// Make sure we can load the connector definition from the encrypted value
	_, err = wrapped.getDefinition()
	if err != nil {
		return nil, err
	}

	return wrapped, nil
}

func (s *service) GetConnectorVersions(ctx context.Context, requested []iface.ConnectorVersionId) (map[iface.ConnectorVersionId]iface.ConnectorVersion, error) {
	results, err := s.db.GetConnectorVersions(ctx, requested)
	if err != nil {
		return nil, err
	}

	if results == nil {
		return nil, nil
	}

	wrappedResults := make(map[iface.ConnectorVersionId]iface.ConnectorVersion, len(results))
	for id, cv := range results {
		tmp := wrapConnectorVersion(*cv, s)

		// Make sure we can load the connector definition from the encrypted value
		_, err = tmp.getDefinition()
		if err != nil {
			return nil, err
		}

		wrappedResults[id] = tmp
	}

	return wrappedResults, nil
}

func (s *service) GetConnectorVersionForState(ctx context.Context, id uuid.UUID, state database.ConnectorVersionState) (iface.ConnectorVersion, error) {
	cv, err := s.db.GetConnectorVersionForState(ctx, id, state)
	if err != nil {
		return nil, err
	}

	if cv == nil {
		return nil, nil
	}

	wrapped := wrapConnectorVersion(*cv, s)

	// Make sure we can load the connector definition from the encrypted value
	_, err = wrapped.getDefinition()
	if err != nil {
		return nil, err
	}

	return wrapped, nil
}

func (s *service) getConnectionForDb(ctx context.Context, dbConn *database.Connection) (*connection, error) {
	logger := aplog.NewBuilder(s.logger).
		WithConnectionId(dbConn.ID).
		Build()

	logger.Debug("getting connector for connection")
	cv, err := s.getConnectorVersion(ctx, dbConn.ConnectorId, dbConn.ConnectorVersion)
	if err != nil {
		logger.Error("failed to get connector for connection", "error", err)
		return nil, err
	}

	return newConnection(dbConn, s, cv), nil
}
func (s *service) getConnection(ctx context.Context, id uuid.UUID) (*connection, error) {
	logger := aplog.NewBuilder(s.logger).
		WithConnectionId(id).
		Build()

	logger.Debug("getting connection")
	dbConn, err := s.db.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(database.ErrNotFound, err) {
			logger.Info("connection not found", "error", err)
			return nil, iface.ErrConnectionNotFound
		}

		logger.Error("failed to get connection", "error", err)
		return nil, err
	}

	return s.getConnectionForDb(ctx, dbConn)
}

func (s *service) GetConnection(ctx context.Context, id uuid.UUID) (iface.Connection, error) {
	return s.getConnection(ctx, id)
}
