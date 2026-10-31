package routes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

// TestProxySendEvidenceConcurrentCompletion prevents a reader from combining
// an earlier unsent attempt with completion of a later confirmed target send.
func TestProxySendEvidenceConcurrentCompletion(t *testing.T) {
	for range 100 {
		ctx, observation := httpf.ObserveSends(context.Background())
		httpf.ReportSendState(ctx, httpf.SendStateNotSent)
		evidence := &proxySendEvidence{}
		evidence.observation.Store(observation)
		start := make(chan struct{})
		var readers sync.WaitGroup
		for range 8 {
			readers.Add(1)
			go func() {
				defer readers.Done()
				<-start
				for range 100 {
					// During dispatch the local attempt is uncertain; after the
					// final report only sent is possible. notSent is never valid.
					if evidence.state() == common.SendEvidenceNotSent {
						t.Error("completed snapshot returned an earlier unsent attempt")
						return
					}
				}
			}()
		}
		close(start)
		httpf.ReportSendState(ctx, httpf.SendStateSent)
		evidence.completed.Store(true)
		readers.Wait()
		require.Equal(t, common.SendEvidenceSent, evidence.state())
	}
}

// TestProxySendEvidenceLifecycle checks the safety boundary between local
// rejection, unfinished dispatch, and the final cumulative target observation.
func TestProxySendEvidenceLifecycle(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		report common.SendEvidence
		panic  bool
		want   common.SendEvidence
	}{
		{"unknown implementation", "", false, common.SendEvidenceMayHaveSent},
		{"known unsent", common.SendEvidenceNotSent, false, common.SendEvidenceNotSent},
		{"possible send", common.SendEvidenceMayHaveSent, false, common.SendEvidenceMayHaveSent},
		{"confirmed response", common.SendEvidenceSent, false, common.SendEvidenceSent},
		{"panic after local attempt", common.SendEvidenceNotSent, true, common.SendEvidenceMayHaveSent},
		{"panic after response", common.SendEvidenceSent, true, common.SendEvidenceSent},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			// Recovery stands in for an error renderer after unwinding. The real
			// bare Gin recovery is allowed to omit response metadata entirely.
			router.Use(func(gctx *gin.Context) {
				defer func() {
					if recover() != nil {
						apgin.WriteErr(gctx, nil, errors.New("proxy panicked"))
						gctx.Abort()
					}
				}()
				gctx.Next()
			})
			router.POST("/proxy", observeProxySendEvidence, func(gctx *gin.Context) {
				require.Equal(t, common.SendEvidenceNotSent, apctx.GetProxySendEvidence(gctx.Request.Context()))
				ctx, complete := beginProxyDispatch(gctx)
				require.Equal(t, common.SendEvidenceMayHaveSent, apctx.GetProxySendEvidence(ctx))
				if scenario.report != "" {
					httpf.ReportSendState(ctx, scenario.report)
				}

				// Intermediate notSent must not claim completion; sent can already
				// establish that at least one target response was received.
				inFlight := common.SendEvidenceMayHaveSent
				if scenario.report == common.SendEvidenceSent {
					inFlight = common.SendEvidenceSent
				}
				require.Equal(t, inFlight, apctx.GetProxySendEvidence(ctx))
				if scenario.panic {
					panic("proxy panicked")
				}
				require.Equal(t, scenario.want, complete())
				apgin.WriteErr(gctx, nil, errors.New("proxy failed"))
			})

			// Assert the public error field after the normal or panic path settles.
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/proxy", nil))
			require.Equal(t, http.StatusInternalServerError, recorder.Code)
			var response httperr.ErrorResponse
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, scenario.want, response.SendEvidence)
		})
	}
}

// TestProxySendEvidenceErrorWriters checks both authentication's native writer
// and the Gin route writer, including debug fields and cancellation mapping.
func TestProxySendEvidenceErrorWriters(t *testing.T) {
	for _, writer := range []string{"native", "gin"} {
		for _, canceled := range []bool{false, true} {
			t.Run(writer+"/"+map[bool]string{false: "validation", true: "canceled"}[canceled], func(t *testing.T) {
				router := gin.New()
				router.POST("/proxy", observeProxySendEvidence, func(gctx *gin.Context) {
					// No dispatch occurs. Authentication/validation and canceled
					// preparation all retain explicit notSent evidence.
					ctx := apctx.WithDebugMode(gctx.Request.Context(), true)
					internal := errors.New("private diagnostic")
					err := httperr.BadRequest("invalid request", httperr.WithInternalErr(internal))
					if canceled {
						var cancel context.CancelFunc
						ctx, cancel = context.WithCancel(ctx)
						cancel()
						internal = context.Canceled
						err = httperr.InternalServerError(httperr.WithInternalErr(internal))
					}
					gctx.Request = gctx.Request.WithContext(ctx)
					if writer == "native" {
						err.WriteResponse(ctx, nil, gctx.Writer)
					} else {
						apgin.WriteError(gctx, nil, err)
						require.ErrorIs(t, gctx.Errors.Last().Err, internal)
					}
					gctx.Abort()
				})

				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/proxy", nil))
				wantStatus := http.StatusBadRequest
				if canceled {
					wantStatus = httperr.StatusClientClosedRequest
				}
				require.Equal(t, wantStatus, recorder.Code)
				var response httperr.ErrorResponse
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
				require.Equal(t, common.SendEvidenceNotSent, response.SendEvidence)
				require.NotEmpty(t, response.StackTrace)
			})
		}
	}
}

// TestProxySendEvidenceIsolation ensures a nested target observation cannot
// reuse earlier caller evidence and unrelated routes keep their error format.
func TestProxySendEvidenceIsolation(t *testing.T) {
	router := gin.New()
	router.POST("/proxy", observeProxySendEvidence, func(gctx *gin.Context) {
		// Simulate earlier non-target work in the caller context. Dispatch owns
		// a fresh observation even when a caller already installed one.
		ctx, outer := httpf.ObserveSends(gctx.Request.Context())
		httpf.ReportSendState(ctx, httpf.SendStateSent)
		gctx.Request = gctx.Request.WithContext(ctx)
		ctx, complete := beginProxyDispatch(gctx)
		httpf.ReportSendState(ctx, httpf.SendStateNotSent)
		require.Equal(t, common.SendEvidenceNotSent, complete())
		require.Equal(t, httpf.SendStateSent, outer.State())
		apgin.WriteError(gctx, nil, httperr.BadRequest("rejected"))
	})
	router.POST("/ordinary", func(gctx *gin.Context) {
		apgin.WriteError(gctx, nil, httperr.BadRequest("rejected"))
	})

	// Reusing the router does not add proxy metadata to the next ordinary error.
	for _, path := range []string{"/proxy", "/ordinary"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
		var response map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		if path == "/proxy" {
			require.Equal(t, "notSent", response["sendEvidence"])
		} else {
			require.Equal(t, map[string]any{"error": "rejected"}, response)
		}
	}
}
