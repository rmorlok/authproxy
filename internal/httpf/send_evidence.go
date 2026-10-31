package httpf

import (
	"context"
	"net/http"
	"sync/atomic"

	"gopkg.in/h2non/gentleman.v2"
	gentlemancontext "gopkg.in/h2non/gentleman.v2/context"
)

// SendState describes transport evidence for a logical target request. It does
// not establish whether the provider completed the requested action.
type SendState string

const (
	// SendStateNotSent means every reported attempt stopped before target transport.
	SendStateNotSent SendState = "notSent"
	// SendStateMayHaveSent means a target handoff occurred without a response, or
	// the implementation cannot account for all possible target sends.
	SendStateMayHaveSent SendState = "mayHaveSent"
	// SendStateSent means the target transport returned an HTTP response on at
	// least one attempt, regardless of its status or subsequent failures.
	SendStateSent SendState = "sent"
)

// SendObservation collects trusted reports for one logical target request,
// including credential-recovery attempts. Its zero value is usable and remains
// uncertain until a report arrives. A confirmed send dominates uncertainty,
// which dominates known-unsent reports. Reporting and inspection are safe
// concurrently; callers must wait for their logical call to finish before
// interpreting its settled state.
type SendObservation struct {
	// state orders explicit evidence without treating initial uncertainty as a
	// report: a first known-unsent report can still establish notSent.
	state atomic.Uint32
}

