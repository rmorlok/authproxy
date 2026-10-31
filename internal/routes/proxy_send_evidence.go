package routes

import (
	"context"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// proxySendEvidenceKey identifies the lifecycle of one inbound proxy request.
type proxySendEvidenceKey struct{}

// proxySendEvidence distinguishes pre-dispatch rejection from an unresolved
// target call. An observation is installed only at dispatch so authentication
// and other preparation cannot contribute evidence for the target request.
type proxySendEvidence struct {
	observation atomic.Pointer[httpf.SendObservation]
	completed   atomic.Bool
}

// state returns response metadata without interpreting an intermediate unsent
// attempt as a final result. Confirmed responses remain useful even during panic
// unwinding; other unfinished calls remain uncertain.
func (e *proxySendEvidence) state() common.SendEvidence {
	observation := e.observation.Load()
	if observation == nil {
		return common.SendEvidenceNotSent
	}
	// Read completion first so a completed snapshot cannot use evidence read
	// before a concurrent final report (for example notSent followed by sent).
	completed := e.completed.Load()
	state := observation.State()
	if completed || state == httpf.SendStateSent {
		return state
	}
	return common.SendEvidenceMayHaveSent
}

// observeProxySendEvidence runs before authentication so both native HTTP and
// Gin error writers can report a rejection before any target call as notSent.
func observeProxySendEvidence(gctx *gin.Context) {
	evidence := &proxySendEvidence{}
	ctx := context.WithValue(gctx.Request.Context(), proxySendEvidenceKey{}, evidence)
	ctx = apctx.WithProxySendEvidence(ctx, evidence.state)
	gctx.Request = gctx.Request.WithContext(ctx)
	gctx.Next()
}

// beginProxyDispatch starts a fresh target observation and returns a completion
// function. Call completion only after the proxy returns, never as a defer: a
// panic must leave incomplete evidence uncertain. The returned context must be
// passed to the proxy, and is also installed for subsequent error rendering.
func beginProxyDispatch(gctx *gin.Context) (context.Context, func() common.SendEvidence) {
	ctx := gctx.Request.Context()
	evidence, ok := ctx.Value(proxySendEvidenceKey{}).(*proxySendEvidence)
	if !ok {
		// Direct handler invocations without the registration middleware still
		// receive conservative evidence once they enter target dispatch.
		evidence = &proxySendEvidence{}
		ctx = context.WithValue(ctx, proxySendEvidenceKey{}, evidence)
		ctx = apctx.WithProxySendEvidence(ctx, evidence.state)
	}
	ctx, observation := httpf.ObserveSends(ctx)
	evidence.observation.Store(observation)
	gctx.Request = gctx.Request.WithContext(ctx)
	return ctx, func() common.SendEvidence {
		evidence.completed.Store(true)
		return evidence.state()
	}
}
