package httpf

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/require"
)

// directTarget installs a fake target behind the real send marker. Factory
// middleware and coverage still execute, but no default transport is contacted.
type directTarget struct {
	transport http.RoundTripper // Scenario-owned fake transport; it owns request bodies.
}

// NewRoundTripper replaces only the final target while preserving send evidence.
func (f directTarget) NewRoundTripper(_ RequestInfo, _ http.RoundTripper) http.RoundTripper {
	return &sendHandoffTransport{transport: f.transport}
}

// directAttribution records the immutable snapshot given to real middleware.
type directAttribution struct {
	info RequestInfo // Written at client construction, then read by the test.
}

// NewRoundTripper observes construction without changing request behavior.
func (f *directAttribution) NewRoundTripper(info RequestInfo, parent http.RoundTripper) http.RoundTripper {
	f.info = info
	return parent
}

// directResponse supplies an ordinary streaming response without network I/O.
func directResponse(req *http.Request, status int) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: http.NoBody, Request: req}
}

// TestDirectClientCapturesAttribution preserves caller-selected identity and
// instrumentation without inheriting connection credentials or recovery.
func TestDirectClientCapturesAttribution(t *testing.T) {
	fx := newTelemetryFixture(t)
	telemetry, err := NewTelemetryFactory(fx.providers, &sconfig.Telemetry{
		Enabled: enabledPtr(true),
		Proxy: &sconfig.TelemetryProxy{
			SpanAttributeLabels: []string{"tenant"}, MetricDimensionLabels: []string{"tenant"},
		},
	})
	require.NoError(t, err)
	var calls atomic.Int32
	var received *http.Request
	target := directTarget{transport: sendEvidenceRoundTripper(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		received = req
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return directResponse(req, http.StatusUnauthorized), nil
	})}
	attribution := &directAttribution{}
	propagation := true
	inherited := RequestInfo{
		Namespace: "root.connection", Type: RequestTypeProxy,
		ConnectionId: apid.New(apid.PrefixConnection), ConnectorId: apid.New(apid.PrefixConnector), ConnectorGeneration: 7,
		Labels:       map[string]string{"inherited": "connection", "apxy/act/-/id": "old-actor"},
		RateLimiting: &connectors.RateLimiting{}, PropagateTraceContext: &propagation,
	}
	factory := CreateFactory(nil, nil, attribution, nil, target, telemetry).ForRequestInfo(inherited)
	actor := &stubActor{id: apid.New(apid.PrefixActor), name: "Captured actor", namespace: "root.actor", labels: map[string]string{"team": "blue"}}
	actorID := actor.id.String()
	labels := map[string]string{"tenant": "acme", "apxy/act/-/id": "forged"}
	client, err := NewDirectClient(factory, DirectClientOptions{
		Namespace: "root.jobs", RequestType: RequestTypeGlobal, Actor: actor, Labels: labels,
	})
	require.NoError(t, err)
	require.Zero(t, calls.Load())

	// Later edits to caller-owned metadata cannot change the built client snapshot.
	labels["tenant"], actor.labels["team"], actor.name = "changed", "red", "Changed actor"
	actor.id = apid.New(apid.PrefixActor)
	request := newRequest(t, http.MethodPost, "https://provider.example.test/item", "upload")
	request.Host = "tenant.example.test"
	request.Header.Set("Authorization", "Bearer authored")
	request.Header.Set("X-Authored", "kept")
	response, state, err := client.Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.Equal(t, aphttp.SendStateSent, state)
	require.Equal(t, int32(1), calls.Load(), "a direct 401 must not trigger connection authentication recovery")
	require.Equal(t, "Bearer authored", received.Header.Get("Authorization"))
	require.Equal(t, "kept", received.Header.Get("X-Authored"))
	require.Equal(t, "tenant.example.test", received.Host)

	// Direct attribution replaces every inherited connection/policy field while
	// retaining copied actor/user labels and leaving the source factory intact.
	require.Equal(t, "root.jobs", attribution.info.Namespace)
	require.Equal(t, RequestTypeGlobal, attribution.info.Type)
	require.True(t, attribution.info.ConnectionId.IsNil())
	require.True(t, attribution.info.ConnectorId.IsNil())
	require.Zero(t, attribution.info.ConnectorGeneration)
	require.Nil(t, attribution.info.RateLimiting)
	require.Nil(t, attribution.info.PropagateTraceContext)
	require.Equal(t, "acme", attribution.info.Labels["tenant"])
	require.Equal(t, "blue", attribution.info.Labels["apxy/act/team"])
	require.Equal(t, actorID, attribution.info.Labels["apxy/act/-/id"])
	require.Equal(t, "Captured actor", attribution.info.Labels["apxy/act/-/name"])
	require.NotContains(t, attribution.info.Labels, "inherited")
	require.Equal(t, inherited, factory.(*clientFactory).requestInfo)
	require.Equal(t, map[string]string{"inherited": "connection", "apxy/act/-/id": "old-actor"}, factory.(*clientFactory).requestInfo.Labels)

	// The existing telemetry middleware receives this same resolved snapshot.
	spans := fx.spans.Ended()
	require.Len(t, spans, 1)
	attributes := attrMap(spans[0].Attributes())
	require.Equal(t, "root.jobs", attributes["authproxy.namespace"])
	require.Equal(t, "global", attributes["authproxy.request.type"])
	require.Equal(t, "acme", attributes["tenant"])
	require.NotContains(t, attributes, "authproxy.connection_id")
	require.NotContains(t, attributes, "authproxy.connector_id")
	metrics := fx.readMetrics(t)
	requireDimEqual(t, findMetric(t, metrics, "authproxy.client.request.duration"), "tenant", "acme")

	// An explicit nil actor also clears prior actor attribution; no inherited
	// factory labels are carried into unattributed background traffic.
	_, err = NewDirectClient(factory, DirectClientOptions{Namespace: "root.jobs", RequestType: RequestTypeGlobal})
	require.NoError(t, err)
	require.Empty(t, attribution.info.Labels)
}

