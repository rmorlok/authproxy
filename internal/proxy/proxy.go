// Package proxy orchestrates a single proxied request through a connection:
// resolve credentials via the auth method's Authenticator, send the request
// through the httpf client (which carries rate-limit / telemetry /
// request-events middleware), and on a 401 from the upstream attempt the
// retry-once-after-recover dance. Owns both the wrapped (structured)
// ProxyRequest and the streaming ProxyRequestRaw paths so the
// per-auth-method packages only have to describe "how to apply this
// credential to a request" — not how to drive a proxy call.
package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/common"
	gentleman "gopkg.in/h2non/gentleman.v2"
)

// ProbeAccelerator is the subset of the core service the proxy relies on to
// nudge probes when the upstream returns a credential-related failure. The
// proxy only needs EnqueueProbeNow; keeping the surface narrow lets test
// constructors pass a tiny fake instead of the full iface.C.
type ProbeAccelerator interface {
	EnqueueProbeNow(ctx context.Context, connectionId apid.ID) error
}

type proxy struct {
	httpf       httpf.F
	conn        iface.Connection
	auth        auth_methods.Authenticator
	accelerator ProbeAccelerator
}

// New constructs an iface.Proxy that orchestrates calls for a single
// connection using the supplied Authenticator. The optional accelerator,
// when non-nil, receives a fire-and-forget EnqueueProbeNow call on
// upstream 401/403 responses so the probe-driven health signal can flip
// without waiting for the next scheduled probe tick. One instance per
// connection — held inside the connection's lazy proxy-impl cache.
func New(h httpf.F, conn iface.Connection, auth auth_methods.Authenticator, accelerator ProbeAccelerator) iface.Proxy {
	return &proxy{httpf: h, conn: conn, auth: auth, accelerator: accelerator}
}

// maybeAccelerateProbes fires a best-effort probe-now enqueue when the
// upstream returns a credential-related status code on a user-initiated
// request. Probe traffic itself is excluded — without that gate the
// probe-now task's own 401 would re-enter this path and (even with the
// per-probe throttle) waste an iteration of bookkeeping per failed probe.
//
// Errors from EnqueueProbeNow are intentionally swallowed: by the time
// this runs, the customer's response is already on its way. Surfacing an
// error would turn a layered optimisation into a brittle dependency on
// the throttle store.
func (p *proxy) maybeAccelerateProbes(ctx context.Context, reqType httpf.RequestType, statusCode int) {
	if p.accelerator == nil {
		return
	}
	if reqType == common.RequestTypeProbe {
		return
	}
	if statusCode != http.StatusUnauthorized && statusCode != http.StatusForbidden {
		return
	}
	_ = p.accelerator.EnqueueProbeNow(ctx, p.conn.GetId())
}

// ProxyRequest resolves credentials, sends the request, and on a 401
// from the upstream attempts to recover (e.g. refresh an OAuth2 token)
// and replay the request exactly once.
//
// If RecoverFrom401 returns auth_methods.ErrCannotRecover the upstream
// 401 is returned to the caller unchanged. If recovery fails for any
// other reason the original 401 is also returned unchanged — the
// customer's app sees the same auth failure it would have without this
// retry path, and the recovery failure was already classified inside
// the authenticator.
func (p *proxy) ProxyRequest(ctx context.Context, reqType httpf.RequestType, req *iface.ProxyRequest) (*iface.ProxyResponse, error) {
	resp, err := p.send(ctx, reqType, req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		recoverErr := p.auth.RecoverFrom401(ctx)
		if recoverErr == nil {
			retried, retryErr := p.send(ctx, reqType, req)
			if retryErr == nil {
				resp = retried
			}
		} else if !errors.Is(recoverErr, auth_methods.ErrCannotRecover) {
			// Refresh failed for a recoverable auth method. Surface the
			// original 401; the recovery failure already self-reported.
			_ = recoverErr
		}
	}

	// After the recover-and-retry dance has run its course, the final
	// status code is what the customer will see. A persistent 401/403
	// here means credentials are genuinely failing — accelerate probes
	// so the probe-driven health signal flips without waiting for the
	// next scheduled probe interval.
	p.maybeAccelerateProbes(ctx, reqType, resp.StatusCode)

	return iface.ProxyResponseFromGentlemen(resp)
}

