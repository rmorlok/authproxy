// Package http runs supplied clients with shared execution limits and bounded
// response bodies. It does not select clients or authorize callers.
package http

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
)

// These classifications deliberately omit upstream URLs, credentials, response
// bodies, and transport error text. Domain adapters map them to their public
// error contracts.
var (
	ErrInvalidInput    = errors.New("invalid HTTP host input")
	ErrRequestLimit    = errors.New("HTTP request limit exceeded")
	ErrResponseLimit   = errors.New("HTTP aggregate response limit exceeded")
	ErrStreamLimit     = errors.New("HTTP response stream limit exceeded")
	ErrHeaderLimit     = errors.New("HTTP response header limit exceeded")
	ErrEncoding        = errors.New("unsupported or invalid HTTP response encoding")
	ErrInvalidResponse = errors.New("invalid HTTP response")
	ErrTransport       = errors.New("HTTP request or response failed")
	// ErrCredentialsUnavailable reflects a trusted source classification. It
	// excludes underlying diagnostics and does not imply that no request ran.
	ErrCredentialsUnavailable = errors.New("HTTP credentials unavailable")
	ErrClosed                 = errors.New("HTTP session closed")
	ErrTimeout                = errors.New("HTTP invocation timeout exceeded")
)

// Limits contains resolved, positive execution policy. The caller combines
// operator ceilings and any authored caps before creating a session.
type Limits struct {
	// Timeout spans the entire invocation, including gaps between requests.
	Timeout time.Duration

	// MaxRequests counts logical client calls. Existing credential recovery,
	// redirects, and credential refresh may cause additional HTTP attempts.
	MaxRequests int

	// MaxResponseBytes bounds decoded bytes accepted across all response reads,
	// including partial responses later discarded by the caller. Exceeding it
	// cancels the entire session, including other requests and pending reads.
	MaxResponseBytes int64

	// MaxStreamBytes bounds each response before this package decodes it. This
	// is not a wire-byte cap: net/http may have already decompressed the stream.
	MaxStreamBytes int64

	// MaxHeaderBytes bounds each copied header map, charging names and values
	// plus line overhead so empty values also consume allowance.
	MaxHeaderBytes int
}

// Session applies an execution budget to one supplied client.
// Concurrent calls share the same allowances. Close it when the invocation
// ends, and close every returned response body, even when discarding a status.
// Its zero value is not usable; create sessions with New.
type Session struct {
	// client fixes the outbound capability and attribution supplied by the caller.
	client Client

	// parent preserves caller cancellation separately from policy expiration.
	parent context.Context

	// ctx preserves caller context values and the shared execution deadline.
	ctx context.Context

	// cancel interrupts requests and response reads when the session ends.
	cancel context.CancelCauseFunc

	// stopTimer releases the invocation timeout and optional admission deadline
	// timers on Close.
	stopTimer context.CancelFunc

	// limits is immutable effective policy for the entire invocation.
	limits Limits

	// mu protects shared consumption across concurrent requests and body reads.
	mu sync.Mutex

	// requests counts handoffs, including those whose transport later fails.
	requests int

	// inFlight counts calls whose send evidence has not settled. They remain
	// uncertain even if every completed call was rejected before target I/O.
	inFlight int

	// sendState accumulates completed calls: confirmed sends dominate possible
	// sends, which dominate known-unsent calls. In-flight uncertainty is separate.
	sendState SendState

	// responseBytes counts accepted decoded bytes; excess marks the session
	// exhausted without overflowing this counter or returning excess bytes.
	responseBytes int64

	// responseExceeded remains true after any read exceeds the shared limit.
	responseExceeded bool
}

// New captures an already admitted client and starts the execution timeout.
// Authorization, configuration snapshots, attribution, and policy resolution
// belong to the caller. The session neither reconfigures nor closes the client.
func New(
	ctx context.Context,
	client Client,
	limits Limits,
) (*Session, error) {
	return NewWithDeadline(ctx, client, limits, time.Time{})
}

