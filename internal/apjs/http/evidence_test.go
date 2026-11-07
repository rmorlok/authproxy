package http_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	"github.com/stretchr/testify/require"
)

// TestSessionSendEvidence retains explicit per-call outcomes independently of
// aggregate history and never infers sends from an HTTP response's status.
func TestSessionSendEvidence(t *testing.T) {
	for _, test := range []struct {
		name   string
		states []aphttp.SendState
		want   []aphttp.SendState
	}{
		{"known unsent", []aphttp.SendState{aphttp.SendStateNotSent}, []aphttp.SendState{aphttp.SendStateNotSent}},
		{"missing evidence", []aphttp.SendState{0}, []aphttp.SendState{aphttp.SendStateMayHaveSent}},
		{"invalid evidence", []aphttp.SendState{255}, []aphttp.SendState{aphttp.SendStateMayHaveSent}},
		{"possible survives unsent", []aphttp.SendState{aphttp.SendStateMayHaveSent, aphttp.SendStateNotSent}, []aphttp.SendState{aphttp.SendStateMayHaveSent, aphttp.SendStateMayHaveSent}},
		{"confirmed dominates", []aphttp.SendState{aphttp.SendStateMayHaveSent, aphttp.SendStateSent, aphttp.SendStateNotSent, aphttp.SendStateMayHaveSent}, []aphttp.SendState{aphttp.SendStateMayHaveSent, aphttp.SendStateSent, aphttp.SendStateSent, aphttp.SendStateSent}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Every client response is identical; only trusted evidence distinguishes target sends.
			call := 0
			client := &sessionTestClient{doWithState: func(req *http.Request) (*http.Response, aphttp.SendState, error) {
				_ = req.Body.Close()
				state := test.states[call]
				call++
				return sessionTestResponse(), state, nil
			}}
			session := sessionTestNew(t, context.Background(), client, sessionTestLimits())
			require.Equal(t, aphttp.SendStateNotSent, session.SendState())

			for i, reported := range test.states {
				request, _ := sessionTestRequest(context.Background())
				response, state, err := session.Do(request)
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
				if reported != aphttp.SendStateNotSent && reported != aphttp.SendStateSent {
					reported = aphttp.SendStateMayHaveSent
				}
				require.Equal(t, reported, state)
				require.Equal(t, test.want[i], session.SendState())
			}

			// A locally invalid request is definitely unsent, without rewriting prior history.
			response, state, err := session.Do(nil)
			require.ErrorIs(t, err, aphttp.ErrInvalidInput)
			require.Nil(t, response)
			require.Equal(t, aphttp.SendStateNotSent, state)
			require.Equal(t, test.want[len(test.want)-1], session.SendState())
			require.Equal(t, len(test.states), call)
		})
	}
}

// TestSessionFailureEvidence retains target evidence when transport or response
// validation fails, independently of the safe error classification.
func TestSessionFailureEvidence(t *testing.T) {
	private := errors.New("private transport diagnostic")
	for _, test := range []struct {
		name   string
		status int
		cause  error
		state  aphttp.SendState
		want   error
	}{
		{"transport after send", http.StatusOK, private, aphttp.SendStateSent, aphttp.ErrTransport},
		{"invalid response after send", 0, nil, aphttp.SendStateSent, aphttp.ErrInvalidResponse},
		{"unavailable before send", http.StatusOK, fmt.Errorf("private wrapper: %w", aphttp.ErrCredentialsUnavailable), aphttp.SendStateNotSent, aphttp.ErrCredentialsUnavailable},
		{"unavailable after send", http.StatusOK, fmt.Errorf("private wrapper: %w", aphttp.ErrCredentialsUnavailable), aphttp.SendStateSent, aphttp.ErrCredentialsUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Rejected raw responses remain owned by the session and must be closed once.
			body := &sessionTestBody{reader: strings.NewReader("private response")}
			client := &sessionTestClient{doWithState: func(req *http.Request) (*http.Response, aphttp.SendState, error) {
				_ = req.Body.Close()
				return &http.Response{StatusCode: test.status, Body: body}, test.state, test.cause
			}}
			session := sessionTestNew(t, context.Background(), client, sessionTestLimits())
			request, _ := sessionTestRequest(context.Background())
			response, state, err := session.Do(request)

			// Diagnostics are removed, while call and session evidence remain independently inspectable.
			require.Nil(t, response)
			require.ErrorIs(t, err, test.want)
			require.NotContains(t, err.Error(), "private")
			require.NotErrorIs(t, err, private)
			require.Equal(t, test.state, state)
			require.Equal(t, test.state, session.SendState())
			require.EqualValues(t, 1, body.closes.Load())
		})
	}
}

// TestSessionConfirmedSendSurvivesPending preserves confirmed evidence while a
// later dispatch is unresolved, and after that dispatch proves it was unsent.
func TestSessionConfirmedSendSurvivesPending(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	client := &sessionTestClient{doWithState: func(req *http.Request) (*http.Response, aphttp.SendState, error) {
		_ = req.Body.Close()
		if req.URL.Path == "/pending" {
			close(entered)
			select {
			case <-release:
			case <-req.Context().Done():
				return nil, aphttp.SendStateMayHaveSent, req.Context().Err()
			}
			return sessionTestResponse(), aphttp.SendStateNotSent, nil
		}
		return sessionTestResponse(), aphttp.SendStateSent, nil
	}}
	session := sessionTestNew(t, context.Background(), client, sessionTestLimits())

	// Finish one confirmed target call before admitting the controlled pending call.
	request, _ := sessionTestRequest(context.Background())
	response, state, err := session.Do(request)
	require.NoError(t, err)
	require.Equal(t, aphttp.SendStateSent, state)
	require.NoError(t, response.Body.Close())
	request, _ = sessionTestRequest(context.Background())
	request.URL.Path = "/pending"
	var pendingErr error
	var pendingState aphttp.SendState
	done := make(chan struct{})
	go func() {
		response, state, err := session.Do(request)
		pendingState, pendingErr = state, err
		if response != nil {
			_ = response.Body.Close()
		}
		close(done)
	}()
	sessionTestWait(t, entered, session.Close)
	require.Equal(t, aphttp.SendStateSent, session.SendState())

	// Settling the later unsent call preserves the earlier confirmed target response.
	unblock()
	sessionTestWait(t, done, session.Close)
	require.NoError(t, pendingErr)
	require.Equal(t, aphttp.SendStateNotSent, pendingState)
	require.Equal(t, aphttp.SendStateSent, session.SendState())
}
