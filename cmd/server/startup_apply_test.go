package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/cli/apply"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

const startupNamespace = `{"apiVersion":"authproxy.net/v1alpha1","kind":"Namespace","metadata":{"id":"root","name":"root"},"spec":{},"status":{"state":"active"}}`

func startupTestConfig(t *testing.T, port int) {
	t.Helper()
	original := cfg
	t.Cleanup(func() { cfg = original })
	cfg = config.FromRoot(&sconfig.Root{
		Api:        sconfig.ServiceApi{ServiceHttp: sconfig.ServiceHttp{PortVal: common.NewIntegerValueDirect(int64(port))}},
		AdminApi:   sconfig.ServiceAdminApi{ServiceHttp: sconfig.ServiceHttp{PortVal: common.NewIntegerValueDirect(int64(port))}},
		SystemAuth: sconfig.SystemAuth{GlobalAESKey: &sconfig.KeyData{InnerVal: &sconfig.KeyDataBase64Val{Base64: "c3RhcnR1cC1hcHBseS10ZXN0LXNlY3JldA=="}}},
	})
}

func startupManifest(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "resources.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`apiVersion: authproxy.net/v1alpha1
kind: Namespace
metadata:
  id: root
  labels:
    team: startup
spec: {}
`), 0600))
	return path
}

func startupOptions(t *testing.T) startupApplyOptions {
	return startupApplyOptions{filenames: []string{startupManifest(t)}, actor: "operator", actorNamespace: "root.ops", timeout: time.Second}
}

func TestStartupApplyWaitsAndSignsAsExistingActor(t *testing.T) {
	var reads, writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		claims, err := jwt.NewJwtTokenParserBuilder().WithSharedKey([]byte("startup-apply-test-secret")).Parse(token)
		if !assertStartupClaims(t, claims, err) {
			w.WriteHeader(400)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if reads.Add(1) == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			fmt.Fprint(w, startupNamespace)
		default:
			writes.Add(1)
			fmt.Fprint(w, startupNamespace)
		}
	}))
	defer server.Close()
	_, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	startupTestConfig(t, port)
	cfg.GetRoot().AdminApi.BaseUrl = common.NewStringValueDirect("https://never-contact.example")
	a, err := prepareStartupApply(context.Background(), "all", startupOptions(t))
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, a.run(context.Background(), &output))
	require.GreaterOrEqual(t, reads.Load(), int32(2))
	require.Equal(t, int32(1), writes.Load())
	require.Contains(t, output.String(), "Namespace root")
}

func assertStartupClaims(t *testing.T, claims *jwt.AuthProxyClaims, err error) bool {
	t.Helper()
	if err != nil {
		t.Error(err)
		return false
	}
	require.True(t, claims.SystemSigned)
	require.Equal(t, "operator", claims.Subject)
	require.Equal(t, "root.ops", claims.GetNamespace())
	require.Contains(t, claims.Audience, "admin-api")
	require.Nil(t, claims.Actor)
	require.Empty(t, claims.Permissions)
	require.NotNil(t, claims.ExpiresAt)
	return true
}

func TestStartupApplyRejectsInvalidOptionsBeforeStartup(t *testing.T) {
	startupTestConfig(t, 8081)
	for _, tc := range []struct {
		name, services, want string
		mutate               func(*startupApplyOptions)
	}{
		{"missing actor", "all", "--apply-actor", func(o *startupApplyOptions) { o.actor = "" }},
		{"worker only", "worker", "requires serving", func(o *startupApplyOptions) {}},
		{"zero timeout", "all", "positive", func(o *startupApplyOptions) { o.timeout = 0 }},
		{"stdin", "all", "stdin", func(o *startupApplyOptions) { o.filenames = []string{"-"} }},
		{"missing namespace", "all", "path is required", func(o *startupApplyOptions) { o.actorNamespace = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := startupOptions(t)
			tc.mutate(&o)
			_, err := prepareStartupApply(context.Background(), tc.services, o)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestStartupApplyDoesNotRetryFailedWrites(t *testing.T) {
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprint(w, startupNamespace)
			return
		}
		writes.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	_, portText, _ := net.SplitHostPort(server.Listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	startupTestConfig(t, port)
	a, err := prepareStartupApply(context.Background(), "api", startupOptions(t))
	require.NoError(t, err)
	err = a.run(context.Background(), io.Discard)
	require.Error(t, err)
	require.Equal(t, int32(1), writes.Load())
}

func TestStartupApplyReadinessTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	_, portText, _ := net.SplitHostPort(server.Listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	startupTestConfig(t, port)
	o := startupOptions(t)
	o.timeout = 20 * time.Millisecond
	a, err := prepareStartupApply(context.Background(), "api", o)
	require.NoError(t, err)
	require.ErrorIs(t, a.run(context.Background(), io.Discard), context.DeadlineExceeded)
}

func TestDevelopmentManifestsLoadStrictly(t *testing.T) {
	for _, name := range []string{"default", "docker"} {
		t.Run(name, func(t *testing.T) {
			docs, err := apply.Load(context.Background(), apply.Options{Filenames: []string{filepath.Join("../../dev_config", name+"-resources.yaml")}, Validation: apply.ValidationStrict})
			require.NoError(t, err)
			require.NotEmpty(t, docs)
			for _, doc := range docs {
				require.NotEmpty(t, doc.Metadata.Namespace)
			}
		})
	}
}

func TestStartupApplyFailureFailsServe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	_, portText, _ := net.SplitHostPort(server.Listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	startupTestConfig(t, port)
	a, err := prepareStartupApply(context.Background(), "api", startupOptions(t))
	require.NoError(t, err)
	original := startServices
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop); startServices = original })
	startServices = func(bool, string) error { <-stop; return nil }
	require.ErrorContains(t, serveWithApply(context.Background(), true, "api", a, io.Discard), "startup apply failed")
}
