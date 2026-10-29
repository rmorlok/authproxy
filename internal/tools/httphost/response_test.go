package httphost

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// responseTestSession isolates response accounting without making proxy calls.
func responseTestSession(t *testing.T, decoded, stream int64) *Session {
	t.Helper()
	ctx, cancel := context.WithCancelCause(context.Background())
	s := &Session{
		parent: context.Background(), ctx: ctx, cancel: cancel, stopTimer: func() {},
		limits: Limits{MaxResponseBytes: decoded, MaxStreamBytes: stream, MaxHeaderBytes: 4096},
	}
	t.Cleanup(s.Close)
	return s
}

// responseTestPrepare supplies request cleanup that behaves like Session.Do.
func responseTestPrepare(t *testing.T, s *Session, raw *http.Response) (*Response, *atomic.Int64) {
	t.Helper()
	ctx, cancel := context.WithCancel(s.ctx)
	cleanups := &atomic.Int64{}
	response, err := s.prepareResponse(ctx, raw, http.MethodGet, func() { cleanups.Add(1); cancel() })
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	return response, cleanups
}

// responseObservedBody records reads and ownership transitions. Its owner
// serializes reads, while Close may run concurrently and updates only an atomic.
type responseObservedBody struct {
	reader io.Reader
	sizes  []int
	closes atomic.Int64
}

// Read records requested sizes before returning the underlying payload.
func (b *responseObservedBody) Read(p []byte) (int, error) {
	b.sizes = append(b.sizes, len(p))
	return b.reader.Read(p)
}

// Close records release without reading or draining the remaining payload.
func (b *responseObservedBody) Close() error { b.closes.Add(1); return nil }

// responseGzip constructs a complete compressed stream for decoding scenarios.
func responseGzip(t *testing.T, payload string) []byte {
	t.Helper()
	var encoded bytes.Buffer
	writer := gzip.NewWriter(&encoded)
	_, err := io.WriteString(writer, payload)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return encoded.Bytes()
}

func TestResponseHeadersPrecedeBodyDecoding(t *testing.T) {
	for _, encoding := range []string{"gzip", "x-gzip", " \tX-GZiP\t ", "br", "gzip, br", "identity"} {
		t.Run(encoding, func(t *testing.T) {
			// Even invalid or unsupported error-body encoding must remain unread
			// until the status policy decides whether content is needed.
			s := responseTestSession(t, 100, 100)
			body := &responseObservedBody{reader: strings.NewReader("not compressed")}
			raw := &http.Response{StatusCode: 429, Header: http.Header{
				"Content-Encoding": {encoding}, "Content-Length": {"14"}, "X-Trace": {"original"},
			}, Body: body, Request: &http.Request{Header: http.Header{"Authorization": {"private"}}}}
			response, cleanups := responseTestPrepare(t, s, raw)
			require.Equal(t, 429, response.StatusCode)
			require.Empty(t, body.sizes)

			// Header edits cannot reach transport metadata, and discarding a status
			// closes ownership once without initializing gzip or draining content.
			response.Header["X-Trace"][0] = "changed"
			require.Equal(t, "original", raw.Header.Get("X-Trace"))
			require.NoError(t, response.Body.Close())
			require.NoError(t, response.Body.Close())
			require.Empty(t, body.sizes)
			require.EqualValues(t, 1, body.closes.Load())
			require.EqualValues(t, 1, cleanups.Load())
			_, err := response.Body.Read(make([]byte, 1))
			require.ErrorIs(t, err, ErrClosed)
		})
	}
}

