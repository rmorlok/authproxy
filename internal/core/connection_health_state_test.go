package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeJSONLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	for dec.More() {
		var rec map[string]any
		require.NoError(t, dec.Decode(&rec))
		out = append(out, rec)
	}
	return out
}

func TestMarkHealthState_HealthyToUnhealthyEmitsEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s, db, _, _, _, _ := FullMockService(t, ctrl)
	conn := newTestConnectionWithService(s)
	var buf bytes.Buffer
	conn.logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	db.EXPECT().
		SetConnectionHealthState(gomock.Any(), conn.Id, database.ConnectionHealthStateUnhealthy).
		Return(nil)

	require.NoError(t, conn.MarkHealthState(context.Background(), database.ConnectionHealthStateUnhealthy, "refresh_invalid_grant"))

	assert.Equal(t, database.ConnectionHealthStateUnhealthy, conn.HealthState)

	recs := decodeJSONLines(t, &buf)
	require.Len(t, recs, 1, "expected exactly one structured event on transition")
	assert.Equal(t, connectionHealthStateChangedMessage, recs[0]["msg"])
	assert.Equal(t, "healthy", recs[0]["previous_health_state"])
	assert.Equal(t, "unhealthy", recs[0]["health_state"])
	assert.Equal(t, "refresh_invalid_grant", recs[0]["reason"])
}

// TestMarkHealthState_IdempotentNoEvent — flipping to the current state is
// the load-bearing idempotency invariant. A refresh that succeeds when the
// connection was already healthy should not produce a "state changed"
// event, otherwise dashboards see spurious recovery transitions on every
// proxy call.
func TestMarkHealthState_IdempotentNoEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s, _, _, _, _, _ := FullMockService(t, ctrl)
	conn := newTestConnectionWithService(s)
	conn.HealthState = database.ConnectionHealthStateHealthy
	var buf bytes.Buffer
	conn.logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// No db.EXPECT call — idempotent path must not hit the DB.
	require.NoError(t, conn.MarkHealthState(context.Background(), database.ConnectionHealthStateHealthy, "refresh_succeeded"))

	assert.Empty(t, decodeJSONLines(t, &buf), "idempotent transition must not emit an event")
}

// TestMarkHealthState_UnhealthyToHealthyEmitsEvent covers the recovery
// direction. A successful refresh on a previously-unhealthy connection is
// the operator-relevant "connection recovered" signal.
func TestMarkHealthState_UnhealthyToHealthyEmitsEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s, db, _, _, _, _ := FullMockService(t, ctrl)
	conn := newTestConnectionWithService(s)
	conn.HealthState = database.ConnectionHealthStateUnhealthy
	var buf bytes.Buffer
	conn.logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	db.EXPECT().
		SetConnectionHealthState(gomock.Any(), conn.Id, database.ConnectionHealthStateHealthy).
		Return(nil)

	require.NoError(t, conn.MarkHealthState(context.Background(), database.ConnectionHealthStateHealthy, "refresh_succeeded"))

	recs := decodeJSONLines(t, &buf)
	require.Len(t, recs, 1)
	assert.Equal(t, "unhealthy", recs[0]["previous_health_state"])
	assert.Equal(t, "healthy", recs[0]["health_state"])
}

// TestMarkHealthState_RejectsInvalidValue — the helper validates the
// supplied state. The DB column has its own CHECK semantically via the
// enum but defensive validation here keeps the structured event out of the
// log on bad input.
func TestMarkHealthState_RejectsInvalidValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s, _, _, _, _, _ := FullMockService(t, ctrl)
	conn := newTestConnectionWithService(s)

	err := conn.MarkHealthState(context.Background(), database.ConnectionHealthState("bogus"), "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid health state")
}

// TestMarkHealthState_DBErrorLeavesLocalStateUnchanged — if the DB write
// fails we must not flip the in-memory state, otherwise subsequent
// idempotent calls see the wrong "current" value and skip retrying the
// write.
func TestMarkHealthState_DBErrorLeavesLocalStateUnchanged(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s, db, _, _, _, _ := FullMockService(t, ctrl)
	conn := newTestConnectionWithService(s)
	conn.HealthState = database.ConnectionHealthStateHealthy

	db.EXPECT().
		SetConnectionHealthState(gomock.Any(), conn.Id, database.ConnectionHealthStateUnhealthy).
		Return(errors.New("db boom"))

	err := conn.MarkHealthState(context.Background(), database.ConnectionHealthStateUnhealthy, "refresh_invalid_grant")
	require.Error(t, err)
	assert.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState)
}
