//go:build integration

// Package api_key holds integration tests for api-key-authenticated connectors.
// Mirrors the structure of integration_tests/oauth2 — the tests drive a stub
// upstream that validates an incoming credential per placement (bearer,
// header, query, basic) and assert on the connection lifecycle the proxy
// produces against it.
package api_key

import (
	"net/http"
	"testing"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHarness_StubUpstreamBearer verifies the api-key stub upstream's bearer
// placement: requests with the configured key return 200, anything else
// returns 401. Sanity check the helper itself before the lifecycle tests
// depend on it.
func TestHarness_StubUpstreamBearer(t *testing.T) {
	stub := helpers.NewApiKeyStubUpstream(t, helpers.ApiKeyStubOptions{
		Placement:   connectors.ApiKeyPlacementBearer,
		AcceptedKey: "smoke-key",
	})

	// Match
	req, err := http.NewRequest(http.MethodGet, stub.BaseURL+"/anything", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer smoke-key")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Wrong key
	req, _ = http.NewRequest(http.MethodGet, stub.BaseURL+"/anything", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// No header
	resp, err = http.DefaultClient.Get(stub.BaseURL + "/anything")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Rotation
	stub.RotateAcceptedKey("new-key")
	req, _ = http.NewRequest(http.MethodGet, stub.BaseURL+"/anything", nil)
	req.Header.Set("Authorization", "Bearer smoke-key") // old key now rejected
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	req, _ = http.NewRequest(http.MethodGet, stub.BaseURL+"/anything", nil)
	req.Header.Set("Authorization", "Bearer new-key")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
