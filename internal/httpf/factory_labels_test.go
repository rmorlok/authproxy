package httpf

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/require"
)

// stubConnection is a minimal Connection implementation for testing the
// label-snapshot semantics of the request-info factory.
type stubConnection struct {
	id        apid.ID
	namespace string
	cvID      apid.ID
	cvVersion uint64
	labels    map[string]string
}

func (s *stubConnection) GetId() apid.ID                  { return s.id }
func (s *stubConnection) GetNamespace() string            { return s.namespace }
func (s *stubConnection) GetConnectorId() apid.ID         { return s.cvID }
func (s *stubConnection) GetConnectorVersion() uint64     { return s.cvVersion }
func (s *stubConnection) GetLabels() map[string]string    { return s.labels }

func newTestFactory() *clientFactory {
	return &clientFactory{
		requestInfo: RequestInfo{},
	}
}

func TestForConnectionCopiesAllLabels(t *testing.T) {
	conn := &stubConnection{
		id:        apid.MustParse("cxn_test1234567890ab"),
		namespace: "root.foo",
		cvID:      apid.MustParse("cxr_test1234567890ab"),
		cvVersion: 1,
		labels: map[string]string{
			"team":             "platform",
			"apxy/cxn/-/id":    "cxn_test1234567890ab",
			"apxy/cxn/-/ns":    "root.foo",
			"apxy/cxr/type":    "google_drive",
			"apxy/ns/tier":     "pro",
		},
	}

	f := newTestFactory().ForConnection(conn).(*clientFactory)
	require.Equal(t, "platform", f.requestInfo.Labels["team"])
	require.Equal(t, "cxn_test1234567890ab", f.requestInfo.Labels["apxy/cxn/-/id"])
	require.Equal(t, "google_drive", f.requestInfo.Labels["apxy/cxr/type"])
	require.Equal(t, "pro", f.requestInfo.Labels["apxy/ns/tier"])
}

func TestForLabelsDoesNotOverrideApxyKeys(t *testing.T) {
	conn := &stubConnection{
		id:        apid.MustParse("cxn_test1234567890ab"),
		namespace: "root.foo",
		labels: map[string]string{
			"apxy/cxn/-/id": "cxn_test1234567890ab",
			"apxy/cxr/type": "google_drive",
		},
	}

	// Per-request input attempts to overwrite an apxy/ key (defense-in-depth:
	// ProxyRequest.Validate normally rejects this; the factory MUST NOT
	// honor it even if a caller bypasses validation).
	perRequest := map[string]string{
		"purpose":       "some_feature",
		"apxy/cxr/type": "evil-override",
	}

	f := newTestFactory().ForConnection(conn).ForLabels(perRequest).(*clientFactory)

	require.Equal(t, "some_feature", f.requestInfo.Labels["purpose"], "user labels merge in")
	require.Equal(t, "google_drive", f.requestInfo.Labels["apxy/cxr/type"], "apxy/ keys are NOT overridable from per-request input")
}

func TestForLabelsAddsUserLabelsAlongsideApxy(t *testing.T) {
	conn := &stubConnection{
		id:        apid.MustParse("cxn_test1234567890ab"),
		namespace: "root.foo",
		labels: map[string]string{
			"apxy/cxn/-/id": "cxn_test1234567890ab",
			"team":          "platform",
		},
	}

	f := newTestFactory().ForConnection(conn).ForLabels(map[string]string{
		"purpose": "ad-hoc",
		"env":     "prod",
	}).(*clientFactory)

	// Connection labels survive.
	require.Equal(t, "platform", f.requestInfo.Labels["team"])
	require.Equal(t, "cxn_test1234567890ab", f.requestInfo.Labels["apxy/cxn/-/id"])

	// Per-request labels added.
	require.Equal(t, "ad-hoc", f.requestInfo.Labels["purpose"])
	require.Equal(t, "prod", f.requestInfo.Labels["env"])
}

func TestForLabelsUserOverridesConnectionUserLabel(t *testing.T) {
	// User-portion overrides ARE allowed — only apxy/ keys are protected.
	conn := &stubConnection{
		id:        apid.MustParse("cxn_test1234567890ab"),
		namespace: "root.foo",
		labels: map[string]string{
			"team": "platform",
		},
	}

	f := newTestFactory().ForConnection(conn).ForLabels(map[string]string{
		"team": "marketplace",
	}).(*clientFactory)

	require.Equal(t, "marketplace", f.requestInfo.Labels["team"])
}
