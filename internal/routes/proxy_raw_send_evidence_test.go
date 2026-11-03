package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

// TestRawSendEvidenceLifecycle distinguishes a settled local response from an
// interrupted call, including panic recovery outside the route middleware.
func TestRawSendEvidenceLifecycle(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		report common.SendEvidence
		panic  bool
		error  bool
		want   common.SendEvidence
	}{
		{name: "unknown implementation", want: common.SendEvidenceMayHaveSent},
		{name: "local response", report: common.SendEvidenceNotSent, want: common.SendEvidenceNotSent},
		{name: "possible send", report: common.SendEvidenceMayHaveSent, want: common.SendEvidenceMayHaveSent},
		{name: "target response", report: common.SendEvidenceSent, want: common.SendEvidenceSent},
		{name: "local error", report: common.SendEvidenceNotSent, error: true, want: common.SendEvidenceNotSent},
		{name: "error after response", report: common.SendEvidenceSent, error: true, want: common.SendEvidenceSent},
		{name: "panic after local attempt", report: common.SendEvidenceNotSent, panic: true, want: common.SendEvidenceMayHaveSent},
		{name: "panic after target response", report: common.SendEvidenceSent, panic: true, want: common.SendEvidenceSent},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			router := gin.New()
			// Recovery writes through the route's outer writer, never through the
			// dispatch writer that knows the proxy reached its response boundary.
			router.Use(func(gctx *gin.Context) {
				defer func() {
					if recover() != nil {
						apgin.WriteErr(gctx, nil, errors.New("proxy panicked"))
						gctx.Abort()
					}
				}()
				gctx.Next()
			})
			router.POST("/raw", observeRawProxySendEvidence, func(gctx *gin.Context) {
				ctx, complete := beginProxyDispatch(gctx)
				require.Empty(t, apctx.GetProxySendEvidence(ctx), "raw error JSON stays unchanged")
				if scenario.report != "" {
					httpf.ReportSendState(ctx, scenario.report)
				}
				if scenario.panic {
					panic("proxy panicked")
				}
				if scenario.error {
					complete()
					apgin.WriteErr(gctx, nil, errors.New("proxy failed"))
					return
				}
				// A raw proxy response settles before the body can finish; even an
				// empty local response must be committed with explicit evidence.
				writer := &rawProxyDispatchWriter{ResponseWriter: gctx.Writer, complete: complete}
				writer.WriteHeader(http.StatusNoContent)
				require.True(t, gctx.Writer.Written())
			})

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/raw", nil))
			require.Equal(t, []string{string(scenario.want)}, recorder.Result().Header.Values(HeaderSendEvidence))
			if scenario.panic || scenario.error {
				var body map[string]any
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
				require.Contains(t, body, "error")
				require.NotContains(t, body, "sendEvidence")
			} else {
				require.Equal(t, http.StatusNoContent, recorder.Code)
			}
		})
	}
}

// TestRawSendEvidenceCommitPaths covers both native auth and Gin renderers,
// including handlers that return without writing and alternate streaming paths.
func TestRawSendEvidenceCommitPaths(t *testing.T) {
	for _, path := range []string{"empty", "nativeError", "ginError", "write", "writeString", "writeHeaderNow", "flush"} {
		t.Run(path, func(t *testing.T) {
			router := gin.New()
			router.POST("/raw", observeRawProxySendEvidence, func(gctx *gin.Context) {
				// Native authentication and validation never enter dispatch.
				switch path {
				case "empty":
					return
				case "nativeError":
					httperr.BadRequest("rejected").WriteResponse(gctx.Request.Context(), nil, gctx.Writer)
					return
				case "ginError":
					apgin.WriteError(gctx, nil, httperr.BadRequest("rejected"))
					return
				}
				ctx, complete := beginProxyDispatch(gctx)
				httpf.ReportSendState(ctx, common.SendEvidenceNotSent)
				writer := &rawProxyDispatchWriter{ResponseWriter: gctx.Writer, complete: complete}
				switch path {
				case "write":
					_, err := writer.Write([]byte("body"))
					require.NoError(t, err)
				case "writeString":
					_, err := writer.WriteString("body")
					require.NoError(t, err)
				case "writeHeaderNow":
					writer.WriteHeaderNow()
				case "flush":
					writer.Flush()
				}
			})

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/raw", nil))
			require.Equal(t, []string{"notSent"}, recorder.Result().Header.Values(HeaderSendEvidence))
		})
	}
}

// TestRawSendEvidenceMetadata protects against noncanonical in-process header
// keys as well as metadata installed after the last stream flush.
func TestRawSendEvidenceMetadata(t *testing.T) {
	router := gin.New()
	router.Use(func(gctx *gin.Context) {
		gctx.Header("Access-Control-Expose-Headers", "X-AuthProxy-Existing")
	})
	router.GET("/raw", observeRawProxySendEvidence, func(gctx *gin.Context) {
		ctx, complete := beginProxyDispatch(gctx)
		httpf.ReportSendState(ctx, common.SendEvidenceMayHaveSent)
		writer := &rawProxyDispatchWriter{ResponseWriter: gctx.Writer, complete: complete}
		// Simulate upstream copying that overwrites the CORS exposure list and
		// supplies duplicate spellings and comma-separated trailer declarations.
		headers := writer.Header()
		headers[HeaderSendEvidence] = []string{"notSent", "sent"}
		headers[strings.ToLower(HeaderSendEvidence)] = []string{"notSent"}
		headers.Set("Trailer", "X-Ordinary-Trailer, "+strings.ToLower(HeaderSendEvidence))
		headers.Set("Access-Control-Expose-Headers", "X-Upstream, x-authproxy-existing")
		headers.Set("X-Ordinary", "preserved")
		writer.WriteHeader(http.StatusAccepted)
		_, err := writer.Write([]byte("body"))
		require.NoError(t, err)

		// Later reports cannot revise the already transmitted value. Late
		// trailers are sanitized at middleware exit even without another flush.
		httpf.ReportSendState(ctx, common.SendEvidenceSent)
		headers[http.TrailerPrefix+strings.ToLower(HeaderSendEvidence)] = []string{"notSent"}
		headers[http.TrailerPrefix+"X-Ordinary-Trailer"] = []string{"kept"}
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/raw", nil))
	response := recorder.Result()
	require.Equal(t, http.StatusAccepted, response.StatusCode)
	require.Equal(t, "body", recorder.Body.String())
	require.Equal(t, []string{"mayHaveSent"}, headerValuesFold(response.Header, HeaderSendEvidence))
	require.Equal(t, "preserved", response.Header.Get("X-Ordinary"))
	require.Equal(t, "X-Ordinary-Trailer", response.Header.Get("Trailer"))
	require.Equal(t, "kept", response.Trailer.Get("X-Ordinary-Trailer"))
	require.Empty(t, headerValuesFold(response.Trailer, HeaderSendEvidence))
	require.ElementsMatch(t, []string{"X-AuthProxy-Existing", "X-Upstream", HeaderSendEvidence},
		splitCommaTrim(response.Header.Get("Access-Control-Expose-Headers")))
}
