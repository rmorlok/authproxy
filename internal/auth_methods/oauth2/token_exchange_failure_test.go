package oauth2

import (
	"context"
	"errors"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func onlyTokenExchangeFailure(records []map[string]any) []map[string]any {
	out := []map[string]any{}
	for _, r := range records {
		if r["msg"] == tokenExchangeFailureMessage {
			out = append(out, r)
		}
	}
	return out
}

func TestEmitTokenExchangeFailure_PopulatesProvidedFieldsOnly(t *testing.T) {
	logger, read := bufLogger(t)
	stateId := apid.New(apid.PrefixOauth2State)
	actorId := apid.New(apid.PrefixActor)

	emitTokenExchangeFailure(context.Background(), logger, tokenExchangeInvalidGrant, tokenExchangeAttrs{
		StateId:            stateId,
		ActorId:            actorId,
		ProviderStatusCode: 400,
		ProviderError:      "invalid_grant",
		Err:                errors.New("status 400"),
	})

	records := onlyTokenExchangeFailure(read())
	require.Len(t, records, 1)
	r := records[0]
	assert.Equal(t, "WARN", r["level"])
	assert.Equal(t, string(tokenExchangeInvalidGrant), r["category"])
	assert.Equal(t, stateId.String(), r["state_id"])
	assert.Equal(t, actorId.String(), r["actor_id"])
	assert.Equal(t, float64(400), r["provider_status_code"])
	assert.Equal(t, "invalid_grant", r["provider_error"])
	assert.Equal(t, "status 400", r["error"])
	_, hasConn := r["connection_id"]
	_, hasNs := r["namespace"]
	assert.False(t, hasConn, "connection_id omitted when not provided")
	assert.False(t, hasNs, "namespace omitted when not provided")
}

func TestEmitTokenExchangeFailure_OmitsZeroStatusCode(t *testing.T) {
	logger, read := bufLogger(t)
	emitTokenExchangeFailure(context.Background(), logger, tokenExchangeNetworkError, tokenExchangeAttrs{
		Err: errors.New("dial tcp: connection refused"),
	})

	records := onlyTokenExchangeFailure(read())
	require.Len(t, records, 1)
	r := records[0]
	assert.Equal(t, string(tokenExchangeNetworkError), r["category"])
	_, hasStatus := r["provider_status_code"]
	_, hasProviderErr := r["provider_error"]
	assert.False(t, hasStatus, "provider_status_code omitted when zero")
	assert.False(t, hasProviderErr, "provider_error omitted when empty")
}

func TestClassifyTokenEndpointStatus_RecognizedRFCErrors(t *testing.T) {
	cases := []struct {
		name             string
		status           int
		body             string
		wantCategory     tokenExchangeCategory
		wantProviderCode string
	}{
		{"invalid_grant", 400, `{"error":"invalid_grant","error_description":"code expired"}`, tokenExchangeInvalidGrant, "invalid_grant"},
		{"invalid_client", 401, `{"error":"invalid_client"}`, tokenExchangeInvalidClient, "invalid_client"},
		{"invalid_request", 400, `{"error":"invalid_request"}`, tokenExchangeInvalidRequest, "invalid_request"},
		{"unauthorized_client", 400, `{"error":"unauthorized_client"}`, tokenExchangeUnauthorizedClient, "unauthorized_client"},
		{"unsupported_grant_type", 400, `{"error":"unsupported_grant_type"}`, tokenExchangeUnsupportedGrantType, "unsupported_grant_type"},
		{"invalid_scope", 400, `{"error":"invalid_scope"}`, tokenExchangeInvalidScope, "invalid_scope"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat, code := classifyTokenEndpointStatus(tc.status, []byte(tc.body))
			assert.Equal(t, tc.wantCategory, cat)
			assert.Equal(t, tc.wantProviderCode, code)
		})
	}
}

func TestClassifyTokenEndpointStatus_5xxAlwaysTransient(t *testing.T) {
	for _, status := range []int{500, 502, 503, 504} {
		cat, code := classifyTokenEndpointStatus(status, []byte(`{"error":"invalid_grant"}`))
		assert.Equal(t, tokenExchangeProvider5xx, cat,
			"5xx must classify as transient regardless of body (status %d)", status)
		assert.Empty(t, code, "providerError empty for 5xx (we don't trust the body)")
	}
}

func TestClassifyTokenEndpointStatus_UnknownErrorCode(t *testing.T) {
	cat, code := classifyTokenEndpointStatus(400, []byte(`{"error":"some_provider_specific_error"}`))
	assert.Equal(t, tokenExchangeProvider4xxOther, cat)
	assert.Equal(t, "some_provider_specific_error", code,
		"raw provider error string preserved for diagnostics even when category is generic")
}

func TestClassifyTokenEndpointStatus_UnparseableBody(t *testing.T) {
	cat, code := classifyTokenEndpointStatus(400, []byte(`<html>not json</html>`))
	assert.Equal(t, tokenExchangeProvider4xxOther, cat)
	assert.Empty(t, code)
}

func TestClassifyTokenEndpointStatus_EmptyBody(t *testing.T) {
	cat, code := classifyTokenEndpointStatus(403, nil)
	assert.Equal(t, tokenExchangeProvider4xxOther, cat)
	assert.Empty(t, code)
}