// NewWithDeadline starts a session with New's configured client and policy,
// additionally honoring an absolute policy deadline captured during admission.
// A zero deadline adds no restriction. An earlier deadline includes preparation
// time in the shared allowance; a later one cannot extend limits.Timeout or a
// caller deadline. Policy expiration returns ErrTimeout while caller cancellation
// retains its original context error, even when both restrict the session.
func NewWithDeadline(
	ctx context.Context,
	client Client,
	limits Limits,
	deadline time.Time,
) (*Session, error) {
	if ctx == nil ||
		client == nil ||
		limits.Timeout <= 0 ||
		limits.MaxRequests <= 0 ||
		limits.MaxResponseBytes <= 0 ||
		limits.MaxStreamBytes <= 0 ||
		limits.MaxHeaderBytes <= 0 {
		return nil, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	timed, stopTimeout := context.WithTimeoutCause(ctx, limits.Timeout, ErrTimeout)
	stopTimer := stopTimeout
	currentDeadline, hasDeadline := timed.Deadline()

	if !deadline.IsZero() && (!hasDeadline || deadline.Before(currentDeadline)) {
		// Do not duplicate an equal caller deadline with a policy timer: its
		// callback could win the race and misclassify caller expiration.
		var stopDeadline context.CancelFunc
		timed, stopDeadline = context.WithDeadlineCause(timed, deadline, ErrTimeout)
		stopTimer = func() {
			stopDeadline()
			stopTimeout()
		}
	}

	run, cancel := context.WithCancelCause(timed)

	return &Session{
		client:    client,
		parent:    ctx,
		ctx:       run,
		cancel:    cancel,
		stopTimer: stopTimer,
		limits:    limits,
		sendState: SendStateNotSent,
	}, nil
}

// Context supplies the shared execution deadline and caller context values to
// request preparation, response processing, and work between requests.
func (s *Session) Context() context.Context { return s.ctx }

// Err classifies invocation cancellation or expiration for callers doing work
// between requests. Caller cancellation retains its context error; the host's
// own timeout returns ErrTimeout. A live session returns nil.
func (s *Session) Err() error { return s.contextError(s.ctx) }

// Close cancels outstanding I/O and releases the invocation timer. It is safe
// to call repeatedly or concurrently with Do and response body reads.
func (s *Session) Close() {
	s.cancel(ErrClosed)
	s.stopTimer()
}

// SendState reports cumulative target-send evidence. No calls means known-unsent;
// pending calls and missing evidence mean possibly-sent. A confirmed send always
// dominates uncertainty, and completed possible sends cannot be erased.
//
// While Do calls are outstanding, the answer is provisional: uncertainty can
// become known-unsent once all calls return with proof they were unsent. Closing
// the session does not wait for those calls. Stop admitting work and wait for
// every Do to return before treating this as settled evidence. A result already
// emitted with uncertainty must retain that outcome despite later evidence.
// Evidence describes sends, never completion of a caller's business action.
func (s *Session) SendState() SendState {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sendState == SendStateSent {
		return SendStateSent
	}
	if s.inFlight != 0 {
		return SendStateMayHaveSent
	}
	return s.sendState
}

// Do transfers ownership of req.Body on entry, including local rejection. It
// returns headers before consuming the response body. The caller must close
// that body and must handle read errors before using a complete response. The
// returned evidence covers this call even on error; local rejection is known
// unsent. SendState reports the aggregate across calls.
//
// The session context supplies caller values; req.Context additionally restricts
// cancellation and deadline, without replacing those values. Host overrides,
// GetBody replay, authentication, and instrumentation retain the supplied client
// behavior. This host adds no retries.
func (s *Session) Do(req *http.Request) (*Response, SendState, error) {
	transferred := false
	defer func() {
		if !transferred && req != nil && req.Body != nil {
			_ = req.Body.Close()
		}
	}()

	if s == nil ||
		s.ctx == nil ||
		req == nil ||
		req.URL == nil ||
		(req.URL.Scheme != "http" && req.URL.Scheme != "https") ||
		req.URL.Host == "" ||
		req.URL.Opaque != "" ||
		req.URL.Fragment != "" ||
		req.RequestURI != "" {
		return nil, SendStateNotSent, ErrInvalidInput
	}

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
	raw, reported, err := s.client.Do(req.WithContext(ctx))
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

// beginRequest reserves one logical call immediately before client handoff.
// Reservations are never refunded, including credential or transport failures.
func (s *Session) beginRequest(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.contextError(ctx); err != nil {
		return err
	}
	if s.requests >= s.limits.MaxRequests {
		return ErrRequestLimit
	}
	s.requests++
	s.inFlight++
	return nil
}

// finishRequest settles one client's evidence while preserving possible sends
// from every earlier call. Response-body failures cannot reverse this state.
func (s *Session) finishRequest(state SendState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inFlight--
	switch normalizeSendState(state) {
	case SendStateSent:
		s.sendState = SendStateSent
	case SendStateMayHaveSent:
		if s.sendState != SendStateSent {
			s.sendState = SendStateMayHaveSent
		}
	}
}

// remainingResponse returns a read-size hint, not a reservation. Concurrent
// reads must still charge their bytes atomically before exposing them.
func (s *Session) remainingResponse() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.responseExceeded {
		return 0
	}
	return s.limits.MaxResponseBytes - s.responseBytes
}

// chargeResponse accepts a decoded chunk only when the aggregate allowance
// permits it. Excess permanently ends the session; consumed bytes are never
// refunded when a response is discarded or its next read fails.
func (s *Session) chargeResponse(n int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.responseExceeded ||
		int64(n) > s.limits.MaxResponseBytes-s.responseBytes {
		s.responseExceeded = true
		s.cancel(ErrResponseLimit)
		return ErrResponseLimit
	}

	s.responseBytes += int64(n)

	return nil
}

// contextError keeps caller cancellation separate from host policy failures,
// and never exposes an arbitrary cancellation cause supplied by external code.
func (s *Session) contextError(ctx context.Context) error {
	if err := s.parent.Err(); err != nil {
		return err
	}

	if ctx.Err() == nil {
		return nil
	}

	switch cause := context.Cause(ctx); cause {
	case ErrTimeout, ErrClosed, ErrResponseLimit, context.Canceled, context.DeadlineExceeded:
		return cause
	default:
		return ctx.Err()
	}
}
