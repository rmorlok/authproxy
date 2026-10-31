package common

import "fmt"

// SendEvidence describes cumulative transport evidence for a logical proxied
// request, including redirects and credential-recovery attempts. It concerns
// the target request, not OAuth credential requests, and does not establish
// whether the provider completed an application action.
type SendEvidence string

const (
	// SendEvidenceNotSent means every target attempt was proven to stop before
	// reaching the underlying target transport.
	SendEvidenceNotSent SendEvidence = "notSent"
	// SendEvidenceMayHaveSent means a target handoff occurred without a confirmed
	// response, or the implementation cannot account for all possible sends.
	SendEvidenceMayHaveSent SendEvidence = "mayHaveSent"
	// SendEvidenceSent means at least one target transport returned an HTTP
	// response, regardless of its status or any subsequent failures.
	SendEvidenceSent SendEvidence = "sent"
)

// Validate rejects values outside the public send-evidence vocabulary. An
// absent value is not a valid observation and must be interpreted conservatively
// by clients reading an older server response.
func (e SendEvidence) Validate() error {
	switch e {
	case SendEvidenceNotSent, SendEvidenceMayHaveSent, SendEvidenceSent:
		return nil
	default:
		return fmt.Errorf("unknown send evidence %q", string(e))
	}
}
