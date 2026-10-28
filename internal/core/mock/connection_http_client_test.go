package mock

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/stretchr/testify/require"
)

// closeRecorder reports whether the mock released a request body.
type closeRecorder struct {
	io.Reader
	closed bool
}

// Close records the release.
func (c *closeRecorder) Close() error { c.closed = true; return nil }

// TestConnectionHTTPClientMock validates options like the real connection,
// records them for assertions, and fails clearly when no client is configured.
func TestConnectionHTTPClientMock(t *testing.T) {
	conn := &Connection{}

	// Missing actor choices are rejected exactly as the real implementation does.
	_, err := conn.HTTPClient(iface.HTTPClientOptions{})
	require.ErrorIs(t, err, iface.ErrHTTPClientActorRequired)
	require.Nil(t, conn.HTTPClientOptions)

	// Valid options are normalized and recorded; an unconfigured client errors
	// instead of panicking on a nil interface, and still closes the body.
	client, err := conn.HTTPClient(iface.HTTPClientOptions{Actor: iface.WithoutActor()})
	require.NoError(t, err)
	require.Equal(t, httpf.RequestTypeProxy, conn.HTTPClientOptions.RequestType)
	require.True(t, conn.HTTPClientOptions.Actor.IsWithoutActor())
	body := &closeRecorder{Reader: strings.NewReader("payload")}
	request, err := http.NewRequest(http.MethodPost, "https://example.test", body)
	require.NoError(t, err)
	response, err := client.Do(request)
	require.ErrorIs(t, err, ErrMockHTTPClientNotConfigured)
	require.Nil(t, response)
	require.True(t, body.closed)

	// A configured scenario client is returned as-is.
	configured := unconfiguredHTTPClient{}
	conn.HTTPClientValue = configured
	client, err = conn.HTTPClient(iface.HTTPClientOptions{Actor: iface.WithoutActor()})
	require.NoError(t, err)
	require.Equal(t, configured, client)
}
