package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/require"
)

// TestConnectionProxyInitializationSendEvidence checks that each proxy entry
// point reports local initialization failures without weakening evidence from
// an earlier target attempt or changing ownership of an unsent streaming body.
func TestConnectionProxyInitializationSendEvidence(t *testing.T) {
	paths := []struct {
		name string
		call func(*testing.T, *connection, context.Context) error
	}{
		{
			name: "buffered",
			call: func(t *testing.T, conn *connection, ctx context.Context) error {
				response, err := conn.ProxyRequest(ctx, httpf.RequestTypeProxy, &iface.ProxyRequest{})
				require.Nil(t, response)
				return err
			},
		},
		{
			name: "raw",
			call: func(t *testing.T, conn *connection, ctx context.Context) error {
				request, body := connectionProxySendEvidenceRequest(t, ctx)
				writer := httptest.NewRecorder()
				writer.Code = 0
				err := conn.ProxyRequestRaw(ctx, httpf.RequestTypeProxy, request, writer)

				// Failure releases the upload once without consuming it or
				// committing a response that would prevent the route's error JSON.
				require.Zero(t, body.reads.Load())
				require.EqualValues(t, 1, body.closes.Load())
				require.Zero(t, writer.Code)
				require.Empty(t, writer.Header())
				require.Empty(t, writer.Body.String())
				require.False(t, writer.Flushed)
				return err
			},
		},
		{
			name: "stream",
			call: func(t *testing.T, conn *connection, ctx context.Context) error {
				request, body := connectionProxySendEvidenceRequest(t, ctx)
				response, err := conn.ProxyRequestStream(ctx, httpf.RequestTypeProxy, request)

				// No response escapes initialization, and only body ownership
				// transfers: the unsent upload is closed without being read.
				require.Nil(t, response)
				require.Zero(t, body.reads.Load())
				require.EqualValues(t, 1, body.closes.Load())
				return err
			},
		},
	}

	for _, path := range paths {
		t.Run(path.name, func(t *testing.T) {
			for _, prior := range []struct {
				name  string
				state httpf.SendState
				want  httpf.SendState
			}{
				{name: "unreported", want: httpf.SendStateNotSent},
				{name: "possible earlier send", state: httpf.SendStateMayHaveSent, want: httpf.SendStateMayHaveSent},
				{name: "confirmed earlier send", state: httpf.SendStateSent, want: httpf.SendStateSent},
			} {
				t.Run(prior.name, func(t *testing.T) {
					// A cached definition with no auth method fails proxy resolution
					// locally, without requiring database or encryption fixtures.
					conn := &connection{
						s:         &service{},
						connector: &Connector{def: &cschema.ConnectorDefinition{}},
					}
					for _, attempt := range []string{"initial", "cached"} {
						t.Run(attempt, func(t *testing.T) {
							// Each inbound call starts its own observation; stronger
							// evidence represents an earlier attempt in that call.
							ctx, observation := httpf.ObserveSends(context.Background())
							if prior.state != "" {
								httpf.ReportSendState(ctx, prior.state)
							}

							err := path.call(t, conn, ctx)
							require.ErrorIs(t, err, ErrProxyNotImplemented)
							require.Equal(t, prior.want, observation.State())
							require.Equal(t, prior.want != httpf.SendStateNotSent, observation.MayHaveSent())
						})

						// Drop the initializer dependency after the first failure:
						// later calls must use the cached error without resolving again.
						conn.connector = nil
					}
				})
			}
		})
	}
}

// connectionProxySendEvidenceRequest creates an upload whose read and close
// counts expose accidental buffering or duplicated cleanup on local failures.
func connectionProxySendEvidenceRequest(t *testing.T, ctx context.Context) (*iface.RawProxyRequest, *connectionHTTPClientTestBody) {
	t.Helper()
	body := &connectionHTTPClientTestBody{reader: strings.NewReader("unsent upload")}
	outbound, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://provider.example/items", body)
	require.NoError(t, err)
	return &iface.RawProxyRequest{Outbound: outbound}, body
}
