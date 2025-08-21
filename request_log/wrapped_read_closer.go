package request_log

import "io"

// wrappedReadCloser is a ReadCloser that wraps an io.Reader and an io.Closer
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
}
