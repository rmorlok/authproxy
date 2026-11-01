package app_metrics

import (
	"io"
	"sync"
	"sync/atomic"
)

/*
 * Utilities that allow us to manipulate readers for requests/responses so we can
 * track the size of the request/responses and or divert the data to storage.
 */

// trackingReader reports a concurrency-safe byte count. Done closes only after
// the body is closed and all reads have returned, so captured bytes are stable.
type trackingReader interface {
	BytesRead() int64
	Done() <-chan interface{}
}

// noOpTrackingReader is a no-op implementation of trackingReader. Used when
// there isn't a response body to track.
type noOpTrackingReader struct{}

func (n *noOpTrackingReader) BytesRead() int64 {
	return 0
}

func (n *noOpTrackingReader) Done() <-chan interface{} {
	// Done immediately
	c := make(chan interface{})
	close(c)
	return c
}

var _ trackingReader = &noOpTrackingReader{}

// splitReadCloser combines an io.Reader with multiple io.Closer that can be
// separate. The last closer will determine the return value for close. It also
// tracks the number of bytes read.
//
// Close is idempotent: the closers run and the done channel closes exactly once,
// no matter how many times Close is called. Some HTTP client libraries (e.g.
// gentleman's Response.Bytes() followed by Response.JSON()) close a response body
// more than once, and a "close of closed channel" panic in a request path is far
// worse than the undefined behavior io.Closer permits after the first Close.
type splitReadCloser struct {
	reader    io.Reader
	closers   []io.Closer
	bytesRead atomic.Int64
	done      chan interface{}
	// readMu serializes reads and provides a barrier after the underlying
	// closers unblock an outstanding read. Close must not acquire it first.
	readMu sync.Mutex
	closed atomic.Bool

	// closeOnce guards the closers and the done channel so repeated Close calls are safe.
	closeOnce sync.Once
	// closeErr is the result of the first Close, returned again by any later Close calls.
	closeErr error
}

// newSplitReadCloser tracks a reader and closes the supplied resources in order.
func newSplitReadCloser(r io.Reader, closers ...io.Closer) *splitReadCloser {
	return &splitReadCloser{
		reader:  r,
		closers: closers,
		done:    make(chan interface{}),
	}
}

// Read accounts for every byte, including a final read unblocked by Close.
// Reads starting after Close cannot append to a completed capture buffer.
func (b *splitReadCloser) Read(p []byte) (n int, err error) {
	b.readMu.Lock()
	defer b.readMu.Unlock()
	if b.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	n, err = b.reader.Read(p)
	b.bytesRead.Add(int64(n))
	return n, err
}

// Close closes every underlying closer in order and then signals Done. Every
// closer is closed even if an earlier one fails; the error from the last closer
// is returned. Only the first call does any work; subsequent calls return the
// same error as the first without touching the closers again. Underlying Close
// runs before waiting for in-flight reads, because it may be what releases a
// blocked Read or a tee writing into a response-capture pipe.
func (b *splitReadCloser) Close() error {
	b.closeOnce.Do(func() {
		b.closed.Store(true)
		for _, c := range b.closers {
			b.closeErr = c.Close()
		}

		b.readMu.Lock()
		close(b.done)
		b.readMu.Unlock()
	})

	return b.closeErr
}

// Done signals that no reader can still mutate the byte count or tee buffer.
func (b *splitReadCloser) Done() <-chan interface{} {
	return b.done
}

// BytesRead returns the number of bytes read from the reader
func (b *splitReadCloser) BytesRead() int64 {
	return b.bytesRead.Load()
}

var _ trackingReader = &splitReadCloser{}
var _ io.ReadCloser = &splitReadCloser{}

// trackingReadCloser uses the same read/close lifecycle without a separate tee.
// Done fires after Close and any in-flight Read have both finished.
type trackingReadCloser struct {
	*splitReadCloser
}

// newTrackingReadCloser tracks a body while preserving idempotent Close.
func newTrackingReadCloser(r io.ReadCloser) *trackingReadCloser {
	return &trackingReadCloser{
		splitReadCloser: newSplitReadCloser(r, r),
	}
}

var _ trackingReader = &trackingReadCloser{}
var _ io.ReadCloser = &trackingReadCloser{}
