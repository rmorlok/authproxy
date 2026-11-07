package http

import "net/http"

// SendState describes target-send evidence for one call or an entire execution.
// It does not establish whether the provider completed a business operation.
// The zero value is conservative uncertainty; only an explicit report proves a
// call was unsent. Unknown values are also treated as uncertainty.
type SendState uint8

const (
	// SendStateMayHaveSent means an attempt may have reached the target, or the
	// client cannot account for every possible send. It is also the zero value.
	SendStateMayHaveSent SendState = iota

	// SendStateNotSent means every target attempt was stopped before transport.
	SendStateNotSent

	// SendStateSent means the target transport returned a response for at least
	// one attempt, regardless of status or subsequent failures.
	SendStateSent
)

// Client is an already configured outbound capability supplied by trusted Go
// code. Implementations own request bodies on entry and retain their existing
// authentication, recovery, redirect, attribution, and instrumentation policy.
// They must support concurrent calls and honor request cancellation. Sessions
// never close or reconfigure a client; it can be reused across executions.
type Client interface {
	// Do returns a raw response and settled target evidence for the whole logical
	// call, including redirects and credential recovery. Credential endpoint I/O
	// is separate from target evidence. Missing evidence must remain uncertain;
	// an HTTP status alone cannot distinguish a local reply from a target reply.
	// Every nonnil response body transfers to the session, including on error.
	// Wrap ErrCredentialsUnavailable only for trusted credential failures; other
	// errors are sanitized as ErrTransport. Do not infer categories from text.
	Do(*http.Request) (*http.Response, SendState, error)
}

// normalizeSendState makes zero and unrecognized reports conservative without
// conflating a confirmed response with a possible send.
func normalizeSendState(state SendState) SendState {
	switch state {
	case SendStateNotSent, SendStateSent:
		return state
	default:
		return SendStateMayHaveSent
	}
}