func TestResponseFiltersCredentialAndHopHeaders(t *testing.T) {
	// Provider metadata can include credentials, handshakes, or fields declared
	// meaningful only for this connection. Normalize all names before filtering.
	headers := http.Header{
		"aUtHoRiZaTiOn":             {"Bearer private-token"},
		"cOoKiE":                    {"private=session"},
		"set-cookie":                {"private=first", "private=second"},
		"SET-COOKIE":                {"private=third"},
		"sEt-CoOkIe2":               {"private=legacy"},
		"wWw-AuThEnTiCaTe":          {"Basic realm=private"},
		"aUtHeNtIcAtIoN-InFo":       {"nextnonce=private"},
		"pRoXy-AuThEnTiCaTiOn-InFo": {"nextnonce=private"},
		"pRoXy-AuThEnTiCaTe":        {"Basic realm=private"},
		"pRoXy-AuThOrIzAtIoN":       {"Basic private"},
		"cOnNeCtIoN":                {"keep-alive, x-connection-only", "\tX-Other-Hop, Connection "},
		"CONNECTION":                {"x-third-hop"},
		"pRoXy-CoNnEcTiOn":          {"keep-alive"},
		"kEeP-aLiVe":                {"timeout=5"},
		"tE":                        {"trailers"},
		"tRaIlEr":                   {"X-Private-Trailer"},
		"tRaIlErS":                  {"X-Private-Trailer"},
		"tRaNsFeR-EnCoDiNg":         {"chunked"},
		"uPgRaDe":                   {"websocket"},
		"X-Connection-Only":         {"private"},
		"x-other-hop":               {"private", "also private"},
		"X-Third-Hop":               {"private"},
		"content-type":              {"application/json"},
		"Retry-After":               {"60"},
		"RateLimit-Limit":           {"100", "200"},
		"X-Trace":                   {"first", "second"},
		"X-Provider-Custom":         {"opaque metadata"},
	}
	before := headers.Clone()
	body := &responseObservedBody{reader: strings.NewReader("{}")}
	response, _ := responseTestPrepare(t, responseTestSession(t, 100, 100), &http.Response{
		StatusCode: 200, Header: headers, Body: body,
	})

	// Useful end-to-end metadata and repeated value order survive; filtering
	// neither mutates the transport map nor reads the response body.
	require.Equal(t, http.Header{
		"Content-Type": {"application/json"}, "Retry-After": {"60"},
		"Ratelimit-Limit": {"100", "200"}, "X-Trace": {"first", "second"},
		"X-Provider-Custom": {"opaque metadata"},
	}, response.Header)
	require.Equal(t, before, headers)
	require.Empty(t, body.sizes)
}

func TestResponseFiltersAfterRawHeaderValidation(t *testing.T) {
	for _, test := range []struct {
		name    string
		headers http.Header
		limit   int
		wantErr error
	}{
		{"credential bytes", http.Header{"Set-Cookie": {strings.Repeat("x", 100)}}, 30, ErrHeaderLimit},
		{"credential values", http.Header{"Set-Cookie": make([]string, 100)}, 100, ErrHeaderLimit},
		{"nominated bytes", http.Header{"Connection": {"X-Hop"}, "X-Hop": {strings.Repeat("x", 100)}}, 50, ErrHeaderLimit},
		{"credential controls", http.Header{"Set-Cookie": {"private\r\nInjected: value"}}, 100, ErrInvalidResponse},
		{"connection controls", http.Header{"Connection": {"X-Hop\x00"}}, 100, ErrInvalidResponse},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Fields destined for removal must still consume the original input
			// allowance and pass HTTP syntax checks before any map is exposed.
			s := responseTestSession(t, 100, 100)
			s.limits.MaxHeaderBytes = test.limit
			body := &responseObservedBody{reader: strings.NewReader("private body")}
			var cleanups atomic.Int64
			response, err := s.prepareResponse(s.ctx, &http.Response{
				StatusCode: 200, Header: test.headers, Body: body,
			}, http.MethodGet, func() { cleanups.Add(1) })
			require.ErrorIs(t, err, test.wantErr)
			require.Nil(t, response)
			require.Empty(t, body.sizes)
			require.EqualValues(t, 1, body.closes.Load())
			require.EqualValues(t, 1, cleanups.Load())
		})
	}
}

