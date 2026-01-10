package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/cmd/cli/config"
	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/spf13/cobra"
)

func httpProxyForHost(baseUrl url.URL, signer jwt.Signer) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		orig_url := *req.URL

		var path string

		if strings.HasSuffix(baseUrl.Path, "/") {
			path = fmt.Sprintf("%s%s", baseUrl.Path, orig_url.Path[1:])
		} else {
			path = fmt.Sprintf("%s%s", baseUrl.Path, orig_url.Path)
		}

		// Combine the data to create a new URL
		u := url.URL{
			Scheme:   baseUrl.Scheme,
			Opaque:   orig_url.Opaque,
			User:     baseUrl.User,
			Host:     baseUrl.Host,
			Path:     path,
			RawQuery: orig_url.RawQuery,
		}

		// Update the request with the new URL
		r, err := http.NewRequest(req.Method, u.String(), req.Body)
		if err != nil {
			errStr := fmt.Sprintf("Error creating request: %s", err.Error())
			fmt.Fprintln(os.Stderr, errStr)
			writeJsonErrorResponse(w, http.StatusInternalServerError, errStr)
			return
		}

		signer.SignAuthHeader(r)

		resp, err := http.DefaultTransport.RoundTrip(r)
		if err != nil {
			errStr := fmt.Sprintf("Error sending request: %s", err.Error())
			fmt.Fprintln(os.Stderr, errStr)
			writeJsonErrorResponse(w, http.StatusInternalServerError, errStr)
			return
		}

		defer func() {
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
		}()

		log.Printf("[%d] %s %s", resp.StatusCode, req.Method, u.String())
		copyHeader(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func writeJsonErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	errorResponse := struct {
		Error string `json:"error"`
	}{
		Error: message,
	}

	response, _ := json.Marshal(errorResponse)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(response)
}

func cmdRawProxy() *cobra.Command {
	var (
		resolver            *config.Resolver
		proxyTo             string
		enableLoginRedirect bool
		ip                  string
		port                int
		proto               string
		pemPath             string
		keyPath             string
	)

	cmd := &cobra.Command{
		Use:   "raw-proxy",
		Short: "Proxy HTTP calls to the server with signed JWT",
		RunE: func(cmd *cobra.Command, args []string) error {
			signer, err := resolver.ResolveSigner()
			if err != nil {
				return err
			}

			baseUrl := ""
			if strings.HasPrefix(proxyTo, "http") {
				baseUrl = proxyTo
			} else if proxyTo == string(sconfig.ServiceIdApi) {
				baseUrl, err = resolver.ResolveApiUrl()
			} else if proxyTo == string(sconfig.ServiceIdPublic) {
				baseUrl, err = resolver.ResolveAuthUrl()
			} else if proxyTo == string(sconfig.ServiceIdAdminApi) {
				baseUrl, err = resolver.ResolveAdminApiUrl()
			}

			if err != nil {
				return err
			}

			if baseUrl == "" {
				return fmt.Errorf("invalid proxyTo value: %s", proxyTo)
			}

			proxyToUrl, err := url.Parse(baseUrl)
			if err != nil {
				return errors.Wrap(err, "invalid proxyTo value")
			}

			log.Printf("Setting up raw-proxy to the host: %s", proxyTo)
			log.Printf("Serving proxy on %s:%d", ip, port)

			proxyHandler := httpProxyForHost(*proxyToUrl, signer)
			handler := proxyHandler

			if enableLoginRedirect {
				marketplaceUrl, _ := resolver.ResolveMarketplaceUrl()
				adminUiUrl, _ := resolver.ResolveAdminUiUrl()
				tb, err := resolver.ResolveBuilder()
				if err != nil {
					return err
				}

				validRedirectUrl := fmt.Sprintf("%s://%s:%d%s", proto, ip, port, loginRedirectPath)
				if marketplaceUrl != "" {
					log.Printf("Configure host_application.initiate_session_url to %s", validRedirectUrl)
				}

				if adminUiUrl != "" {
					log.Printf("Configure admin_api.ui.initiate_session_url to %s", validRedirectUrl)
				}

				marketplaceRedirectHandler := httpServerForLoginRedirect(validRedirectUrl, marketplaceUrl, adminUiUrl, tb)

				handler = func(w http.ResponseWriter, req *http.Request) {
					if req.URL.Path == loginRedirectPath {
						marketplaceRedirectHandler(w, req)
						return
					}

					proxyHandler(w, req)
				}
			}
			server := &http.Server{
				Addr:    fmt.Sprintf("%s:%d", ip, port),
				Handler: http.HandlerFunc(handler),
				// Disable HTTP/2.
				TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
			}
			if proto == "http" {
				log.Fatal(server.ListenAndServe())
			} else {
				log.Fatal(server.ListenAndServeTLS(pemPath, keyPath))
			}

			return nil
		},
	}

	resolver = config.WithConfigParams(cmd)
	cmd.Flags().StringVar(&proxyTo, "proxyTo", "", "The service name or URL to proxy to")
	cmd.Flags().BoolVar(&enableLoginRedirect, "enableLoginRedirect", false, "Enable the marketplace login and/or admin ui redirect handler, in addition to proxying other calls")
	cmd.Flags().StringVar(&ip, "ip", "127.0.0.1", "The IP to listen on")
	cmd.Flags().IntVar(&port, "port", 8888, "The port to listen on")
	cmd.Flags().StringVar(&proto, "proto", "http", "The protocol to use (http or https)")
	cmd.Flags().StringVar(&pemPath, "pemPath", "", "The path to the PEM file to use for TLS")
	cmd.Flags().StringVar(&keyPath, "keyPath", "", "The path to the key file to use for TLS")

	cmd.MarkFlagRequired("proxyTo")

	return cmd
}
