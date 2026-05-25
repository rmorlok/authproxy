// Command demo-shell is the SSO stand-in host application for the AuthProxy
// demo environment.
//
// AuthProxy is normally embedded in a host application that handles user
// authentication; the host signs a JWT vouching for the authenticated user
// using an admin signing key registered with AuthProxy, then redirects the
// user to the appropriate AuthProxy UI (marketplace or admin) with the JWT
// as a query parameter. AuthProxy validates the signature against the
// admin's stored public key and establishes a session.
//
// The demo shell short-circuits the "actual auth" step: it presents three
// well-known demo actor identities as a dropdown and signs a JWT for the
// picked one. **Not** something you'd ship to customers — lives only in
// the demo environment.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/rmorlok/authproxy/demos/shell/backend/embed"
	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

// demoActors enumerates the three identities the shell can sign JWTs for.
// The same three are expected to exist as ConfiguredActors on the AuthProxy
// side (the umbrella chart's seed job pre-creates them — see #A11).
var demoActors = map[string]struct{}{
	"demo-admin": {},
	"demo-user":  {},
	"fresh-user": {},
}

// demoDestinations maps the UI's `destination` form field to the env var
// the backend reads to know where to redirect.
var demoDestinations = map[string]string{
	"admin":       "AUTHPROXY_ADMIN_UI_URL",
	"marketplace": "AUTHPROXY_MARKETPLACE_URL",
}

type settings struct {
	addr                string
	adminPrivateKeyPath string
	adminUsername       string
	authUrl             string
	destinationUrls     map[string]string
	devFrontendUrl      string
	tokenTtl            time.Duration
}

func mustGetenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "missing required env var %s\n", key)
		os.Exit(2)
	}
	return v
}

func loadSettings() settings {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8888"
	}

	destURLs := make(map[string]string, len(demoDestinations))
	for dest, envKey := range demoDestinations {
		destURLs[dest] = mustGetenv(envKey)
	}

	return settings{
		addr:                ":" + port,
		adminPrivateKeyPath: mustGetenv("ADMIN_PRIVATE_KEY_PATH"),
		adminUsername:       mustGetenv("ADMIN_USERNAME"),
		// AUTHPROXY_AUTH_URL isn't strictly required for the redirect itself
		// (the destination URLs are what we redirect to) but it's kept here
		// in case future routes need to call back into AuthProxy directly.
		authUrl: os.Getenv("AUTHPROXY_AUTH_URL"),
		// DEV_FRONTEND_URL=http://localhost:5175 makes GET / proxy to a
		// running vite dev server for HMR. Empty in production — frontend
		// is served from the embedded FS.
		devFrontendUrl:  os.Getenv("DEV_FRONTEND_URL"),
		destinationUrls: destURLs,
		tokenTtl:        15 * time.Minute,
	}
}

// signTokenFor mints a JWT signed by the admin keypair, claiming the
// picked actor's external_id. AuthProxy validates the signature against
// the admin's stored public key and — because the admin has trust to
// vouch for arbitrary actors — establishes a session for that actor.
func signTokenFor(s settings, actorExternalId string) (string, error) {
	b := jwt.NewJwtTokenBuilder().
		WithActorExternalId(actorExternalId).
		WithActorSigned().
		WithServiceIds(config.AllServiceIds()).
		WithNonce().
		WithExpiresIn(s.tokenTtl).
		WithPrivateKeyPath(s.adminPrivateKeyPath)

	return b.Token()
}

func ssoHandler(s settings, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}

		actor := strings.TrimSpace(r.FormValue("actor"))
		destination := strings.TrimSpace(r.FormValue("destination"))

		if _, ok := demoActors[actor]; !ok {
			http.Error(w, "unknown actor", http.StatusBadRequest)
			return
		}
		destURL, ok := s.destinationUrls[destination]
		if !ok {
			http.Error(w, "unknown destination", http.StatusBadRequest)
			return
		}

		token, err := signTokenFor(s, actor)
		if err != nil {
			logger.Error("failed to sign token", "err", err, "actor", actor)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		parsed, err := url.Parse(destURL)
		if err != nil {
			logger.Error("invalid destination URL", "err", err, "destination", destination, "url", destURL)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		q := parsed.Query()
		q.Set("auth_token", token)
		parsed.RawQuery = q.Encode()

		logger.Info("signed token, redirecting", "actor", actor, "destination", destination)
		http.Redirect(w, r, parsed.String(), http.StatusSeeOther)
	}
}

// frontendHandler serves the SPA. In production, it serves the embedded
// build at embed/dist/. In dev (DEV_FRONTEND_URL set), it 302-redirects
// to the vite dev server so HMR + source maps work.
func frontendHandler(s settings) http.Handler {
	if s.devFrontendUrl != "" {
		base, err := url.Parse(s.devFrontendUrl)
		if err != nil {
			panic(fmt.Errorf("invalid DEV_FRONTEND_URL %q: %w", s.devFrontendUrl, err))
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			redirectURL := *base
			redirectURL.Path = r.URL.Path
			redirectURL.RawQuery = r.URL.RawQuery
			http.Redirect(w, r, redirectURL.String(), http.StatusTemporaryRedirect)
		})
	}

	root, err := fs.Sub(embed.FS(), ".")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(root))
}

func main() {
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	s := loadSettings()

	mux := http.NewServeMux()
	mux.Handle("POST /sso", ssoHandler(s, logger))
	mux.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	mux.Handle("/", frontendHandler(s))

	srv := &http.Server{
		Addr:              s.addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("demo-shell listening", "addr", s.addr, "dev_frontend", s.devFrontendUrl != "")
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server exited", "err", err)
		os.Exit(1)
	}
}