func TestResponseCapturesEncodingBeforeHeaderFiltering(t *testing.T) {
	for _, encoding := range []string{"gzip", "x-gzip", "br"} {
		t.Run(encoding, func(t *testing.T) {
			// Connection can nominate encoding metadata for removal, but the
			// transport bytes still require that captured decoding policy.
			payload := responseGzip(t, "decoded payload")
			headers := http.Header{
				"connection":       {" CONTENT-encoding, X-Hop "},
				"content-encoding": {encoding}, "Content-Length": {strconv.Itoa(len(payload))},
				"X-Hop": {"private"}, "Content-Type": {"text/plain"},
			}
			before := headers.Clone()
			body := &responseObservedBody{reader: bytes.NewReader(payload)}
			response, _ := responseTestPrepare(t, responseTestSession(t, 100, 100), &http.Response{
				StatusCode: 200, Header: headers, Body: body,
			})
			require.NotContains(t, response.Header, "Connection")
			require.NotContains(t, response.Header, "Content-Encoding")
			require.NotContains(t, response.Header, "X-Hop")
			require.Equal(t, "text/plain", response.Header.Get("Content-Type"))
			require.Equal(t, before, headers)
			require.Empty(t, body.sizes)

			// Gzip still decodes lazily; nominating unsupported coding cannot
			// make the raw compressed stream appear to be an identity response.
			actual, err := io.ReadAll(response.Body)
			if encoding != "br" {
				require.NoError(t, err)
				require.Equal(t, "decoded payload", string(actual))
				require.NotContains(t, response.Header, "Content-Length")
			} else {
				require.ErrorIs(t, err, ErrEncoding)
				require.Empty(t, actual)
				require.Empty(t, body.sizes)
			}
		})
	}
}

func TestResponseGzipAndTransportDecoding(t *testing.T) {
	// Host decoding removes encoded-length metadata and retains other headers.
	payload := strings.Repeat("café", 10000)
	encoded := responseGzip(t, payload)
	s := responseTestSession(t, int64(len(payload)), int64(len(encoded)))
	body := &responseObservedBody{reader: bytes.NewReader(encoded)}
	raw := &http.Response{StatusCode: 200, Header: http.Header{
		"Content-Encoding": {"GZip"}, "Content-Length": {strconv.Itoa(len(encoded))}, "X-Trace": {"kept"},
	}, Body: body}
	response, cleanups := responseTestPrepare(t, s, raw)
	require.Empty(t, body.sizes)
	require.Empty(t, response.Header.Get("Content-Encoding"))
	require.Empty(t, response.Header.Get("Content-Length"))
	require.Equal(t, "kept", response.Header.Get("X-Trace"))
	actual, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, payload, string(actual))
	require.Zero(t, s.remainingResponse())
	for _, size := range body.sizes {
		require.LessOrEqual(t, size, responseReadSize)
	}

	// EOF remains stable even though releasing this body cancels its request.
	_, err = response.Body.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
	require.EqualValues(t, 1, cleanups.Load())
	require.EqualValues(t, 1, body.closes.Load())

	// A transport that already decoded the stream must never be decoded twice.
	for _, encoding := range []string{"gzip", "x-gzip"} {
		s = responseTestSession(t, 100, 100)
		response, _ = responseTestPrepare(t, s, &http.Response{
			StatusCode: 200, Uncompressed: true, Header: http.Header{"Content-Encoding": {encoding}},
			Body: io.NopCloser(strings.NewReader("already decoded")),
		})
		actual, err = io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, "already decoded", string(actual))
	}
}

