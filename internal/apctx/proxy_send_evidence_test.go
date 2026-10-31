package apctx

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

// TestProxySendEvidenceReader distinguishes ordinary errors from unavailable
// proxy evidence, and reads the current snapshot rather than an initial value.
func TestProxySendEvidenceReader(t *testing.T) {
	// Non-proxy contexts omit metadata; installed but unavailable readers do not.
	require.Empty(t, GetProxySendEvidence(nil))
	require.Empty(t, GetProxySendEvidence(context.Background()))
	require.Equal(t, common.SendEvidenceMayHaveSent,
		GetProxySendEvidence(WithProxySendEvidence(context.Background(), nil)))

	var current atomic.Value
	current.Store(common.SendEvidenceNotSent)
	ctx := WithProxySendEvidence(context.Background(), func() common.SendEvidence {
		return current.Load().(common.SendEvidence)
	})
	for _, state := range []common.SendEvidence{
		common.SendEvidenceNotSent, common.SendEvidenceMayHaveSent, common.SendEvidenceSent,
	} {
		// Error rendering observes the latest evidence after target dispatch.
		current.Store(state)
		require.Equal(t, state, GetProxySendEvidence(ctx))
	}

	// Invalid provider values cannot accidentally claim the request was unsent.
	for _, invalid := range []common.SendEvidence{"", "futureValue"} {
		current.Store(invalid)
		require.Equal(t, common.SendEvidenceMayHaveSent, GetProxySendEvidence(ctx))
	}
}
