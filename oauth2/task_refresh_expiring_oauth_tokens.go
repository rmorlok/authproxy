package oauth2

import (
	"context"
	"fmt"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/config"
	context2 "github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
)

const taskTypeRefreshExpiringOAuthTokens = "oauth2:refresh_expiring_oauth_tokens"

func newRefreshExpiringOauth2TokensTask() (*asynq.Task, error) {
	return asynq.NewTask(taskTypeRefreshExpiringOAuthTokens, nil), nil
}

func (th *taskHandler) refreshExpiringOauth2Tokens(rctx context.Context, t *asynq.Task) error {
	ctx := context2.AsContext(rctx)

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

	fmt.Printf("refreshing oauth tokens expiring within %v\n", refreshWithin)
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
