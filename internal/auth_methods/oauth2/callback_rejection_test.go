package oauth2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bufLogger returns a slog logger writing JSON records to an in-memory
// buffer plus a parser that returns each emitted record as a map. Tests
// use this to assert on category/field shape without coupling to text
// formatting.
func bufLogger(t *testing.T) (*slog.Logger, func() []map[string]any) {
	t.Helper()
	var (
		mu  sync.Mutex
		buf bytes.Buffer
	)
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(h)
	read := func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		records := []map[string]any{}
		dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
		for dec.More() {
			var rec map[string]any
			if err := dec.Decode(&rec); err != nil {
				t.Fatalf("decode log record: %v", err)
			}
			records = append(records, rec)
		}
		return records
	}
	return logger, read
}

// onlyRejection filters log records to just the "oauth callback rejected"
// events. Other emissions (debug logging, etc.) are ignored so assertions
// don't have to know about adjacent log lines.
func onlyRejection(records []map[string]any) []map[string]any {
	out := []map[string]any{}
	for _, r := range records {
		if r["msg"] == rejectionEventMessage {
			out = append(out, r)
		}
	}
	return out
}

func TestEmitCallbackRejection_PopulatesProvidedFieldsOnly(t *testing.T) {
	logger, read := bufLogger(t)
	stateId := apid.New(apid.PrefixOauth2State)
	actorId := apid.New(apid.PrefixActor)

	emitCallbackRejection(context.Background(), logger, rejectionUnknownState, rejectionAttrs{
		StateId: stateId,
		ActorId: actorId,
		Err:     errors.New("not found"),
	})

	records := onlyRejection(read())
	require.Len(t, records, 1)
	r := records[0]
	assert.Equal(t, "WARN", r["level"])
	assert.Equal(t, string(rejectionUnknownState), r["category"])
	assert.Equal(t, stateId.String(), r["state_id"])
	assert.Equal(t, actorId.String(), r["actor_id"])
	assert.Equal(t, "not found", r["error"])
	// Connection id and namespace were not set, so they must not appear.
	_, hasConn := r["connection_id"]
	_, hasNs := r["namespace"]
	assert.False(t, hasConn, "connection_id should be omitted when not provided")
	assert.False(t, hasNs, "namespace should be omitted when not provided")
}

func TestEmitCallbackRejection_AllFieldsPresent(t *testing.T) {
	logger, read := bufLogger(t)
	stateId := apid.New(apid.PrefixOauth2State)
	actorId := apid.New(apid.PrefixActor)
	connId := apid.New(apid.PrefixConnection)

	emitCallbackRejection(context.Background(), logger, rejectionNamespaceMismatchActor, rejectionAttrs{
		StateId:      stateId,
		ActorId:      actorId,
		ConnectionId: connId,
		Namespace:    "root.tenant-a",
		Err:          errors.New("namespace mismatch"),
	})

	records := onlyRejection(read())
	require.Len(t, records, 1)
	r := records[0]
	assert.Equal(t, string(rejectionNamespaceMismatchActor), r["category"])
	assert.Equal(t, connId.String(), r["connection_id"])
	assert.Equal(t, "root.tenant-a", r["namespace"])
}

func TestEmitMissingStateRejection_RouteWrapper(t *testing.T) {
	logger, read := bufLogger(t)
	EmitMissingStateRejection(context.Background(), logger, errors.New("no state"))

	records := onlyRejection(read())
	require.Len(t, records, 1)
	assert.Equal(t, string(rejectionMissingState), records[0]["category"])
	// State id never parsed, so no state_id field should be emitted.
	_, hasStateId := records[0]["state_id"]
	assert.False(t, hasStateId)
}

func TestEmitInvalidStateFormatRejection_RouteWrapper(t *testing.T) {
	logger, read := bufLogger(t)
	EmitInvalidStateFormatRejection(context.Background(), logger, errors.New("bad format"))

	records := onlyRejection(read())
	require.Len(t, records, 1)
	assert.Equal(t, string(rejectionInvalidStateFormat), records[0]["category"])
}
