package httpf

import (
	"context"
	"net/http"
	"sync/atomic"
)

// SendObservation collects trusted reports for one logical target request,
// including credential-recovery attempts. Its zero value is usable and remains
// uncertain until a report arrives. Reports of possible sends are permanent.
// It is safe to report and inspect evidence concurrently.
type SendObservation struct {
	// state distinguishes unreported uncertainty from an explicit report, so a
	// later known-unsent attempt cannot erase an earlier possible send.
	state atomic.Uint32
}

const (
	sendUnreported      uint32 = iota // No implementation has reported evidence.
	sendNotAttempted                  // Every reported attempt is known unsent.
	sendMayHaveOccurred               // At least one reported attempt may have sent.
)

// sendObservationKey isolates caller observations from transport tracking.
type sendObservationKey struct{}

// ObserveSends returns a derived context carrying a fresh observation. The
// parent must be non-nil. Callers should create an observation per logical
// client call; a client that never reports leaves its outcome uncertain.
func ObserveSends(ctx context.Context) (context.Context, *SendObservation) {
	observation := &SendObservation{}
	return context.WithValue(ctx, sendObservationKey{}, observation), observation
}

// MayHaveSent reports whether the observed call might have reached its target.
// It returns true for missing evidence, including a nil observation. Only an
// explicit trusted report that no target handoff occurred can make it false.
func (o *SendObservation) MayHaveSent() bool {
	return o == nil || o.state.Load() != sendNotAttempted
}

// ReportSendEvidence adds a trusted target-send report to ctx's observation.
// Passing false asserts that the reported work never reached the target
// transport. Passing true records either a possible send or missing evidence;
// it is permanent even if a later attempt is known unsent. Without an installed
// observation this function does nothing. Credential endpoint I/O must not be
// reported as a target send.
func ReportSendEvidence(ctx context.Context, mayHaveSent bool) {
	if ctx == nil {
		return
	}

	observation, _ := ctx.Value(sendObservationKey{}).(*SendObservation)
	if observation == nil {
		return
	}

	if mayHaveSent {
		observation.state.Store(sendMayHaveOccurred)
		return
	}

	observation.state.CompareAndSwap(sendUnreported, sendNotAttempted)
}

// targetSendEvidence accumulates transport evidence across a single client.Do,
// including redirects. It is separate from SendObservation so credential HTTP
// calls inheriting an observation cannot be mistaken for target requests.
type targetSendEvidence struct {
	// covered means the factory's complete middleware stack observed the call.
	// Without this acknowledgement, an absent handoff marker proves nothing.
	covered atomic.Bool

	// handedOff remains true after any attempt enters the underlying transport.
	// It deliberately includes failures before actual network writes.
	handedOff atomic.Bool
}

// targetSendEvidenceKey can only be installed by DoWithSendEvidence.
type targetSendEvidenceKey struct{}

// targetEvidenceFrom returns the private transport tracker, when this request
// was prepared by DoWithSendEvidence. Ordinary HTTP calls have no tracker.
func targetEvidenceFrom(ctx context.Context) *targetSendEvidence {
	evidence, _ := ctx.Value(targetSendEvidenceKey{}).(*targetSendEvidence)
	return evidence
}

// DoWithSendEvidence sends req and reports whether any target attempt may have
// reached the underlying transport. The client and request must be non-nil;
// response/body ownership and errors retain client.Do semantics. The caller's
// request context is not changed.
//
// Only a client using the factory's complete middleware stack directly can
// prove no transport handoff; an unknown wrapper may perform untracked I/O
// before delegating and therefore remains uncertain. Entering the underlying
// transport counts even if dialing or TLS later fails. The private tracker
// follows redirects but is not installed in the original context, so callers
// can resolve credentials with that context without counting token requests.
// This helper returns evidence without reporting to a SendObservation.
func DoWithSendEvidence(
	client *http.Client,
	req *http.Request,
) (resp *http.Response, mayHaveReached bool, err error) {
	// Check if we have the coverage transport to record what we need
	_, supported := client.Transport.(*sendCoverageTransport)

	// Attach the evidence to the context for tracking
	evidence := &targetSendEvidence{}
	ctx := context.WithValue(req.Context(), targetSendEvidenceKey{}, evidence)

	// Execute the request
	resp, err = client.Do(req.WithContext(ctx))

	// Evaluate if we may have reached transport
	mayHaveReached = !supported ||
		!evidence.covered.Load() ||
		evidence.handedOff.Load()

	return resp, mayHaveReached, err
}

// sendCoverageTransport acknowledges that the complete factory middleware
// stack participates in tracking. It surrounds all middleware so a local
// rejection can be distinguished from an uninstrumented client's result.
type sendCoverageTransport struct {
	// transport is the complete registered middleware stack.
	transport http.RoundTripper
}

// RoundTrip records coverage without reading or modifying caller observations.
func (t *sendCoverageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if evidence := targetEvidenceFrom(req.Context()); evidence != nil {
		evidence.covered.Store(true)
	}

	return t.transport.RoundTrip(req)
}

// sendHandoffTransport records possible sends after all local middleware has
// accepted a request. Middleware must delegate target I/O through its supplied
// parent and preserve the request's context for this evidence to be reliable.
type sendHandoffTransport struct {
	// transport is the underlying transport captured when the factory is built.
	transport http.RoundTripper
}

// RoundTrip records the handoff before calling the underlying transport, so
// transport failures and retries cannot erase evidence of a possible send.
func (t *sendHandoffTransport) RoundTrip(
	req *http.Request,
) (*http.Response, error) {
	if evidence := targetEvidenceFrom(req.Context()); evidence != nil {
		evidence.handedOff.Store(true)
	}

	return t.transport.RoundTrip(req)
}