// ProxyRequestRaw is the streaming raw-proxy path: the caller's inbound
// HTTP body flows directly into the outbound request, the upstream
// response streams back into w with flushing after each successful read,
// and trailers are passed through.
//
// On a 401 we may attempt a single retry, but only if the inbound body
// has not yet been consumed (Resolve succeeded but the body reader is
// still at position zero). For streaming inbound bodies — the common
// case here — once any bytes have been sent the 401 is surfaced to the
// caller. Real SSE / LLM / S3 callers typically send POST bodies from
// buffered or seekable sources, so a more sophisticated rewind path can
// be added later if the streaming-body 401-retry case actually bites.
func (p *proxy) ProxyRequestRaw(ctx context.Context, reqType httpf.RequestType, req *iface.RawProxyRequest, w http.ResponseWriter) error {
	if req == nil || req.Outbound == nil {
		return errors.New("raw proxy request requires an outbound *http.Request")
	}

	client := p.httpf.
		ForRequestType(reqType).
		ForConnection(p.conn).
		ForActor(apauthcore.ActorFromContext(ctx)).
		ForLabels(req.Labels).
		NewHTTPClient()

	// Snapshot of the inbound body so we can issue a single retry if the
	// upstream rejects with 401 *before* any inbound bytes were forwarded.
	// Once forwarding starts the body's read position is unrecoverable;
	// see the function comment for the rationale.
	outbound := req.Outbound.WithContext(ctx)
	originalBody := outbound.Body

	resp, err := p.sendRaw(ctx, client, outbound)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusUnauthorized && canRetryRawAfter401(originalBody) {
		recoverErr := p.auth.RecoverFrom401(ctx)
		if recoverErr == nil {
			drainAndClose(resp.Body)
			retried, retryErr := p.sendRaw(ctx, client, req.Outbound.WithContext(ctx))
			if retryErr == nil {
				resp = retried
			}
		} else if !errors.Is(recoverErr, auth_methods.ErrCannotRecover) {
			_ = recoverErr
		}
	}

	// Same 401/403 acceleration as the wrapped path. See ProxyRequest's
	// comment for the rationale.
	p.maybeAccelerateProbes(ctx, reqType, resp.StatusCode)

	return streamResponse(w, resp)
}

// canRetryRawAfter401 reports whether the inbound body is in a state
// where a retry could replay it. With no body (GET/HEAD/DELETE) retry is
// safe. With a body that's already been consumed we cannot rewind, so we
// surface the 401. http.NoBody is the sentinel net/http uses for the
// "no body" case after the request is built; nil also occurs in
// constructed requests.
func canRetryRawAfter401(body io.ReadCloser) bool {
	if body == nil {
		return true
	}
	if body == http.NoBody {
		return true
	}
	// Conservative default: assume the body has been (or is being)
	// consumed and the retry would not see the same bytes. Real callers
	// that need streaming-body 401-retry will need a rewindable body
	// abstraction — a future PR.
	return false
}

// sendRaw applies the credentials to the outbound request and sends it
// via the supplied client. Split out so the retry-after-401 path can
// build a new request with a fresh body and reuse the same client.
func (p *proxy) sendRaw(ctx context.Context, client *http.Client, outbound *http.Request) (*http.Response, error) {
	app, err := p.auth.Resolve(ctx)
	if err != nil {
		return nil, err
	}

	// Caller-supplied headers are already on outbound. Credential headers
	// take precedence (Set, not Add) — same order/precedence as the
	// wrapped path.
	for h, v := range app.Headers {
		outbound.Header.Set(h, v)
	}
	if len(app.QueryParams) > 0 {
		q := outbound.URL.Query()
		for k, v := range app.QueryParams {
			q.Set(k, v)
		}
		outbound.URL.RawQuery = q.Encode()
	}

	return client.Do(outbound)
}

