package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// runToolRead executes a real CLI command with an isolated signing config.
// The command's output is captured separately from error diagnostics.
func runToolRead(t *testing.T, cmd *cobra.Command, endpoint string, args ...string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	// Apply's test helper already writes the configuration and signing key.
	// Skip its input and namespace flags so each read can set its own scope.
	cmd.SetArgs(append(args, executionArgs(t, endpoint)[3:]...))
	err := cmd.Execute()
	return stdout.String(), err
}

// toolReadServer checks the method and signer on every API request.
func toolReadServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		assert.True(t, found)
		assert.Len(t, strings.Split(token, "."), 3)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// toolReadFixture returns a management Tool with a bound connection and revision.
func toolReadFixture(id, connection apid.ID, name, namespace string) string {
	return fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Tool","metadata":{"id":%q,"name":%q,"namespace":%q},"spec":{"connectionRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","id":%q},"description":"List items","verbs":["read"],"inputSchema":{"type":"object"},"proxyHttp":{"method":"GET","url":"https://example.com/items"}},"status":{"revision":1}}`, id, name, namespace, connection)
}

// toolSetReadFixture returns a logical or generation-specific ToolSet view.
func toolSetReadFixture(id apid.ID, name, namespace string, generation uint64, state string) string {
	return fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":"ToolSet","metadata":{"id":%q,"name":%q,"namespace":%q,"generation":%d},"spec":{"connectionSelector":{"matchLabels":{}},"release":{"desiredState":"primary"},"definition":{"source":{"explicit":{"tools":[]}}}},"status":{"release":{"state":%q}}}`, id, name, namespace, generation, state)
}

// writeToolReadList serves a typed management resource list with an opaque cursor.
func writeToolReadList(w http.ResponseWriter, kind, cursor string, items ...string) {
	fmt.Fprintf(w, `{"apiVersion":"authproxy.net/v1alpha1","kind":%q,"metadata":{"continue":%q},"items":[%s]}`, kind, cursor, strings.Join(items, ","))
}

// TestListToolsReadsEveryPage verifies query names, cursor handling, and the
// default output shape using the same list command a shell user runs.
func TestListToolsReadsEveryPage(t *testing.T) {
	connection := apid.New(apid.PrefixConnection)
	first := toolReadFixture(apid.New(apid.PrefixTool), connection, "list-items", "root.product")
	second := toolReadFixture(apid.New(apid.PrefixTool), connection, "list-items", "root.product.team")
	const cursor = "A+/Z="
	var requests atomic.Int32

	endpoint := toolReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/tools", r.URL.Path)
		switch requests.Add(1) {
		case 1:
			// Send the full filter set only on the first page.
			assert.Equal(t, url.Values{
				"namespace": {"root.product.**"}, "name": {"list-items"}, "labelSelector": {"provider=inventory"},
				"orderBy": {"name desc"}, "connectionId": {connection.String()},
			}, r.URL.Query())
			writeToolReadList(w, "ToolList", cursor, first)
		case 2:
			assert.Equal(t, url.Values{"cursor": {cursor}}, r.URL.Query())
			writeToolReadList(w, "ToolList", "", second)
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	output, err := runToolRead(t, cmdList(), endpoint, "tools", "-n", "root.product.**", "--name", "list-items",
		"-l", "provider=inventory", "--order", "name desc", "--connection", connection.String())
	require.NoError(t, err)
	require.EqualValues(t, 2, requests.Load())
	require.JSONEq(t, "["+first+","+second+"]", output)
}

// TestListToolSetsOutput checks both the default JSON array and resource
// projections through name and template output.
func TestListToolSetsOutput(t *testing.T) {
	id := apid.New(apid.PrefixToolSet)
	resource := toolSetReadFixture(id, "inventory", "root.team", 2, "primary")
	endpoint := toolReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/toolsets", r.URL.Path)
		assert.Equal(t, url.Values{"namespace": {"root.team"}}, r.URL.Query())
		writeToolReadList(w, "ToolSetList", "", resource)
	})

	output, err := runToolRead(t, cmdList(), endpoint, "toolsets", "--namespace=root.team")
	require.NoError(t, err)
	require.JSONEq(t, "["+resource+"]", output)

	for _, tc := range []struct {
		format, want string
	}{
		{"name", "toolset/" + id.String()},
		{"go-template={{range .}}{{.metadata.name}}{{end}}", "inventory"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			output, err := runToolRead(t, cmdList(), endpoint, "toolsets", "--namespace=root.team", "-o", tc.format)
			require.NoError(t, err)
			require.Equal(t, tc.want, strings.TrimSpace(output))
		})
	}
}

