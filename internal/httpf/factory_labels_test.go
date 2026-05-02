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

// stubActor is a minimal Actor implementation for testing.
type stubActor struct {
	id        apid.ID
	namespace string
	labels    map[string]string
}

func (s *stubActor) GetId() apid.ID               { return s.id }
func (s *stubActor) GetNamespace() string         { return s.namespace }
func (s *stubActor) GetLabels() map[string]string { return s.labels }

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

func TestForActorAddsActorLabels(t *testing.T) {
	conn := &stubConnection{
		id:        apid.MustParse("cxn_test1234567890ab"),
		namespace: "root.foo",
		labels: map[string]string{
			"apxy/cxn/-/id": "cxn_test1234567890ab",
			"apxy/cxn/-/ns": "root.foo",
			"apxy/ns/tier":  "pro",
		},
	}
	actor := &stubActor{
		id:        apid.MustParse("act_test1234567890ab"),
		namespace: "root.bar",
		labels: map[string]string{
			"team":  "marketplace",
			"role":  "admin",
		},
	}

	f := newTestFactory().ForConnection(conn).ForActor(actor).(*clientFactory)

	// Actor's user labels are re-keyed under apxy/act/<k>.
	require.Equal(t, "marketplace", f.requestInfo.Labels["apxy/act/team"])
	require.Equal(t, "admin", f.requestInfo.Labels["apxy/act/role"])

	// Actor's self-implicit identifier labels are present.
	require.Equal(t, "act_test1234567890ab", f.requestInfo.Labels["apxy/act/-/id"])
	require.Equal(t, "root.bar", f.requestInfo.Labels["apxy/act/-/ns"])

	// Connection's apxy/ labels survive — actor doesn't trample them.
	require.Equal(t, "cxn_test1234567890ab", f.requestInfo.Labels["apxy/cxn/-/id"])
	require.Equal(t, "root.foo", f.requestInfo.Labels["apxy/cxn/-/ns"])
	require.Equal(t, "pro", f.requestInfo.Labels["apxy/ns/tier"])
}

func TestForActorDoesNotForwardActorApxyKeys(t *testing.T) {
	// Even if the actor's labels contain apxy/ entries (e.g. read fresh
	// from a stored actor row), only the user portion is re-keyed under
	// apxy/act/<k>. The actor's own apxy/ns/* would otherwise collide
	// with the connection's namespace context.
	actor := &stubActor{
		id:        apid.MustParse("act_test1234567890ab"),
		namespace: "root.bar",
		labels: map[string]string{
			"team":          "marketplace",
			"apxy/ns/tier":  "free",     // should NOT propagate
			"apxy/act/-/id": "ignored",  // self-implicit comes from id, not from labels
		},
	}

	f := newTestFactory().ForActor(actor).(*clientFactory)

	require.Equal(t, "marketplace", f.requestInfo.Labels["apxy/act/team"])
	require.Equal(t, "act_test1234567890ab", f.requestInfo.Labels["apxy/act/-/id"])
	_, hasNs := f.requestInfo.Labels["apxy/ns/tier"]
	require.False(t, hasNs, "actor's apxy/ns/* must not leak through to the request")
}

func TestForActorNilIsNoop(t *testing.T) {
	f := newTestFactory()
	require.Equal(t, f, f.ForActor(nil), "nil actor must be a no-op")

	// And typed-nil pointer wrapped in interface — callers should not
	// produce this (per actorFromContext helper) but be defensive: a
	// nil-id actor short-circuits.
	emptyActor := &stubActor{}
	out := f.ForActor(emptyActor).(*clientFactory)
	require.Empty(t, out.requestInfo.Labels)
}

func TestForActorAfterForLabelsApxyStillProtected(t *testing.T) {
	// Order: connection → actor → per-request labels.
	// Per-request labels with apxy/ are still filtered out by ForLabels
	// (defense-in-depth).
	conn := &stubConnection{
		id:        apid.MustParse("cxn_test1234567890ab"),
		namespace: "root.foo",
		labels: map[string]string{
			"apxy/cxn/-/id": "cxn_test1234567890ab",
		},
	}
	actor := &stubActor{
		id:        apid.MustParse("act_test1234567890ab"),
		namespace: "root.bar",
	}

	f := newTestFactory().
		ForConnection(conn).
		ForActor(actor).
		ForLabels(map[string]string{
			"purpose":       "ad-hoc",
			"apxy/act/team": "evil-override",
		}).(*clientFactory)

	require.Equal(t, "ad-hoc", f.requestInfo.Labels["purpose"])
	// apxy/act/team would have been set by actor (if user labels included it)
	// or absent (as here). Either way, per-request input cannot insert it.
	_, hasEvil := f.requestInfo.Labels["apxy/act/team"]
	require.False(t, hasEvil)
}
