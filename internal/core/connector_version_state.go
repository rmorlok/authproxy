package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/database"
)

func (cv *ConnectorVersion) SetState(ctx context.Context, state database.ConnectorVersionState) error {
	cv.l.Debug("setting connector version state", "current_memory_state", cv.ConnectorVersion.State, "to_state", state)
	err := cv.s.db.SetConnectorVersionState(ctx, cv.ConnectorVersion.Id, cv.ConnectorVersion.Version, state)
	if err == nil {
		cv.ConnectorVersion.State = state
	}

	return err
}
