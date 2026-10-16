package oauth2

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	asynqmock "github.com/rmorlok/authproxy/internal/apasynq/mock"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	dbmock "github.com/rmorlok/authproxy/internal/database/mock"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

// refreshCore is a fake core service exposing only the connector lookups the
// background refresh task uses.
type refreshCore struct {
	iface.C
	builder *refreshGenerations

	// byGeneration backs GetConnectorGenerations; requests records each
	// batch requested so tests can assert on lookup counts.
	byGeneration map[iface.ConnectorGenerationId]iface.Connector
	requests     [][]iface.ConnectorGenerationId
}

func (c *refreshCore) ListConnectorGenerationsBuilder() iface.ListConnectorGenerationsBuilder {
	return c.builder
}

func (c *refreshCore) GetConnectorGenerations(_ context.Context, requested []iface.ConnectorGenerationId) (map[iface.ConnectorGenerationId]iface.Connector, error) {
	c.requests = append(c.requests, requested)
	result := make(map[iface.ConnectorGenerationId]iface.Connector)
	for _, id := range requested {
		if connector, ok := c.byGeneration[id]; ok {
			result[id] = connector
		}
	}
	return result, nil
}

// refreshGenerations is a fake connector-generation list builder that returns
// a fixed set of connectors in one page.
type refreshGenerations struct {
	iface.ListConnectorGenerationsBuilder
	states     []database.ConnectorGenerationState
	connectors []iface.Connector
	err        error
}

func (b *refreshGenerations) ForStates(states []database.ConnectorGenerationState) iface.ListConnectorGenerationsBuilder {
	b.states = states
	return b
}
func (b *refreshGenerations) Enumerate(ctx context.Context, cb pagination.EnumerateCallback[iface.Connector]) error {
	if b.err != nil {
		return b.err
	}
	_, err := cb(pagination.PageResult[iface.Connector]{Results: b.connectors})
	return err
}

// refreshConnector is a fake connector generation with a fixed definition.
type refreshConnector struct {
	iface.Connector
	id         apid.ID
	generation uint64
	definition *cschema.ConnectorDefinition
}

func (c refreshConnector) GetId() apid.ID                              { return c.id }
func (c refreshConnector) GetGeneration() uint64                       { return c.generation }
func (c refreshConnector) GetDefinition() *cschema.ConnectorDefinition { return c.definition }

func (c refreshConnector) generationId() iface.ConnectorGenerationId {
	return iface.ConnectorGenerationId{Id: c.id, Generation: c.generation}
}

// oauth2RefreshConnector builds a fake OAuth2 connector generation with the
// given token refresh settings; nil leaves the setting unset.
func oauth2RefreshConnector(refreshInBackground *bool, refreshTimeBeforeExpiry *time.Duration) refreshConnector {
	token := cschema.AuthOauth2Token{RefreshInBackground: refreshInBackground}
	if refreshTimeBeforeExpiry != nil {
		token.RefreshTimeBeforeExpiry = &sconfig.HumanDuration{Duration: *refreshTimeBeforeExpiry}
	}
	return refreshConnector{
		id:         apid.New(apid.PrefixConnector),
		generation: 1,
		definition: &cschema.ConnectorDefinition{
			Auth: &cschema.Auth{InnerVal: &cschema.AuthOAuth2{Token: token}},
		},
	}
}

func TestRefreshScanUsesStoredConnectorOverrides(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "stored override", true: "read failure"}[fail], func(t *testing.T) {
			ctrl := gomock.NewController(t)
			db := dbmock.NewMockDB(ctrl)
			builder := &refreshGenerations{connectors: []iface.Connector{
				refreshConnector{id: apid.New(apid.PrefixConnector), generation: 1, definition: &cschema.ConnectorDefinition{}},
				oauth2RefreshConnector(nil, util.ToPtr(45*time.Minute)),
				// A connector that opted out of background refresh must not
				// widen the scan window.
				oauth2RefreshConnector(util.ToPtr(false), util.ToPtr(3*time.Hour)),
			}}
			if fail {
				builder.err = errors.New("cannot read stored generations")
			} else {
				db.EXPECT().EnumerateOAuth2TokensExpiringWithin(gomock.Any(), 45*time.Minute, gomock.Any()).Return(nil)
			}

			handler := taskHandler{cfg: config.FromRoot(&sconfig.Root{}), db: db, core: &refreshCore{builder: builder}, logger: aplog.NewNoopLogger()}
			task, err := newRefreshExpiringOauth2TokensTask()
			require.NoError(t, err)

			err = handler.refreshExpiringOauth2Tokens(context.Background(), task)
			if fail {
				require.ErrorIs(t, err, builder.err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, []database.ConnectorGenerationState{
				database.ConnectorGenerationStatePrimary,
				database.ConnectorGenerationStateActive,
			}, builder.states)
		})
	}
}