// TestResponseXGzipDecoding verifies lazy alias decoding and header normalization
// while permitting a payload exactly at both byte limits.
func TestResponseXGzipDecoding(t *testing.T) {
	for _, encoding := range []string{"x-gzip", " \tX-GZiP\t "} {
		t.Run(encoding, func(t *testing.T) {
			// The legacy alias accepts the same case and optional whitespace as
			// gzip, normalizes exposed metadata, and leaves raw headers untouched.
			payload := "decoded café"
			encoded := responseGzip(t, payload)
			s := responseTestSession(t, int64(len(payload)), int64(len(encoded)))
			body := &responseObservedBody{reader: bytes.NewReader(encoded)}
			headers := http.Header{
				"Content-Encoding": {encoding}, "Content-Length": {strconv.Itoa(len(encoded))},
				"Content-Type": {"text/plain"},
			}
			before := headers.Clone()
			response, cleanups := responseTestPrepare(t, s, &http.Response{StatusCode: 200, Header: headers, Body: body})
			require.NotContains(t, response.Header, "Content-Encoding")
			require.NotContains(t, response.Header, "Content-Length")
			require.Equal(t, "text/plain", response.Header.Get("Content-Type"))
			require.Equal(t, before, headers)

			// Neither preparing nor making an empty read starts the decoder.
			n, err := response.Body.Read(nil)
			require.NoError(t, err)
			require.Zero(t, n)
			require.Empty(t, body.sizes)

			// Exact compressed and decoded limits permit the complete payload,
			// including its terminating EOF, and release ownership once.
			actual, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, payload, string(actual))
			require.Zero(t, s.remainingResponse())
			require.EqualValues(t, 1, body.closes.Load())
			require.EqualValues(t, 1, cleanups.Load())
		})
	}
}

func TestResponseBodylessMetadata(t *testing.T) {
	for _, test := range []struct {
		method string
		status int
	}{
		{http.MethodHead, 200}, {http.MethodGet, 204}, {http.MethodGet, 304}, {http.MethodGet, 101},
	} {
		for _, encoding := range []string{"gzip", "x-gzip", "br"} {
			t.Run(test.method+strconv.Itoa(test.status)+encoding, func(t *testing.T) {
				// These responses describe a representation but carry no message
				// body, so their encoding/length headers must not trigger decoding.
				s := responseTestSession(t, 100, 100)
				body := &responseObservedBody{reader: strings.NewReader("must remain unread")}
				ctx, cancel := context.WithCancel(s.ctx)
				var cleanups atomic.Int64
				response, err := s.prepareResponse(ctx, &http.Response{
					StatusCode: test.status, Body: body,
					Header: http.Header{"Content-Encoding": {encoding}, "Content-Length": {"100"}},
				}, test.method, func() { cleanups.Add(1); cancel() })
				require.NoError(t, err)
				actual, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				require.Empty(t, actual)
				require.Equal(t, encoding, response.Header.Get("Content-Encoding"))
				require.Equal(t, "100", response.Header.Get("Content-Length"))

				// Releasing an empty body still closes its owned transport once.
				require.NoError(t, response.Body.Close())
				require.Empty(t, body.sizes)
				require.EqualValues(t, 1, body.closes.Load())
				require.EqualValues(t, 1, cleanups.Load())
			})
		}
	}
}

func TestResponseReadLimits(t *testing.T) {
	for _, test := range []struct {
		name            string
		decoded, stream int64
		want            string
		wantErr         error
	}{
		{"both exact", 6, 6, "abcdef", nil},
		{"decoded over", 3, 20, "abc", ErrResponseLimit},
		{"stream over", 20, 3, "abc", ErrStreamLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Reading the exact allowance succeeds; only the following one-byte
			// probe distinguishes clean EOF from data that exceeds a cap.
			s := responseTestSession(t, test.decoded, test.stream)
			body := &responseObservedBody{reader: strings.NewReader("abcdef")}
			response, cleanups := responseTestPrepare(t, s, &http.Response{StatusCode: 200, Body: body})
			actual, err := io.ReadAll(response.Body)
			require.ErrorIs(t, err, test.wantErr)
			require.Equal(t, test.want, string(actual))
			require.Equal(t, 1, body.sizes[len(body.sizes)-1])

			// Neither success nor failure rereads transport data after termination.
			reads := len(body.sizes)
			_, repeated := response.Body.Read(make([]byte, 100))
			if test.wantErr == nil {
				require.ErrorIs(t, repeated, io.EOF)
			} else {
				require.ErrorIs(t, repeated, test.wantErr)
			}
			require.Len(t, body.sizes, reads)
			require.EqualValues(t, 1, body.closes.Load())
			require.EqualValues(t, 1, cleanups.Load())
		})
	}

	// A very large caller buffer still produces only one bounded scratch read.
	s := responseTestSession(t, 100000, 100000)
	body := &responseObservedBody{reader: strings.NewReader(strings.Repeat("x", 100000))}
	response, _ := responseTestPrepare(t, s, &http.Response{StatusCode: 200, Body: body})
	n, err := response.Body.Read(make([]byte, 100000))
	require.NoError(t, err)
	require.Equal(t, responseReadSize, n)
	require.Equal(t, []int{responseReadSize}, body.sizes)
}

