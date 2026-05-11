package httpf

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// recordingFactory wraps every constructed RoundTripper so we can record
// the order in which each layer runs against an actual request. Used by
// the middleware-ordering test below.
type recordingFactory struct {
	name      string
	recorder  *orderRecorder
	skipBuild bool // when true, NewRoundTripper returns nil — exercises the if-nil branch
}

type orderRecorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *orderRecorder) record(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, name)
}

func (r *orderRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.calls))
	copy(out, r.calls)
	return out
}

func (rf *recordingFactory) NewRoundTripper(_ RequestInfo, transport http.RoundTripper) http.RoundTripper {
	if rf.skipBuild {
		return nil
	}
	return &recordingRT{name: rf.name, recorder: rf.recorder, transport: transport}
}

type recordingRT struct {
	name      string
	recorder  *orderRecorder
	transport http.RoundTripper
}

func (rt *recordingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.recorder.record(rt.name)
	return rt.transport.RoundTrip(req)
}

// TestCreateFactory_MiddlewareOrder pins the contract the chain depends
// on: requestLog is the outermost middleware (called first on the way in,
// last to see the response on the way back) and additional middlewares
// run in slice order from outermost to innermost — i.e., the *first* slice
// entry is the closest to the upstream transport.
//
// This test exists because of the latent bug closed in #223: prior to
// that PR the loop placed requestLog at the *innermost* position despite
// the comment claiming otherwise. The bug went undetected because no
// test pinned the execution order. This guards against the same kind of
// regression.
func TestCreateFactory_MiddlewareOrder(t *testing.T) {
	rec := &orderRecorder{}

	// Mock upstream — records itself like everything else.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record("upstream")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	requestLog := &recordingFactory{name: "requestLog", recorder: rec}
	mwA := &recordingFactory{name: "A", recorder: rec}
	mwB := &recordingFactory{name: "B", recorder: rec}
	mwC := &recordingFactory{name: "C", recorder: rec}

	// nil cfg / nil redis are fine — CreateFactory doesn't dereference
	// them for this code path.
	f := CreateFactory(nil, nil, requestLog, nil, mwA, mwB, mwC).(*clientFactory)

	// Build a request via gentleman so we hit the real chain.
	client := f.New()
	req := client.Request().URL(upstream.URL)
	_, err := req.Send()
	require.NoError(t, err)

	// Execution order must be: requestLog runs first (outermost), then
	// each additional middleware in slice order (so C runs after B which
	// runs after A — because additional middlewares are added before
	// requestLog and the *last* slice entry becomes the *outermost* in
	// the wrapper chain… meaning the FIRST slice entry runs LAST among
	// the wrappers before hitting upstream). With requestLog appended
	// to the end of the slice inside CreateFactory, it becomes outermost.
	//
	// Slice (after CreateFactory's internal append): [A, B, C, requestLog]
	// Wrapping (innermost → outermost): requestLog(C(B(A(upstream))))
	// Execution: requestLog → C → B → A → upstream
	require.Equal(t, []string{"requestLog", "C", "B", "A", "upstream"}, rec.snapshot())
}

// TestCreateFactory_NilMiddlewareSkipped guards the existing nil-filter
// in CreateFactory so a middleware that opts out for a given request
// info (e.g., the rate-limit factory returning nil for non-proxy traffic)
// doesn't blow up the chain.
func TestCreateFactory_NilMiddlewareSkipped(t *testing.T) {
	rec := &orderRecorder{}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record("upstream")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	requestLog := &recordingFactory{name: "requestLog", recorder: rec}
	skipped := &recordingFactory{name: "skipped", recorder: rec, skipBuild: true}
	mwA := &recordingFactory{name: "A", recorder: rec}

	f := CreateFactory(nil, nil, requestLog, nil, skipped, mwA).(*clientFactory)
	client := f.New()
	_, err := client.Request().URL(upstream.URL).Send()
	require.NoError(t, err)

	// skipped doesn't appear: NewRoundTripper returned nil so the chain
	// links A directly to upstream and requestLog wraps that.
	require.Equal(t, []string{"requestLog", "A", "upstream"}, rec.snapshot())
}
