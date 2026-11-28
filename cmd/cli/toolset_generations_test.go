package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/cli/apply"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toolSetGenerationServer checks signing on both lookup and mutation requests.
func toolSetGenerationServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		assert.True(t, found)
		assert.Len(t, strings.Split(token, "."), 3)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// draftToolSetGenerationFixture returns a ToolSet view with matching desired and
// observed draft states.
func draftToolSetGenerationFixture(id apid.ID, generation uint64) string {
	return strings.Replace(toolSetReadFixture(id, "inventory", "root", generation, "draft"),
		`"desiredState":"primary"`, `"desiredState":"draft"`, 1)
}

// TestToolSetGenerationList follows opaque cursors and emits each generation
// from the same logical ToolSet, in the order supplied by the server.
func TestToolSetGenerationList(t *testing.T) {
	id := apid.New(apid.PrefixToolSet)
	const cursor = "NeXT+/="
	first := draftToolSetGenerationFixture(id, 2)
	second := toolSetReadFixture(id, "inventory", "root", 1, "primary")
	var requests atomic.Int32
	endpoint := toolReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/toolsets/"+id.String()+"/generations", r.URL.Path)
		switch requests.Add(1) {
		case 1:
			assert.Equal(t, url.Values{"orderBy": {"generation desc"}}, r.URL.Query())
			writeToolReadList(w, "ToolSetList", cursor, first)
		case 2:
			// The continuation carries the original order on the server.
			assert.Equal(t, url.Values{"cursor": {cursor}}, r.URL.Query())
			writeToolReadList(w, "ToolSetList", "", second)
		default:
			t.Errorf("unexpected request: %s", r.URL)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	out, err := runToolRead(t, cmdToolSets(), endpoint, "generations", "list", id.String(), "--order", "generation desc")
	require.NoError(t, err)
	require.EqualValues(t, 2, requests.Load())
	require.JSONEq(t, "["+first+","+second+"]", out)
}

// TestToolSetGenerationListRejectsInvalidItems checks that an invalid page does
// not leave the user with a partial list on stdout.
func TestToolSetGenerationListRejectsInvalidItems(t *testing.T) {
	id := apid.New(apid.PrefixToolSet)
	first := draftToolSetGenerationFixture(id, 2)
	for _, tc := range []struct {
		name string
		item string
	}{
		{"missing generation number", draftToolSetGenerationFixture(id, 0)},
		{"repeated generation", first},
		{"different ToolSet", draftToolSetGenerationFixture(apid.New(apid.PrefixToolSet), 3)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			endpoint := toolReadServer(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v1/toolsets/"+id.String()+"/generations", r.URL.Path)
				if requests.Add(1) == 1 {
					writeToolReadList(w, "ToolSetList", "next", first)
					return
				}
				assert.Equal(t, url.Values{"cursor": {"next"}}, r.URL.Query())
				writeToolReadList(w, "ToolSetList", "", tc.item)
			})

			out, err := runToolRead(t, cmdToolSets(), endpoint, "generations", "list", id.String())
			require.Error(t, err)
			require.Empty(t, out)
			require.EqualValues(t, 2, requests.Load())
		})
	}
}

// TestToolSetGenerationCreate sends one empty request to clone the server's
// newest generation. An unqualified ID needs no separate logical read.
func TestToolSetGenerationCreate(t *testing.T) {
	id := apid.New(apid.PrefixToolSet)
	draft := draftToolSetGenerationFixture(id, 2)
	var requests atomic.Int32
	endpoint := toolSetGenerationServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/toolsets/"+id.String()+"/generations", r.URL.Path)
		assert.Empty(t, r.URL.Query())
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Empty(t, body)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, draft)
	})

	out, err := runToolRead(t, cmdToolSets(), endpoint, "generations", "create", id.String())
	require.NoError(t, err)
	require.EqualValues(t, 1, requests.Load())
	require.JSONEq(t, draft, out)
}

// TestToolSetGenerationPublishByName resolves the exact namespace and sends
// only release intent to the generation endpoint of the resolved ToolSet.
func TestToolSetGenerationPublishByName(t *testing.T) {
	id := apid.New(apid.PrefixToolSet)
	logical := toolSetReadFixture(id, "inventory", "root", 1, "primary")
	descendant := toolSetReadFixture(apid.New(apid.PrefixToolSet), "inventory", "root.team", 1, "primary")
	published := toolSetReadFixture(id, "inventory", "root", 2, "primary")
	var requests atomic.Int32
	endpoint := toolSetGenerationServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch requests.Add(1) {
		case 1:
			// A same-name descendant cannot change which ToolSet is published.
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "/api/v1/toolsets", r.URL.Path)
			assert.Equal(t, url.Values{"namespace": {"root"}, "name": {"inventory"}}, r.URL.Query())
			writeToolReadList(w, "ToolSetList", "", descendant, logical)
		case 2:
			assert.Equal(t, http.MethodPatch, r.Method)
			assert.Equal(t, "/api/v1/toolsets/"+id.String()+"/generations/2", r.URL.Path)
			assert.Empty(t, r.URL.Query())
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			body, err := io.ReadAll(r.Body)
			assert.NoError(t, err)
			assert.JSONEq(t, `{"apiVersion":"authproxy.net/v1alpha1","kind":"ToolSet","metadata":{},"spec":{"release":{"desiredState":"primary"}}}`, string(body))
			fmt.Fprint(w, published)
		default:
			t.Errorf("unexpected request: %s", r.URL)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	out, err := runToolRead(t, cmdToolSets(), endpoint, "generations", "publish", "inventory", "-n", "root", "--generation=2")
	require.NoError(t, err)
	require.EqualValues(t, 2, requests.Load())
	require.JSONEq(t, published, out)
}

