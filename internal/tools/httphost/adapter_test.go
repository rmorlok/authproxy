package httphost_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
	"github.com/stretchr/testify/require"
)

// TestConfiguredClientEvidence preserves the transport's richer evidence while
// retaining the Tool contract that a send alone cannot establish action success.
func TestConfiguredClientEvidence(t *testing.T) {
	for _, test := range []struct {
		name    string
		reports []httpf.SendState
		want    aphttp.SendState
		tool    api.ToolExecutionState
	}{
		{"missing", nil, aphttp.SendStateMayHaveSent, api.ToolExecutionStateMayHaveExecuted},
		{"not sent", []httpf.SendState{httpf.SendStateNotSent}, aphttp.SendStateNotSent, api.ToolExecutionStateNotStarted},
		{"possible", []httpf.SendState{httpf.SendStateMayHaveSent}, aphttp.SendStateMayHaveSent, api.ToolExecutionStateMayHaveExecuted},
		{"confirmed", []httpf.SendState{httpf.SendStateSent}, aphttp.SendStateSent, api.ToolExecutionStateMayHaveExecuted},
		{"invalid", []httpf.SendState{"invalid"}, aphttp.SendStateMayHaveSent, api.ToolExecutionStateMayHaveExecuted},
		{"confirmed then local failure", []httpf.SendState{httpf.SendStateSent, httpf.SendStateNotSent, httpf.SendStateMayHaveSent}, aphttp.SendStateSent, api.ToolExecutionStateMayHaveExecuted},
	} {
		t.Run(test.name, func(t *testing.T) {
			// One configured-client handoff can contain several target attempts.
			calls := 0
			client := &sessionTestClient{do: func(req *http.Request) (*http.Response, error) {
				calls++
				for _, report := range test.reports {
					httpf.ReportSendState(req.Context(), report)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
			}}
			session := sessionTestNew(t, context.Background(), client, sessionTestLimits())
			request, err := http.NewRequest(http.MethodGet, "https://provider.example/items", nil)
			require.NoError(t, err)
			response, err := session.Do(request)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())

			// The bridge never retries and does not collapse confirmed evidence.
			require.Equal(t, 1, calls)
			require.Equal(t, test.want, session.SendState())
			require.Equal(t, test.tool, session.ExecutionState())
		})
	}
}

// TestAdapterInvalidSession prevents a wrapper from concealing absent clients
// or bypassing the shared session's upload-ownership contract.
func TestAdapterInvalidSession(t *testing.T) {
	// Invalid construction takes precedence even when the caller is canceled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	session, err := httphost.New(ctx, nil, sessionTestLimits())
	require.ErrorIs(t, err, aphttp.ErrInvalidInput)
	require.Nil(t, session)

	// Both nil and zero adapters reject calls and close their transferred upload.
	for _, empty := range []*httphost.Session{nil, {}} {
		require.Nil(t, empty.Context())
		request, body := sessionTestRequest(context.Background())
		response, err := empty.Do(request)
		require.ErrorIs(t, err, aphttp.ErrInvalidInput)
		require.Nil(t, response)
		require.EqualValues(t, 1, body.closes.Load())
	}
}

// TestConfiguredClientPanicDiscardsPartialEvidence keeps an interrupted client
// call uncertain even if it reported a local rejection before panicking.
func TestConfiguredClientPanicDiscardsPartialEvidence(t *testing.T) {
	var observed context.Context
	calls := 0
	client := &sessionTestClient{do: func(req *http.Request) (*http.Response, error) {
		// A report is only settled evidence after the configured call returns.
		calls++
		observed = req.Context()
		httpf.ReportSendEvidence(observed, false)
		panic("configured client failure")
	}}
	limits := sessionTestLimits()
	limits.MaxRequests = 1
	session := sessionTestNew(t, context.Background(), client, limits)
	request, err := http.NewRequest(http.MethodGet, "https://provider.example/items", nil)
	require.NoError(t, err)

	// Recovery outside the session must not trust the incomplete local report.
	require.PanicsWithValue(t, "configured client failure", func() { _, _ = session.Do(request) })
	require.Equal(t, aphttp.SendStateMayHaveSent, session.SendState())
	require.Equal(t, api.ToolExecutionStateMayHaveExecuted, session.ExecutionState())
	require.ErrorIs(t, observed.Err(), context.Canceled)
	require.NoError(t, session.Err())

	// The interrupted handoff stays charged and cannot be repeated after recovery.
	response, err := session.Do(request)
	require.ErrorIs(t, err, aphttp.ErrRequestLimit)
	require.Nil(t, response)
	require.Equal(t, 1, calls)
}
