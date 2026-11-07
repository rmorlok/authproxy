// Package httphost adapts configured AuthProxy clients and transport evidence to
// the shared HTTP session, then maps aggregate evidence into Tool outcomes.
package httphost

import (
	"context"
	"errors"
	"net/http"
	"time"

	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/api"
)

// Session supplies Tool execution-state interpretation around the shared HTTP
// budget and response owner. The embedded session owns all request accounting,
// cancellation, and response decoding. Create sessions with New; the zero value
// admits no requests. Concurrent calls share one invocation allowance.
type Session struct {
	// Session owns the shared HTTP lifecycle, not the configured client's lifetime.
	*aphttp.Session
}

// New captures the already admitted connection client and starts the invocation
// timeout. The caller configures attribution and an explicit actor choice matching
// ctx; authorization, snapshots, and effective policy remain caller decisions.
func New(ctx context.Context, client iface.HTTPClient, limits aphttp.Limits) (*Session, error) {
	return NewWithDeadline(ctx, client, limits, time.Time{})
}

// NewWithDeadline also honors an earlier absolute policy deadline, including
// admission/preparation time. A zero deadline adds no restriction. Caller
// cancellation remains distinct from policy expiration in the shared session.
func NewWithDeadline(ctx context.Context, client iface.HTTPClient, limits aphttp.Limits, deadline time.Time) (*Session, error) {
	// Validate before wrapping: a nonnil adapter must not conceal a nil client
	// or let caller cancellation take precedence over invalid constructor input.
	if client == nil {
		return nil, aphttp.ErrInvalidInput
	}
	session, err := aphttp.NewWithDeadline(ctx, configuredClient{client: client}, limits, deadline)
	if err != nil {
		return nil, err
	}
	return &Session{Session: session}, nil
}

// Context returns the invocation's deadline and identity context. An
// uninitialized adapter has no context and can be rejected before ownership
// transfer, matching the shared session's zero-value admission behavior.
func (s *Session) Context() context.Context {
	if s == nil || s.Session == nil {
		return nil
	}
	return s.Session.Context()
}

// Do retains the Tool caller's response/error contract while the shared session
// collects per-call and aggregate send evidence. It owns req.Body even when
// validation rejects the call, and transfers every successful response body to
// the caller. The configured client retains credential recovery and attribution.
func (s *Session) Do(req *http.Request) (*aphttp.Response, error) {
	var shared *aphttp.Session
	if s != nil {
		shared = s.Session
	}
	response, _, err := shared.Do(req)
	return response, err
}

// ExecutionState maps shared target-send evidence to Tool execution uncertainty.
// Only known-unsent evidence yields notStarted; both possible and confirmed sends
// yield mayHaveExecuted. HTTP evidence does not establish whole-action success.
// Pending calls remain provisional: callers returning before they finish must
// retain the conservative snapshot rather than revise it from late evidence.
func (s *Session) ExecutionState() api.ToolExecutionState {
	if s == nil || s.Session == nil || s.Session.SendState() == aphttp.SendStateNotSent {
		return api.ToolExecutionStateNotStarted
	}
	return api.ToolExecutionStateMayHaveExecuted
}

// configuredClient bridges a configured connection client into the neutral
// transport contract without replacing authentication, retries, or telemetry.
type configuredClient struct {
	// client captures the connection, actor, labels, and request attribution.
	client iface.HTTPClient
}

// Do observes one logical configured-client call, including its redirect and
// credential-recovery attempts. Missing evidence remains uncertain. Only the
// trusted credential marker becomes the shared unavailable category; private
// diagnostics are discarded. Raw response/body ownership stays with the caller.
func (c configuredClient) Do(req *http.Request) (*http.Response, aphttp.SendState, error) {
	ctx, observation := httpf.ObserveSends(req.Context())
	response, err := c.client.Do(req.WithContext(ctx))
	state := aphttp.SendStateMayHaveSent
	switch observation.State() {
	case httpf.SendStateNotSent:
		state = aphttp.SendStateNotSent
	case httpf.SendStateSent:
		state = aphttp.SendStateSent
	}
	if err != nil {
		if errors.Is(err, auth_methods.ErrCredentialsUnavailable) {
			err = aphttp.ErrCredentialsUnavailable
		} else {
			err = aphttp.ErrTransport
		}
	}
	return response, state, err
}