// TestToolSetGenerationMutationPreflight catches invalid local input before
// a lookup or write reaches the cluster.
func TestToolSetGenerationMutationPreflight(t *testing.T) {
	id := apid.New(apid.PrefixToolSet).String()
	endpoint := toolSetGenerationServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusInternalServerError)
	})
	for _, args := range [][]string{
		{"create", "inventory"},
		{"create", id, "-o", "go-template={{"},
		{"publish", id},
		{"publish", id, "--generation=0"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := runToolRead(t, cmdToolSets(), endpoint, append([]string{"generations"}, args...)...)
			require.Error(t, err)
		})
	}
}

// TestToolSetGenerationIDVerifiesNamespace rejects a write when the supplied
// namespace does not match the ToolSet resolved by ID.
func TestToolSetGenerationIDVerifiesNamespace(t *testing.T) {
	id := apid.New(apid.PrefixToolSet)
	logical := toolSetReadFixture(id, "inventory", "root.team", 1, "primary")
	var requests atomic.Int32
	endpoint := toolReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, "/api/v1/toolsets/"+id.String(), r.URL.Path)
		fmt.Fprint(w, logical)
	})

	out, err := runToolRead(t, cmdToolSets(), endpoint, "generations", "create", id.String(), "-n", "root")
	require.Error(t, err)
	require.Empty(t, out)
	require.EqualValues(t, 1, requests.Load())
}

// TestToolSetGenerationMutationsDoNotReplay returns API and decoding failures
// after one request, even when the server might already have committed a write.
func TestToolSetGenerationMutationsDoNotReplay(t *testing.T) {
	id := apid.New(apid.PrefixToolSet)
	for _, tc := range []struct {
		name       string
		action     string
		method     string
		pathSuffix string
		status     int
		body       string
		diagnostic string
	}{
		{"existing draft", "create", http.MethodPost, "", http.StatusConflict, "", "HTTP 409"},
		{"forbidden publish", "publish", http.MethodPatch, "/2", http.StatusForbidden, "", "HTTP 403"},
		{"bad success response", "create", http.MethodPost, "", http.StatusCreated, "{}", "response"},
		{"create requires draft", "create", http.MethodPost, "", http.StatusCreated, toolSetReadFixture(id, "inventory", "root", 2, "primary"), "did not confirm draft"},
		{"wrong publication generation", "publish", http.MethodPatch, "/2", http.StatusOK, toolSetReadFixture(id, "inventory", "root", 3, "primary"), "response identity"},
		{"historical publication is a no-op", "publish", http.MethodPatch, "/2", http.StatusOK, toolSetReadFixture(id, "inventory", "root", 2, "active"), "did not confirm primary"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			endpoint := toolSetGenerationServer(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, tc.method, r.Method)
				assert.Equal(t, "/api/v1/toolsets/"+id.String()+"/generations"+tc.pathSuffix, r.URL.Path)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			args := []string{"generations", tc.action, id.String()}
			if tc.action == "publish" {
				args = append(args, "--generation=2")
			}

			out, err := runToolRead(t, cmdToolSets(), endpoint, args...)
			require.ErrorContains(t, err, tc.diagnostic)
			if tc.status >= 400 {
				var apiErr *apply.APIError
				require.ErrorAs(t, err, &apiErr)
				require.Equal(t, tc.status, apiErr.StatusCode)
			}
			require.Empty(t, out)
			require.EqualValues(t, 1, requests.Load())
		})
	}

	t.Run("output failure after successful create", func(t *testing.T) {
		var requests atomic.Int32
		endpoint := toolSetGenerationServer(t, func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/api/v1/toolsets/"+id.String()+"/generations", r.URL.Path)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, draftToolSetGenerationFixture(id, 2))
		})
		cmd := cmdToolSets()
		cmd.SetOut(applyFailWriter{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(append([]string{"generations", "create", id.String()}, executionArgs(t, endpoint)[3:]...))

		require.ErrorContains(t, cmd.Execute(), "generation 2 operation succeeded but output failed")
		require.EqualValues(t, 1, requests.Load())
	})
}
