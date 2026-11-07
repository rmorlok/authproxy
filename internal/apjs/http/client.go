package http

import (
	"context"
	"errors"
	"net/http"
)

// SendState describes target-send evidence for one call or an entire execution.
// It does not establish whether the provider completed a business operation.
// The zero value is conservative uncertainty; only an explicit report proves a
// call was unsent. Unknown values are also treated as uncertainty.
type SendState uint8

const (
	// SendStateMayHaveSent means an attempt may have reached the target, or the
	// client cannot account for every possible send. It is also the zero value.
	SendStateMayHaveSent SendState = iota

	// SendStateNotSent means every target attempt was stopped before transport.
	SendStateNotSent

	// SendStateSent means the target transport returned a response for at least
	// one attempt, regardless of status or subsequent failures.
	SendStateSent
)

// Client is an already configured outbound capability supplied by trusted Go
// code. Implementations own request bodies on entry and retain their existing
// authentication, recovery, redirect, attribution, and instrumentation policy.
// They must support concurrent calls and honor request cancellation. Sessions
// never close or reconfigure a client; it can be reused across executions.
type Client interface {
	// Do returns a raw response and settled target evidence for the whole logical
	// call, including redirects and credential recovery. Credential endpoint I/O
	// is separate from target evidence. Missing evidence must remain uncertain;
	// an HTTP status alone cannot distinguish a local reply from a target reply.
	// Every nonnil response body transfers to the session, including on error.
	// Wrap ErrCredentialsUnavailable only for trusted credential failures; other
	// errors are sanitized as ErrTransport. Do not infer categories from text.
	Do(*http.Request) (*http.Response, SendState, error)
}

// ClientHandle permanently binds a supplied client to one execution's Session.
// Its calls share that owner's allowances, deadline, and evidence with all other
// handles. Copying the handle or borrowing its Do method keeps the same client
// and owner; it never creates another budget or looks up a client from requests.
// Obtain handles through Session.Bind; nil and zero handles reject requests.
type ClientHandle struct {
	// session owns the execution lifecycle and all mutable accounting. A handle
	// neither closes that owner nor extends its lifetime with another timer.
	session *Session

	// client fixes the outbound capability, configuration, and attribution.
	// The session never closes or reconfigures the caller-owned client.
	client Client
}

// normalizeSendState makes zero and unrecognized reports conservative without
// conflating a confirmed response with a possible send.
func normalizeSendState(state SendState) SendState {
	switch state {
	case SendStateNotSent, SendStateSent:
		return state
	default:
		return SendStateMayHaveSent
	}
}

// Do transfers ownership of req.Body on entry, including local rejection. It
// returns headers before consuming the response body. The caller must close
// that body and must handle read errors before using a complete response. The
// returned evidence covers this call even on error; local rejection is known
// unsent. The owning Session.SendState reports the aggregate across all handles.
//
// The session context supplies caller values; req.Context additionally restricts
// cancellation and deadline, without replacing those values. Host overrides,
// GetBody replay, authentication, and instrumentation retain the supplied client
// behavior. This host adds no retries.
func (h *ClientHandle) Do(req *http.Request) (*Response, SendState, error) {
	transferred := false
	defer func() {
		if !transferred && req != nil && req.Body != nil {
			_ = req.Body.Close()
		}
	}()

	if h == nil ||
		h.session == nil ||
		h.session.ctx == nil ||
		h.client == nil ||
		req == nil ||
		req.URL == nil ||
		(req.URL.Scheme != "http" && req.URL.Scheme != "https") ||
		req.URL.Host == "" ||
		req.URL.Opaque != "" ||
		req.URL.Fragment != "" ||
		req.RequestURI != "" {
		return nil, SendStateNotSent, ErrInvalidInput
	}

	s := h.session
	base, stopDeadline := s.ctx, func() {}
	if deadline, ok := req.Context().Deadline(); ok {
		if current, hasDeadline := base.Deadline(); !hasDeadline || deadline.Before(current) {
			// Preserve the earlier deadline for transports and authentication
			// helpers that inspect Deadline rather than only waiting on Done.
			base, stopDeadline = context.WithDeadline(base, deadline)
		}
	}

	ctx, cancel := context.WithCancelCause(base)
	stopRequest := context.AfterFunc(req.Context(), func() { cancel(req.Context().Err()) })

	// AfterFunc is asynchronous, so an already-canceled request must also be
	// rejected synchronously before it can consume a request or reach the client.
	if err := req.Context().Err(); err != nil {
		cancel(err)
	}
	cleanup := func() { stopRequest(); cancel(context.Canceled); stopDeadline() }

	retained := false
	defer func() {
		if !retained {
			cleanup()
		}
	}()
	if err := s.beginRequest(ctx); err != nil {
		return nil, SendStateNotSent, err
	}
	// Always balance the in-flight count, including when caller code recovers
	// a client or middleware panic. Until Do returns, its evidence is unknown.
	state := SendStateMayHaveSent
	defer func() { s.finishRequest(state) }()

	transferred = true
	// Adapters collect target evidence across their complete logical call,
	// including recovery and redirects. Missing or invalid reports stay uncertain.
	raw, reported, err := h.client.Do(req.WithContext(ctx))
	state = normalizeSendState(reported)
	if err != nil {
		if raw != nil && raw.Body != nil {
			_ = raw.Body.Close()
		}

		if contextErr := s.contextError(ctx); contextErr != nil {
			return nil, state, contextErr
		}

		// Trust only the source's explicit classification, never an error's
		// text or HTTP status. Keep private causes out of the execution boundary;
		// cumulative send evidence independently describes possible effects.
		if errors.Is(err, ErrCredentialsUnavailable) {
			return nil, state, ErrCredentialsUnavailable
		}

		return nil, state, ErrTransport
	}

	// Response preparation owns cleanup on failure and transfers it to the
	// returned body on success.
	retained = true

	response, err := s.prepareResponse(ctx, raw, req.Method, cleanup)
	return response, state, err
}
