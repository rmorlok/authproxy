package oauth2

import (
	"context"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apctx"
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

// connectorRefreshPolicy is the background-refresh configuration that applies
// to tokens of connections on one connector generation.
type connectorRefreshPolicy struct {
	// refreshInBackground is false when the connector opted out of proactive
	// access-token refresh (token.refreshInBackground: false).
	refreshInBackground bool

	// refreshWithin is how long before access-token expiry the token should be
	// refreshed: the connector's token.refreshTimeBeforeExpiry, or the global
	// oauth.refreshTokensTimeBeforeExpiry when the connector doesn't set one.
	refreshWithin time.Duration
}

// refreshPolicyForConnector derives the background-refresh policy for a
// connector generation. A nil connector, or one whose auth is not OAuth2,
// gets the global defaults: background refresh enabled with the global
// window.
func refreshPolicyForConnector(
	connector iface.Connector,
	globalWithin time.Duration,
) connectorRefreshPolicy {
	policy := connectorRefreshPolicy{
		refreshInBackground: true,
		refreshWithin:       globalWithin,
	}

	if connector == nil {
		return policy
	}

	def := connector.GetDefinition()
	if def == nil || def.Auth == nil {
		return policy
	}

	o2, ok := def.Auth.Inner().(*config.AuthOAuth2)
	if !ok {
		return policy
	}

	policy.refreshInBackground = o2.Token.GetRefreshInBackgroundOrDefault()
	policy.refreshWithin = o2.Token.GetRefreshTimeBeforeExpiryOrDefault(globalWithin)

	return policy
}

// shouldRefreshInBackground decides whether the background task should
// enqueue a refresh for a token returned by the expiring-token scan, given
// the policy of the token's connector generation.
//
// The refresh-token criterion (refresh_token_refresh_at has passed) always
// applies: that column is only populated at insert time when the connector
// allows background refresh, and it already encodes the connector's own lead
// time. The access-token criterion applies only when the connector allows
// background refresh, using the connector's own window rather than the
// widest window across all connectors that the scan was run with.
func shouldRefreshInBackground(
	token *database.OAuth2Token,
	policy connectorRefreshPolicy,
	now time.Time,
) bool {
	if token.RefreshTokenRefreshAt != nil && !token.RefreshTokenRefreshAt.After(now) {
		return true
	}

	if !policy.refreshInBackground || token.AccessTokenExpiresAt == nil {
		return false
	}

	return !token.AccessTokenExpiresAt.After(now.Add(policy.refreshWithin))
}

// refreshExpiringOauth2Tokens is the periodic task that enqueues a refresh for
// every OAuth2 token due for proactive refresh.
//
// The database scan runs with the widest access-token window across all
// stored connector generations that refresh in the background, so that it
// returns a superset of candidates. Each candidate is then filtered against
// the policy of its own connection's connector generation (see
// shouldRefreshInBackground). Connector definitions are cached per
// (connector id, generation) for the run: the cache is seeded from the
// window pre-scan and misses are fetched in one batch per page of tokens.
func (th *taskHandler) refreshExpiringOauth2Tokens(
	ctx context.Context,
	t *asynq.Task,
) error {
	logger := aplog.NewBuilder(th.logger).
		WithTask(t).
		WithCtx(ctx).
		Build()
	logger.Info("refresh expiring oauth tokens task started")
	defer logger.Info("refresh expiring oauth tokens task completed")

	if !th.cfg.GetRoot().Oauth.GetRefreshTokensInBackgroundOrDefault() {
		return nil
	}

	globalWithin := th.cfg.GetRoot().Oauth.GetRefreshTokensTimeBeforeExpiryOrDefault()
	refreshWithin := globalWithin

	// policies caches the refresh policy per connector generation for this
	// run. Generations that could not be found map to the global defaults.
	policies := make(map[iface.ConnectorGenerationId]connectorRefreshPolicy)

	// The scan window must include overrides on stored generations, including
	// active generations still used by existing connections.
	err := th.core.
		ListConnectorGenerationsBuilder().
		ForStates([]database.ConnectorGenerationState{
			database.ConnectorGenerationStatePrimary,
			database.ConnectorGenerationStateActive,
		}).
		Enumerate(ctx, func(page pagination.PageResult[iface.Connector]) (pagination.KeepGoing, error) {
			for _, connector := range page.Results {
				policy := refreshPolicyForConnector(connector, globalWithin)
				policies[iface.ConnectorGenerationId{Id: connector.GetId(), Generation: connector.GetGeneration()}] = policy

				if policy.refreshInBackground && policy.refreshWithin > refreshWithin {
					refreshWithin = policy.refreshWithin
				}
			}
			return pagination.Continue, nil
		})
	if err != nil {
		return err
	}

	logger.Info("tokens being refreshed within", "within", refreshWithin)
	queuedForRefresh := 0
	skipped := 0
	err = th.db.EnumerateOAuth2TokensExpiringWithin(
		ctx,
		refreshWithin,
		func(tokensWithConnections []*database.OAuth2TokenWithConnection, lastPage bool) (keepGoing pagination.KeepGoing, err error) {
			if err := th.loadMissingRefreshPolicies(
				ctx,
				logger,
				tokensWithConnections,
				policies,
				globalWithin,
			); err != nil {
				return pagination.Stop, err
			}

			now := apctx.GetClock(ctx).Now()
			for _, tokenWithConnection := range tokensWithConnections {
				policy := policies[connectorGenerationIdForConnection(&tokenWithConnection.Connection)]
				if !shouldRefreshInBackground(
					&tokenWithConnection.Token,
					policy,
					now,
				) {
					skipped++
					continue
				}

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

	logger.Info(
		"completed queuing for expiring OAuth tokens",
		"queued", queuedForRefresh,
		"skipped", skipped,
	)

	return err
}

// connectorGenerationIdForConnection returns the connector generation a
// connection is bound to.
func connectorGenerationIdForConnection(
	c *database.Connection,
) iface.ConnectorGenerationId {
	return iface.ConnectorGenerationId{
		Id:         c.ConnectorId,
		Generation: c.ConnectorGeneration,
	}
}

// loadMissingRefreshPolicies fills policies for any connector generation
// referenced by tokens that is not already cached, using a single batch
// lookup. Generations that the lookup does not return (e.g. deleted
// connectors) are cached with the global defaults so they are not looked up
// again, and are logged since the refresh itself is likely to fail.
func (th *taskHandler) loadMissingRefreshPolicies(
	ctx context.Context,
	logger *slog.Logger,
	tokens []*database.OAuth2TokenWithConnection,
	policies map[iface.ConnectorGenerationId]connectorRefreshPolicy,
	globalWithin time.Duration,
) error {
	var missing []iface.ConnectorGenerationId
	seen := make(map[iface.ConnectorGenerationId]struct{})

	// Establish the connectro generations we need to load
	for _, tokenWithConnection := range tokens {
		id := connectorGenerationIdForConnection(&tokenWithConnection.Connection)
		if _, ok := policies[id]; ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		missing = append(missing, id)
	}

	if len(missing) == 0 {
		return nil
	}

	connectors, err := th.core.GetConnectorGenerations(ctx, missing)
	if err != nil {
		return err
	}

	for _, id := range missing {
		connector, ok := connectors[id]
		if !ok {
			logger.Warn(
				"connector generation not found for oauth token refresh; using global refresh settings",
				"connector_id", id.Id,
				"connector_generation", id.Generation,
			)
		}

		policies[id] = refreshPolicyForConnector(connector, globalWithin)
	}

	return nil
}
