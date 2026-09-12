package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
)

func (s *service) GetConnectorGeneration(ctx context.Context, id apid.ID, generation uint64) (iface.Connector, error) {
	return s.getConnectorGeneration(ctx, id, generation)
}

func (s *service) getConnectorGeneration(ctx context.Context, id apid.ID, generation uint64) (*Connector, error) {
	cv, err := s.db.GetConnectorGeneration(ctx, id, generation)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	wrapped := wrapConnector(*cv, s)

	// Make sure we can load the connector definition from the encrypted value
	_, err = wrapped.getDefinition()
	if err != nil {
		return nil, err
	}

	return wrapped, nil
}

func (s *service) getConnectorGenerations(ctx context.Context, requested []iface.ConnectorGenerationId) (map[iface.ConnectorGenerationId]*Connector, error) {
	results, err := s.db.GetConnectorGenerations(ctx, requested)
	if err != nil {
		return nil, err
	}

	if results == nil {
		return nil, nil
	}

	wrappedResults := make(map[iface.ConnectorGenerationId]*Connector, len(results))
	for id, cv := range results {
		tmp := wrapConnector(*cv, s)

		// Make sure we can load the connector definition from the encrypted value
		_, err = tmp.getDefinition()
		if err != nil {
			return nil, err
		}

		wrappedResults[id] = tmp
	}

	return wrappedResults, nil
}

func (s *service) GetConnectorGenerations(ctx context.Context, requested []iface.ConnectorGenerationId) (map[iface.ConnectorGenerationId]iface.Connector, error) {
	results, err := s.getConnectorGenerations(ctx, requested)
	if err != nil {
		return nil, err
	}

	wrappedResults := make(map[iface.ConnectorGenerationId]iface.Connector, len(results))
	for k, v := range results {
		wrappedResults[k] = v
	}

	return wrappedResults, nil
}

func (s *service) GetConnectorGenerationForState(ctx context.Context, id apid.ID, state database.ConnectorGenerationState) (iface.Connector, error) {
	cv, err := s.db.GetConnectorGenerationForState(ctx, id, state)
	if err != nil {
		return nil, err
	}

	if cv == nil {
		return nil, nil
	}

	wrapped := wrapConnector(*cv, s)

	// Make sure we can load the connector definition from the encrypted value
	_, err = wrapped.getDefinition()
	if err != nil {
		return nil, err
	}

	return wrapped, nil
}

func (s *service) getConnectionForDb(ctx context.Context, dbConn *database.Connection) (*connection, error) {
	logger := aplog.NewBuilder(s.logger).
		WithConnectionId(dbConn.Id).
		Build()

	logger.Debug("getting connector for connection")
	c, err := s.getConnectorGeneration(ctx, dbConn.ConnectorId, dbConn.ConnectorGeneration)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			logger.Error("connector is missing for connector generation", "error", err)
			return nil, fmt.Errorf("connector is missing for connector generation: %w", err)
		}

		logger.Error("failed to get connector for connection", "error", err)
		return nil, fmt.Errorf("failed to get connector for connection: %w", err)
	}

	return wrapConnection(dbConn, c, s), nil
}
