package routes

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/httpf"
)

const (
	// HeaderUpstreamURL identifies the upstream URL to forward to.
	// Required on every /_proxyRaw request. Spelled with the API's
	// public capitalization (X-AuthProxy-…); Go's http.Header
	// canonicalizes on lookup so the comparison still hits.
	HeaderUpstreamURL = "X-AuthProxy-Upstream-URL"
	// HeaderLabel carries a single key=value label, repeated once per
	// label. Repeated rather than a single comma-joined header so label
	// keys may contain `/` and other characters not safe inside an HTTP
	// header value alongside comma delimiters.
	HeaderLabel = "X-AuthProxy-Label"
	// HeaderSendEvidence reports AuthProxy's cumulative knowledge of target
	// sends. Caller and upstream values are reserved and cannot override it.
	HeaderSendEvidence = "X-AuthProxy-Send-Evidence"
)

// Canonical-form variants used for switch comparisons after we've already
// run http.CanonicalHeaderKey on the inbound header name. Go canonicalizes
// "X-AuthProxy-Upstream-URL" to "X-Authproxy-Upstream-Url" (only the first
// rune of each dash-segment is upper-cased), so the public constants above
// don't compare equal to a canonical-form name. Defining these privately
// keeps the public API spelling intact.
var (
	headerUpstreamURLCanonical  = http.CanonicalHeaderKey(HeaderUpstreamURL)
	headerLabelCanonical        = http.CanonicalHeaderKey(HeaderLabel)
	headerSendEvidenceCanonical = http.CanonicalHeaderKey(HeaderSendEvidence)
)

// proxyRaw is the streaming raw-proxy handler. Parses the inbound
// envelope (X-AuthProxy-Upstream-URL, repeated X-AuthProxy-Label, plus
// the request body), builds an outbound *http.Request, and delegates to
// Connection.ProxyRequestRaw which applies the connection's credentials
// and streams the upstream response back to the caller.
//
// Bound on the same `connections:proxy` verb as the wrapped /_proxy
// route — both are "make a proxied call as this connection".
//
// @Summary Stream a request through a connection
// @Description Forward the request method and body to X-AuthProxy-Upstream-URL using the connection's credentials. The upstream status, body, ordinary headers and trailers are streamed back. X-AuthProxy-Send-Evidence is AuthProxy-owned: notSent means no target request was sent, mayHaveSent means a send is possible without a confirmed response, and sent means a target HTTP response was observed, regardless of its status. Evidence accumulates across redirects and credential recovery; OAuth token traffic is excluded. The header is fixed before streaming starts, including for empty responses, and later body errors cannot change it. It does not imply business success or safe retry. Ordinary authentication, validation and proxy errors also carry the header; older servers, global middleware, panic recovery or infrastructure failures may omit it. GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS, CONNECT and TRACE are accepted; CORS preflight may be handled by global middleware.
// @Tags proxy
// @Param id path string true "Connection UUID"
// @Param X-AuthProxy-Upstream-URL header string true "Absolute upstream http(s) URL"
// @Param X-AuthProxy-Label header []string false "Repeated key=value request labels" collectionFormat(multi)
// @Param request body string false "Raw upstream request body"
// @Success 200 {string} string "Upstream response body; any upstream HTTP status is preserved"
// @Success default {string} string "Upstream response with its original status and body"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 429 {string} string "Local rate limit or upstream response"
// @Failure 500 {object} ErrorResponse
// @Header all {string} X-AuthProxy-Send-Evidence "notSent | mayHaveSent | sent; cumulative target-send evidence, fixed before the response body"
// @Security BearerAuth
// @Router /connections/{id}/_proxyRaw [get]
// @Router /connections/{id}/_proxyRaw [post]
// @Router /connections/{id}/_proxyRaw [put]
// @Router /connections/{id}/_proxyRaw [patch]
// @Router /connections/{id}/_proxyRaw [delete]
// @Router /connections/{id}/_proxyRaw [head]
// @Router /connections/{id}/_proxyRaw [options]
func (r *ConnectionsProxyRoutes) proxyRaw(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	connectionUuid, err := apid.Parse(gctx.Param("id"))
	if err != nil || connectionUuid == apid.Nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("invalid connection id", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	conn, err := r.core.GetConnection(ctx, connectionUuid)
	if err != nil {
		if errors.Is(err, iface.ErrConnectionNotFound) {
			apgin.WriteError(gctx, r.logger, httperr.NotFound("connection not found"))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(conn); httpErr != nil {
		apgin.WriteError(gctx, r.logger, httpErr)
		return
	}

	parsed, perr := parseRawProxyEnvelope(gctx.Request)
	if perr != nil {
		apgin.WriteError(gctx, r.logger, perr)
		return
	}

	outbound, oerr := http.NewRequestWithContext(ctx, gctx.Request.Method, parsed.upstreamURL.String(), gctx.Request.Body)
	if oerr != nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("could not build outbound request", httperr.WithInternalErr(oerr)))
		return
	}
	// Forward the inbound Content-Length so chunked-vs-known framing
	// survives the hop. Go's http.NewRequest only sets this for known
	// body types; for our io.ReadCloser it stays unset.
	outbound.ContentLength = gctx.Request.ContentLength
	if hl := gctx.Request.Header.Get("Content-Length"); hl != "" {
		outbound.Header.Set("Content-Length", hl)
	}
	copyInboundHeadersForRawProxy(outbound.Header, gctx.Request.Header)

	rawReq := &iface.RawProxyRequest{
		Outbound: outbound,
		Labels:   parsed.labels,
	}

	// Early response headers can arrive while the target is still reading an
	// upload. Disable HTTP/1's automatic request-body drain at response commit
	// so it cannot consume bytes owned by the outgoing transport. HTTP/2 is
	// already full duplex; test writers may not implement this capability.
	if err := http.NewResponseController(gctx.Writer).EnableFullDuplex(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		apgin.WriteErr(gctx, r.logger, err)
		return
	}

	// Only the writer passed to the proxy can settle dispatch at the first
	// response write. Error/panic renderers use gctx.Writer directly, so they
	// cannot mistake an interrupted attempt for a completed notSent result.
	ctx, complete := beginProxyDispatch(gctx)
	writer := &rawProxyDispatchWriter{ResponseWriter: gctx.Writer, complete: complete}
	err = conn.ProxyRequestRaw(ctx, httpf.RequestTypeProxy, rawReq, writer)
	complete()
	if err != nil {
		// If headers haven't been flushed yet we can still surface a
		// JSON error; once they are on the wire we can only log.
		if !gctx.Writer.Written() {
			apgin.WriteErr(gctx, r.logger, err)
			return
		}
		r.logger.WarnContext(ctx, "raw proxy stream aborted after headers were sent", "error", err)
		_ = gctx.Error(err)
	}
}

// rawProxyEnvelope is the parsed result of pulling AuthProxy envelope
// headers off the inbound request.
type rawProxyEnvelope struct {
	upstreamURL *url.URL
	labels      map[string]string
}

func parseRawProxyEnvelope(req *http.Request) (*rawProxyEnvelope, *httperr.Error) {
	rawURL := req.Header.Get(HeaderUpstreamURL)
	if rawURL == "" {
		return nil, httperr.BadRequest("missing required header " + HeaderUpstreamURL)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, httperr.BadRequest("invalid "+HeaderUpstreamURL+": "+err.Error(), httperr.WithInternalErr(err))
	}
	if !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, httperr.BadRequest(HeaderUpstreamURL + " must be an absolute http(s) URL")
	}
	if u.Host == "" {
		return nil, httperr.BadRequest(HeaderUpstreamURL + " must include a host")
	}

	labels, lerr := parseLabelHeaders(req.Header.Values(HeaderLabel))
	if lerr != nil {
		return nil, lerr
	}

	return &rawProxyEnvelope{upstreamURL: u, labels: labels}, nil
}