func TestResponseGzipFailuresAreSafe(t *testing.T) {
	encoded := responseGzip(t, "private decoded payload")
	badChecksum := bytes.Clone(encoded)
	badChecksum[len(badChecksum)-8] ^= 1
	for _, test := range []struct {
		name, encoding string
		body           []byte
		stream         int64
		wantErr        error
	}{
		{"invalid gzip", "gzip", []byte("private malformed gzip"), 100, ErrEncoding},
		{"truncated gzip", "gzip", encoded[:len(encoded)-4], 100, ErrEncoding},
		{"checksum", "gzip", badChecksum, 100, ErrEncoding},
		{"compressed limit", "gzip", encoded, int64(len(encoded) - 1), ErrStreamLimit},
		{"invalid x-gzip", "x-gzip", []byte("private malformed gzip"), 100, ErrEncoding},
		{"truncated x-gzip", "x-gzip", encoded[:len(encoded)-4], 100, ErrEncoding},
		{"x-gzip checksum", "x-gzip", badChecksum, 100, ErrEncoding},
		{"x-gzip compressed limit", "x-gzip", encoded, int64(len(encoded) - 1), ErrStreamLimit},
		{"unsupported", "br", []byte("private body"), 100, ErrEncoding},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Decoder and limit failures retain only host classifications, and an
			// unsupported encoding is rejected without reading the raw stream.
			s := responseTestSession(t, 100, test.stream)
			body := &responseObservedBody{reader: bytes.NewReader(test.body)}
			response, _ := responseTestPrepare(t, s, &http.Response{
				StatusCode: 200, Header: http.Header{"Content-Encoding": {test.encoding}}, Body: body,
			})
			_, err := io.ReadAll(response.Body)
			require.ErrorIs(t, err, test.wantErr)
			require.Equal(t, test.wantErr.Error(), err.Error())
			if test.encoding == "br" {
				require.Empty(t, body.sizes)
			}
			reads := len(body.sizes)
			_, err = response.Body.Read(make([]byte, 10))
			require.ErrorIs(t, err, test.wantErr)
			require.Len(t, body.sizes, reads)
		})
	}
}

// TestResponseIdentityReadReusesScratch isolates steady-state reads to detect
// repeated scratch allocation without measuring setup or terminal cleanup.
func TestResponseIdentityReadReusesScratch(t *testing.T) {
	// Keep the source and both byte budgets above the measured reads so this
	// isolates steady-state copying, excluding setup and terminal cleanup.
	const runs, readSize = 1000, 64
	payload := bytes.Repeat([]byte("x"), (runs+3)*readSize)
	s := responseTestSession(t, int64(len(payload)), int64(len(payload)))
	response, _ := responseTestPrepare(t, s, &http.Response{
		StatusCode: 200, Body: io.NopCloser(bytes.NewReader(payload)),
	})
	buffer := make([]byte, readSize)
	n, err := response.Body.Read(buffer)
	require.NoError(t, err)
	require.Equal(t, len(buffer), n)

	// After initializing the lazy scratch buffer, even small caller buffers
	// must not allocate a fresh 32 KiB scratch area on every read.
	var readErr error
	var shortRead bool
	allocations := testing.AllocsPerRun(runs, func() {
		n, err := response.Body.Read(buffer)
		if err != nil {
			readErr = err
		}
		shortRead = shortRead || n != len(buffer)
	})
	require.NoError(t, readErr)
	require.False(t, shortRead)
	require.Zero(t, allocations)
}

