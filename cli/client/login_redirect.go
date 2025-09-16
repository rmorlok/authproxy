package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/rmorlok/authproxy/cli/client/config"
	"github.com/rmorlok/authproxy/jwt"
	"github.com/spf13/cobra"
)

const loginRedirectPath = "/login-redirect"

func httpServerForLoginRedirect(
	validRedirectUrl string,
	marketplaceUrl string,
	adminUiUrl string,
	tb jwt.TokenBuilder,
) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {

		if req.Method != "GET" || req.URL.Path != loginRedirectPath {
			configProps := make([]string, 0)
			if marketplaceUrl != "" {
				configProps = append(configProps, "<tt>host_application.initiate_session_url</tt>")
			}

			if adminUiUrl != "" {
				configProps = append(configProps, "<tt>admin_api.ui.initiate_session_url</tt>")
			}

			log.Printf("[404] %s %s", req.Method, req.URL)
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(404)
			w.Write([]byte(fmt.Sprintf(`
<!DOCTYPE html>
<html>
	<head>
		<meta charset="UTF-8">
		<title>Not Found</title>
	</head>
	<body>
		<h1>Not Found</h1>
		<p>Path '%s' does not eixst on this server. Configure %s to '%s'</p>
	</body>
</html>
`, req.URL, strings.Join(configProps, " and "), validRedirectUrl)))
			return
		}

		returnTo := req.URL.Query().Get("return_to")

		if returnTo == "" {
			log.Printf("[400] %s %s", req.Method, req.URL)
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(400)
			w.Write([]byte(`
<!DOCTYPE html>
<html>
	<head>
		<meta charset="UTF-8">
		<title>No Return To Specified</title>
	</head>
	<body>
		<h1>No Return To Specified</h1>
		<p>Request did not include the <tt>return_to</tt> query parameter to specify path to return auth token.</p>
	</body>
</html>
`))
			return
		}

		if (marketplaceUrl != "" && !strings.HasPrefix(returnTo, marketplaceUrl)) &&
			(adminUiUrl != "" && !strings.HasPrefix(returnTo, adminUiUrl)) {
			tmp := make([]string, 0)
			if marketplaceUrl != "" {
				tmp = append(tmp, fmt.Sprintf("marketplace app at '%s'", marketplaceUrl))
			}

			if adminUiUrl != "" {
				tmp = append(tmp, fmt.Sprintf("admin ui at '%s'", adminUiUrl))
			}

			log.Printf("[404] %s %s", req.Method, req.URL)
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(400)
			w.Write([]byte(fmt.Sprintf(`
<!DOCTYPE html>
<html>
	<head>
		<meta charset="UTF-8">
		<title>Invalid Return To</title>
	</head>
	<body>
		<h1>Invalid Return To</h1>
		<p>Requested return to url '%s' is not from the %s</p>
	</body>
</html>
`, returnTo, strings.Join(tmp, " or "))))
			return
		}

		s, err := tb.
			WithExpiresIn(60 * time.Second).
			WithNonce().
			Signer()

		if err != nil {
			panic(err)
		}

		signedReturnTo := s.SignUrlQuery(returnTo)
		log.Printf("[302] %s %s", req.Method, req.URL)
		http.Redirect(w, req, signedReturnTo, http.StatusFound)
	}
}

func cmdMarketplaceLoginRedirect() *cobra.Command {
	var (
		resolver *config.Resolver
		ip       string
		port     int
		proto    string
		pemPath  string
		keyPath  string
	)

	cmd := &cobra.Command{
		Use:   "login-redirect",
		Short: "Login redirect handler for marketplace SPA and/or admin SPA to simulate host application",
		RunE: func(cmd *cobra.Command, args []string) error {
			tb, err := resolver.ResolveBuilder()
			if err != nil {
				return err
			}

			marketplaceUrl, _ := resolver.ResolveMarketplaceUrl()
			adminUiUrl, _ := resolver.ResolveAdminUiUrl()

			if marketplaceUrl == "" && adminUiUrl == "" {
				return fmt.Errorf("neither marketplace url nor admin ui url specified")
			}

			validRedirectUrl := fmt.Sprintf("%s://%s:%d%s", proto, ip, port, loginRedirectPath)
			if marketplaceUrl != "" {
				log.Printf("Configure host_application.initiate_session_url to %s", validRedirectUrl)
			}

			if adminUiUrl != "" {
				log.Printf("Configure admin_api.ui.initiate_session_url to %s", validRedirectUrl)
			}

			server := &http.Server{
				Addr:    fmt.Sprintf("%s:%d", ip, port),
				Handler: http.HandlerFunc(httpServerForLoginRedirect(validRedirectUrl, marketplaceUrl, adminUiUrl, tb)),
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
	cmd.Flags().StringVar(&ip, "ip", "127.0.0.1", "The IP to listen on")
	cmd.Flags().IntVar(&port, "port", 8889, "The port to listen on")
	cmd.Flags().StringVar(&proto, "proto", "http", "The protocol to use (http or https)")
	cmd.Flags().StringVar(&pemPath, "pemPath", "", "The path to the PEM file to use for TLS")
	cmd.Flags().StringVar(&keyPath, "keyPath", "", "The path to the key file to use for TLS")

	return cmd
}
