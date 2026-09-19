package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apply"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func executionArgs(t *testing.T, url string) []string {
	t.Helper()

	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.yaml")
	keyPath := filepath.Join(directory, "key")
	require.NoError(t, os.WriteFile(configPath, []byte("{}"), 0600))
	require.NoError(t, os.WriteFile(keyPath, []byte("test-signing-secret"), 0600))

	return []string{"-f", "-", "--namespace=root", "--config=" + configPath, "--secretKeyPath=" + keyPath, "--actorId=" + apid.New(apid.PrefixActor).String(), "--apiUrl=" + url}
}

func executionServer(t *testing.T, failName string, existing bool) (*httptest.Server, *int) {
	t.Helper()

	writes := 0
	id := apid.New(apid.PrefixActor).String()
	live := fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Actor","metadata":{"id":%q,"name":"bob","namespace":"root"},"spec":{"externalId":"subject"}}`, id)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "Bearer "))
		if r.Method == "GET" {
			switch r.URL.Path {
			case "/api/v1/namespaces/root":
				fmt.Fprint(w, `{"apiVersion":"authproxy.net/v1alpha1","kind":"Namespace","metadata":{"id":"root","name":"root"},"spec":{}}`)
			case "/api/v1/actors":
				items := ""
				if existing {
					items = live
				}
				fmt.Fprintf(w, `{"apiVersion":"authproxy.net/v1alpha1","kind":"ActorList","metadata":{},"items":[%s]}`, items)
			default:
				t.Errorf("unexpected read: %s", r.URL.Path)
				w.WriteHeader(404)
			}

			return
		}

		writes++

		var resource map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&resource))

		metadata := resource["metadata"].(map[string]any)
		if metadata["name"] == failName {
			w.WriteHeader(500)
			fmt.Fprint(w, `{"message":"sensitive-error"}`)
			return
		}

		require.Contains(t, metadata["annotations"], apply.LastAppliedAnnotation)

		metadata["id"] = id
		spec := resource["spec"].(map[string]any)
		delete(spec, "signingKey")

		require.NoError(t, json.NewEncoder(w).Encode(resource))
	}))

	t.Cleanup(server.Close)

	return server, &writes
}

func TestApplyExecutesSignedRequestsWithStructuredResults(t *testing.T) {
	for _, format := range []string{"json", "yaml", "name", ""} {
		t.Run(format, func(t *testing.T) {
			server, writes := executionServer(t, "", false)

			cmd := cmdApply()

			var out, stderr bytes.Buffer
			cmd.SetIn(strings.NewReader(strings.Replace(applyActor, "spec: {}", `spec: {externalId: subject, signingKey: {sharedKey: {value: secret-material}}}`, 1)))
			cmd.SetOut(&out)
			cmd.SetErr(&stderr)

			args := executionArgs(t, server.URL)
			if format != "" {
				args = append(args, "-o", format)
			}
			cmd.SetArgs(args)

			require.NoError(t, cmd.Execute())
			require.Equal(t, 1, *writes)
			require.NotContains(t, out.String(), "secret-material")
			require.NotContains(t, out.String(), "test-signing-secret")

			if format == "json" {
				var results []apply.Result
				require.NoError(t, json.Unmarshal(out.Bytes(), &results))
				require.Len(t, results, 1)
				require.Equal(t, "created", results[0].Status)
			}

			if format == "yaml" {
				var result apply.Result
				require.NoError(t, yaml.Unmarshal(out.Bytes(), &result))
				require.Equal(t, "created", result.Status)
			}

			if format == "name" {
				require.True(t, strings.HasPrefix(out.String(), "actor/act_"))
				require.NotContains(t, out.String(), "created")
			}
		})
	}
}

func TestApplyStructuredPartialFailureAndOutputFailure(t *testing.T) {
	t.Run("partial failure", func(t *testing.T) {
		server, writes := executionServer(t, "bad", false)

		cmd := cmdApply()

		var out, stderr bytes.Buffer
		actor := strings.Replace(applyActor, "spec: {}", "spec: {externalId: subject}", 1)
		cmd.SetIn(strings.NewReader(strings.Replace(actor, "bob", "bad", 1) + "---\n" + actor))
		cmd.SetOut(&out)
		cmd.SetErr(&stderr)
		cmd.SetArgs(append(executionArgs(t, server.URL), "-o", "json"))

		require.ErrorIs(t, cmd.Execute(), apply.ErrBatchFailed)
		require.Equal(t, 2, *writes)

		var results []apply.Result
		require.NoError(t, json.Unmarshal(out.Bytes(), &results))
		require.Len(t, results, 2)
		require.Equal(t, "failed", results[0].Status)
		require.Equal(t, "created", results[1].Status)
		require.NotContains(t, out.String(), "sensitive-error")
	})

	t.Run("output error after write", func(t *testing.T) {
		server, writes := executionServer(t, "", false)

		cmd := cmdApply()

		cmd.SetIn(strings.NewReader(strings.Replace(applyActor, "spec: {}", "spec: {externalId: subject}", 1)))
		cmd.SetOut(applyFailWriter{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(append(executionArgs(t, server.URL), "-o", "json"))

		require.ErrorContains(t, cmd.Execute(), "successful writes remain applied")
		require.Equal(t, 1, *writes)
	})

	t.Run("warning error before write", func(t *testing.T) {
		server, writes := executionServer(t, "", true)

		cmd := cmdApply()

		cmd.SetIn(strings.NewReader(applyActor))
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(applyFailWriter{})
		cmd.SetArgs(executionArgs(t, server.URL))

		require.ErrorContains(t, cmd.Execute(), "output unavailable")
		require.Zero(t, *writes)
	})
}

func TestApplyClientDryRunNeverResolvesConfigOrCluster(t *testing.T) {
	cmd := cmdApply()

	var out bytes.Buffer

	cmd.SetIn(strings.NewReader(applyActor))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"-f", "-", "-n", "root", "--dry-run=client", "--config=/nonexistent/apply-test-config", "--apiUrl=http://127.0.0.1:1"})

	require.NoError(t, cmd.Execute())
	require.Contains(t, out.String(), "validated (client dry run)")
}
