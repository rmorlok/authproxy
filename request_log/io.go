package request_log

import "io"

/*
 * Utilities that allow us to manipulate readers for requests/responses so we can track the
 * size of the request/responses and or divert the data to storage.
 */

// trackingReader is an interface for tracking the number of bytes read from an io.Reader.
type trackingReader interface {
	BytesRead() int64
	Done() <-chan interface{}
}

/* // wrappedReadCloser is a ReadCloser that wraps an io.Reader and an io.Closer
type wrappedReadCloser struct {
	io.Reader
	closer     io.ReadCloser
	pipeWriter *io.PipeWriter
}

// Read reads from the underlying reader
func (w *wrappedReadCloser) Read(p []byte) (n int, err error) {
	return w.Reader.Read(p)
}

// Close closes the underlying closer
func (w *wrappedReadCloser) Close() error {
	w.pipeWriter.Close()
	return w.closer.Close()
}*/

// splitReadCloser combines an io.Reader with multip io.Closer that can be separate.
// The last closer will determine the return value for close. It also tracks the number of bytes read.
type splitReadCloser struct {
	reader    io.Reader
	closers   []io.Closer
	bytesRead int64
	done      chan interface{}
}

// Read delegates to the embedded reader
func (b *splitReadCloser) Read(p []byte) (n int, err error) {
	n, err = b.reader.Read(p)
	b.bytesRead += int64(n)
	return n, err
}

// Close delegates to the embedded closer
func (b *splitReadCloser) Close() (err error) {
	if len(b.closers) == 0 {
		return nil
	}

	for _, c := range b.closers {
		err = c.Close()
	}

	close(b.done)

	return err
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

// trackingReader wraps an io.Reader and tracks the number of bytes read from it.
type trackingReadCloser struct {
	io.ReadCloser
	bytesRead int64
	done      chan interface{}
}

// Read delegates to the embedded reader
func (b *trackingReadCloser) Read(p []byte) (n int, err error) {
	n, err = b.ReadCloser.Read(p)
	b.bytesRead += int64(n)
	return n, err
}

func (b *trackingReadCloser) Close() error {
	err := b.ReadCloser.Close()
	close(b.done)
	return err
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
