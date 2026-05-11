package oauth2

import (
	"context"
	"errors"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func onlyTokenRefreshFailure(records []map[string]any) []map[string]any {
	out := []map[string]any{}
	for _, r := range records {
		if r["msg"] == tokenRefreshFailureMessage {
			out = append(out, r)
		}
	}
	return out
}

func TestEmitTokenRefreshFailure_PopulatesProvidedFieldsOnly(t *testing.T) {
	logger, read := bufLogger(t)
	connId := apid.New(apid.PrefixConnection)

	emitTokenRefreshFailure(context.Background(), logger, tokenRefreshInvalidGrant, tokenRefreshAttrs{
		ConnectionId:       connId,
		Namespace:          "ns1",
		ProviderStatusCode: 400,
		ProviderError:      "invalid_grant",
		Err:                errors.New("status 400"),
	})

	records := onlyTokenRefreshFailure(read())
	require.Len(t, records, 1)
	r := records[0]
	assert.Equal(t, "WARN", r["level"])
	assert.Equal(t, string(tokenRefreshInvalidGrant), r["category"])
	assert.Equal(t, connId.String(), r["connection_id"])
	assert.Equal(t, "ns1", r["namespace"])
	assert.Equal(t, float64(400), r["provider_status_code"])
	assert.Equal(t, "invalid_grant", r["provider_error"])
	assert.Equal(t, "status 400", r["error"])
}

func TestEmitTokenRefreshFailure_OmitsZeroAndEmpty(t *testing.T) {
	logger, read := bufLogger(t)
	emitTokenRefreshFailure(context.Background(), logger, tokenRefreshNetworkError, tokenRefreshAttrs{
		Err: errors.New("dial tcp: connection refused"),
	})

	records := onlyTokenRefreshFailure(read())
	require.Len(t, records, 1)
	r := records[0]
	assert.Equal(t, string(tokenRefreshNetworkError), r["category"])
	_, hasStatus := r["provider_status_code"]
	_, hasProviderErr := r["provider_error"]
	_, hasConn := r["connection_id"]
	_, hasNs := r["namespace"]
	assert.False(t, hasStatus, "provider_status_code omitted when zero")
	assert.False(t, hasProviderErr, "provider_error omitted when empty")
	assert.False(t, hasConn, "connection_id omitted when nil")
	assert.False(t, hasNs, "namespace omitted when empty")
}

func TestEmitTokenRefreshSucceeded(t *testing.T) {
	logger, read := bufLogger(t)
	connId := apid.New(apid.PrefixConnection)

	emitTokenRefreshSucceeded(context.Background(), logger, tokenRefreshAttrs{
		ConnectionId: connId,
		Namespace:    "ns1",
	})

	records := read()
	require.Len(t, records, 1)
	r := records[0]
	assert.Equal(t, "INFO", r["level"])
	assert.Equal(t, tokenRefreshSuccessMessage, r["msg"])
	assert.Equal(t, connId.String(), r["connection_id"])
	assert.Equal(t, "ns1", r["namespace"])
}

func TestTokenRefreshCategory_IsPermanent(t *testing.T) {
	cases := []struct {
		category tokenRefreshCategory
		want     bool
	}{
		{tokenRefreshNoRefreshToken, true},
		{tokenRefreshInvalidGrant, true},
		{tokenRefreshInvalidClient, true},
		{tokenRefreshProvider4xxOther, true},
		{tokenRefreshMalformedResponse, true},
		{tokenRefreshNetworkError, false},
		{tokenRefreshProvider5xx, false},
		{tokenRefreshInternalError, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.category), func(t *testing.T) {
			assert.Equal(t, tc.want, tc.category.IsPermanent())
		})
	}
}

func TestClassifyTokenRefreshStatus_RecognizedRFCErrors(t *testing.T) {
	cases := []struct {
		name             string
		status           int
		body             string
		wantCategory     tokenRefreshCategory
		wantProviderCode string
	}{
		{"invalid_grant", 400, `{"error":"invalid_grant","error_description":"token revoked"}`, tokenRefreshInvalidGrant, "invalid_grant"},
		{"invalid_client", 401, `{"error":"invalid_client"}`, tokenRefreshInvalidClient, "invalid_client"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat, code := classifyTokenRefreshStatus(tc.status, []byte(tc.body))
			assert.Equal(t, tc.wantCategory, cat)
			assert.Equal(t, tc.wantProviderCode, code)
		})
	}
}

func TestClassifyTokenRefreshStatus_5xxAlwaysTransient(t *testing.T) {
	for _, status := range []int{500, 502, 503, 504} {
		cat, code := classifyTokenRefreshStatus(status, []byte(`{"error":"invalid_grant"}`))
		assert.Equal(t, tokenRefreshProvider5xx, cat,
			"5xx must classify as transient regardless of body (status %d)", status)
		assert.Empty(t, code, "providerError empty for 5xx (we don't trust the body)")
	}
}

func TestClassifyTokenRefreshStatus_UnknownErrorCode(t *testing.T) {
	cat, code := classifyTokenRefreshStatus(400, []byte(`{"error":"some_provider_specific_error"}`))
	assert.Equal(t, tokenRefreshProvider4xxOther, cat)
	assert.Equal(t, "some_provider_specific_error", code,
		"raw provider error string preserved for diagnostics even when category is generic")
}

func TestClassifyTokenRefreshStatus_UnparseableBody(t *testing.T) {
	cat, code := classifyTokenRefreshStatus(400, []byte(`<html>not json</html>`))
	assert.Equal(t, tokenRefreshProvider4xxOther, cat)
	assert.Empty(t, code)
}

func TestClassifyTokenRefreshStatus_EmptyBody(t *testing.T) {
	cat, code := classifyTokenRefreshStatus(403, nil)
	assert.Equal(t, tokenRefreshProvider4xxOther, cat)
	assert.Empty(t, code)
}