// TestDirectClientSendEvidence distinguishes local rejection, target response,
// redirect history, and uncertain transport failure without exposing diagnostics.
func TestDirectClientSendEvidence(t *testing.T) {
	for _, test := range []struct {
		path  string
		state aphttp.SendState
		calls int32
		err   error
	}{
		{"/blocked", aphttp.SendStateNotSent, 0, nil},
		{"/ok", aphttp.SendStateSent, 1, nil},
		{"/redirect", aphttp.SendStateSent, 1, nil},
		{"/failure", aphttp.SendStateMayHaveSent, 1, aphttp.ErrTransport},
	} {
		t.Run(test.path, func(t *testing.T) {
			var calls atomic.Int32
			target := directTarget{transport: sendEvidenceRoundTripper(func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				if req.Body != nil {
					_ = req.Body.Close()
				}
				if req.URL.Path == "/failure" {
					return nil, errors.New("private transport credentials")
				}
				response := directResponse(req, http.StatusOK)
				if req.URL.Path == "/redirect" {
					response.StatusCode = http.StatusFound
					response.Header.Set("Location", "/blocked")
				}
				return response, nil
			})}
			factory := CreateFactory(nil, nil, sendEvidenceMiddleware{rejectPath: "/blocked"}, nil, target)
			client, err := NewDirectClient(factory, DirectClientOptions{Namespace: "root.jobs", RequestType: RequestTypeGlobal})
			require.NoError(t, err)

			// A redirected request can be rejected locally after a confirmed send;
			// transport errors retain uncertainty with only a fixed public error.
			response, state, err := client.Do(newRequest(t, http.MethodGet, "https://example.test"+test.path, ""))
			require.ErrorIs(t, err, test.err)
			require.Equal(t, test.state, state)
			require.Equal(t, test.calls, calls.Load())
			if response != nil {
				require.NoError(t, response.Body.Close())
			}
		})
	}
}

