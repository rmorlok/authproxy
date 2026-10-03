package oauth2

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
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
	logger.Info("refresh expiring oauth tokens task started")
	defer logger.Info("refresh expiring oauth tokens task completed")

	if !th.cfg.GetRoot().Oauth.GetRefreshTokensInBackgroundOrDefault() {
		return nil
	}

	refreshWithin := th.cfg.GetRoot().Oauth.GetRefreshTokensTimeBeforeExpiryOrDefault()

	// The scan window must include overrides on stored generations, including
	// active generations still used by existing connections.
	err := th.core.ListConnectorGenerationsBuilder().ForStates([]database.ConnectorGenerationState{
		database.ConnectorGenerationStatePrimary, database.ConnectorGenerationStateActive,
	}).Enumerate(ctx, func(page pagination.PageResult[iface.Connector]) (pagination.KeepGoing, error) {
		for _, connector := range page.Results {
			auth := connector.GetDefinition().Auth
			if auth == nil {
				continue
			}
			if o2, ok := auth.Inner().(*config.AuthOAuth2); ok {
				if o2.Token.GetRefreshInBackgroundOrDefault() && o2.Token.GetRefreshTimeBeforeExpiryOrDefault(refreshWithin) > refreshWithin {
					refreshWithin = o2.Token.GetRefreshTimeBeforeExpiryOrDefault(refreshWithin)
				}
			}
		}
		return pagination.Continue, nil
	})
	if err != nil {
		return err
	}

	logger.Info("tokens being refreshed within", "within", refreshWithin)
	queuedForRefresh := 0
	err = th.db.EnumerateOAuth2TokensExpiringWithin(
		ctx,
		refreshWithin,
		func(tokensWithConnections []*database.OAuth2TokenWithConnection, lastPage bool) (keepGoing pagination.KeepGoing, err error) {
			for _, tokenWithConnection := range tokensWithConnections {
				t, err := newRefreshOauth2TokenTask(tokenWithConnection.Token.ConnectionId)
				if err != nil {
					return pagination.Stop, err
				}

				ti, err := th.asynq.EnqueueContext(ctx, t)
				if err != nil {
					return pagination.Stop, err
				}
				logger.Debug(
					"token refresh task enqueued for connection",
					"connection_id", tokenWithConnection.Token.ConnectionId,
					"token_id", tokenWithConnection.Token.Id,
					"task_id", ti.ID,
				)
				queuedForRefresh++
			}

			return pagination.Continue, nil
		},
	)

	logger.Info("completed queuing for expiring OAuth tokens", "queued", queuedForRefresh)

	return err
}
