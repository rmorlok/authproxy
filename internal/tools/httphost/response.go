package httphost

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/textproto"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/net/http/httpguts"
)

// Response exposes only bounded upstream metadata and body. It
// deliberately omits http.Response.Request, which can contain credentials.
type Response struct {
	// StatusCode is the validated upstream status, including unsuccessful ones.
	StatusCode int

	// ContentType captures the media type, including parameters, before header
	// filtering. Use it for body decoding even if Header omits Content-Type or
	// the caller changes the map. An absent Content-Type produces an empty value.
	ContentType string

	// Header is a detached, canonicalized map owned by the caller. Known
	// authentication, cookie, and hop-by-hop fields are removed before exposure.
	Header http.Header

	// Body is read lazily and must be closed, including when discarding a status.
	Body io.ReadCloser
}

// responseReadSize bounds each decoded read and each underlying stream read.
// Concurrent reads can consume scratch bytes before aggregate charging decides
// which chunks fit, but no rejected bytes are copied into a caller's buffer.
const responseReadSize = 32 * 1024

// prepareResponse assumes ownership of raw.Body and cleanup on every path. It
// validates/copies headers without reading content, so status rules can discard
// a response without initializing gzip or otherwise decoding an error body.
func (s *Session) prepareResponse(
	ctx context.Context,
	raw *http.Response,
	method string,
	cleanup func(),
) (*Response, error) {
	body := &responseBody{
		session: s,
		ctx:     ctx,
		cleanup: cleanup,
	}

	if raw != nil {
		body.raw = raw.Body
	}

	if err := s.contextError(ctx); err != nil {
		_ = body.Close()
		return nil, err
	}
	if raw == nil ||
		raw.Body == nil ||
		raw.StatusCode < 100 ||
		raw.StatusCode > 599 {
		_ = body.Close()
		return nil, ErrInvalidResponse
	}

	headers, err := copyResponseHeaders(raw.Header, s.limits.MaxHeaderBytes)
	if err != nil {
		_ = body.Close()
		return nil, err
	}
	contentType := headers.Get("Content-Type")

	// HEAD and statuses that forbid a message body retain representation
	// metadata without treating it as instructions to decode a missing payload.
	bodyless := method == http.MethodHead ||
		raw.StatusCode < 200 ||
		raw.StatusCode == http.StatusNoContent ||
		raw.StatusCode == http.StatusNotModified

	if bodyless {
		body.reader = http.NoBody
	} else if !raw.Uncompressed {
		encodings := headers.Values("Content-Encoding")
		if len(encodings) > 1 {
			body.encoding = "unsupported"
		} else if len(encodings) == 1 {
			body.encoding = strings.ToLower(strings.TrimSpace(encodings[0]))
			// RFC 9110 treats the legacy x-gzip coding as equivalent to gzip.
			if body.encoding == "x-gzip" {
				body.encoding = "gzip"
			}
		}
	}

	if body.encoding == "gzip" {
		// These fields describe the encoded stream, not the bytes callers read.
		headers.Del("Content-Encoding")
		headers.Del("Content-Length")
	}

	// Capture decoding instructions first: a Connection nomination can remove
	// Content-Type or Content-Encoding without changing the actual stream.
	filterResponseHeaders(headers)

	body.stream = &responseStream{
		raw:       raw.Body,
		remaining: s.limits.MaxStreamBytes,
	}

	// Synchronize installing the stop function with an immediately scheduled
	// cancellation callback; Close never needs the read mutex to unblock I/O.
	body.cancelMu.Lock()
	body.stopCancel = context.AfterFunc(ctx, func() { _ = body.Close() })
	body.cancelMu.Unlock()

	if err := s.contextError(ctx); err != nil {
		_ = body.Close()
		return nil, err
	}

	return &Response{
		StatusCode:  raw.StatusCode,
		ContentType: contentType,
		Header:      headers,
		Body:        body,
	}, nil
}