// responsePartialFailure supplies bytes and a private error in the same read.
type responsePartialFailure struct{}

// Read demonstrates that unusable partial bytes still consume invocation budget.
func (responsePartialFailure) Read(p []byte) (int, error) {
	return copy(p, "abc"), errors.New("private transport details")
}

func TestResponseSharedBudgetAndDiscardedBytes(t *testing.T) {
	// A partial failed read is charged even though its bytes cannot be exposed.
	s := responseTestSession(t, 5, 100)
	response, _ := responseTestPrepare(t, s, &http.Response{
		StatusCode: 200, Body: io.NopCloser(responsePartialFailure{}),
	})
	actual, err := io.ReadAll(response.Body)
	require.ErrorIs(t, err, ErrTransport)
	require.Empty(t, actual)
	require.EqualValues(t, 2, s.remainingResponse())

	// Another response may use exactly the remainder, and an empty response
	// still reaches EOF when no decoded allowance remains.
	for _, payload := range []string{"de", ""} {
		response, _ = responseTestPrepare(t, s, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload))})
		actual, err = io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, payload, string(actual))
	}
	response, _ = responseTestPrepare(t, s, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("f"))})
	actual, err = io.ReadAll(response.Body)
	require.ErrorIs(t, err, ErrResponseLimit)
	require.Empty(t, actual)
}

// responseBarrierReader holds every body's first read until all participants
// have captured the same aggregate-budget hint, forcing concurrent charging.
type responseBarrierReader struct {
	reader  io.Reader
	entered chan<- struct{}
	release <-chan struct{}
	once    sync.Once
}

// Read waits at the shared barrier once, then delegates subsequent reads.
func (r *responseBarrierReader) Read(p []byte) (int, error) {
	r.once.Do(func() { r.entered <- struct{}{}; <-r.release })
	return r.reader.Read(p)
}

func TestResponseConcurrentBudget(t *testing.T) {
	// Several bodies compete for one allowance; only atomically charged chunks
	// may reach their readers, and excess permanently exhausts the invocation.
	s := responseTestSession(t, 1000, 2000)
	entered, release := make(chan struct{}, 4), make(chan struct{})
	var responses []*Response
	for range 4 {
		response, _ := responseTestPrepare(t, s, &http.Response{
			StatusCode: 200, Body: io.NopCloser(&responseBarrierReader{
				reader: strings.NewReader(strings.Repeat("x", 700)), entered: entered, release: release,
			}),
		})
		responses = append(responses, response)
	}
	var wg sync.WaitGroup
	var accepted, rejected atomic.Int64
	for _, response := range responses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, err := io.ReadAll(response.Body)
			accepted.Add(int64(len(body)))
			if errors.Is(err, ErrResponseLimit) {
				rejected.Add(1)
			}
		}()
	}
	// Release only after all readers took their size hint; the scheduler cannot
	// turn this into four independent sequential budget checks.
	for range responses {
		select {
		case <-entered:
		case <-time.After(time.Second):
			close(release)
			wg.Wait()
			t.Fatal("concurrent reads did not reach the barrier")
		}
	}
	close(release)
	wg.Wait()
	require.LessOrEqual(t, accepted.Load(), int64(1000))
	require.Positive(t, rejected.Load())
	require.Zero(t, s.remainingResponse())
}

