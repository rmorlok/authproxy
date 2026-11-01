package routes

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// observeRawProxySendEvidence installs metadata before authentication without
// opting raw errors into the wrapped endpoint's JSON sendEvidence field. The
// writer owns the response header, including on native authentication errors.
func observeRawProxySendEvidence(gctx *gin.Context) {
	evidence := &proxySendEvidence{}
	ctx := context.WithValue(gctx.Request.Context(), proxySendEvidenceKey{}, evidence)
	gctx.Request = gctx.Request.WithContext(ctx)
	writer := &rawSendEvidenceWriter{
		ResponseWriter: gctx.Writer,
		evidence:       evidence,
		exposeHeaders:  headerValuesFold(gctx.Writer.Header(), "Access-Control-Expose-Headers"),
	}
	gctx.Writer = writer
	// Gin may commit an empty response after the middleware returns. Also
	// remove trailer values installed after the last body write, including
	// during unwinding, before net/http serializes its final trailer block.
	defer writer.prepareHeaders()
	gctx.Next()
}

// rawSendEvidenceWriter protects AuthProxy-owned metadata at every Gin commit
// path. Embedding preserves Gin's status/size accounting and optional HTTP
// interfaces. Once committed, evidence cannot be revised by later stream errors.
type rawSendEvidenceWriter struct {
	gin.ResponseWriter
	evidence      *proxySendEvidence
	exposeHeaders []string
	committed     common.SendEvidence
}

// prepareHeaders replaces spoofed header values and trailer declarations while
// preserving unrelated upstream metadata. Before commitment it reads the final
// dispatch state; afterward it retains the value already visible to the caller.
func (w *rawSendEvidenceWriter) prepareHeaders() {
	written := w.Written()
	if !written {
		w.committed = w.evidence.state()
	}
	headers := w.Header()
	for key, values := range headers {
		switch {
		case strings.EqualFold(key, HeaderSendEvidence),
			strings.EqualFold(key, http.TrailerPrefix+HeaderSendEvidence):
			delete(headers, key)
		case strings.EqualFold(key, "Trailer"):
			// A reserved trailer would compete with the authoritative header.
			// Handle comma lists, repeated values and noncanonical map keys.
			var names []string
			for _, value := range values {
				for _, name := range splitCommaTrim(value) {
					if !strings.EqualFold(name, HeaderSendEvidence) {
						names = append(names, name)
					}
				}
			}
			if len(names) == 0 {
				delete(headers, key)
			} else {
				headers[key] = names
			}
		}
	}
	headers.Set(HeaderSendEvidence, string(w.committed))
	if written {
		// Only trailer cleanup can affect the wire after commitment. Avoid
		// rebuilding the CORS list on every chunk of a long-running stream.
		return
	}

	// Upstream response copying can replace the CORS middleware's exposure
	// list. Merge that original list back, then expose our own header even if
	// the provider advertised a wildcard (which excludes credentialed CORS).
	values := append([]string(nil), w.exposeHeaders...)
	values = append(values, headerValuesFold(headers, "Access-Control-Expose-Headers")...)
	values = append(values, HeaderSendEvidence)
	var names []string
	seen := make(map[string]bool)
	for _, value := range values {
		for _, name := range splitCommaTrim(value) {
			key := strings.ToLower(name)
			if !seen[key] {
				seen[key] = true
				names = append(names, name)
			}
		}
	}
	for key := range headers {
		if strings.EqualFold(key, "Access-Control-Expose-Headers") {
			delete(headers, key)
		}
	}
	headers.Set("Access-Control-Expose-Headers", strings.Join(names, ", "))
}

// WriteHeaderNow protects metadata before Gin explicitly commits an empty body.
func (w *rawSendEvidenceWriter) WriteHeaderNow() {
	w.prepareHeaders()
	w.ResponseWriter.WriteHeaderNow()
}

// Write protects metadata before the first body bytes commit the response.
func (w *rawSendEvidenceWriter) Write(body []byte) (int, error) {
	w.prepareHeaders()
	return w.ResponseWriter.Write(body)
}

// WriteString covers Gin and native handlers that use the string-write shortcut.
func (w *rawSendEvidenceWriter) WriteString(body string) (int, error) {
	w.prepareHeaders()
	return w.ResponseWriter.WriteString(body)
}

// Flush keeps headers authoritative before streaming bytes or final trailers.
func (w *rawSendEvidenceWriter) Flush() {
	w.prepareHeaders()
	w.ResponseWriter.Flush()
}

// Unwrap allows http.ResponseController to reach the underlying Gin writer.
func (w *rawSendEvidenceWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// headerValuesFold reads all spellings of a header, including noncanonical map
// keys from in-process middleware; Header.Values only looks up canonical keys.
func headerValuesFold(headers http.Header, name string) []string {
	var values []string
	for key, vv := range headers {
		if strings.EqualFold(key, name) {
			values = append(values, vv...)
		}
	}
	return values
}

// rawProxyDispatchWriter is passed only to ProxyRequestRaw, which finishes
// target sends and credential recovery before writing a response. That boundary
// settles evidence while the response body can still be an open-ended stream.
// Other writers (notably panic recovery) cannot mark an unfinished call complete.
type rawProxyDispatchWriter struct {
	gin.ResponseWriter
	complete func() common.SendEvidence
}

// WriteHeader settles dispatch after upstream headers have been copied and
// immediately flushes them so clients can read evidence before the first body
// byte, even when the upstream stream is idle. It also commits empty responses.
func (w *rawProxyDispatchWriter) WriteHeader(status int) {
	w.complete()
	w.ResponseWriter.WriteHeader(status)
	w.ResponseWriter.Flush()
}

// Write handles proxy implementations that write a body without explicit status.
func (w *rawProxyDispatchWriter) Write(body []byte) (int, error) {
	w.complete()
	return w.ResponseWriter.Write(body)
}

// WriteString preserves the same boundary for optimized string-body writers.
func (w *rawProxyDispatchWriter) WriteString(body string) (int, error) {
	w.complete()
	return w.ResponseWriter.WriteString(body)
}

// WriteHeaderNow settles implementations that explicitly commit a Gin response.
func (w *rawProxyDispatchWriter) WriteHeaderNow() {
	w.complete()
	w.ResponseWriter.WriteHeaderNow()
}

// Flush settles implementations that begin a response with a flush alone.
func (w *rawProxyDispatchWriter) Flush() {
	w.complete()
	w.ResponseWriter.Flush()
}

// Unwrap retains ResponseController support without bypassing normal writes.
func (w *rawProxyDispatchWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