// TestListToolsRejectsBadContinuation verifies that a failed second page
// does not print a successful first page or an incomplete resource list.
func TestListToolsRejectsBadContinuation(t *testing.T) {
	tool := toolReadFixture(apid.New(apid.PrefixTool), apid.New(apid.PrefixConnection), "inventory", "root")
	for _, tc := range []struct {
		name, body string
	}{
		{"wrong kind", `{"apiVersion":"authproxy.net/v1alpha1","kind":"ToolSetList","metadata":{},"items":[]}`},
		{"incomplete", `{"apiVersion":"authproxy.net/v1alpha1","kind":"ToolList","metadata":{"remainingItemCount":1},"items":[]}`},
		{"malformed", `{"apiVersion":"authproxy.net/v1alpha1","kind":"ToolList","items":[`},
		{"repeated cursor", `{"apiVersion":"authproxy.net/v1alpha1","kind":"ToolList","metadata":{"continue":"next"},"items":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			endpoint := toolReadServer(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v1/tools", r.URL.Path)
				if requests.Add(1) == 1 {
					writeToolReadList(w, "ToolList", "next", tool)
					return
				}
				assert.Equal(t, url.Values{"cursor": {"next"}}, r.URL.Query())
				fmt.Fprint(w, tc.body)
			})

			output, err := runToolRead(t, cmdList(), endpoint, "tools")
			require.Error(t, err)
			require.Empty(t, output)
			require.EqualValues(t, 2, requests.Load())
		})
	}
}

// TestGetToolByID verifies default JSON, YAML, and JSONPath address the resource
// itself, including its revision status.
func TestGetToolByID(t *testing.T) {
	id := apid.New(apid.PrefixTool)
	resource := toolReadFixture(id, apid.New(apid.PrefixConnection), "inventory", "root")
	endpoint := toolReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/tools/"+id.String(), r.URL.Path)
		assert.Empty(t, r.URL.Query())
		fmt.Fprint(w, resource)
	})

	output, err := runToolRead(t, cmdGet(), endpoint, "tools", id.String())
	require.NoError(t, err)
	require.JSONEq(t, resource, output)

	output, err = runToolRead(t, cmdGet(), endpoint, "tools", id.String(), "-o", "yaml")
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(output), &result))
	require.Equal(t, "Tool", result["kind"])
	require.Equal(t, id.String(), result["metadata"].(map[string]any)["id"])
	require.Equal(t, 1, result["status"].(map[string]any)["revision"])

	output, err = runToolRead(t, cmdGet(), endpoint, "tools", id.String(), "-o", "jsonpath={.status.revision}")
	require.NoError(t, err)
	require.Equal(t, "1", strings.TrimSpace(output))
}

// TestGetToolSetGenerationByName verifies exact namespace selection before
// requesting the specified generation by the resolved logical ID.
func TestGetToolSetGenerationByName(t *testing.T) {
	id := apid.New(apid.PrefixToolSet)
	logical := toolSetReadFixture(id, "inventory", "root.team", 7, "primary")
	historical := toolSetReadFixture(id, "inventory", "root.team", 3, "active")
	descendant := toolSetReadFixture(apid.New(apid.PrefixToolSet), "inventory", "root.team.child", 1, "primary")
	var requests atomic.Int32

	endpoint := toolReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch requests.Add(1) {
		case 1:
			// The list endpoint can include descendants with the same name.
			assert.Equal(t, "/api/v1/toolsets", r.URL.Path)
			assert.Equal(t, url.Values{"name": {"inventory"}, "namespace": {"root.team"}}, r.URL.Query())
			writeToolReadList(w, "ToolSetList", "NEXT+/=", descendant)
		case 2:
			assert.Equal(t, "/api/v1/toolsets", r.URL.Path)
			assert.Equal(t, url.Values{"cursor": {"NEXT+/="}}, r.URL.Query())
			writeToolReadList(w, "ToolSetList", "", logical)
		case 3:
			assert.Equal(t, "/api/v1/toolsets/"+id.String()+"/generations/3", r.URL.Path)
			assert.Empty(t, r.URL.Query())
			fmt.Fprint(w, historical)
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	output, err := runToolRead(t, cmdGet(), endpoint, "toolsets", "inventory", "-n", "root.team", "--generation=3")
	require.NoError(t, err)
	require.EqualValues(t, 3, requests.Load())
	require.JSONEq(t, historical, output)
}

// TestGetToolResourcesRejectInvalidTargets verifies malformed addressing
// fails before the CLI performs any API requests.
func TestGetToolResourcesRejectInvalidTargets(t *testing.T) {
	endpoint := toolReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected read for invalid target: %s", r.URL)
		w.WriteHeader(http.StatusInternalServerError)
	})
	for _, args := range [][]string{
		{"tools", apid.New(apid.PrefixConnection).String(), "--namespace=root"},
		{"tools", "inventory"},
		{"toolsets", "inventory", "--namespace=root", "--generation=0"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			output, err := runToolRead(t, cmdGet(), endpoint, args...)
			require.Error(t, err)
			require.Empty(t, output)
		})
	}
}