// responseBlockingBody models a transport read that is released by Close.
type responseBlockingBody struct {
	started chan struct{}
	closed  chan struct{}
	once    sync.Once
	closes  atomic.Int64
}

// Read waits until cancellation or caller Close releases the transport.
func (b *responseBlockingBody) Read([]byte) (int, error) {
	close(b.started)
	<-b.closed
	return 0, errors.New("private canceled transport")
}

// Close unblocks Read without accessing the response reader's mutex.
func (b *responseBlockingBody) Close() error {
	b.closes.Add(1)
	b.once.Do(func() { close(b.closed) })
	return nil
}

func TestResponseCloseAndCancellationUnblockRead(t *testing.T) {
	for _, cancelContext := range []bool{false, true} {
		t.Run(strconv.FormatBool(cancelContext), func(t *testing.T) {
			// Start a real blocked read before closing the body or its context.
			s := responseTestSession(t, 100, 100)
			body := &responseBlockingBody{started: make(chan struct{}), closed: make(chan struct{})}
			response, cleanups := responseTestPrepare(t, s, &http.Response{StatusCode: 200, Body: body})
			done := make(chan error, 1)
			go func() { _, err := response.Body.Read(make([]byte, 10)); done <- err }()
			select {
			case <-body.started:
			case <-time.After(time.Second):
				t.Fatal("read did not start")
			}
			want := ErrClosed
			if cancelContext {
				s.cancel(ErrTimeout)
				want = ErrTimeout
			} else {
				require.NoError(t, response.Body.Close())
			}

			// The closing path must not wait for the read mutex or leak cleanup.
			select {
			case err := <-done:
				require.ErrorIs(t, err, want)
			case <-time.After(time.Second):
				t.Fatal("close did not unblock read")
			}
			require.EqualValues(t, 1, body.closes.Load())
			require.EqualValues(t, 1, cleanups.Load())
		})
	}
}

func TestResponseValidationClosesOwnership(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		headers http.Header
		limit   int
		wantErr error
	}{
		{"status", 600, nil, 100, ErrInvalidResponse},
		{"header limit", 200, http.Header{"X-Test": {"value"}}, 14, ErrHeaderLimit},
		{"empty value width", 200, http.Header{"X": make([]string, 100)}, 100, ErrHeaderLimit},
		{"invalid name", 200, http.Header{"bad name": {"value"}}, 100, ErrInvalidResponse},
		{"invalid value", 200, http.Header{"X-Test": {"private\r\n"}}, 100, ErrInvalidResponse},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Preparation failure releases the body and request scope without reads.
			s := responseTestSession(t, 100, 100)
			s.limits.MaxHeaderBytes = test.limit
			body := &responseObservedBody{reader: strings.NewReader("private body")}
			var cleanups atomic.Int64
			response, err := s.prepareResponse(s.ctx, &http.Response{StatusCode: test.status, Header: test.headers, Body: body}, http.MethodGet, func() { cleanups.Add(1) })
			require.ErrorIs(t, err, test.wantErr)
			require.Nil(t, response)
			require.Empty(t, body.sizes)
			require.EqualValues(t, 1, body.closes.Load())
			require.EqualValues(t, 1, cleanups.Load())
		})
	}

	// The exact serialized line size fits and both header slices are detached.
	headers := http.Header{"x-test": {"value"}}
	copy, err := copyResponseHeaders(headers, 15)
	require.NoError(t, err)
	require.Equal(t, http.Header{"X-Test": {"value"}}, copy)
	copy["X-Test"][0] = "changed"
	require.Equal(t, "value", headers["x-test"][0])
	for _, raw := range []*http.Response{nil, {StatusCode: 200}} {
		s := responseTestSession(t, 100, 100)
		var cleanups atomic.Int64
		_, err := s.prepareResponse(s.ctx, raw, http.MethodGet, func() { cleanups.Add(1) })
		require.ErrorIs(t, err, ErrInvalidResponse)
		require.EqualValues(t, 1, cleanups.Load())
	}
}
