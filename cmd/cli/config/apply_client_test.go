package config

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/apply"
	"github.com/stretchr/testify/require"
)

func TestResolveApplyClientSelectsServiceAndSignsRequests(t *testing.T) {
	for _, admin := range []bool{false, true} {
		t.Run(fmt.Sprint(admin), func(t *testing.T) {
			var apiCalls, adminCalls int
			handler := func(counter *int) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					*counter++
					require.Equal(t, "/api/v1/namespaces/root", r.URL.Path)
					token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
					claims := parseTestToken(t, context.Background(), token, []byte("apply-test-secret"))
					require.Equal(t, "operator", claims.Subject)
					fmt.Fprint(w, `{"apiVersion":"authproxy.net/v1alpha1","kind":"Namespace","metadata":{"id":"root","name":"root"},"spec":{}}`)
				}
			}
			api := httptest.NewServer(handler(&apiCalls))
			defer api.Close()
			adminAPI := httptest.NewServer(handler(&adminCalls))
			defer adminAPI.Close()
			r := &Resolver{admin: admin, actorId: "operator", apis: "all", secretKeyPath: writeTestFile(t, "apply-test-secret"), apiUrl: api.URL, adminApiUrl: adminAPI.URL}
			c, err := r.ResolveApplyClient(apply.DefaultRequestTimeout)
			require.NoError(t, err)
			docs, err := apply.Load(context.Background(), apply.Options{Filenames: []string{"-"}, Stdin: strings.NewReader("apiVersion: authproxy.net/v1alpha1\nkind: Namespace\nmetadata: {id: root}\nspec: {}")})
			require.NoError(t, err)
			_, err = c.Resolve(context.Background(), docs[0])
			require.NoError(t, err)
			if admin {
				require.Equal(t, 1, adminCalls)
				require.Zero(t, apiCalls)
			} else {
				require.Equal(t, 1, apiCalls)
				require.Zero(t, adminCalls)
			}
		})
	}
}

func TestResolveApplyClientDoesNotFallbackFromMissingAdminEndpoint(t *testing.T) {
	r := &Resolver{admin: true, actorId: "operator", apis: "all", secretKeyPath: writeTestFile(t, "apply-test-secret"), apiUrl: "http://localhost:8081", root: &Root{}}
	_, err := r.ResolveApplyClient(apply.DefaultRequestTimeout)
	require.ErrorContains(t, err, "admin API")
}
