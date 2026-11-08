// Package httphost adapts configured AuthProxy clients and transport evidence to
// the shared HTTP session, then maps aggregate evidence into Tool outcomes.
package httphost

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"time"

	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// Session supplies Tool execution-state interpretation around the shared HTTP
// budget and response owner. The shared session owns all request accounting,
// cancellation, and response decoding. Create sessions with New; the zero value
// admits no requests. Concurrent calls share one invocation allowance.
type Session struct {
	// session owns the shared HTTP lifecycle, not the configured client's lifetime.
	// Keep it private so the budget owner cannot diverge from client's captured owner.
	session *aphttp.Session

	// client is the Tool's only admitted connection. Each call binds this same
	// captured client, with its own labels, to the shared budget owner.
	client iface.HTTPClient
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
	session, err := aphttp.NewWithDeadline(ctx, limits, deadline)
	if err != nil {
		return nil, err
	}
	return &Session{session: session, client: client}, nil
}

// Context returns the invocation's deadline and identity context. An
// uninitialized adapter has no context and can be rejected before ownership
// transfer, matching the shared session's zero-value admission behavior.
func (s *Session) Context() context.Context {
	if s == nil || s.session == nil {
		return nil
	}
	return s.session.Context()
}

// Err distinguishes caller cancellation from the shared owner's policy failures.
// The session must have been constructed successfully.
func (s *Session) Err() error { return s.session.Err() }

// Close cancels the Tool's shared owner and releases its timer without closing
// the configured client. It is idempotent for a successfully constructed session.
func (s *Session) Close() { s.session.Close() }

// SendState returns the shared owner's cumulative target-send evidence. It does
// not imply completion of the Tool action; use ExecutionState for the Tool mapping.
func (s *Session) SendState() aphttp.SendState { return s.session.SendState() }

// Do retains the Tool caller's response/error contract while the shared session
// collects per-call and aggregate send evidence. It owns req.Body even when
// validation rejects the call, and transfers every successful response body to
// the caller. The configured client retains credential recovery and attribution.
func (s *Session) Do(req *http.Request) (*aphttp.Response, error) {
	return s.DoWithLabels(req, nil)
}

// ValidateLabels checks user labels without I/O or consuming request allowance.
// Authored input cannot replace protected apxy/ attribution. Runtime adapters call
// it before worker admission; DoWithLabels repeats the check for direct Go callers.
// Invalid labels or an uninitialized adapter return one fixed error without data.
func (s *Session) ValidateLabels(labels map[string]string) error {
	if s == nil ||
		s.session == nil ||
		s.client == nil ||
		meta.ValidateUserLabels(labels) != nil {
		return aphttp.ErrInvalidInput
	}

	return nil
}

// DoWithLabels copies validated labels for this call without changing the
// configured client. Keep the supplied map stable during the call. Each bound
// handle uses the same execution owner, so labels cannot reset request/response
// budgets, deadlines, or evidence. It takes request-body ownership on every path.
func (s *Session) DoWithLabels(
	req *http.Request,
	labels map[string]string,
) (*aphttp.Response, error) {
	transferred := false

	defer func() {
		if !transferred && req != nil && req.Body != nil {
			_ = req.Body.Close()
		}
	}()

	if err := s.ValidateLabels(labels); err != nil {
		return nil, err
	}

	client, err := s.session.Bind(configuredClient{
		client: s.client,
		labels: maps.Clone(labels),
	})
	if err != nil {
		return nil, err
	}

	transferred = true

	response, _, err := client.Do(req)

	return response, err
}

// ExecutionState maps shared target-send evidence to Tool execution uncertainty.
// Only known-unsent evidence yields notStarted; both possible and confirmed sends
// yield mayHaveExecuted. HTTP evidence does not establish whole-action success.
// Pending calls remain provisional: callers returning before they finish must
// retain the conservative snapshot rather than revise it from late evidence.
func (s *Session) ExecutionState() api.ToolExecutionState {
	if s == nil ||
		s.session == nil ||
		s.session.SendState() == aphttp.SendStateNotSent {
		return api.ToolExecutionStateNotStarted
	}

	return api.ToolExecutionStateMayHaveExecuted
}

// configuredClient bridges a configured connection client into the neutral
// transport contract without replacing authentication, retries, or telemetry.
type configuredClient struct {
	// client captures the connection, actor, labels, and request attribution.
	client iface.HTTPClient
	// labels is one call's private user-label override, detached before handoff.
	labels map[string]string
}

// Do observes one logical configured-client call, including its redirect and
// credential-recovery attempts. Missing evidence remains uncertain. Only the
// trusted credential marker becomes the shared unavailable category; private
// diagnostics are discarded. Raw response/body ownership stays with the caller.
func (c configuredClient) Do(req *http.Request) (*http.Response, aphttp.SendState, error) {
	ctx, observation := httpf.ObserveSends(req.Context())

	var response *http.Response
	var err error

	if len(c.labels) == 0 {
		response, err = c.client.Do(req.WithContext(ctx))
	} else {
		response, err = c.client.DoWithLabels(req.WithContext(ctx), c.labels)
	}

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
