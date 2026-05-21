package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rmorlok/authproxy/cmd/cli/config"
	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/spf13/cobra"
)

// rawProxyEnvelopeHeader is the upstream-URL header consumed by
// /_proxy_raw. Spelled in the API's display form; net/http canonicalises
// on read.
const rawProxyEnvelopeHeader = "X-AuthProxy-Upstream-URL"

// hopByHopHeaders is the RFC 7230 §6.1 list. Stripped both directions so
// connection-scoped framing doesn't leak across the proxy hop.
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

// rawProxyHandler returns an http.Handler that forwards each inbound
// request through {apiBaseURL}/api/v1/connections/{connectionID}/_proxy_raw
// with the JWT signer attached and bodies streamed both directions.
//
// If upstreamBase is non-nil, the inbound path+query is appended to it
// to derive X-AuthProxy-Upstream-URL automatically. Otherwise the
// caller must set the header on the inbound request.
func rawProxyHandler(apiBaseURL string, connectionID string, upstreamBase *url.URL, signer jwt.Signer) http.HandlerFunc {
	rawProxyURL := strings.TrimRight(apiBaseURL, "/") + "/api/v1/connections/" + connectionID + "/_proxy_raw"

	return func(w http.ResponseWriter, req *http.Request) {
		upstream := req.Header.Get(rawProxyEnvelopeHeader)
		if upstream == "" && upstreamBase != nil {
			u := *upstreamBase
			inbound := req.URL
			if strings.HasSuffix(u.Path, "/") && strings.HasPrefix(inbound.Path, "/") {
				u.Path += strings.TrimPrefix(inbound.Path, "/")
			} else {
				u.Path += inbound.Path
			}
			u.RawQuery = inbound.RawQuery
			upstream = u.String()
		}
		if upstream == "" {
			http.Error(w, "ap proxy: no upstream — set "+rawProxyEnvelopeHeader+" on the request or launch with --upstream-base", http.StatusBadGateway)
			return
		}

		outbound, err := http.NewRequestWithContext(req.Context(), req.Method, rawProxyURL, req.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("ap proxy: build outbound request: %s", err), http.StatusInternalServerError)
			return
		}
		// Copy inbound headers (minus hop-by-hop / Host) onto the outbound
		// request. The envelope header is overlaid below so a derived
		// upstream-base value wins over whatever the caller sent.
		for k, vv := range req.Header {
			canon := http.CanonicalHeaderKey(k)
			if _, ok := hopByHopHeaders[canon]; ok {
				continue
			}
			if canon == "Host" || canon == http.CanonicalHeaderKey(rawProxyEnvelopeHeader) {
				continue
			}
			outbound.Header[canon] = append([]string(nil), vv...)
		}
		outbound.Header.Set(rawProxyEnvelopeHeader, upstream)
		// Forward the inbound Content-Length so the chunked-vs-known
		// framing decision propagates end-to-end. -1 means chunked.
		outbound.ContentLength = req.ContentLength
		signer.SignAuthHeader(outbound)

		resp, err := http.DefaultTransport.RoundTrip(outbound)
		if err != nil {
			http.Error(w, fmt.Sprintf("ap proxy: %s", err), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		log.Printf("[%d] %s %s", resp.StatusCode, req.Method, upstream)

		for k, vv := range resp.Header {
			canon := http.CanonicalHeaderKey(k)
			if _, ok := hopByHopHeaders[canon]; ok {
				continue
			}
			for _, v := range vv {
				w.Header().Add(canon, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		// Flush after headers so SSE clients see the response open
		// before any body bytes arrive.
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = io.Copy(&flushWriter{w: w}, resp.Body)
	}
}

// flushWriter calls Flush after every Write so chunked / SSE bodies
// tick out as the upstream emits them rather than being held in the
// server's default bufio.Writer.
type flushWriter struct {
	w http.ResponseWriter
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if f, ok := fw.w.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}

// startRawProxyListener boots a streaming reverse-proxy listener on
// addr (use ":0" for an ephemeral port). Returns the bound URL and a
// shutdown function. Used by both modes of `ap proxy`: long-running
// (--port) and one-shot (the `curl ...` positional form).
func startRawProxyListener(addr, apiBaseURL, connectionID string, upstreamBase *url.URL, signer jwt.Signer) (string, func(context.Context) error, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, err
	}
	srv := &http.Server{
		Handler: rawProxyHandler(apiBaseURL, connectionID, upstreamBase, signer),
		// Disable HTTP/2 — h2's framing rules collide with the
		// streaming semantics we're trying to preserve.
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("ap proxy listener: %v", err)
		}
	}()
	return "http://" + ln.Addr().String(), srv.Shutdown, nil
}

func cmdProxy() *cobra.Command {
	var (
		resolver     *config.Resolver
		connectionID string
		upstreamBase string
		ip           string
		port         int
	)

	cmd := &cobra.Command{
		Use:   "proxy --connection <id> [--upstream-base <url>] [curl <curl args>]",
		Short: "Reverse proxy that forwards requests through a connection's /_proxy_raw endpoint",
		Long: `Boots a streaming reverse-proxy through the connection's /_proxy_raw
endpoint. Bodies stream both directions so chunked uploads and SSE
responses pass through without buffering.

Two modes:

  Long-running listener (default):
    ap proxy --connection <id> --upstream-base https://api.example.com
    Listens on --port (default 9999). Each inbound request derives its
    X-AuthProxy-Upstream-URL by appending path+query to --upstream-base,
    or the caller may set the header explicitly (then --upstream-base
    is optional).

  One-shot curl:
    ap proxy --connection <id> curl https://api.example.com/v1/things -X POST -d @body.json
    Boots an ephemeral-port listener, derives --upstream-base from the
    URL's scheme+host, rewrites the URL to point at the listener, and
    shells out to real curl with every arg after "curl" forwarded
    verbatim. All ap proxy flags must come before the literal "curl".`,
		// SetInterspersed(false) is what makes the `curl ...` tail
		// work: cobra stops flag parsing at the first positional, so
		// curl's own flags (-X, -d, -H, --config, …) reach us as args
		// without colliding with ours.
		RunE: func(cmd *cobra.Command, args []string) error {
			signer, err := resolver.ResolveSigner()
			if err != nil {
				return err
			}
			apiURL, err := resolver.ResolveApiUrl()
			if err != nil {
				return err
			}
			if apiURL == "" {
				return fmt.Errorf("api URL not configured — set --apiUrl or server.api in ~/.authproxy.yaml")
			}

			var base *url.URL
			if upstreamBase != "" {
				base, err = parseUpstreamBase(upstreamBase)
				if err != nil {
					return err
				}
			}

			// `curl` discriminator: anything after it is curl's argv.
			if len(args) > 0 && args[0] == "curl" {
				return runProxyCurl(apiURL, connectionID, base, signer, args[1:])
			}
			if len(args) > 0 {
				return fmt.Errorf("ap proxy: unexpected positional %q (only `curl <args>` is supported)", args[0])
			}

			return runProxyListener(apiURL, connectionID, base, signer, ip, port)
		},
	}

	resolver = config.WithConfigParams(cmd)
	cmd.Flags().StringVar(&connectionID, "connection", "", "Connection ID to proxy through (cxn_…)")
	cmd.Flags().StringVar(&upstreamBase, "upstream-base", "", "Optional upstream base URL; inbound path+query is appended to derive X-AuthProxy-Upstream-URL. In `curl` mode this is auto-derived from the URL when omitted.")
	cmd.Flags().StringVar(&ip, "ip", "127.0.0.1", "IP to listen on (long-running mode)")
	cmd.Flags().IntVar(&port, "port", 9999, "Port to listen on (long-running mode)")
	cmd.MarkFlagRequired("connection")
	// Stop flag parsing at the first non-flag arg so the `curl …` tail
	// reaches RunE untouched.
	cmd.Flags().SetInterspersed(false)
	return cmd
}

func parseUpstreamBase(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("invalid --upstream-base: %w", err)
	}
	if !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("--upstream-base must be an absolute http(s) URL")
	}
	return u, nil
}

