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
	// Required on every /_proxy_raw request. Spelled with the API's
	// public capitalization (X-AuthProxy-…); Go's http.Header
	// canonicalizes on lookup so the comparison still hits.
	HeaderUpstreamURL = "X-AuthProxy-Upstream-URL"
	// HeaderLabel carries a single key=value label, repeated once per
	// label. Repeated rather than a single comma-joined header so label
	// keys may contain `/` and other characters not safe inside an HTTP
	// header value alongside comma delimiters.
	HeaderLabel = "X-AuthProxy-Label"
)

// Canonical-form variants used for switch comparisons after we've already
// run http.CanonicalHeaderKey on the inbound header name. Go canonicalizes
// "X-AuthProxy-Upstream-URL" to "X-Authproxy-Upstream-Url" (only the first
// rune of each dash-segment is upper-cased), so the public constants above
// don't compare equal to a canonical-form name. Defining these privately
// keeps the public API spelling intact.
var (
	headerUpstreamURLCanonical = http.CanonicalHeaderKey(HeaderUpstreamURL)
	headerLabelCanonical       = http.CanonicalHeaderKey(HeaderLabel)
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
//	ANY /connections/{id}/_proxy_raw
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

	if err := conn.ProxyRequestRaw(ctx, httpf.RequestTypeProxy, rawReq, gctx.Writer); err != nil {
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
		case headerUpstreamURLCanonical, headerLabelCanonical:
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
