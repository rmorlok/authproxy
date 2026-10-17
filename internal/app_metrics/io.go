package app_metrics

import (
	"io"
	"sync"
)

/*
 * Utilities that allow us to manipulate readers for requests/responses so we can track the
 * size of the request/responses and or divert the data to storage.
 */

// trackingReader is an interface for tracking the number of bytes read from an io.Reader.
type trackingReader interface {
	BytesRead() int64
	Done() <-chan interface{}
}

// noOpTrackingReader is a no-op implementation of trackingReader. Used when there isn't a response body to track.
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

// splitReadCloser combines an io.Reader with multiple io.Closer that can be separate.
// The last closer will determine the return value for close. It also tracks the number of bytes read.
//
// Close is idempotent: the closers run and the done channel closes exactly once, no matter how many
// times Close is called. Some HTTP client libraries (e.g. gentleman's Response.Bytes() followed by
// Response.JSON()) close a response body more than once, and a "close of closed channel" panic in a
// request path is far worse than the undefined behavior io.Closer permits after the first Close.
type splitReadCloser struct {
	reader    io.Reader
	closers   []io.Closer
	bytesRead int64
	done      chan interface{}

	// closeOnce guards the closers and the done channel so repeated Close calls are safe.
	closeOnce sync.Once
	// closeErr is the result of the first Close, returned again by any later Close calls.
	closeErr error
}

func newSplitReadCloser(r io.Reader, closers ...io.Closer) *splitReadCloser {
	return &splitReadCloser{
		reader:  r,
		closers: closers,
		done:    make(chan interface{}),
	}
}

// Read delegates to the embedded reader
func (b *splitReadCloser) Read(p []byte) (n int, err error) {
	n, err = b.reader.Read(p)
	b.bytesRead += int64(n)
	return n, err
}

// Close closes every underlying closer in order and then signals Done. Every closer is closed even
// if an earlier one fails; the error from the last closer is returned. Only the first call does any
// work; subsequent calls return the same error as the first without touching the closers again.
func (b *splitReadCloser) Close() error {
	b.closeOnce.Do(func() {
		for _, c := range b.closers {
			b.closeErr = c.Close()
		}

		close(b.done)
	})

	return b.closeErr
}

func (b *splitReadCloser) Done() <-chan interface{} {
	return b.done
}

// BytesRead returns the number of bytes read from the reader
func (b *splitReadCloser) BytesRead() int64 {
	return b.bytesRead
}

var _ trackingReader = &splitReadCloser{}
var _ io.ReadCloser = &splitReadCloser{}

// trackingReadCloser wraps an io.ReadCloser and tracks the number of bytes read from it. Done fires
// once the body is closed.
//
// Close is idempotent for the same reason as splitReadCloser: callers that close a body twice must
// not panic on the done channel.
type trackingReadCloser struct {
	io.ReadCloser
	bytesRead int64
	done      chan interface{}

	// closeOnce guards the underlying Close and the done channel so repeated Close calls are safe.
	closeOnce sync.Once
	// closeErr is the result of the first Close, returned again by any later Close calls.
	closeErr error
}

func newTrackingReadCloser(r io.ReadCloser) *trackingReadCloser {
	return &trackingReadCloser{
		ReadCloser: r,
		done:       make(chan interface{}),
	}
}

// Read delegates to the embedded reader
func (b *trackingReadCloser) Read(p []byte) (n int, err error) {
	n, err = b.ReadCloser.Read(p)
	b.bytesRead += int64(n)
	return n, err
}

// Close closes the wrapped reader and signals Done. Only the first call does any work; subsequent
// calls return the same error as the first without closing the wrapped reader again.
func (b *trackingReadCloser) Close() error {
	b.closeOnce.Do(func() {
		b.closeErr = b.ReadCloser.Close()
		close(b.done)
	})

	return b.closeErr
}

// BytesRead returns the number of bytes read from the reader
func (b *trackingReadCloser) BytesRead() int64 {
	return b.bytesRead
}

func (b *trackingReadCloser) Done() <-chan interface{} {
	return b.done
}

var _ trackingReader = &trackingReadCloser{}
var _ io.ReadCloser = &trackingReadCloser{}