// copyResponseHeaders validates and detaches metadata before exposing it.
// Accounting charges a serialized line's name, value, colon/space, and CRLF;
// even an empty value list consumes name/line overhead before allocation.
func copyResponseHeaders(source http.Header, maxBytes int) (http.Header, error) {
	remaining := maxBytes
	if len(source) > remaining/5 {
		return nil, ErrHeaderLimit
	}

	headers := make(http.Header, len(source))
	for name, values := range source {
		if len(name) > remaining || remaining-len(name) < 4 {
			return nil, ErrHeaderLimit
		}

		if !httpguts.ValidHeaderFieldName(name) {
			return nil, ErrInvalidResponse
		}

		lineBytes := len(name) + 4
		if len(values) == 0 {
			remaining -= lineBytes
		} else {
			// Check slice width before iterating a large list of empty strings.
			if len(values) > remaining/lineBytes {
				return nil, ErrHeaderLimit
			}

			for _, value := range values {
				if remaining < lineBytes || len(value) > remaining-lineBytes {
					return nil, ErrHeaderLimit
				}

				remaining -= lineBytes + len(value)
				if !httpguts.ValidHeaderFieldValue(value) {
					return nil, ErrInvalidResponse
				}
			}
		}

		name = http.CanonicalHeaderKey(name)
		headers[name] = append(headers[name], values...)
	}

	return headers, nil
}

// filterResponseHeaders removes known authentication/cookie fields and
// hop-by-hop metadata from an already bounded, validated, canonicalized copy.
// Connection nominations are removed before Connection itself. The fixed list
// includes standard auth fields, legacy Set-Cookie2/Proxy-Connection/Trailers,
// and transport-only fields; it cannot identify secrets in arbitrary provider
// headers or bodies. Decoding must capture needed metadata before this call.
func filterResponseHeaders(headers http.Header) {
	for _, value := range headers.Values("Connection") {
		for name := range strings.SplitSeq(value, ",") {
			if name = textproto.TrimString(name); name != "" {
				headers.Del(name)
			}
		}
	}

	for _, name := range []string{
		"Authorization",
		"Cookie",
		"Set-Cookie",
		"Set-Cookie2",
		"Www-Authenticate",
		"Authentication-Info",
		"Proxy-Authentication-Info",
		"Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailer",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	} {
		headers.Del(name)
	}
}

// responseBody coordinates lazy decoding, serialized reads, and an independent
// close path that can interrupt blocked transport reads. Terminal read outcomes
// survive cleanup's context cancellation and are returned on subsequent reads.
type responseBody struct {
	// session supplies shared decoded-byte accounting and safe context errors.
	session *Session

	// ctx is the request's invocation-derived cancellation scope.
	ctx context.Context

	// raw is closed exactly once; stream bounds bytes before host decoding.
	raw    io.ReadCloser
	stream *responseStream

	// encoding is captured before returned headers can be changed by the caller.
	encoding string

	// readMu protects reader initialization, scratch, and the terminal outcome.
	readMu sync.Mutex

	// reader is initialized on first nonempty Read, except bodyless responses
	// use http.NoBody immediately. Status-only callers never initialize gzip.
	reader io.Reader

	// scratch is allocated lazily and reused for decoded chunks until a terminal
	// read. Bytes reach the caller only after charging and cancellation checks.
	scratch []byte

	// terminal retains EOF or a safe failure after the underlying stream closes.
	terminal error

	// closed can be observed without blocking an in-progress Read.
	closed atomic.Bool

	// explicitlyClosed distinguishes caller Close from a cancellation callback;
	// request cleanup's own cancellation must not obscure an explicit Close.
	explicitlyClosed atomic.Bool

	// closeOnce owns raw.Close and cleanup; closeErr is published by sync.Once.
	closeOnce sync.Once
	closeErr  error

	// cleanup releases request-context resources exactly once, after raw.Close.
	cleanup func()

	// cancelMu protects installation of the cancellation callback's stop handle.
	cancelMu   sync.Mutex
	stopCancel func() bool
}

