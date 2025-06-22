package connectors

import (
	"context"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/database"
)

func (s *service) GetConnectorVersion(ctx context.Context, id uuid.UUID, version uint64) (*ConnectorVersion, error) {
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

func (s *service) GetConnectorVersionForState(ctx context.Context, id uuid.UUID, state database.ConnectorVersionState) (*ConnectorVersion, error) {
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