// parseLabelHeaders splits each X-AuthProxy-Label value at the first
// `=`. Keys are validated downstream by the rate-limit / request-events
// label pipeline (same path the wrapped ProxyRequest takes), so this
// layer only enforces structural validity — key non-empty, separator
// present, value may be empty.
func parseLabelHeaders(values []string) (map[string]string, *httperr.Error) {
	if len(values) == 0 {
		return nil, nil
	}
	labels := make(map[string]string, len(values))
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		idx := strings.IndexByte(raw, '=')
		if idx <= 0 {
			return nil, httperr.BadRequest("invalid " + HeaderLabel + ": expected <key>=<value>")
		}
		labels[raw[:idx]] = raw[idx+1:]
	}
	return labels, nil
}

// copyInboundHeadersForRawProxy copies the caller's headers onto the
// outbound request, stripping headers that don't make sense on the new
// hop: authentication (the connector's auth replaces it), our envelope
// headers, and RFC 7230 §6.1 hop-by-hop headers. Host is set
// automatically by http.NewRequest from the outbound URL.
func copyInboundHeadersForRawProxy(dst, src http.Header) {
	hopByConnection := map[string]struct{}{}
	for _, v := range src.Values("Connection") {
		for _, name := range splitCommaTrim(v) {
			hopByConnection[http.CanonicalHeaderKey(name)] = struct{}{}
		}
	}
	for k, vv := range src {
		canon := http.CanonicalHeaderKey(k)
		switch canon {
		case "Authorization":
			continue
		case headerUpstreamURLCanonical, headerLabelCanonical, headerSendEvidenceCanonical:
			continue
		case "Host", "Content-Length":
			continue
		}
		if isHopByHopHeader(canon) {
			continue
		}
		if _, ok := hopByConnection[canon]; ok {
			continue
		}
		dst[canon] = append([]string(nil), vv...)
	}
}

// isHopByHopHeader mirrors the same list used inside internal/proxy for
// the response direction. Duplicated rather than imported to keep
// internal/proxy a clean leaf package.
func isHopByHopHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
		"Te", "Trailers", "Transfer-Encoding", "Upgrade":
		return true
	}
	return false
}

func splitCommaTrim(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, seg := range strings.Split(s, ",") {
		seg = strings.TrimSpace(seg)
		if seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

// Compile-time check that gin.ResponseWriter satisfies what the proxy
// orchestrator needs (Header + WriteHeader + Write + Flush) — caught at
// build time rather than at the first request.
var _ http.ResponseWriter = gin.ResponseWriter(nil)
var _ io.Writer = gin.ResponseWriter(nil)
