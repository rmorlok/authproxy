// Package httphost sends Tool requests through one captured connection with
// shared invocation limits. It does not perform admission or authorize callers.
package httphost

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/api"
)

// These classifications deliberately omit upstream URLs, credentials, response
// bodies, and transport error text. Callers map them to safe Tool errors.
var (
	ErrInvalidInput    = errors.New("invalid Tool HTTP host input")
	ErrRequestLimit    = errors.New("Tool HTTP request limit exceeded")
	ErrResponseLimit   = errors.New("Tool HTTP aggregate response limit exceeded")
	ErrStreamLimit     = errors.New("Tool HTTP response stream limit exceeded")
	ErrHeaderLimit     = errors.New("Tool HTTP response header limit exceeded")
	ErrEncoding        = errors.New("unsupported or invalid Tool HTTP response encoding")
	ErrInvalidResponse = errors.New("invalid Tool HTTP response")
	ErrTransport       = errors.New("Tool HTTP request or response failed")
	// ErrCredentialsUnavailable reflects a trusted source classification. It
	// excludes underlying diagnostics and does not imply that no request ran.
	ErrCredentialsUnavailable = errors.New("Tool HTTP credentials unavailable")
	ErrClosed                 = errors.New("Tool HTTP session closed")
	ErrTimeout                = errors.New("Tool HTTP invocation timeout exceeded")
)

// Limits contains resolved, positive execution policy. The caller combines
// operator ceilings and the Tool's optional caps before creating a session.
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

// Session applies an invocation budget to one configured connection client.
// Concurrent calls share the same allowances. Close it when the invocation
// ends, and close every returned response body, even when discarding a status.
// Its zero value is not usable; create sessions with New.
type Session struct {
	// client is configured for the admitted connection and request attribution.
	client iface.HTTPClient

	// parent preserves caller cancellation separately from policy expiration.
	parent context.Context

	// ctx carries the admitted actor and the shared invocation deadline.
	ctx context.Context

	// cancel interrupts requests and response reads when the session ends.
	cancel context.CancelCauseFunc

	// stopTimer releases the total invocation timer on Close.
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

	// mayHaveExecuted stays true after any completed call reports a possible
	// target send or provides no evidence. Later local rejections cannot undo it.
	mayHaveExecuted bool

	// responseBytes counts accepted decoded bytes; excess marks the session
	// exhausted without overflowing this counter or returning excess bytes.
	responseBytes int64

	// responseExceeded remains true after any read exceeds the shared limit.
	responseExceeded bool
}

// New captures a client for an already admitted Tool and starts its timeout.
// Callers must configure client from the admitted connection with proxy request
// attribution and an explicit actor choice matching any actor carried by ctx.
// Authorization, input validation, connection configuration snapshots, and
// policy resolution also belong to callers.
func New(
	ctx context.Context,
	client iface.HTTPClient,
	limits Limits,
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

	timed, stopTimer := context.WithTimeoutCause(ctx, limits.Timeout, ErrTimeout)
	run, cancel := context.WithCancelCause(timed)

	return &Session{
		client:    client,
		parent:    ctx,
		ctx:       run,
		cancel:    cancel,
		stopTimer: stopTimer,
		limits:    limits,
	}, nil
}

// Context supplies the shared invocation deadline and admitted identity to
// request rendering, response transformation, and other invocation work.
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

// ExecutionState reports cumulative uncertainty about upstream effects. Calls
// still in flight, possible target sends, and clients without send evidence
// remain uncertain. Completed calls proven unsent do not imply execution, even
// though they consumed the request allowance. HTTP success alone does not
// establish that the complete Tool action finished.
//
// While Do calls are outstanding, the answer is provisional: mayHaveExecuted
// can become notStarted when all calls return with proof they were unsent.
// Canceling or closing the session does not wait for those calls to finish.
// Callers must stop starting requests and wait for every Do to return before
// treating this as the settled invocation state. A result already reported
// while work was outstanding must retain its conservative execution state.
func (s *Session) ExecutionState() api.ToolExecutionState {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.inFlight == 0 && !s.mayHaveExecuted {
		return api.ToolExecutionStateNotStarted
	}

	return api.ToolExecutionStateMayHaveExecuted
}

// Do transfers ownership of req.Body on entry, including local rejection. It
// returns headers before consuming the response body. The caller must close
// that body and must handle read errors before using a complete response.
//
// The session context supplies actor and other values; req.Context additionally
// restricts cancellation and deadline, without replacing that admitted identity.
// Host overrides, GetBody replay, authentication, and transport instrumentation
// retain the existing connection proxy behavior. This host adds no retries.
func (s *Session) Do(req *http.Request) (*Response, error) {
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
		return nil, ErrInvalidInput
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
	// rejected synchronously before it can consume a request or reach the proxy.
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
		return nil, err
	}
	// Always balance the in-flight count, including when caller code recovers
	// a client or middleware panic. Until Do returns, its evidence is unknown.
	mayHaveSent := true
	defer func() { s.finishRequest(mayHaveSent) }()

	transferred = true
	// Each logical call gets its own observation. Unknown clients remain
	// conservative; supported clients distinguish local rejection from target
	// transport handoff without changing the standard Do signature.
	observedCtx, observation := httpf.ObserveSends(ctx)
	raw, err := s.client.Do(req.WithContext(observedCtx))
	mayHaveSent = observation.MayHaveSent()
	if err != nil {
		if raw != nil && raw.Body != nil {
			_ = raw.Body.Close()
		}

		if contextErr := s.contextError(ctx); contextErr != nil {
			return nil, contextErr
		}

		// Trust only the source's explicit classification, never an error's
		// text or HTTP status. Keep private causes out of the Tool boundary;
		// cumulative send evidence independently describes possible effects.
		if errors.Is(err, auth_methods.ErrCredentialsUnavailable) {
			return nil, ErrCredentialsUnavailable
		}

		return nil, ErrTransport
	}

	// Response preparation owns cleanup on failure and transfers it to the
	// returned body on success.
	retained = true

	return s.prepareResponse(ctx, raw, req.Method, cleanup)
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
func (s *Session) finishRequest(mayHaveSent bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inFlight--
	s.mayHaveExecuted = s.mayHaveExecuted || mayHaveSent
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
