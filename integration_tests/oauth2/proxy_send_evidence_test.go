//go:build integration

package oauth2

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/ratelimit"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
)

// oauthSendEvidenceResponse captures the public wrapped response and ordinary
// error shapes without deriving expected evidence from implementation types.
type oauthSendEvidenceResponse struct {
	StatusCode   int               `json:"statusCode"`
	Headers      map[string]string `json:"headers"`
	BodyJSON     map[string]any    `json:"bodyJson"`
	Error        string            `json:"error"`
	SendEvidence string            `json:"sendEvidence"`
}

// newOAuthSendEvidenceRig exercises both API calls and OAuth callbacks over
// real listeners. Provider scripts are shared, so these tests stay sequential.
func newOAuthSendEvidenceRig(t *testing.T, name string) *proxyRefreshRig {
	t.Helper()
	rig := newProxyRefreshRigWithOptions(t, name, nil, func(opts *helpers.SetupOptions) {
		opts.StartHTTPServer = true
	})
	require.Nil(t, rig.env.ApiGin, "proxy requests must traverse the real HTTP listener")
	require.NotEmpty(t, rig.env.PublicURL, "OAuth callbacks must use the real public listener")
	return rig
}

// TestProxySendEvidence_OAuthRefreshFailure proves a provider's rejected token
// exchange is credential traffic, not evidence that the target was called.
func TestProxySendEvidence_OAuthRefreshFailure(t *testing.T) {
	rig := newOAuthSendEvidenceRig(t, "send-evidence-refresh-failure")
	connID := rig.completeAuthFlow(t)
	rig.forceTokenExpired(t, connID, false)
	rig.provider.Script(rig.clientKey, helpers.EndpointRefresh, helpers.ScriptAction{
		Status: http.StatusBadRequest,
		Body:   `{"error":"invalid_grant"}`,
	})

	// The provider sees one failed refresh, but no resource request carrying
	// this unique marker; the outer response is an AuthProxy error envelope.
	status, response := doOAuthSendEvidenceProxy(t, rig, connID, connID)
	require.Equal(t, http.StatusInternalServerError, status)
	require.NotEmpty(t, response.Error)
	require.Zero(t, response.StatusCode)
	require.Equal(t, "notSent", response.SendEvidence)
	require.Equal(t, 1, rig.refreshCallCount())
	require.Empty(t, oauthSendEvidenceResourceRequests(rig, connID))
}

// TestProxySendEvidence_OAuthRefreshThenLocalBlock separates a successful
// refresh from a target request rejected by the real rate-limit middleware.
func TestProxySendEvidence_OAuthRefreshThenLocalBlock(t *testing.T) {
	rig := newOAuthSendEvidenceRig(t, "send-evidence-refresh-blocked")
	connID := rig.completeAuthFlow(t)
	limitOAuthSendEvidenceTargets(t, rig, connID)

	// Spend the rule's one target token, then expire the OAuth access token.
	// Refresh calls are excluded from this rule and remain able to succeed.
	status, first := doOAuthSendEvidenceProxy(t, rig, connID, connID+"-prime")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, http.StatusOK, first.StatusCode)
	require.Equal(t, "sent", first.SendEvidence)
	require.Len(t, oauthSendEvidenceResourceRequests(rig, connID+"-prime"), 1)
	rig.forceTokenExpired(t, connID, false)
	expired := rig.env.GetOAuth2Token(t, connID)

	// The next call refreshes successfully but has no target quota left.
	status, response := doOAuthSendEvidenceProxy(t, rig, connID, connID)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, http.StatusTooManyRequests, response.StatusCode)
	require.Equal(t, "true", response.Headers["X-Authproxy-Ratelimited"])
	require.Equal(t, "notSent", response.SendEvidence)
	require.Equal(t, 1, rig.refreshCallCount())
	require.NotEqual(t, expired.Id, rig.env.GetOAuth2Token(t, connID).Id)
	require.Empty(t, oauthSendEvidenceResourceRequests(rig, connID))
}

