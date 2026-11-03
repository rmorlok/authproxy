package app_metrics

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// countingCloser is an io.Closer test double that records how many times Close was called and
// returns a fixed error from every call.
type countingCloser struct {
	calls int
	err   error
}

// Close records the call and returns the configured error.
func (c *countingCloser) Close() error {
	c.calls++
	return c.err
}

// countingReadCloser pairs a reader with a countingCloser so it can stand in for an HTTP body.
type countingReadCloser struct {
	io.Reader
	countingCloser
}

// requireDone asserts that the channel has been closed.
func requireDone(t *testing.T, done <-chan interface{}) {
	t.Helper()
	select {
	case <-done:
	default:
		require.Fail(t, "expected Done channel to be closed")
	}
}

// requireNotDone asserts that the channel has not been closed yet.
func requireNotDone(t *testing.T, done <-chan interface{}) {
	t.Helper()
	select {
	case <-done:
		require.Fail(t, "expected Done channel to still be open")
	default:
	}
}

func TestSplitReadCloser(t *testing.T) {
	t.Run("tracks bytes read", func(t *testing.T) {
		r := newSplitReadCloser(strings.NewReader("hello world"), &countingCloser{})
		data, err := io.ReadAll(r)
		require.NoError(t, err)
		require.Equal(t, "hello world", string(data))
		require.EqualValues(t, 11, r.BytesRead())
	})

	t.Run("close closes all closers once and signals done", func(t *testing.T) {
		c1 := &countingCloser{}
		c2 := &countingCloser{}
		r := newSplitReadCloser(strings.NewReader("x"), c1, c2)

		requireNotDone(t, r.Done())
		require.NoError(t, r.Close())
		requireDone(t, r.Done())
		require.Equal(t, 1, c1.calls)
		require.Equal(t, 1, c2.calls)
	})

	t.Run("double close does not panic or re-close", func(t *testing.T) {
		c1 := &countingCloser{}
		c2 := &countingCloser{}
		r := newSplitReadCloser(strings.NewReader("x"), c1, c2)

		require.NotPanics(t, func() {
			require.NoError(t, r.Close())
			require.NoError(t, r.Close())
			require.NoError(t, r.Close())
		})
		requireDone(t, r.Done())
		require.Equal(t, 1, c1.calls)
		require.Equal(t, 1, c2.calls)
	})

	t.Run("later closes return the first error", func(t *testing.T) {
		boom := errors.New("boom")
		c1 := &countingCloser{err: errors.New("ignored, not last")}
		c2 := &countingCloser{err: boom}
		r := newSplitReadCloser(strings.NewReader("x"), c1, c2)

		require.ErrorIs(t, r.Close(), boom)
		require.ErrorIs(t, r.Close(), boom)
		require.Equal(t, 1, c1.calls)
		require.Equal(t, 1, c2.calls)
		requireDone(t, r.Done())
	})

	t.Run("close with no closers signals done", func(t *testing.T) {
		r := newSplitReadCloser(strings.NewReader("x"))
		require.NoError(t, r.Close())
		require.NoError(t, r.Close())
		requireDone(t, r.Done())
	})
}

func TestTrackingReadCloser(t *testing.T) {
	t.Run("tracks bytes read", func(t *testing.T) {
		body := &countingReadCloser{Reader: strings.NewReader("hello")}
		r := newTrackingReadCloser(body)
		data, err := io.ReadAll(r)
		require.NoError(t, err)
		require.Equal(t, "hello", string(data))
		require.EqualValues(t, 5, r.BytesRead())
	})

	t.Run("double close does not panic or re-close", func(t *testing.T) {
		body := &countingReadCloser{Reader: strings.NewReader("x")}
		r := newTrackingReadCloser(body)

		requireNotDone(t, r.Done())
		require.NotPanics(t, func() {
			require.NoError(t, r.Close())
			require.NoError(t, r.Close())
		})
		requireDone(t, r.Done())
		require.Equal(t, 1, body.calls)
	})

	t.Run("later closes return the first error", func(t *testing.T) {
		boom := errors.New("boom")
		body := &countingReadCloser{Reader: strings.NewReader("x"), countingCloser: countingCloser{err: boom}}
		r := newTrackingReadCloser(body)

		require.ErrorIs(t, r.Close(), boom)
		require.ErrorIs(t, r.Close(), boom)
		require.Equal(t, 1, body.calls)
		requireDone(t, r.Done())
	})
}

// closeGatedBody allows Close to return before an outstanding Read finishes,
// matching transports that unblock reads during cancellation or early response.
type closeGatedBody struct {
	readStarted chan struct{}
	closed      chan struct{}
	finishRead  chan struct{}
	closeOnce   sync.Once
}

// Read waits for Close and a test gate before returning the last body bytes.
func (b *closeGatedBody) Read(p []byte) (int, error) {
	close(b.readStarted)
	<-b.closed
	<-b.finishRead
	return copy(p, "last bytes"), io.EOF
}

// Close unblocks the first stage of the read without waiting for it to return.
func (b *closeGatedBody) Close() error {
	b.closeOnce.Do(func() { close(b.closed) })
	return nil
}

// TestTrackingBodiesConcurrentClose verifies Done is a stable capture barrier
// even when the underlying Close returns before an in-flight tee has finished.
func TestTrackingBodiesConcurrentClose(t *testing.T) {
	for _, tee := range []bool{false, true} {
		name := "tracking"
		if tee {
			name = "tee capture"
		}
		t.Run(name, func(t *testing.T) {
			// Keep the final read alive after the underlying close has returned.
			body := &closeGatedBody{
				readStarted: make(chan struct{}), closed: make(chan struct{}), finishRead: make(chan struct{}),
			}
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(body.finishRead) }) }
			t.Cleanup(release)
			capture := &bytes.Buffer{}
			var reader interface {
				io.ReadCloser
				trackingReader
			} = newTrackingReadCloser(body)
			if tee {
				reader = newSplitReadCloser(io.TeeReader(body, capture), body)
			}
			readDone := make(chan error, 1)
			go func() {
				_, err := reader.Read(make([]byte, 64))
				readDone <- err
			}()
			<-body.readStarted
			closeDone := make(chan error, 1)
			go func() { closeDone <- reader.Close() }()
			<-body.closed

			// Closing must unblock the read without publishing Done too early.
			select {
			case <-reader.Done():
				t.Fatal("body marked complete while its read is still active")
			case <-time.After(20 * time.Millisecond):
			}
			require.EqualValues(t, 0, reader.BytesRead())
			release()
			require.ErrorIs(t, <-readDone, io.EOF)
			require.NoError(t, <-closeDone)

			// Once complete, both captured data and the final count are stable.
			requireDone(t, reader.Done())
			require.EqualValues(t, len("last bytes"), reader.BytesRead())
			if tee {
				require.Equal(t, "last bytes", capture.String())
			}
			_, err := reader.Read(make([]byte, 1))
			require.ErrorIs(t, err, io.ErrClosedPipe)
			require.NoError(t, reader.Close())
		})
	}
}
