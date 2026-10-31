package apctx

import (
	"context"

	"github.com/rmorlok/authproxy/internal/schema/common"
)

// proxySendEvidenceKey keeps response metadata separate from transport tracking.
type proxySendEvidenceKey struct{}

// proxySendEvidenceReader defers the snapshot until an error is rendered, after
// authentication, validation, or target dispatch has determined its outcome.
type proxySendEvidenceReader struct {
	read func() common.SendEvidence
}

// WithProxySendEvidence installs a request-scoped reader for public proxy
// response metadata. The reader must be safe to call concurrently and must not
// perform I/O. A nil reader represents unavailable evidence on a proxy request.
func WithProxySendEvidence(ctx context.Context, read func() common.SendEvidence) context.Context {
	return context.WithValue(ctx, proxySendEvidenceKey{}, proxySendEvidenceReader{read: read})
}

// GetProxySendEvidence returns the proxy's current evidence, or the empty value
// for an ordinary request so shared error responses omit the field. Missing or
// unrecognized evidence from an installed reader is conservatively uncertain.
func GetProxySendEvidence(ctx context.Context) common.SendEvidence {
	if ctx == nil {
		return ""
	}
	reader, ok := ctx.Value(proxySendEvidenceKey{}).(proxySendEvidenceReader)
	if !ok {
		return ""
	}
	if reader.read != nil {
		switch state := reader.read(); state {
		case common.SendEvidenceNotSent, common.SendEvidenceMayHaveSent, common.SendEvidenceSent:
			return state
		}
	}
	return common.SendEvidenceMayHaveSent
}