// TestProxySendEvidence_OAuth401Recovery retains the first target response's
// sent evidence across successful, dropped, and locally rejected replays.
func TestProxySendEvidence_OAuth401Recovery(t *testing.T) {
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
				Status: http.StatusOK,
				Body:   `{"result":"recovered"}`,
			},
			wantStatus:   http.StatusOK,
			wantRequests: 2,
		},
		{
			name:         "dropped retry preserves original 401",
			retry:        helpers.ScriptAction{DropConnection: true},
			wantStatus:   http.StatusUnauthorized,
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
			rig := newOAuthSendEvidenceRig(t, "send-evidence-recovery")
			connID := rig.completeAuthFlow(t)
			if scenario.localBlock {
				limitOAuthSendEvidenceTargets(t, rig, connID)
			}

			// The first target attempt always returns a distinguishable 401.
			// Wildcard resource scripts are used sequentially; token traffic
			// has a separate endpoint queue and provider-side request log.
			actions := []helpers.ScriptAction{{
				Status:  http.StatusUnauthorized,
				Headers: map[string]string{"X-Original-Response": "retained"},
				Body:    `{"error":"original_target_401"}`,
			}}
			if !scenario.localBlock {
				actions = append(actions, scenario.retry)
			}
			rig.provider.Script("", helpers.EndpointResource, actions...)

			// POST prevents net/http's automatic replay of a dropped GET on
			// a reused connection; AuthProxy alone performs the one 401 replay.
			status, response := doOAuthSendEvidenceProxy(t, rig, connID, connID)
			require.Equal(t, http.StatusOK, status)
			require.Equal(t, scenario.wantStatus, response.StatusCode)
			require.Equal(t, "sent", response.SendEvidence)
			require.Empty(t, response.Error)
			require.Equal(t, 1, rig.refreshCallCount())
			requests := oauthSendEvidenceResourceRequests(rig, connID)
			require.Len(t, requests, scenario.wantRequests)
			for _, request := range requests {
				require.Equal(t, http.MethodPost, request.Method)
			}

			// Preserve the existing wrapped fallback: a transport error on
			// replay returns the original 401, including its body and headers.
			switch scenario.wantStatus {
			case http.StatusUnauthorized:
				require.Equal(t, "original_target_401", response.BodyJSON["error"])
				require.Equal(t, "retained", response.Headers["X-Original-Response"])
			case http.StatusTooManyRequests:
				require.Equal(t, "true", response.Headers["X-Authproxy-Ratelimited"])
			case http.StatusOK:
				require.Equal(t, "recovered", response.BodyJSON["result"])
			}
		})
	}
}

// doOAuthSendEvidenceProxy posts a replayable target request through the real
// API listener and decodes either the wrapped response or AuthProxy error.
func doOAuthSendEvidenceProxy(t *testing.T, rig *proxyRefreshRig, connectionID, marker string) (int, oauthSendEvidenceResponse) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"url":      rig.provider.ResourceURL("/echo") + "?sendEvidence=" + url.QueryEscape(marker),
		"method":   http.MethodPost,
		"bodyJson": map[string]string{"operation": marker},
	})
	require.NoError(t, err)
	path := "/api/v1/connections/" + connectionID + "/_proxy"
	req, err := rig.env.ApiAuthUtil.NewSignedRequestForActorExternalId(
		http.MethodPost, rig.env.ServerURL+path, bytes.NewReader(body),
		sconfig.RootNamespace, "test-actor", aschema.AllPermissions(),
	)
	require.NoError(t, err)
	req.RequestURI = ""
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var response oauthSendEvidenceResponse
	require.NoErrorf(t, json.Unmarshal(data, &response), "invalid proxy response: %s", data)
	return resp.StatusCode, response
}

// oauthSendEvidenceResourceRequests selects independent provider recordings by
// a unique query marker, avoiding global counts and host/container clock skew.
func oauthSendEvidenceResourceRequests(rig *proxyRefreshRig, marker string) []helpers.RecordedRequest {
	var matched []helpers.RecordedRequest
	for _, request := range rig.provider.Requests(helpers.RequestsFilter{Endpoint: helpers.EndpointResource}) {
		if values := request.Query["sendEvidence"]; len(values) == 1 && values[0] == marker {
			matched = append(matched, request)
		}
	}
	return matched
}

// limitOAuthSendEvidenceTargets installs a real per-connection policy with one
// target token and a one-hour refill. Explicit proxy selection leaves OAuth
// refreshes uncounted; synchronous cache installation avoids polling or sleeps.
func limitOAuthSendEvidenceTargets(t *testing.T, rig *proxyRefreshRig, connectionID string) {
	t.Helper()
	resource := &rlschema.RateLimit{
		TypeMeta: meta.NewTypeMeta(rlschema.RateLimitKind),
		Metadata: meta.ObjectMeta{Namespace: sconfig.RootNamespace},
		Spec: rlschema.RateLimitSpec{
			Mode: rlschema.ModeEnforce,
			Scope: &rlschema.RateLimitScope{ConnectionRef: &meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       "Connection",
				ID:         connectionID,
			}},
			Selector: rlschema.Selector{RequestTypes: []common.RequestType{common.RequestTypeProxy}},
			Bucket:   rlschema.Bucket{Dimensions: []string{rlschema.DimensionConnection}},
			Algorithm: rlschema.Algorithm{TokenBucket: &rlschema.TokenBucket{
				Capacity:   1,
				RefillRate: 1.0 / 3600,
			}},
		},
	}
	created, err := rig.env.Core.CreateRateLimit(context.Background(), resource)
	require.NoError(t, err)
	stored, err := rig.env.Db.GetRateLimit(context.Background(), created.GetId())
	require.NoError(t, err)
	cache := rig.env.DM.GetRateLimitCache().(ratelimit.MutableCache)
	cache.Replace([]*database.RateLimit{stored}, time.Now())
}
