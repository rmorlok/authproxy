package oauth2

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/config"
	context2 "github.com/rmorlok/authproxy/context"
)

const taskTypeRefreshOAuthToken = "oauth2:refresh_oauth_token"

func newRefreshOauth2TokenTask(connectionId uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(refreshOAuthTokenTaskPayload{connectionId})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(taskTypeRefreshOAuthToken, payload), nil
}

type refreshOAuthTokenTaskPayload struct {
	ConnectionId uuid.UUID `json:"connection_id"`
}

func (th *taskHandler) refreshOauth2Token(rctx context.Context, t *asynq.Task) error {
	ctx := context2.AsContext(rctx)

	var p refreshOAuthTokenTaskPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("%s json.Unmarshal failed: %v: %w", taskTypeRefreshOAuthToken, err, asynq.SkipRetry)
	}

	if p.ConnectionId == uuid.Nil {
		return fmt.Errorf("%s connection id not specified: %w", taskTypeRefreshOAuthToken, asynq.SkipRetry)
	}

	connection, err := th.db.GetConnection(ctx, p.ConnectionId)
	if err != nil {
		return fmt.Errorf("failed to load connection: %v", err)
	}

	if connection == nil {
		return fmt.Errorf("connection not found: %w", asynq.SkipRetry)
	}

	var connector config.Connector
	found := false
	for _, connector = range th.cfg.GetRoot().Connectors {
		if connector.Id == connection.ConnectorId {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("connector '%s' not found for connection %v: %w", connection.ConnectorId, connection.ID, asynq.SkipRetry)
	}

	token, err := th.db.GetOAuth2Token(ctx, connection.ID)
	if err != nil {
		return fmt.Errorf("failed to load oauth token: %v", err)
	}

	if token == nil {
		return fmt.Errorf("oauth token not found for connection %v: %w", connection.ID, asynq.SkipRetry)
	}

	o2 := th.factory.NewOAuth2(*connection, connector)
	_, err = o2.refreshAccessToken(ctx, token, refreshModeAlways)
	return err
}
