package connectors

import (
	"context"
	"github.com/google/uuid"
	connIface "github.com/rmorlok/authproxy/connectors/interface"
	"github.com/rmorlok/authproxy/database"
)

func (s *service) GetConnectorVersion(ctx context.Context, id uuid.UUID, version uint64) (connIface.ConnectorVersion, error) {
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

func (s *service) GetConnectorVersions(ctx context.Context, requested []connIface.ConnectorVersionId) (map[connIface.ConnectorVersionId]connIface.ConnectorVersion, error) {
	results, err := s.db.GetConnectorVersions(ctx, requested)
	if err != nil {
		return nil, err
	}

	if results == nil {
		return nil, nil
	}

	wrappedResults := make(map[connIface.ConnectorVersionId]connIface.ConnectorVersion, len(results))
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

func (s *service) GetConnectorVersionForState(ctx context.Context, id uuid.UUID, state database.ConnectorVersionState) (connIface.ConnectorVersion, error) {
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
