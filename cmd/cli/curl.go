package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rmorlok/authproxy/cmd/cli/config"
	"github.com/spf13/cobra"
)

// cmdCurl runs `curl` with its URL rewritten to point at an in-process
// ap proxy listener, so the request travels through the connection's
// /_proxy_raw endpoint. Flag parsing is disabled because curl's flag
// surface clashes with ours — `--connection <id>` must be the literal
// first two args; everything after is forwarded verbatim to curl.
//
//	ap curl --connection cxn_abc https://api.example.com/v1/things -X POST -d @body.json
func cmdCurl() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "curl --connection <id> <curl args>",
		Short:              "Run curl, routing the request through a connection's raw proxy",
		DisableFlagParsing: true,
		Long: `Boots an in-process ap proxy listener, rewrites the supplied URL to
point at it, and shells out to the real curl binary. The original URL's
scheme+host is used as the upstream base so curl's path/query is preserved
verbatim.

--connection <id> must be the first two arguments; everything after is
forwarded to curl unchanged. Reads server / signing config from
~/.authproxy.yaml — for flag-style overrides, run ap proxy separately
and aim curl at it.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: ap curl --connection <id> <curl args>")
			}
			// Allow --help / -h to print our own help text.
			if args[0] == "--help" || args[0] == "-h" {
				return cmd.Help()
			}
			if len(args) < 2 || args[0] != "--connection" {
				return fmt.Errorf("ap curl: --connection <id> must be the first two arguments")
			}
			connectionID := args[1]
			curlArgs := append([]string(nil), args[2:]...)

			resolver := config.NewResolverFromDefaults()
			signer, err := resolver.ResolveSigner()
			if err != nil {
				return fmt.Errorf("ap curl: resolve signer (configure ~/.authproxy.yaml): %w", err)
			}
			apiURL, err := resolver.ResolveApiUrl()
			if err != nil {
				return err
			}
			if apiURL == "" {
				return fmt.Errorf("ap curl: api URL not configured in ~/.authproxy.yaml")
			}

			urlIdx, origURL, err := findURLArg(curlArgs)
			if err != nil {
				return fmt.Errorf("ap curl: %w", err)
			}
			upstreamBase, _ := url.Parse(origURL.Scheme + "://" + origURL.Host)

			listenURL, shutdown, err := startRawProxyListener("127.0.0.1:0", apiURL, connectionID, upstreamBase, signer)
			if err != nil {
				return fmt.Errorf("ap curl: start listener: %w", err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = shutdown(ctx)
			}()

			// Rewrite the URL in-place: keep path/query, swap scheme+host
			// for the local listener.
			listenU, _ := url.Parse(listenURL)
			rewritten := *origURL
			rewritten.Scheme = listenU.Scheme
			rewritten.Host = listenU.Host
			curlArgs[urlIdx] = rewritten.String()

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
		},
	}
	return cmd
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
