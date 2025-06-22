package oauth2

import (
	"context"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/aplog"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
)

const taskTypeRefreshExpiringOAuthTokens = "oauth2:refresh_expiring_oauth_tokens"

func newRefreshExpiringOauth2TokensTask() (*asynq.Task, error) {
	return asynq.NewTask(taskTypeRefreshExpiringOAuthTokens, nil), nil
}

func (th *taskHandler) refreshExpiringOauth2Tokens(ctx context.Context, t *asynq.Task) error {
	logger := aplog.NewBuilder(th.logger).
		WithTask(t).
		WithCtx(ctx).
		Build()
	logger.Info("Refresh expiring oauth tokens task started")
	defer logger.Info("Refresh expiring oauth tokens task completed")

	if !th.cfg.GetRoot().Oauth.GetRefreshTokensInBackgroundOrDefault() {
		return nil
	}

	connectorIdToConnector := make(map[uuid.UUID]*config.Connector)
	refreshWithin := th.cfg.GetRoot().Oauth.GetRefreshTokensTimeBeforeExpiryOrDefault()

	// Establish the smallest value of refreshWithIn for all active connector versions
	// TODO: migrate this to use the database stored versions
	for _, connector := range th.cfg.GetRoot().Connectors.GetConnectors() {
		connectorIdToConnector[connector.Id] = &connector

		if o2, ok := connector.Auth.(*config.AuthOAuth2); ok {
			if o2.Token.GetRefreshInBackgroundOrDefault() &&
				o2.Token.GetRefreshTimeBeforeExpiryOrDefault(refreshWithin) > refreshWithin {
				refreshWithin = o2.Token.GetRefreshTimeBeforeExpiryOrDefault(refreshWithin)
			}
		}
	}

	logger.Info("Tokens being refreshed within", "within", refreshWithin)
	queuedForRefresh := 0
	err := th.db.EnumerateOAuth2TokensExpiringWithin(
		ctx,
		refreshWithin,
		func(tokensWithConnections []*database.OAuth2TokenWithConnection, lastPage bool) (stop bool, err error) {
			for _, tokenWithConnection := range tokensWithConnections {
				t, err := newRefreshOauth2TokenTask(tokenWithConnection.ConnectionID)
				if err != nil {
					return true, err
				}

				_, err = th.asynq.EnqueueContext(ctx, t)
				if err != nil {
					return true, err
				}
				queuedForRefresh++
			}

			return false, nil
		},
	)

	logger.Info("Completed queuing for expiring OAuth tokens", "queued", queuedForRefresh)

	return err
}
