//go:build integration

package oauth2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/stretchr/testify/require"
)

// TestRawProxySendEvidence_OAuthRefreshFailure proves credential endpoint
// traffic does not count as a target send when proactive refresh fails.
func TestRawProxySendEvidence_OAuthRefreshFailure(t *testing.T) {
	rig := newOAuthSendEvidenceRig(t, "raw-evidence-refresh-failure")
	connID := rig.completeAuthFlow(t)
	rig.forceTokenExpired(t, connID, false)
	rig.provider.Script(rig.clientKey, helpers.EndpointRefresh, helpers.ScriptAction{
		Status: http.StatusBadRequest,
		Body:   `{"error":"invalid_grant"}`,
	})

	// The refresh reached the provider, while the uniquely marked resource
	// did not. Raw errors keep their ordinary JSON shape and add a header.
	response := doOAuthRawSendEvidenceProxy(t, rig, connID, connID)
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.Equal(t, []string{"notSent"}, response.Header().Values("X-AuthProxy-Send-Evidence"))
	assertOAuthRawSendEvidenceError(t, response)
	require.Equal(t, 1, rig.refreshCallCount())
	require.Empty(t, oauthSendEvidenceResourceRequests(rig, connID))
}

// TestRawProxySendEvidence_OAuthRefreshThenLocalBlock distinguishes a successful
// refresh response from a target attempt blocked before transport handoff.
func TestRawProxySendEvidence_OAuthRefreshThenLocalBlock(t *testing.T) {
	rig := newOAuthSendEvidenceRig(t, "raw-evidence-refresh-blocked")
	connID := rig.completeAuthFlow(t)
	limitOAuthSendEvidenceTargets(t, rig, connID)

	// Consume the real policy's one proxy token before expiring the OAuth
	// access token. Its request-type selector excludes refresh traffic.
	first := doOAuthRawSendEvidenceProxy(t, rig, connID, connID+"-prime")
	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, []string{"sent"}, first.Header().Values("X-AuthProxy-Send-Evidence"))
	require.Len(t, oauthSendEvidenceResourceRequests(rig, connID+"-prime"), 1)
	rig.forceTokenExpired(t, connID, false)
	expired := rig.env.GetOAuth2Token(t, connID)

	// A successful refresh replaces the token, but the raw target response
	// comes from local quota enforcement and proves no target was sent.
	response := doOAuthRawSendEvidenceProxy(t, rig, connID, connID)
	require.Equal(t, http.StatusTooManyRequests, response.Code)
	require.Equal(t, "true", response.Header().Get("X-Authproxy-Ratelimited"))
	require.Equal(t, []string{"notSent"}, response.Header().Values("X-AuthProxy-Send-Evidence"))
	require.Equal(t, 1, rig.refreshCallCount())
	require.NotEqual(t, expired.Id, rig.env.GetOAuth2Token(t, connID).Id)
	require.Empty(t, oauthSendEvidenceResourceRequests(rig, connID))
}

// TestRawProxySendEvidence_OAuth401Recovery keeps the first target response's
// evidence across a successful, dropped, or locally rejected raw replay.
func TestRawProxySendEvidence_OAuth401Recovery(t *testing.T) {
	for _, scenario := range []struct {
		name         string
		retry        helpers.ScriptAction
		localBlock   bool
		wantStatus   int
		wantRequests int
	}{
		{
			name: "successful retry",
			retry: helpers.ScriptAction{
				Status:  http.StatusCreated,
				Headers: map[string]string{"X-Recovered-Response": "preserved"},
				Body:    `{"result":"recovered"}`,
			},
			wantStatus:   http.StatusCreated,
			wantRequests: 2,
		},
		{
			name:         "dropped retry remains an AuthProxy error",
			retry:        helpers.ScriptAction{DropConnection: true},
			wantStatus:   http.StatusInternalServerError,
			wantRequests: 2,
		},
		{
			name:         "locally blocked retry",
			localBlock:   true,
			wantStatus:   http.StatusTooManyRequests,
			wantRequests: 1,
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			rig := newOAuthSendEvidenceRig(t, "raw-evidence-recovery")
			connID := rig.completeAuthFlow(t)
			if scenario.localBlock {
				limitOAuthSendEvidenceTargets(t, rig, connID)
			}

			// Resource scripts are consumed sequentially and independently
			// from token scripts. The first 401 always proves a target send.
			actions := []helpers.ScriptAction{{
				Status:  http.StatusUnauthorized,
				Headers: map[string]string{"X-Original-Response": "discarded"},
				Body:    `{"error":"original_target_401"}`,
			}}
			if !scenario.localBlock {
				actions = append(actions, scenario.retry)
			}
			rig.provider.Script("", helpers.EndpointResource, actions...)

			// Bodyless POST is replay-eligible under the existing raw rules
			// and prevents net/http from silently retrying the dropped attempt.
			response := doOAuthRawSendEvidenceProxy(t, rig, connID, connID)
			require.Equal(t, scenario.wantStatus, response.Code)
			require.Equal(t, []string{"sent"}, response.Header().Values("X-AuthProxy-Send-Evidence"))
			require.Equal(t, 1, rig.refreshCallCount())
			requests := oauthSendEvidenceResourceRequests(rig, connID)
			require.Len(t, requests, scenario.wantRequests)
			for _, request := range requests {
				require.Equal(t, http.MethodPost, request.Method)
			}

			// Raw replay errors remain errors rather than falling back to
			// the first 401; successful replay preserves its own status/body.
			require.Empty(t, response.Header().Get("X-Original-Response"))
			switch scenario.wantStatus {
			case http.StatusInternalServerError:
				assertOAuthRawSendEvidenceError(t, response)
				require.NotContains(t, response.Body.String(), "original_target_401")
			case http.StatusTooManyRequests:
				require.Equal(t, "true", response.Header().Get("X-Authproxy-Ratelimited"))
			case http.StatusCreated:
				require.JSONEq(t, `{"result":"recovered"}`, response.Body.String())
				require.Equal(t, "preserved", response.Header().Get("X-Recovered-Response"))
			}
		})
	}
}

// doOAuthRawSendEvidenceProxy sends an empty POST over the real API listener.
// No inbound body means AuthProxy may replay once after 401; the POST method
// prevents transparent transport replay from obscuring provider request counts.
func doOAuthRawSendEvidenceProxy(t *testing.T, rig *proxyRefreshRig, connectionID, marker string) *httptest.ResponseRecorder {
	t.Helper()
	target := rig.provider.ResourceURL("/echo") + "?sendEvidence=" + url.QueryEscape(marker)
	return rig.env.DoProxyRawRequest(t, connectionID, target, http.MethodPost, nil, false, nil)
}

// assertOAuthRawSendEvidenceError checks that raw AuthProxy errors preserve the
// existing JSON envelope, carrying evidence exclusively in the response header.
func assertOAuthRawSendEvidenceError(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.NotEmpty(t, body["error"])
	require.NotContains(t, body, "sendEvidence")
	require.NotContains(t, body, "statusCode")
}