// streamResponse copies the upstream response back to the caller with
// flushing after each read so SSE / chunked-transfer streams reach the
// client incrementally instead of being buffered into a single
// response-completion write. Trailers (declared via the Trailer header
// per RFC 7230 §4.4) are forwarded after the body completes.
func streamResponse(w http.ResponseWriter, resp *http.Response) error {
	defer drainAndClose(resp.Body)

	copyHeaderExceptHopByHop(w.Header(), resp.Header)
	// Announce trailers up front so http.ResponseWriter accepts them
	// after the body is written.
	if len(resp.Trailer) > 0 {
		trailerNames := make([]string, 0, len(resp.Trailer))
		for k := range resp.Trailer {
			trailerNames = append(trailerNames, k)
		}
		w.Header()["Trailer"] = trailerNames
	}
	w.WriteHeader(resp.StatusCode)

	flusher, _ := w.(http.Flusher)
	if _, err := flushingCopy(w, resp.Body, flusher); err != nil {
		return err
	}

	if len(resp.Trailer) > 0 {
		for k, vv := range resp.Trailer {
			for _, v := range vv {
				w.Header().Add(http.TrailerPrefix+k, v)
			}
		}
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

// flushingCopy is io.Copy with a Flush call after each non-empty read.
// Defaults to a 32KiB buffer (same as io.copyBuffer) — the chunk size
// is the SSE event size or the upstream's preferred TCP write size, not
// our concern; we just push whatever lands on the upstream socket
// downstream without waiting for EOF.
func flushingCopy(dst io.Writer, src io.Reader, flusher http.Flusher) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
				if flusher != nil {
					flusher.Flush()
				}
			}
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				return written, nil
			}
			return written, er
		}
	}
}

// copyHeaderExceptHopByHop copies upstream response headers onto the
// downstream ResponseWriter, omitting hop-by-hop headers per RFC 7230
// §6.1. Hop-by-hop headers (and anything listed in the Connection
// header) are scoped to a single connection and must not be forwarded.
func copyHeaderExceptHopByHop(dst, src http.Header) {
	// Headers listed in the Connection header are also hop-by-hop for
	// this hop only.
	hopByConnection := map[string]struct{}{}
	for _, v := range src.Values("Connection") {
		for _, name := range splitCommaTrim(v) {
			hopByConnection[http.CanonicalHeaderKey(name)] = struct{}{}
		}
	}
	for k, vv := range src {
		if isHopByHopHeader(k) {
			continue
		}
		if _, ok := hopByConnection[http.CanonicalHeaderKey(k)]; ok {
			continue
		}
		dst[k] = append([]string(nil), vv...)
	}
}

// hopByHopHeaders enumerated in RFC 7230 §6.1.
var hopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailers":            {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

func isHopByHopHeader(name string) bool {
	_, ok := hopByHopHeaders[http.CanonicalHeaderKey(name)]
	return ok
}

func splitCommaTrim(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			seg := s[start:i]
			// Trim ASCII spaces and tabs.
			for len(seg) > 0 && (seg[0] == ' ' || seg[0] == '\t') {
				seg = seg[1:]
			}
			for len(seg) > 0 && (seg[len(seg)-1] == ' ' || seg[len(seg)-1] == '\t') {
				seg = seg[:len(seg)-1]
			}
			if seg != "" {
				out = append(out, seg)
			}
			start = i + 1
		}
	}
	return out
}

func drainAndClose(rc io.ReadCloser) {
	if rc == nil {
		return
	}
	_, _ = io.Copy(io.Discard, rc)
	_ = rc.Close()
}

// send builds a fresh gentleman request with the resolved credential
// applied and sends it. Split out so the retry-once-after-recover path
// can construct a new request rather than mutate the existing one —
// gentleman requests are single-use (Send panics on the second call).
//
// Order is deliberate: caller-supplied headers go on first (via
// ProxyRequest.Apply), then the authenticator's headers via SetHeader
// so the credential always wins. Same for query params.
func (p *proxy) send(ctx context.Context, reqType httpf.RequestType, req *iface.ProxyRequest) (*gentleman.Response, error) {
	app, err := p.auth.Resolve(ctx)
	if err != nil {
		return nil, err
	}

	r := p.httpf.
		ForRequestType(reqType).
		ForConnection(p.conn).
		ForActor(apauthcore.ActorFromContext(ctx)).
		ForLabels(req.Labels).
		New().
		UseContext(ctx).
		Request()

	req.Apply(r)
	for h, v := range app.Headers {
		r.SetHeader(h, v)
	}
	for k, v := range app.QueryParams {
		r.SetQuery(k, v)
	}
	return r.Do()
}
