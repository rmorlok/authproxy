// Package httphost sends Tool requests through one captured connection with
// shared invocation limits. It does not perform admission or authorize callers.
package httphost

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"sync"
	"time"

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
	ErrClosed          = errors.New("Tool HTTP session closed")
	ErrTimeout         = errors.New("Tool HTTP invocation timeout exceeded")
)

// Limits contains resolved, positive execution policy. The caller combines
// operator ceilings and the Tool's optional caps before creating a session.
type Limits struct {
	// Timeout spans the entire invocation, including gaps between requests.
	Timeout time.Duration

	// MaxRequests counts logical proxy calls. Existing credential recovery,
	// redirects, and credential refresh may cause additional HTTP attempts.
	MaxRequests int

	// MaxResponseBytes bounds decoded bytes accepted across all response reads,
	// including partial responses later discarded by the caller.
	MaxResponseBytes int64

	// MaxStreamBytes bounds each response before this package decodes it. This
	// is not a wire-byte cap: net/http may have already decompressed the stream.
	MaxStreamBytes int64

	// MaxHeaderBytes bounds each copied header map, charging names and values
	// plus line overhead so empty values also consume allowance.
	MaxHeaderBytes int
}

// Session owns the connection and budget for one admitted Tool invocation.
// Concurrent calls share the same allowances. Close it when the invocation
// ends, and close every returned response body, even when discarding a status.
// Its zero value is not usable; create sessions with New.
type Session struct {
	// connection is the sole routing capability available to this invocation.
	connection iface.Connection

	// parent preserves caller cancellation separately from policy expiration.
	parent context.Context

	// ctx carries the admitted actor and the shared invocation deadline.
	ctx context.Context

	// cancel interrupts requests and response reads when the session ends.
	cancel context.CancelCauseFunc

	// stopTimer releases the total invocation timer on Close.
	stopTimer context.CancelFunc

	// limits is immutable effective policy; labels is a detached request snapshot.
	limits Limits
	labels map[string]string

	// mu protects shared consumption across concurrent requests and body reads.
	mu sync.Mutex

	// requests counts handoffs, including those whose transport later fails.
	requests int

	// responseBytes counts accepted decoded bytes; excess marks the session
	// exhausted without overflowing this counter or returning excess bytes.
	responseBytes int64

	// responseExceeded remains true after any read exceeds the shared limit.
	responseExceeded bool
}

// New captures a connection and detached labels for an already admitted Tool.
// It starts the total timeout immediately. Authorization, input validation,
// connection configuration snapshots, and policy resolution belong to callers.
func New(
	ctx context.Context,
	connection iface.Connection,
	limits Limits,
	labels map[string]string,
) (*Session, error) {
	if ctx == nil ||
		connection == nil ||
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
		connection: connection,
		parent:     ctx,
		ctx:        run,
		cancel:     cancel,
		stopTimer:  stopTimer,
		limits:     limits,
		labels:     maps.Clone(labels),
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

// ExecutionState reports cumulative uncertainty about upstream effects. Once a
// request reaches the proxy, even a transport failure cannot prove it was unsent.
// HTTP success alone does not establish that the complete Tool action finished.
func (s *Session) ExecutionState() api.ToolExecutionState {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.requests == 0 {
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

	transferred = true
	raw, err := s.connection.ProxyRequestStream(
		ctx,
		httpf.RequestTypeProxy,
		&iface.RawProxyRequest{
			Outbound: req,
			Labels:   maps.Clone(s.labels),
		})
	if err != nil {
		if raw != nil && raw.Body != nil {
			_ = raw.Body.Close()
		}

		if contextErr := s.contextError(ctx); contextErr != nil {
			return nil, contextErr
		}

		// The proxy does not yet report stable failure categories or cumulative
		// send evidence across recovery attempts. Do not infer reconnect advice
		// or safe replay from generic error text or an HTTP-aware error's status.
		return nil, ErrTransport
	}

	// Response preparation owns cleanup on failure and transfers it to the
	// returned body on success.
	retained = true

	return s.prepareResponse(ctx, raw, req.Method, cleanup)
}

// beginRequest reserves one logical call immediately before proxy handoff.
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
	return nil
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