func TestRefreshExpiringTokensHonorsConnectorSettings(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	// Global window is the default 10m.
	defaultConnector := oauth2RefreshConnector(nil, nil)
	wideConnector := oauth2RefreshConnector(nil, util.ToPtr(2*time.Hour))
	optOutConnector := oauth2RefreshConnector(util.ToPtr(false), nil)
	// Generation 2 of the default connector: not primary/active, so it is
	// absent from the pre-scan and must be fetched on demand.
	archivedConnector := oauth2RefreshConnector(util.ToPtr(false), nil)
	archivedConnector.id = defaultConnector.id
	archivedConnector.generation = 2
	missingConnector := oauth2RefreshConnector(nil, nil)

	type tokenCase struct {
		name           string
		connector      refreshConnector
		accessExpiry   time.Duration
		refreshAt      *time.Duration
		expectEnqueued bool
	}
	cases := []tokenCase{
		// Default connector uses the 10m global window, not the 2h window
		// from wideConnector that the scan ran with.
		{name: "default within global window", connector: defaultConnector, accessExpiry: 5 * time.Minute, expectEnqueued: true},
		{name: "default outside global window", connector: defaultConnector, accessExpiry: time.Hour, expectEnqueued: false},
		{name: "wide within own window", connector: wideConnector, accessExpiry: time.Hour, expectEnqueued: true},
		{name: "wide outside own window", connector: wideConnector, accessExpiry: 3 * time.Hour, expectEnqueued: false},
		// Opted-out connectors skip the access-token criterion entirely...
		{name: "opt-out access expiring", connector: optOutConnector, accessExpiry: time.Minute, expectEnqueued: false},
		// ...but a due refresh_token_refresh_at is still honored.
		{name: "opt-out refresh token due", connector: optOutConnector, accessExpiry: time.Minute, refreshAt: util.ToPtr(-time.Minute), expectEnqueued: true},
		{name: "refresh token due far from access expiry", connector: defaultConnector, accessExpiry: 24 * time.Hour, refreshAt: util.ToPtr(time.Duration(0)), expectEnqueued: true},
		{name: "refresh token not yet due", connector: defaultConnector, accessExpiry: 24 * time.Hour, refreshAt: util.ToPtr(time.Minute), expectEnqueued: false},
		// Generations outside the pre-scan are resolved via batch lookup.
		{name: "archived generation opted out", connector: archivedConnector, accessExpiry: time.Minute, expectEnqueued: false},
		// Missing connectors fall back to the global settings.
		{name: "missing connector within global window", connector: missingConnector, accessExpiry: 5 * time.Minute, expectEnqueued: true},
		{name: "missing connector outside global window", connector: missingConnector, accessExpiry: time.Hour, expectEnqueued: false},
	}

	var rows []*database.OAuth2TokenWithConnection
	var expected []apid.ID
	for _, tc := range cases {
		cxnId := apid.New(apid.PrefixConnection)
		row := &database.OAuth2TokenWithConnection{
			Token: database.OAuth2Token{
				Id:                   apid.New(apid.PrefixOAuth2Token),
				ConnectionId:         cxnId,
				AccessTokenExpiresAt: util.ToPtr(now.Add(tc.accessExpiry)),
			},
			Connection: database.Connection{
				Id:                  cxnId,
				ConnectorId:         tc.connector.id,
				ConnectorGeneration: tc.connector.generation,
			},
		}
		if tc.refreshAt != nil {
			row.Token.RefreshTokenRefreshAt = util.ToPtr(now.Add(*tc.refreshAt))
		}
		rows = append(rows, row)
		if tc.expectEnqueued {
			expected = append(expected, cxnId)
		}
	}

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockDB(ctrl)
	ac := asynqmock.NewMockClient(ctrl)

	db.EXPECT().
		EnumerateOAuth2TokensExpiringWithin(gomock.Any(), 2*time.Hour, gomock.Any()).
		DoAndReturn(func(
			_ context.Context,
			_ time.Duration,
			cb func([]*database.OAuth2TokenWithConnection, bool) (pagination.KeepGoing, error),
		) error {
			// Two pages, so the second page exercises the cache populated
			// by the first.
			mid := len(rows) / 2
			if _, err := cb(rows[:mid], false); err != nil {
				return err
			}
			_, err := cb(rows[mid:], true)
			return err
		})

	var enqueued []apid.ID
	ac.EXPECT().EnqueueContext(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
		var payload refreshOAuthTokenTaskPayload
		require.NoError(t, json.Unmarshal(task.Payload(), &payload))
		enqueued = append(enqueued, payload.ConnectionId)
		return &asynq.TaskInfo{ID: "task"}, nil
	}).AnyTimes()

	core := &refreshCore{
		builder: &refreshGenerations{connectors: []iface.Connector{defaultConnector, wideConnector, optOutConnector}},
		byGeneration: map[iface.ConnectorGenerationId]iface.Connector{
			archivedConnector.generationId(): archivedConnector,
		},
	}
	handler := taskHandler{
		cfg:    config.FromRoot(&sconfig.Root{}),
		db:     db,
		core:   core,
		asynq:  ac,
		logger: aplog.NewNoopLogger(),
	}

	task, err := newRefreshExpiringOauth2TokensTask()
	require.NoError(t, err)
	require.NoError(t, handler.refreshExpiringOauth2Tokens(ctx, task))

	sortIds := func(ids []apid.ID) {
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	}
	sortIds(expected)
	sortIds(enqueued)
	require.Equal(t, expected, enqueued)

	// Only generations absent from the pre-scan are looked up, each once,
	// even though the missing connector appears on both pages.
	var requested []iface.ConnectorGenerationId
	for _, batch := range core.requests {
		requested = append(requested, batch...)
	}
	require.ElementsMatch(t, []iface.ConnectorGenerationId{
		archivedConnector.generationId(),
		missingConnector.generationId(),
	}, requested)
}