// Read exposes only successfully charged bytes. Each call consumes at most
// 32 KiB of decoded scratch, or a one-byte probe after the aggregate cap is
// reached. A terminal error discards this call's scratch and never reads again.
func (b *responseBody) Read(p []byte) (int, error) {
	b.readMu.Lock()
	defer b.readMu.Unlock()

	if b.terminal != nil {
		return 0, b.terminal
	}
	if b.explicitlyClosed.Load() {
		return b.finish(ErrClosed)
	}
	if err := b.session.contextError(b.ctx); err != nil {
		return b.finish(err)
	}
	if b.closed.Load() {
		return b.finish(ErrClosed)
	}
	if len(p) == 0 {
		return 0, nil
	}

	if b.reader == nil {
		switch b.encoding {
		case "", "identity":
			b.reader = b.stream
		case "gzip":
			reader, err := gzip.NewReader(b.stream)
			if err != nil {
				return b.finish(b.readError(err))
			}
			b.reader = reader
		default:
			return b.finish(ErrEncoding)
		}
	}

	size := min(len(p), responseReadSize)

	if remaining := b.session.remainingResponse(); remaining < int64(size) {
		size = max(int(remaining), 1)
	}

	if b.scratch == nil {
		b.scratch = make([]byte, responseReadSize)
	}
	n, err := b.reader.Read(b.scratch[:size])
	if n < 0 || n > size {
		return b.finish(ErrTransport)
	}

	// Charge consumed decoded bytes even if their accompanying error means the
	// caller cannot use this chunk. No partial response refunds the invocation.
	chargeErr := b.session.chargeResponse(n)
	if b.explicitlyClosed.Load() {
		return b.finish(ErrClosed)
	}

	if contextErr := b.session.contextError(b.ctx); contextErr != nil {
		return b.finish(contextErr)
	}

	if chargeErr != nil {
		return b.finish(chargeErr)
	}
	if b.closed.Load() {
		return b.finish(ErrClosed)
	}

	if err != nil && err != io.EOF {
		return b.finish(b.readError(err))
	}

	copy(p, b.scratch[:n])

	if err == io.EOF {
		_, _ = b.finish(io.EOF)
	}

	return n, err
}

// readError preserves host limits and transport failures without forwarding
// upstream error text. Other errors from the gzip decoder indicate bad coding.
func (b *responseBody) readError(err error) error {
	if b.explicitlyClosed.Load() {
		return ErrClosed
	}

	if contextErr := b.session.contextError(b.ctx); contextErr != nil {
		return contextErr
	}

	if errors.Is(err, ErrStreamLimit) {
		return ErrStreamLimit
	}

	if errors.Is(err, ErrTransport) {
		return ErrTransport
	}

	return ErrEncoding
}

// finish records a stable terminal outcome before closing the raw stream and
// releasing scratch and the request context. The caller must hold readMu;
// concurrent Close cannot release scratch while Read may still be using it.
func (b *responseBody) finish(err error) (int, error) {
	b.terminal = err
	b.scratch = nil
	_ = b.Close()
	return 0, err
}

// Close closes the transport without draining or waiting on readMu. This lets
// cancellation unblock ordinary HTTP reads while cleanup executes exactly once.
func (b *responseBody) Close() error {
	b.closeOnce.Do(func() {
		if b.session.contextError(b.ctx) == nil {
			b.explicitlyClosed.Store(true)
		}

		b.closed.Store(true)
		b.cancelMu.Lock()
		if b.stopCancel != nil {
			b.stopCancel()
		}
		b.cancelMu.Unlock()

		if b.raw != nil {
			if err := b.raw.Close(); err != nil {
				b.closeErr = ErrTransport
			}
		}

		if b.cleanup != nil {
			b.cleanup()
		}
	})

	return b.closeErr
}

// responseStream caps bytes before host decoding. It never asks the underlying
// reader for more than 32 KiB, using one extra byte only to distinguish an exact
// cap followed by EOF from an oversized stream. Its owner serializes reads.
type responseStream struct {
	// raw is the original transport body, which responseBody exclusively closes.
	raw io.Reader

	// remaining counts bytes available before host decoding; zero permits only
	// a one-byte EOF probe, never additional data passed to the decoder.
	remaining int64
}

// Read preserves EOF and replaces arbitrary transport errors with a sentinel.
// Probe bytes beyond the allowance are discarded, never passed to the decoder.
func (s *responseStream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	if s.remaining == 0 {
		var probe [1]byte
		n, err := s.raw.Read(probe[:])
		if n != 0 {
			return 0, ErrStreamLimit
		}
		if err != nil && err != io.EOF {
			return 0, ErrTransport
		}
		return 0, err
	}

	size := min(len(p), responseReadSize)
	if int64(size) > s.remaining {
		size = int(s.remaining)
	}

	n, err := s.raw.Read(p[:size])
	if n < 0 || n > size {
		return 0, ErrTransport
	}

	s.remaining -= int64(n)

	if err != nil && err != io.EOF {
		return n, ErrTransport
	}

	return n, err
}
