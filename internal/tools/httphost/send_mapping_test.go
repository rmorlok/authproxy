package httphost_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// TestSessionSendEvidenceResponseMapping distinguishes local and upstream 429
// responses all the way through status mapping into a valid public ToolResult.
func TestSessionSendEvidenceResponseMapping(t *testing.T) {
	for _, local := range []bool{true, false} {
		name := "upstream 429"
		if local {
			name = "local 429"
		}
		t.Run(name, func(t *testing.T) {
			// Real middleware either rejects locally or reaches a provider that
			// returns the same status; status alone cannot distinguish the two.
			var targetCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				targetCalls.Add(1)
				w.WriteHeader(http.StatusTooManyRequests)
			}))
			t.Cleanup(server.Close)
			fixture := newEvidenceFixture(t)
			session, _ := fixture.session(t, &integrationAuth{}, 1)
			wantState := api.ToolExecutionStateMayHaveExecuted
			if local {
				require.NoError(t, fixture.block(context.Background()))
				wantState = api.ToolExecutionStateNotStarted
			}
			response, err := session.Do(evidenceRequest(t, server.URL))
			require.NoError(t, err)
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusTooManyRequests, response.StatusCode)
			require.Equal(t, wantState, session.ExecutionState())
			if local {
				require.Zero(t, targetCalls.Load())
			} else {
				require.Equal(t, int32(1), targetCalls.Load())
			}

			for _, policy := range []struct {
				name string
				rule *tools.HTTPErrorRule
			}{
				{"unmatched", nil},
				{"exact", &tools.HTTPErrorRule{Statuses: []int{http.StatusTooManyRequests}}},
				{"class", &tools.HTTPErrorRule{StatusClass: util.ToPtr(4)}},
				{"default", &tools.HTTPErrorRule{Default: true}},
			} {
				t.Run(policy.name, func(t *testing.T) {
					// Authored status rules and the platform fallback retain the
					// executor's evidence without inferring it from the status.
					var source *tools.HTTPResponse
					wantCode := "UPSTREAM_FAILED"
					if policy.rule != nil {
						policy.rule.Code, policy.rule.Message = "REQUEST_THROTTLED", "The request was throttled."
						source = &tools.HTTPResponse{Errors: []tools.HTTPErrorRule{*policy.rule}}
						wantCode = policy.rule.Code
					}
					mapper, err := httpcodec.CompileResponse(source, httpcodec.ResponseLimits{
						MaxSourceBytes: 4096, MaxResponseBytes: 4096,
						MaxOutputBytes: 4096, MaxEncodedResultBytes: 8192,
					})
					require.NoError(t, err)
					result, err := mapper.Map(session.Context(), httpcodec.ResponseInput{
						StatusCode: response.StatusCode, ContentType: response.ContentType,
						Headers: response.Header, Body: body,
						ToolID: apid.New(apid.PrefixTool).String(), Revision: 1,
						InvocationID:   apid.New(apid.PrefixToolInvocation).String(),
						Metadata:       api.ToolResultMetadata{ConnectionID: fixture.connection.GetId().String()},
						ExecutionState: session.ExecutionState(),
					})
					require.NoError(t, err)
					require.NoError(t, result.Validate(nil))
					require.True(t, result.IsError)
					require.Equal(t, wantCode, result.Error.Code)
					require.Equal(t, wantState, result.Error.ExecutionState)
					require.JSONEq(t, `{"status":429}`, string(result.Error.Details))
				})
			}
		})
	}
}