const (
	sendUnreported       uint32 = iota // No implementation has reported evidence.
	sendNotAttempted                   // Every reported attempt is known unsent.
	sendMayHaveOccurred                // At least one reported attempt may have sent.
	sendResponseReceived               // At least one target HTTP response was received.
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

// State returns cumulative target evidence. Missing evidence, including a nil
// observation, remains mayHaveSent. A sent report cannot be erased by later
// uncertainty or local rejection.
func (o *SendObservation) State() SendState {
	if o != nil {
		switch o.state.Load() {
		case sendNotAttempted:
			return SendStateNotSent
		case sendResponseReceived:
			return SendStateSent
		}
	}
	return SendStateMayHaveSent
}

// MayHaveSent preserves the conservative boolean contract: only known-unsent
// evidence is false, while confirmed sends and missing evidence are true.
func (o *SendObservation) MayHaveSent() bool {
	return o.State() != SendStateNotSent
}

// ReportSendState adds trusted target evidence to ctx's observation. Reports
// accumulate across attempts: sent wins over mayHaveSent, which wins over
// notSent. Invalid states are treated as uncertainty. Without an observation
// this does nothing. Credential endpoint I/O must not be reported here.
func ReportSendState(ctx context.Context, state SendState) {
	if ctx == nil {
		return
	}

	observation, _ := ctx.Value(sendObservationKey{}).(*SendObservation)
	if observation == nil {
		return
	}

	report := sendMayHaveOccurred
	switch state {
	case SendStateNotSent:
		report = sendNotAttempted
	case SendStateSent:
		report = sendResponseReceived
	}

	for previous := observation.state.Load(); previous < report; previous = observation.state.Load() {
		if observation.state.CompareAndSwap(previous, report) {
			return
		}
	}
}

// ReportSendEvidence preserves boolean reporting for existing callers. True is
// uncertainty, never proof of a received response; false is trusted notSent.
func ReportSendEvidence(ctx context.Context, mayHaveSent bool) {
	state := SendStateNotSent
	if mayHaveSent {
		state = SendStateMayHaveSent
	}
	ReportSendState(ctx, state)
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

	// responseReceived remains true after an underlying transport response,
	// including when a later redirect or middleware produces an error.
	responseReceived atomic.Bool
}

// targetSendEvidenceKey is installed only by the target-send helpers.
type targetSendEvidenceKey struct{}

// targetEvidenceFrom returns the private transport tracker, when this request
// was prepared by a target-send helper. Ordinary HTTP calls have no tracker.
func targetEvidenceFrom(ctx context.Context) *targetSendEvidence {
	evidence, _ := ctx.Value(targetSendEvidenceKey{}).(*targetSendEvidence)
	return evidence
}

// state returns the settled evidence for one client call. A confirmed target
// response is useful even through an unknown outer wrapper. Only direct use of
// the complete factory stack can establish that no target send occurred.
func (e *targetSendEvidence) state(supported bool) SendState {
	if e.responseReceived.Load() {
		return SendStateSent
	}
	if supported && e.covered.Load() && !e.handedOff.Load() {
		return SendStateNotSent
	}
	return SendStateMayHaveSent
}

// usesSendCoverage reports direct use of the complete factory stack. Check it
// before and after dispatch because plugins or redirect callbacks can install
// an unknown wrapper during a call. A nil client is not instrumented.
func usesSendCoverage(client *http.Client) bool {
	if client == nil {
		return false
	}
	_, supported := client.Transport.(*sendCoverageTransport)
	return supported
}

// DoWithSendState sends req and returns cumulative target evidence across
// redirects. The client and request must be non-nil; response/body ownership
// and errors retain client.Do semantics. The caller's context is not changed.
//
// Only a client using the factory's complete middleware stack directly can
// prove no transport handoff; an unknown wrapper may perform untracked I/O
// before delegating and therefore remains uncertain. Entering the underlying
// transport counts even if dialing or TLS later fails. The private tracker
// follows redirects but is not installed in the original context, so callers
// can resolve credentials with that context without counting token requests.
// A response observed below middleware establishes sent even if later work
// fails. This helper returns evidence without reporting to a SendObservation.
func DoWithSendState(
	client *http.Client,
	req *http.Request,
) (resp *http.Response, state SendState, err error) {
	// Both checks are required: redirect callbacks can change the transport.
	supported := usesSendCoverage(client)

	// Attach the evidence to the context for tracking
	evidence := &targetSendEvidence{}
	ctx := context.WithValue(req.Context(), targetSendEvidenceKey{}, evidence)

	// Execute the request
	resp, err = client.Do(req.WithContext(ctx))

	return resp, evidence.state(supported && usesSendCoverage(client)), err
}

// DoWithSendEvidence preserves the original boolean helper contract. It does
// not report to an observation; only notSent maps to false.
func DoWithSendEvidence(
	client *http.Client,
	req *http.Request,
) (*http.Response, bool, error) {
	resp, state, err := DoWithSendState(client, req)
	return resp, state != SendStateNotSent, err
}

// DoGentlemanWithSendState sends a single-use Gentleman request with the same
// target evidence as DoWithSendState. It installs tracking after request
// middleware has configured the effective HTTP client and request context.
// Early interception or an unknown transport remains uncertain without a
// confirmed target response. The helper does not report to an observation or
// change Gentleman's response, error, and body ownership behavior.
func DoGentlemanWithSendState(req *gentleman.Request) (*gentleman.Response, SendState, error) {
	evidence := &targetSendEvidence{}
	supported := false
	var effectiveContext *gentlemancontext.Context
	req.UseHandler("before dial", func(ctx *gentlemancontext.Context, handler gentlemancontext.Handler) {
		effectiveContext = ctx
		supported = usesSendCoverage(ctx.Client)
		tracked := context.WithValue(ctx.Request.Context(), targetSendEvidenceKey{}, evidence)
		// SetRequest would discard the new context; preserve Gentleman's store
		// while adding tracking directly to the final outbound request.
		ctx.Request = ctx.Request.WithContext(tracked)
		handler.Next(ctx)
	})

	resp, err := req.Do()
	// A parent before-dial handler can replace either the client or its transport
	// after Next returns, so inspect the final context rather than a saved client.
	stillSupported := effectiveContext != nil && usesSendCoverage(effectiveContext.Client)
	return resp, evidence.state(supported && stillSupported), err
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

// RoundTrip records the handoff and any valid underlying response. Recording
// below middleware prevents synthetic local responses from looking sent.
func (t *sendHandoffTransport) RoundTrip(
	req *http.Request,
) (*http.Response, error) {
	if evidence := targetEvidenceFrom(req.Context()); evidence != nil {
		evidence.handedOff.Store(true)
	}

	resp, err := t.transport.RoundTrip(req)
	// RoundTripper must return a nil error when it obtained a response. Reject
	// invalid placeholder responses rather than promoting them to sent.
	if err == nil && resp != nil && resp.StatusCode >= 100 && resp.StatusCode <= 999 {
		if evidence := targetEvidenceFrom(req.Context()); evidence != nil {
			evidence.responseReceived.Store(true)
		}
	}
	return resp, err
}
