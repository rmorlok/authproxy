package core

import (
	"context"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
)

func (s *service) GetConnectorVersion(ctx context.Context, id uuid.UUID, version uint64) (iface.ConnectorVersion, error) {
	return s.getConnectorVersion(ctx, id, version)
}

func (s *service) getConnectorVersion(ctx context.Context, id uuid.UUID, version uint64) (*ConnectorVersion, error) {
	cv, err := s.db.GetConnectorVersion(ctx, id, version)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	wrapped := wrapConnectorVersion(*cv, s)

	// Make sure we can load the connector definition from the encrypted value
	_, err = wrapped.getDefinition()
	if err != nil {
		return nil, err
	}

	return wrapped, nil
}

func (s *service) getConnectorVersions(ctx context.Context, requested []iface.ConnectorVersionId) (map[iface.ConnectorVersionId]*ConnectorVersion, error) {
	results, err := s.db.GetConnectorVersions(ctx, requested)
	if err != nil {
		return nil, err
	}

	if results == nil {
		return nil, nil
	}

	wrappedResults := make(map[iface.ConnectorVersionId]*ConnectorVersion, len(results))
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

func (s *service) GetConnectorVersions(ctx context.Context, requested []iface.ConnectorVersionId) (map[iface.ConnectorVersionId]iface.ConnectorVersion, error) {
	results, err := s.getConnectorVersions(ctx, requested)
	if err != nil {
		return nil, err
	}

	wrappedResults := make(map[iface.ConnectorVersionId]iface.ConnectorVersion, len(results))
	for k, v := range results {
		wrappedResults[k] = v
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
		if errors.Is(err, ErrNotFound) {
			logger.Error("connector is missing for connector version", "error", err)
			return nil, errors.Wrap(err, "connector is missing for connector version")
		}

		logger.Error("failed to get connector for connection", "error", err)
		return nil, errors.Wrap(err, "failed to get connector for connection")
	}

	return wrapConnection(dbConn, cv, s), nil
}
