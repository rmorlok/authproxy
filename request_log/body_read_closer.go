package request_log

import "io"

// bodyReadCloser combines an io.Reader with an io.Closer
type bodyReadCloser struct {
	reader io.Reader
	closer io.ReadCloser
}

// Read delegates to the embedded reader
func (b *bodyReadCloser) Read(p []byte) (n int, err error) {
	return b.reader.Read(p)
}

// Close delegates to the embedded closer
func (b *bodyReadCloser) Close() error {
	return b.closer.Close()
}