// directNilFactory models a trusted factory that cannot supply a client.
type directNilFactory struct{ F }

// ForRequestInfo retains the nil-client fixture through attribution setup.
func (f directNilFactory) ForRequestInfo(RequestInfo) F { return f }

// ForActor retains the nil-client fixture through actor setup.
func (f directNilFactory) ForActor(Actor) F { return f }

// ForLabels retains the nil-client fixture through label setup.
func (f directNilFactory) ForLabels(map[string]string) F { return f }

// NewHTTPClient deliberately refuses to construct a transport capability.
func (directNilFactory) NewHTTPClient() *http.Client { return nil }

// directUpload records ownership release when local validation rejects a call.
type directUpload struct {
	io.Reader
	closed bool // Used synchronously by the validation test.
}

// Close marks the upload as released without touching the underlying reader.
func (b *directUpload) Close() error { b.closed = true; return nil }

// TestDirectClientInvalidInput rejects missing configuration and releases owned
// uploads even for nil or uninitialized clients, without a default-client fallback.
func TestDirectClientInvalidInput(t *testing.T) {
	for _, test := range []struct {
		name    string
		factory F
		options DirectClientOptions
	}{
		{"factory", nil, DirectClientOptions{Namespace: "root", RequestType: RequestTypeGlobal}},
		{"namespace", newTestFactory(), DirectClientOptions{RequestType: RequestTypeGlobal}},
		{"type", newTestFactory(), DirectClientOptions{Namespace: "root", RequestType: "private-invalid"}},
		{"client", directNilFactory{}, DirectClientOptions{Namespace: "root", RequestType: RequestTypeGlobal}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Invalid attribution never reaches client dispatch or leaks its values.
			client, err := NewDirectClient(test.factory, test.options)
			require.ErrorIs(t, err, aphttp.ErrInvalidInput)
			require.NotContains(t, err.Error(), "private-invalid")
			require.Nil(t, client)
		})
	}
	for _, client := range []*DirectClient{nil, {}} {
		upload := &directUpload{Reader: strings.NewReader("upload")}
		request := newRequest(t, http.MethodPost, "https://example.test", "")
		request.Body = upload
		response, state, err := client.Do(request)
		require.ErrorIs(t, err, aphttp.ErrInvalidInput)
		require.Equal(t, aphttp.SendStateNotSent, state)
		require.Nil(t, response)
		require.True(t, upload.closed)
	}

	// A constructed client also rejects absent requests without dispatch.
	client, err := NewDirectClient(newTestFactory(), DirectClientOptions{Namespace: "root", RequestType: RequestTypeGlobal})
	require.NoError(t, err)
	response, state, err := client.Do(nil)
	require.ErrorIs(t, err, aphttp.ErrInvalidInput)
	require.Equal(t, aphttp.SendStateNotSent, state)
	require.Nil(t, response)
}

// TestDirectClientPreservesResponseOnRedirectError retains stock HTTP ownership
// when the client returns both a response and a sanitized redirect-policy error.
func TestDirectClientPreservesResponseOnRedirectError(t *testing.T) {
	target := directTarget{transport: sendEvidenceRoundTripper(func(req *http.Request) (*http.Response, error) {
		response := directResponse(req, http.StatusFound)
		response.Header.Set("Location", "/next")
		return response, nil
	})}
	client, err := NewDirectClient(CreateFactory(nil, nil, target, nil), DirectClientOptions{Namespace: "root", RequestType: RequestTypeGlobal})
	require.NoError(t, err)
	client.client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("private redirect details") }

	// A target response remains evidence even when later redirect handling fails.
	response, state, err := client.Do(newRequest(t, http.MethodGet, "https://example.test", ""))
	require.ErrorIs(t, err, aphttp.ErrTransport)
	require.Equal(t, aphttp.SendStateSent, state)
	require.NotNil(t, response)
	require.Equal(t, http.StatusFound, response.StatusCode)
	require.NoError(t, response.Body.Close())
}
