// Package proxy orchestrates a single proxied request through a connection:
// resolve credentials via the auth method's Authenticator, send the request
// through the httpf client (which carries rate-limit / telemetry /
// request-events middleware), and on a 401 from the upstream attempt the
// retry-once-after-recover dance. Owns the wrapped (structured)
// ProxyRequest, ProxyRequestStream, and ProxyRequestRaw paths so the
// per-auth-method packages only have to describe "how to apply this
// credential to a request" — not how to drive a proxy call.
package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

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
	logger      *slog.Logger
}

// New constructs an iface.Proxy that orchestrates calls for a single
// connection using the supplied Authenticator. The optional accelerator,
// when non-nil, receives a fire-and-forget EnqueueProbeNow call on
// upstream 401/403 responses so the probe-driven health signal can flip
// without waiting for the next scheduled probe tick. One instance per
// connection — held inside the connection's lazy proxy-impl cache.
func New(h httpf.F, conn iface.Connection, auth auth_methods.Authenticator, accelerator ProbeAccelerator) iface.Proxy {
	return &proxy{httpf: h, conn: conn, auth: auth, accelerator: accelerator, logger: proxyLogger(conn)}
}

type loggedConnection interface {
	Logger() *slog.Logger
}

func proxyLogger(conn iface.Connection) *slog.Logger {
	if connWithLogger, ok := conn.(loggedConnection); ok {
		if logger := connWithLogger.Logger(); logger != nil {
			return logger
		}
	}
	return slog.Default()
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
			p.logUpstreamRetryAttempted(ctx, reqType, resp.StatusCode)
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
	p.logFinalUpstreamStatus(ctx, reqType, resp)

	return iface.ProxyResponseFromGentlemen(resp)
}

// ProxyRequestStream applies connection credentials and returns the upstream
// response without consuming its body. The caller owns the response body and
// must close it, including when it stops reading before EOF.
//
// Ownership of the outgoing body transfers on entry: validation or credential
// resolution failures close it; once sent, the HTTP client closes it. A 401
// permits one credential-recovery retry when the request is bodyless or GetBody
// can provide an independent copy. Bodies are never buffered or rewound here.
func (p *proxy) ProxyRequestStream(ctx context.Context, reqType httpf.RequestType, req *iface.RawProxyRequest) (*http.Response, error) {
	if req == nil || req.Outbound == nil {
		return nil, errors.New("raw proxy request requires an outbound *http.Request")
	}
	if req.Outbound.URL == nil {
		closeRawRequestBody(req.Outbound)
		return nil, errors.New("raw proxy request requires an outbound URL")
	}

	client := p.httpf.
		ForRequestType(reqType).
		ForConnection(p.conn).
		ForActor(apauthcore.ActorFromContext(ctx)).
		ForLabels(req.Labels).
		NewHTTPClient()

	resp, err := p.sendRaw(ctx, client, req.Outbound)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized && canRetryRawAfter401(req.Outbound) {
		if err := ctx.Err(); err != nil {
			_ = resp.Body.Close()
			return nil, err
		}
		recoverErr := p.auth.RecoverFrom401(ctx)
		if err := ctx.Err(); err != nil {
			_ = resp.Body.Close()
			return nil, err
		}
		// Recovery failures preserve the original, still-readable 401. Delay
		// opening a replay body until recovery succeeds so there is no unused
		// upload reader to clean up on that fallback path.
		if recoverErr == nil {
			// The discarded response could itself be an open-ended stream.
			// Close it directly rather than waiting to drain it before retrying.
			_ = resp.Body.Close()
			retry, err := rawRequestForRetry(ctx, req.Outbound)
			if err != nil {
				return nil, err
			}
			p.logUpstreamRetryAttempted(ctx, reqType, resp.StatusCode)
			resp, err = p.sendRaw(ctx, client, retry)
			if err != nil {
				return nil, err
			}
		}
	}

	// Same 401/403 acceleration as the wrapped path. See ProxyRequest's
	// comment for the rationale.
	p.maybeAccelerateProbes(ctx, reqType, resp.StatusCode)
	p.logFinalRawUpstreamStatus(ctx, reqType, resp)

	return resp, nil
}

// ProxyRequestRaw forwards a streaming response into w with flushing after
// each successful read and passes through trailers. ProxyRequestStream owns
// credential application, middleware attribution, and replayable 401 recovery.
func (p *proxy) ProxyRequestRaw(ctx context.Context, reqType httpf.RequestType, req *iface.RawProxyRequest, w http.ResponseWriter) error {
	resp, err := p.ProxyRequestStream(ctx, reqType, req)
	if err != nil {
		return err
	}

	return streamResponse(w, resp)
}