// runProxyListener is the long-running mode. Blocks on the HTTP server
// until interrupted.
func runProxyListener(apiURL, connectionID string, base *url.URL, signer jwt.Signer, ip string, port int) error {
	addr := fmt.Sprintf("%s:%d", ip, port)
	log.Printf("ap proxy listening on %s", addr)
	log.Printf("forwarding through %s/api/v1/connections/%s/_proxy_raw", strings.TrimRight(apiURL, "/"), connectionID)
	if base != nil {
		log.Printf("upstream-base: %s", base.String())
	} else {
		log.Printf("no --upstream-base; callers must set %s on each inbound request", rawProxyEnvelopeHeader)
	}
	server := &http.Server{
		Addr:         addr,
		Handler:      rawProxyHandler(apiURL, connectionID, base, signer),
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}
	return server.ListenAndServe()
}

// runProxyCurl is the one-shot mode. Boots an ephemeral listener,
// rewrites the URL in curlArgs to point at it (preserving path+query),
// and shells out to real curl. If --upstream-base wasn't supplied
// explicitly, it's derived from the URL's scheme+host so curl's view
// of the request is unchanged.
func runProxyCurl(apiURL, connectionID string, base *url.URL, signer jwt.Signer, curlArgs []string) error {
	urlIdx, origURL, err := findURLArg(curlArgs)
	if err != nil {
		return fmt.Errorf("ap proxy curl: %w", err)
	}
	if base == nil {
		base, _ = url.Parse(origURL.Scheme + "://" + origURL.Host)
	}

	listenURL, shutdown, err := startRawProxyListener("127.0.0.1:0", apiURL, connectionID, base, signer)
	if err != nil {
		return fmt.Errorf("ap proxy curl: start listener: %w", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = shutdown(ctx)
	}()

	listenU, _ := url.Parse(listenURL)
	rewritten := *origURL
	rewritten.Scheme = listenU.Scheme
	rewritten.Host = listenU.Host

	// Replace the URL in argv. For --url=<value> the whole arg gets the
	// flag prefix back; for --url <value> and positional we replace the
	// single arg in place.
	curlArgs = append([]string(nil), curlArgs...)
	switch {
	case strings.HasPrefix(curlArgs[urlIdx], "--url="):
		curlArgs[urlIdx] = "--url=" + rewritten.String()
	default:
		curlArgs[urlIdx] = rewritten.String()
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	ccmd := exec.CommandContext(ctx, "curl", curlArgs...)
	ccmd.Stdin = os.Stdin
	ccmd.Stdout = os.Stdout
	ccmd.Stderr = os.Stderr
	if err := ccmd.Run(); err != nil {
		// Propagate curl's exit code so callers can script on it.
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		return err
	}
	return nil
}

// findURLArg locates the first http(s) URL inside the curl arg list,
// returning its index and the parsed URL. Recognises both positional
// URLs and `--url <url>` / `--url=<url>`.
func findURLArg(args []string) (int, *url.URL, error) {
	for i, a := range args {
		// `--url <value>` — the value is at i+1.
		if a == "--url" && i+1 < len(args) {
			if u, ok := parseAbsHTTP(args[i+1]); ok {
				return i + 1, u, nil
			}
		}
		// `--url=<value>` — value is the suffix.
		if strings.HasPrefix(a, "--url=") {
			if u, ok := parseAbsHTTP(strings.TrimPrefix(a, "--url=")); ok {
				return i, u, nil
			}
		}
		// Bare positional URL.
		if !strings.HasPrefix(a, "-") {
			if u, ok := parseAbsHTTP(a); ok {
				return i, u, nil
			}
		}
	}
	return -1, nil, fmt.Errorf("could not find an http(s) URL in the curl args")
}

func parseAbsHTTP(s string) (*url.URL, bool) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, false
	}
	if !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, false
	}
	return u, true
}
