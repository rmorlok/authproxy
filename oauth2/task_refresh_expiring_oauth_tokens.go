package oauth2

import (
	"context"
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

	connectorIdToConnector := make(map[string]*config.Connector)
	refreshWithin := th.cfg.GetRoot().Oauth.GetRefreshTokensTimeBeforeExpiryOrDefault()

	for _, connector := range th.cfg.GetRoot().Connectors {
		connectorIdToConnector[connector.Id] = &connector

		if o2, ok := connector.Auth.(*config.AuthOAuth2); ok {
			if o2.Token.GetRefreshInBackgroundOrDefault() &&
				o2.Token.GetRefreshTimeBeforeExpiryOrDefault(refreshWithin) > refreshWithin {
				refreshWithin = o2.Token.GetRefreshTimeBeforeExpiryOrDefault(refreshWithin)
			}
		}
	}

	th.logger.Info("Tokens being refreshed within", "within", refreshWithin)
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
			}

			return false, nil
		},
	)

	return err
}