func (p *proxy) logUpstreamRetryAttempted(ctx context.Context, reqType httpf.RequestType, statusCode int) {
	p.logger.InfoContext(ctx, "proxy upstream retry attempted",
		"connection_id", p.conn.GetId().String(),
		"request_type", reqType.String(),
		"provider_status_code", statusCode,
		"reason", "upstream_401",
	)
}

func (p *proxy) logFinalUpstreamStatus(ctx context.Context, reqType httpf.RequestType, resp *gentleman.Response) {
	if resp == nil {
		return
	}
	p.logFinalStatus(ctx, reqType, resp.StatusCode, resp.Header.Get("Retry-After"))
}

func (p *proxy) logFinalRawUpstreamStatus(ctx context.Context, reqType httpf.RequestType, resp *http.Response) {
	if resp == nil {
		return
	}
	p.logFinalStatus(ctx, reqType, resp.StatusCode, resp.Header.Get("Retry-After"))
}

func (p *proxy) logFinalStatus(
	ctx context.Context,
	reqType httpf.RequestType,
	statusCode int,
	retryAfter string,
) {
	switch {
	case statusCode == http.StatusUnauthorized ||
		statusCode == http.StatusForbidden:
		p.logger.WarnContext(ctx, "proxy upstream auth failure",
			"connection_id", p.conn.GetId().String(),
			"request_type", reqType.String(),
			"provider_status_code", statusCode,
		)
	case statusCode == http.StatusTooManyRequests:
		args := []any{
			"connection_id", p.conn.GetId().String(),
			"request_type", reqType.String(),
			"provider_status_code", statusCode,
		}
		if retryAfter != "" {
			args = append(args, "retry_after", retryAfter)
			if seconds, err := strconv.Atoi(retryAfter); err == nil {
				args = append(args, "retry_after_seconds", seconds)
			}
		}
		p.logger.WarnContext(ctx, "proxy upstream rate limited", args...)
	}
}

// canRetryRawAfter401 permits recovery when the request is bodyless or its body
// can be recreated. This is independent of HTTP method: replayability alone
// does not make the operation idempotent, so only an upstream 401 enables retry.
func canRetryRawAfter401(outbound *http.Request) bool {
	return outbound.Body == nil ||
		outbound.Body == http.NoBody ||
		outbound.GetBody != nil
}

// rawRequestForRetry clones an eligible request without changing its framing,
// URL, or headers. An existing body must come from GetBody as an independent
// reader: net/http may still be reading or closing the original upload after
// Do returns, so seeking or reusing that reader would race with the transport.
// The caller owns the returned body until handing it to sendRaw. Factory errors
// or cancellation close any newly created body and never touch the original.
func rawRequestForRetry(
	ctx context.Context,
	outbound *http.Request,
) (*http.Request, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	retry := outbound.Clone(ctx)
	if outbound.Body == nil || outbound.Body == http.NoBody {
		return retry, nil
	}

	var err error
	retry.Body, err = outbound.GetBody()
	if contextErr := ctx.Err(); contextErr != nil {
		closeRawRequestBody(retry)
		return nil, contextErr
	}
	if err != nil {
		closeRawRequestBody(retry)
		return nil, fmt.Errorf("recreate raw proxy request body: %w", err)
	}
	if retry.Body == nil {
		return nil, errors.New("raw proxy request GetBody returned a nil body")
	}
	return retry, nil
}

// closeRawRequestBody closes an outgoing body that has not been handed to the
// HTTP client. It does not consume the stream or report close errors.
func closeRawRequestBody(outbound *http.Request) {
	if outbound.Body != nil {
		_ = outbound.Body.Close()
	}
}

// sendRaw clones the request's URL and headers for each authenticated attempt
// so credential application never mutates the caller's request or carries
// previous credentials into a retry. The body is not cloned and is owned by
// this method until handed to the HTTP client; cancellation or credential
// resolution errors before that handoff close the unsent body.
func (p *proxy) sendRaw(
	ctx context.Context,
	client *http.Client,
	outbound *http.Request,
) (*http.Response, error) {
	outbound = outbound.Clone(ctx)
	if err := ctx.Err(); err != nil {
		closeRawRequestBody(outbound)
		return nil, err
	}
	app, err := p.auth.Resolve(ctx)
	if contextErr := ctx.Err(); contextErr != nil {
		closeRawRequestBody(outbound)
		return nil, contextErr
	}
	if err != nil {
		closeRawRequestBody(outbound)
		return nil, err
	}
	if outbound.Header == nil {
		outbound.Header = make(http.Header)
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
	defer resp.Body.Close()

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