func TestShouldRefreshInBackground(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	enabled := connectorRefreshPolicy{refreshInBackground: true, refreshWithin: 10 * time.Minute}
	disabled := connectorRefreshPolicy{refreshInBackground: false, refreshWithin: 10 * time.Minute}

	// No access-token expiry and no refresh-token schedule: nothing to do.
	require.False(t, shouldRefreshInBackground(&database.OAuth2Token{}, enabled, now))

	// Boundaries are inclusive, matching the database scan's <= comparisons.
	require.True(t, shouldRefreshInBackground(&database.OAuth2Token{AccessTokenExpiresAt: util.ToPtr(now.Add(10 * time.Minute))}, enabled, now))
	require.False(t, shouldRefreshInBackground(&database.OAuth2Token{AccessTokenExpiresAt: util.ToPtr(now.Add(10*time.Minute + time.Second))}, enabled, now))
	require.True(t, shouldRefreshInBackground(&database.OAuth2Token{RefreshTokenRefreshAt: util.ToPtr(now)}, disabled, now))
	require.False(t, shouldRefreshInBackground(&database.OAuth2Token{AccessTokenExpiresAt: util.ToPtr(now)}, disabled, now))
}

func TestRefreshPolicyForConnector(t *testing.T) {
	global := 10 * time.Minute
	require.Equal(t, connectorRefreshPolicy{true, global}, refreshPolicyForConnector(nil, global))
	require.Equal(t, connectorRefreshPolicy{true, global}, refreshPolicyForConnector(refreshConnector{definition: &cschema.ConnectorDefinition{}}, global))
	require.Equal(t, connectorRefreshPolicy{true, global}, refreshPolicyForConnector(oauth2RefreshConnector(nil, nil), global))
	require.Equal(t, connectorRefreshPolicy{true, time.Hour}, refreshPolicyForConnector(oauth2RefreshConnector(nil, util.ToPtr(time.Hour)), global))
	require.Equal(t, connectorRefreshPolicy{false, global}, refreshPolicyForConnector(oauth2RefreshConnector(util.ToPtr(false), nil), global))
}
