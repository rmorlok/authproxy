package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/cli/apply"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const generationUpdateYAML = `source:
  explicit:
    tools:
      - key: lookup
        spec:
          description: Look up an item
          verbs: [lookup]
          inputSchema: {type: object}
          proxyHttp:
            method: POST
            url: https://example.com/items
            bodyJson: {itemId: 9007199254740993, options: [null, true]}
`

const generationUpdateJSON = `{"source":{"explicit":{"tools":[{"key":"lookup","spec":{
  "description":"Look up an item","verbs":["lookup"],"inputSchema":{"type":"object"},
  "proxyHttp":{"method":"POST","url":"https://example.com/items","bodyJson":{"itemId":9007199254740993,"options":[null,true]}}
}}]}}}`

// runToolSetGenerationUpdate executes the command with controlled stdin and
// keeps errors separate from the emitted resource.
func runToolSetGenerationUpdate(t *testing.T, endpoint, stdin string, args ...string) (string, error) {
	t.Helper()
	cmd := cmdToolSets()
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader(stdin))
	command := append([]string{"generations", "update"}, args...)
	cmd.SetArgs(append(command, executionArgs(t, endpoint)[3:]...))
	err := cmd.Execute()
	return out.String(), err
}

// TestToolSetGenerationUpdate replaces only the addressed draft's definition.
// Both file and stdin forms preserve opaque JSON embedded in the definition.
func TestToolSetGenerationUpdate(t *testing.T) {
	for _, format := range []string{"YAML on stdin", "JSON file"} {
		t.Run(format, func(t *testing.T) {
			id := apid.New(apid.PrefixToolSet)
			draft := strings.Replace(draftToolSetGenerationFixture(id, 3),
				`{"source":{"explicit":{"tools":[]}}}`, generationUpdateJSON, 1)
			var requests atomic.Int32
			endpoint := toolSetGenerationServer(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, http.MethodPatch, r.Method)
				assert.Equal(t, "/api/v1/toolsets/"+id.String()+"/generations/3", r.URL.Path)
				assert.Empty(t, r.URL.Query())
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"apiVersion":"authproxy.net/v1alpha1","kind":"ToolSet","metadata":{},"spec":{"definition":`+
					generationUpdateJSON+`}}`, string(body))
				// Decode-free assertion detects precision loss beyond float64's
				// exact integer range, which JSON equality could miss.
				assert.Contains(t, string(body), "9007199254740993")
				fmt.Fprint(w, draft)
			})

			input := generationUpdateYAML
			args := []string{id.String(), "--generation=3", "-f", "-"}
			if format == "JSON file" {
				path := filepath.Join(t.TempDir(), "definition.json")
				require.NoError(t, os.WriteFile(path, []byte(generationUpdateJSON), 0600))
				input = ""
				args = []string{id.String(), "--generation=3", "--filename", path}
			}

			out, err := runToolSetGenerationUpdate(t, endpoint, input, args...)
			require.NoError(t, err)
			require.EqualValues(t, 1, requests.Load())
			require.JSONEq(t, draft, out)
			require.Contains(t, out, "9007199254740993")
		})
	}
}

// TestToolSetGenerationUpdateRejectsInvalidInput checks strict definition
// loading and local argument validation before the command reads or writes.
func TestToolSetGenerationUpdateRejectsInvalidInput(t *testing.T) {
	id := apid.New(apid.PrefixToolSet)
	endpoint := toolSetGenerationServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("invalid input sent a request: %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusInternalServerError)
	})
	for _, tc := range []struct {
		name       string
		input      string
		generation string
		diagnostic string
	}{
		{"full manifest", toolSetReadFixture(id, "inventory", "root", 3, "primary"), "3", "contents of spec.definition"},
		{"logical selector", "connectionSelector: {matchLabels: {}}\n" + generationUpdateYAML, "3", "contents of spec.definition"},
		{"multiple documents", generationUpdateYAML + "---\n" + generationUpdateYAML, "3", "exactly one ToolSet definition document"},
		{"duplicate nested key", "source: {explicit: {tools: [], tools: []}}\n", "3", "duplicate mapping key"},
		{"unknown field", "source: {explicit: {tools: [], unknown: true}}\n", "3", "invalid ToolSet definition fields"},
		{"wrong field type", "source: {explicit: {tools: false}}\n", "3", "invalid ToolSet definition fields"},
		{"missing generation", generationUpdateYAML, "", "--generation must be positive"},
		{"zero generation", generationUpdateYAML, "0", "--generation must be positive"},
		{"negative generation", generationUpdateYAML, "-1", "invalid argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{id.String(), "-f", "-"}
			if tc.generation != "" {
				args = append(args, "--generation="+tc.generation)
			}
			_, err := runToolSetGenerationUpdate(t, endpoint, tc.input, args...)
			require.ErrorContains(t, err, tc.diagnostic)
		})
	}
}

// TestToolSetGenerationUpdateConflict verifies that a draft edited or
// published concurrently is reported without replaying the patch.
func TestToolSetGenerationUpdateConflict(t *testing.T) {
	id := apid.New(apid.PrefixToolSet)
	var requests atomic.Int32
	endpoint := toolSetGenerationServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v1/toolsets/"+id.String()+"/generations/3", r.URL.Path)
		w.WriteHeader(http.StatusConflict)
	})

	out, err := runToolSetGenerationUpdate(t, endpoint, generationUpdateYAML, id.String(), "--generation=3", "-f", "-")
	var apiErr *apply.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusConflict, apiErr.StatusCode)
	require.Empty(t, out)
	require.EqualValues(t, 1, requests.Load())
}
