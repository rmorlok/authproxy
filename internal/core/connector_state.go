package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/database"
)

func (cv *Connector) SetState(ctx context.Context, state database.ConnectorDefinitionVersionState) error {
	cv.l.Debug("setting connector version state", "current_memory_state", cv.ConnectorWithDefinition.State, "to_state", state)
	err := cv.s.db.SetConnectorDefinitionVersionState(ctx, cv.ConnectorWithDefinition.Id, cv.ConnectorWithDefinition.Version, state)
	if err == nil {
		cv.ConnectorWithDefinition.State = state
	}

	return err
}
