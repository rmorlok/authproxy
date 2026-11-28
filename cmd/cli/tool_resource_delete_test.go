package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/cli/apply"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestDeleteToolResourceByID checks that each output mode reports a deletion
// from a single signed request to the logical resource endpoint.
func TestDeleteToolResourceByID(t *testing.T) {
	for _, tc := range []struct {
		collection string
		kind       meta.Kind
		prefix     apid.Prefix
	}{
		{"tools", "Tool", apid.PrefixTool},
		{"toolsets", "ToolSet", apid.PrefixToolSet},
	} {
		t.Run(tc.collection, func(t *testing.T) {
			id := apid.New(tc.prefix).String()
			name := strings.ToLower(string(tc.kind)) + "/" + id
			var requests atomic.Int32
			endpoint := toolSetGenerationServer(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, http.MethodDelete, r.Method)
				assert.Equal(t, "/api/v1/"+tc.collection+"/"+id, r.URL.Path)
				assert.Empty(t, r.URL.Query())
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Empty(t, body)
				// Both a successful deletion and an already absent ID return 204.
				w.WriteHeader(http.StatusNoContent)
			})

			for _, format := range []string{"", "name", "json", "yaml", "jsonpath={range [*]}{.identity}{end}"} {
				t.Run(format, func(t *testing.T) {
					before := requests.Load()
					args := []string{tc.collection, id}
					if format != "" {
						args = append(args, "-o", format)
					}
					out, err := runToolRead(t, cmdDelete(), endpoint, args...)
					require.NoError(t, err)
					require.Equal(t, before+1, requests.Load(), "delete by ID requires no preceding lookup")

					switch format {
					case "":
						require.Equal(t, name+" deleted\n", out)
					case "name":
						require.Equal(t, name+"\n", out)
					case "json", "yaml":
						var result apply.Result
						if format == "json" {
							var results []apply.Result
							require.NoError(t, json.Unmarshal([]byte(out), &results))
							require.Len(t, results, 1)
							result = results[0]
						} else {
							require.NoError(t, yaml.Unmarshal([]byte(out), &result))
						}
						require.Equal(t, tc.kind, result.Kind)
						require.Equal(t, id, result.Identity)
						require.Equal(t, "delete", string(result.Operation))
						require.Equal(t, "deleted", result.Status)
						require.Nil(t, result.Resource)
					default:
						require.Equal(t, id, strings.TrimSpace(out))
					}
				})
			}
		})
	}
}

// TestDeleteToolSetByName verifies that lookup skips same-name descendants and
// deletes the exact match by its pinned ID after all list pages are checked.
func TestDeleteToolSetByName(t *testing.T) {
	id := apid.New(apid.PrefixToolSet)
	descendant := toolSetReadFixture(apid.New(apid.PrefixToolSet), "inventory", "root.team.child", 1, "primary")
	exact := toolSetReadFixture(id, "inventory", "root.team", 2, "primary")
	const cursor = "neXT+/="
	var requests atomic.Int32
	endpoint := toolSetGenerationServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch requests.Add(1) {
		case 1:
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "/api/v1/toolsets", r.URL.Path)
			assert.Equal(t, url.Values{"namespace": {"root.team"}, "name": {"inventory"}}, r.URL.Query())
			writeToolReadList(w, "ToolSetList", cursor, descendant)
		case 2:
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "/api/v1/toolsets", r.URL.Path)
			assert.Equal(t, url.Values{"cursor": {cursor}}, r.URL.Query())
			writeToolReadList(w, "ToolSetList", "", exact)
		case 3:
			assert.Equal(t, http.MethodDelete, r.Method)
			assert.Equal(t, "/api/v1/toolsets/"+id.String(), r.URL.Path)
			assert.Empty(t, r.URL.Query())
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	out, err := runToolRead(t, cmdDelete(), endpoint, "toolsets", "inventory", "-n", "root.team")
	require.NoError(t, err)
	require.Equal(t, "toolset/"+id.String()+" deleted\n", out)
	require.EqualValues(t, 3, requests.Load())
}

// TestDeleteToolResourceIDVerifiesNamespace prevents a delete if the supplied
// namespace and the current resource's exact namespace differ.
func TestDeleteToolResourceIDVerifiesNamespace(t *testing.T) {
	id := apid.New(apid.PrefixTool)
	tool := toolReadFixture(id, apid.New(apid.PrefixConnection), "inventory", "root.team")
	var requests atomic.Int32
	endpoint := toolReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, "/api/v1/tools/"+id.String(), r.URL.Path)
		fmt.Fprint(w, tool)
	})

	out, err := runToolRead(t, cmdDelete(), endpoint, "tools", id.String(), "-n", "root")
	require.Error(t, err)
	require.Empty(t, out)
	require.EqualValues(t, 1, requests.Load(), "an exact namespace mismatch stops before deletion")
}

// TestDeleteToolResourceRejection asserts the API status survives without a
// retry or a successful deletion message.
func TestDeleteToolResourceRejection(t *testing.T) {
	for _, status := range []int{http.StatusConflict, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			id := apid.New(apid.PrefixTool).String()
			var requests atomic.Int32
			endpoint := toolSetGenerationServer(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, http.MethodDelete, r.Method)
				assert.Equal(t, "/api/v1/tools/"+id, r.URL.Path)
				w.WriteHeader(status)
			})

			out, err := runToolRead(t, cmdDelete(), endpoint, "tools", id, "-o", "json")
			var apiErr *apply.APIError
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, status, apiErr.StatusCode)
			require.Empty(t, out)
			require.EqualValues(t, 1, requests.Load())
		})
	}
}

// TestDeleteToolResourceOutputFailure reports an acknowledged deletion even
// when the command cannot print the result.
func TestDeleteToolResourceOutputFailure(t *testing.T) {
	id := apid.New(apid.PrefixTool).String()
	var requests atomic.Int32
	endpoint := toolSetGenerationServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v1/tools/"+id, r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})
	cmd := cmdDelete()
	cmd.SetOut(applyFailWriter{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"tools", id}, executionArgs(t, endpoint)[3:]...))

	err := cmd.Execute()
	require.ErrorContains(t, err, "tool/"+id+" was deleted but output failed")
	require.ErrorContains(t, err, "output unavailable")
	require.EqualValues(t, 1, requests.Load())
}

// TestDeleteToolResourceRejectsInvalidInput checks kind, scope, and output
// validation before the CLI reads or mutates any resource.
func TestDeleteToolResourceRejectsInvalidInput(t *testing.T) {
	id := apid.New(apid.PrefixToolSet).String()
	endpoint := toolSetGenerationServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusInternalServerError)
	})
	for _, args := range [][]string{
		{"tools", "inventory"},
		{"tools", id, "-n", "root"},
		{"toolsets", id, "-n", "root.**"},
		{"toolsets", id, "inventory"},
		{"toolsets", id, "--generation=2"},
		{"toolsets", id, "-o", "go-template={{"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := runToolRead(t, cmdDelete(), endpoint, args...)
			require.Error(t, err)
		})
	}
}
