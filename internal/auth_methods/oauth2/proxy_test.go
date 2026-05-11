package oauth2

import (
	"context"
	"errors"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClassifyAndRecordRefreshFailure_PermanentFlipsUnhealthy is the
// load-bearing wiring test for this PR: every permanent refresh failure
// category must flip the connection's health_state to unhealthy with a
// reason of "refresh_<category>". A regression here is what user-visible
// reauth surfaces (the marketplace "reconnect" prompt) keys off — if a
// permanent failure stops flipping unhealthy, the user never sees the
// prompt and the integration silently dies.
func TestClassifyAndRecordRefreshFailure_PermanentFlipsUnhealthy(t *testing.T) {
	permanent := []tokenRefreshCategory{
		tokenRefreshNoRefreshToken,
		tokenRefreshInvalidGrant,
		tokenRefreshInvalidClient,
		tokenRefreshProvider4xxOther,
		tokenRefreshMalformedResponse,
	}
	for _, cat := range permanent {
		t.Run(string(cat), func(t *testing.T) {
			logger, read := bufLogger(t)
			conn := &mockCore.Connection{
				Id:          apid.New(apid.PrefixConnection),
				HealthState: database.ConnectionHealthStateHealthy,
			}
			o := &oAuth2Connection{logger: logger, connection: conn}

			err := o.classifyAndRecordRefreshFailure(
				context.Background(),
				cat,
				400,
				"invalid_grant",
				errors.New("status 400"),
			)
			require.Error(t, err, "the underlying error must be propagated")
			assert.Equal(t, database.ConnectionHealthStateUnhealthy, conn.HealthState,
				"permanent category must flip the connection to unhealthy")

			records := onlyTokenRefreshFailure(read())
			require.Len(t, records, 1)
			assert.Equal(t, string(cat), records[0]["category"])
		})
	}
}

// TestClassifyAndRecordRefreshFailure_TransientLeavesHealthAlone — the
// other half of the permanent/transient invariant. A 5xx or transport
// failure should *not* flip the connection: the next proxy call gets
// another attempt, and a transient hiccup must not generate a spurious
// "please reconnect" prompt for the end user.
func TestClassifyAndRecordRefreshFailure_TransientLeavesHealthAlone(t *testing.T) {
	transient := []tokenRefreshCategory{
		tokenRefreshNetworkError,
		tokenRefreshProvider5xx,
		tokenRefreshInternalError,
	}
	for _, cat := range transient {
		t.Run(string(cat), func(t *testing.T) {
			logger, read := bufLogger(t)
			conn := &mockCore.Connection{
				Id:          apid.New(apid.PrefixConnection),
				HealthState: database.ConnectionHealthStateHealthy,
			}
			o := &oAuth2Connection{logger: logger, connection: conn}

			err := o.classifyAndRecordRefreshFailure(
				context.Background(),
				cat,
				503,
				"",
				errors.New("status 503"),
			)
			require.Error(t, err)
			assert.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState,
				"transient category must not flip the connection")

			records := onlyTokenRefreshFailure(read())
			require.Len(t, records, 1, "the structured event still emits — only the unhealthy flip is suppressed")
		})
	}
}

// TestClassifyAndRecordRefreshFailure_PermanentOnAlreadyUnhealthyNoop —
// MarkHealthState is idempotent, so a second permanent failure on a
// connection that is already unhealthy should not emit a duplicate
// "connection health state changed" event downstream. We verify here
// that classifyAndRecordRefreshFailure still emits the failure event
// (operators want to see every failed refresh) but the unhealthy state
// is preserved.
func TestClassifyAndRecordRefreshFailure_PermanentOnAlreadyUnhealthyNoop(t *testing.T) {
	logger, read := bufLogger(t)
	conn := &mockCore.Connection{
		Id:          apid.New(apid.PrefixConnection),
		HealthState: database.ConnectionHealthStateUnhealthy,
	}
	o := &oAuth2Connection{logger: logger, connection: conn}

	err := o.classifyAndRecordRefreshFailure(
		context.Background(),
		tokenRefreshInvalidGrant,
		400,
		"invalid_grant",
		errors.New("status 400"),
	)
	require.Error(t, err)
	assert.Equal(t, database.ConnectionHealthStateUnhealthy, conn.HealthState)

	records := onlyTokenRefreshFailure(read())
	require.Len(t, records, 1)
}

// TestClassifyAndRecordRefreshFailure_NilConnectionDoesNotPanic — the
// internal-error code paths in refreshAccessToken can fire before the
// connection field is fully populated (defensive coding around the
// constructor). Calling classifyAndRecordRefreshFailure with
// connection==nil must still emit the event and not panic.
func TestClassifyAndRecordRefreshFailure_NilConnectionDoesNotPanic(t *testing.T) {
	logger, read := bufLogger(t)
	o := &oAuth2Connection{logger: logger, connection: nil}

	err := o.classifyAndRecordRefreshFailure(
		context.Background(),
		tokenRefreshInternalError,
		0,
		"",
		errors.New("decrypt failed"),
	)
	require.Error(t, err)
	records := onlyTokenRefreshFailure(read())
	require.Len(t, records, 1)
	assert.Equal(t, string(tokenRefreshInternalError), records[0]["category"])
}

// TestClassifyAndRecordRefreshFailure_ReasonStringFormat pins the
// "refresh_<category>" reason string that lands on the
// "connection health state changed" event. Dashboards correlate this
// with the failure event by category — if the format ever drifts, the
// correlation breaks silently.
func TestClassifyAndRecordRefreshFailure_ReasonStringFormat(t *testing.T) {
	logger, _ := bufLogger(t)
	captured := &reasonCapturingConnection{
		Connection: mockCore.Connection{Id: apid.New(apid.PrefixConnection)},
	}
	o := &oAuth2Connection{logger: logger, connection: captured}

	_ = o.classifyAndRecordRefreshFailure(
		context.Background(),
		tokenRefreshInvalidGrant,
		400,
		"invalid_grant",
		errors.New("status 400"),
	)

	assert.Equal(t, "refresh_invalid_grant", captured.lastReason,
		"reason must be refresh_<category> so dashboards can correlate")
}

// reasonCapturingConnection wraps mockCore.Connection to record the
// reason argument passed to MarkHealthState. The embedded type provides
// the rest of the iface.Connection methods so we only override the one
// we want to observe.
type reasonCapturingConnection struct {
	mockCore.Connection
	lastReason string
}

func (c *reasonCapturingConnection) MarkHealthState(ctx context.Context, state database.ConnectionHealthState, reason string) error {
	c.lastReason = reason
	return c.Connection.MarkHealthState(ctx, state, reason)
}
