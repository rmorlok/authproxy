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
	"strings"

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
// shutdown function. Used by both `ap proxy` and `ap curl`.
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
		Use:   "proxy",
		Short: "Reverse proxy that forwards inbound requests through a connection's /_proxy_raw endpoint",
		Long: `Boots a local HTTP listener that forwards each inbound request through
the connection's /_proxy_raw endpoint on the AuthProxy server.

The upstream URL can be supplied two ways:
  - --upstream-base <url>: inbound path+query is appended to derive the
    X-AuthProxy-Upstream-URL header automatically.
  - The caller sets X-AuthProxy-Upstream-URL on each inbound request.

Bodies are streamed both directions so chunked uploads and SSE responses
pass through without buffering.`,
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
				base, err = url.Parse(upstreamBase)
				if err != nil {
					return fmt.Errorf("invalid --upstream-base: %w", err)
				}
				if !base.IsAbs() || (base.Scheme != "http" && base.Scheme != "https") {
					return fmt.Errorf("--upstream-base must be an absolute http(s) URL")
				}
			}

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
		},
	}

	resolver = config.WithConfigParams(cmd)
	cmd.Flags().StringVar(&connectionID, "connection", "", "Connection ID to proxy through (cxn_…)")
	cmd.Flags().StringVar(&upstreamBase, "upstream-base", "", "Optional upstream base URL; inbound path+query is appended to derive X-AuthProxy-Upstream-URL")
	cmd.Flags().StringVar(&ip, "ip", "127.0.0.1", "IP to listen on")
	cmd.Flags().IntVar(&port, "port", 9999, "Port to listen on")
	cmd.MarkFlagRequired("connection")
	return cmd
}
