package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/require"
)

func manifestText(kind, metadata, spec string) string {
	return fmt.Sprintf("apiVersion: authproxy.net/v1alpha1\nkind: %s\nmetadata:\n%s\nspec: %s\n", kind, metadata, spec)
}
func loadText(t *testing.T, input, namespace string) ([]Document, error) {
	t.Helper()
	return Load(context.Background(), Options{Filenames: []string{"-"}, Stdin: strings.NewReader(input), Namespace: namespace})
}
func TestNamespaceAndIdentity(t *testing.T) {
	id := apid.New(apid.PrefixActor).String()
	cases := []struct {
		name, kind, metadata, namespace, wantNS, wantName string
		wantErr                                           bool
	}{
		{"explicit wins", "Actor", "  name: bob\n  namespace: root.prod", "root.dev", "root.prod", "bob", false},
		{"flag fills missing", "Actor", "  name: bob", "root.prod", "root.prod", "bob", false},
		{"no root default", "Actor", "  name: bob", "", "", "", true},
		{"ID only", "Actor", "  id: " + id, "", "", "", false},
		{"ID and default", "Actor", "  id: " + id, "root.prod", "root.prod", "", false},
		{"missing identity", "Actor", "  namespace: root", "", "", "", true},
		{"invalid ID", "Actor", "  id: cxr_wrong", "", "", "", true},
		{"parent default", "Namespace", "  name: prod", "root", "root", "prod", false},
		{"namespace requires parent", "Namespace", "  name: prod", "", "", "", true},
		{"root special", "Namespace", "  name: root", "root.prod", "", "root", false},
		{"namespace ID", "Namespace", "  id: root.prod", "root.other", "root", "prod", false},
		{"namespace ID mismatch", "Namespace", "  id: root.prod\n  name: other", "", "", "", true},
		{"root ID mismatch", "Namespace", "  id: root\n  namespace: root.other", "", "", "", true},
		{"empty namespace", "Actor", "  name: bob\n  namespace: ''", "root", "", "", true},
		{"null namespace", "Actor", "  name: bob\n  namespace: null", "root", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			docs, err := loadText(t, manifestText(tc.kind, tc.metadata, "{}"), tc.namespace)
			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, docs)
				return
			}
			require.NoError(t, err)
			require.Len(t, docs, 1)
			require.Equal(t, tc.wantNS, docs[0].Metadata.Namespace)
			require.Equal(t, tc.wantName, string(docs[0].Metadata.Name))
		})
	}
}

func TestAllKindsAndPresence(t *testing.T) {
	for _, kind := range []string{"Namespace", "Actor", "Connector", "Key", "RateLimit", "Connection"} {
		t.Run(kind, func(t *testing.T) {
			docs, err := loadText(t, manifestText(kind, "  name: example\n  labels: {}", "{}"), "root")
			require.NoError(t, err)
			require.Len(t, docs, 1)
			require.Equal(t, map[string]any{}, docs[0].Object["spec"])
			safe, err := docs[0].RedactedObject()
			require.NoError(t, err)
			require.Equal(t, docs[0].Object, safe)
		})
	}
	input := manifestText("Actor", "  name: bob", "{externalId: '', permissions: null}")
	docs, err := loadText(t, input, "root")
	require.NoError(t, err)
	spec := docs[0].Object["spec"].(map[string]any)
	require.Contains(t, spec, "permissions")
	require.Nil(t, spec["permissions"])
	require.NotContains(t, spec, "signingKey")
	safe, err := docs[0].RedactedObject()
	require.NoError(t, err)
	require.Equal(t, docs[0].Object, safe)
	safe.(map[string]any)["spec"].(map[string]any)["externalId"] = "changed"
	require.Equal(t, "", spec["externalId"], "display must not mutate retained input")
}

func TestInvalidInputIsAtomicAndSourceLocated(t *testing.T) {
	valid := manifestText("Actor", "  name: bob", "{}")
	invalid := []string{
		strings.Replace(valid, "kind: Actor", "kind: Unknown", 1),
		strings.Replace(valid, "v1alpha1", "v999", 1),
		strings.Replace(valid, "spec: {}", "spec: {typo: secret-value}", 1),
		valid + "status: {}\n",
		strings.Replace(valid, "name: bob", "name: bob\n  createdAt: null", 1),
		strings.Replace(valid, "name: bob", "name: bob\n  name: duplicate", 1),
		strings.Replace(valid, "name: bob", "name: bob\n  labels: {x: a, x: b}", 1),
		strings.Replace(valid, "spec: {}", "spec: [bad]", 1),
		"[one, two]",
		strings.Replace(valid, "spec: {}", "spec: {externalId: [secret-value]}", 1),
		strings.Replace(valid, "spec: {}", "spec: &alias {}\nother: *alias", 1),
		manifestText("Connection", "  name: example", "{configuration: {secret: value}}"),
		`{"apiVersion":"authproxy.net/v1alpha1","kind":"Actor","metadata":{"name":"a","name":"b"},"spec":{}}`,
	}
	for i, input := range invalid {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			docs, err := loadText(t, valid+"---\n"+input, "root")
			require.Error(t, err)
			require.Nil(t, docs)
			require.Contains(t, err.Error(), "stdin: document 2")
			require.NotContains(t, err.Error(), "secret-value")
		})
	}
}

func TestListsAndDuplicates(t *testing.T) {
	item := map[string]any{"apiVersion": "authproxy.net/v1alpha1", "kind": "Actor", "metadata": map[string]any{"name": "bob"}, "spec": map[string]any{}}
	for _, kind := range []string{"ActorList", "List"} {
		data, err := json.Marshal(map[string]any{"apiVersion": "authproxy.net/v1alpha1", "kind": kind, "items": []any{item}})
		require.NoError(t, err)
		docs, err := loadText(t, string(data), "root")
		require.NoError(t, err)
		require.Len(t, docs, 1)
		require.Contains(t, docs[0].Source, "item 1")
	}
	data, _ := json.Marshal(map[string]any{"apiVersion": "authproxy.net/v1alpha1", "kind": "KeyList", "items": []any{item}})
	_, err := loadText(t, string(data), "root")
	require.ErrorContains(t, err, "does not match")
	data, _ = json.Marshal(map[string]any{"apiVersion": "authproxy.net/v1alpha1", "kind": "ActorList", "metadata": map[string]any{"continue": "cursor"}, "items": []any{item}})
	_, err = loadText(t, string(data), "root")
	require.ErrorContains(t, err, "incomplete")
	a := manifestText("Actor", "  name: bob", "{}")
	docs, err := loadText(t, a+"---\n"+a, "root")
	require.Nil(t, docs)
	require.ErrorContains(t, err, "duplicate resource identity")
	require.Contains(t, err.Error(), "document 1")
	require.Contains(t, err.Error(), "document 2")
	id := apid.New(apid.PrefixActor).String()
	a = manifestText("Actor", "  id: "+id, "{}")
	_, err = loadText(t, a+"---\n"+manifestText("Actor", "  id: "+id+"\n  name: bob\n  namespace: root", "{}"), "")
	require.ErrorContains(t, err, "duplicate resource identity")
}

func TestDiscoverySelectionAndEmptyDocuments(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "nested"), 0700))
	write := func(path, name, label string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, path), []byte(manifestText("Actor", "  name: "+name+"\n  labels: {env: "+label+"}", "{}")), 0600))
	}
	write("b.yaml", "b", "prod")
	write("a.json", "a", "dev")
	write("ignored.txt", "ignored", "prod")
	write("nested/c.yml", "c", "prod")
	opts := Options{Filenames: []string{dir}, Namespace: "root"}
	docs, err := Load(context.Background(), opts)
	require.NoError(t, err)
	require.Len(t, docs, 2)
	require.Equal(t, "a", string(docs[0].Metadata.Name))
	opts.Recursive = true
	opts.Selector = "env=prod"
	docs, err = Load(context.Background(), opts)
	require.NoError(t, err)
	require.Len(t, docs, 2)
	require.Equal(t, "b", string(docs[0].Metadata.Name))
	require.Equal(t, "c", string(docs[1].Metadata.Name))
	opts.Selector = "env=missing"
	docs, err = Load(context.Background(), opts)
	require.NoError(t, err)
	require.Empty(t, docs)
	_, err = loadText(t, "# comment\n---\n", "")
	require.ErrorContains(t, err, "no resource")
	docs, err = loadText(t, "---\n# comment\n---\n"+manifestText("Actor", "  name: bob", "{}"), "root")
	require.NoError(t, err)
	require.Contains(t, docs[0].Source, "document 2")
	_, err = Load(context.Background(), Options{Filenames: []string{"-", "-"}, Stdin: strings.NewReader(manifestText("Namespace", "  name: root", "{}"))})
	require.ErrorContains(t, err, "stdin may only")
}

func TestURLInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("Cookie"))
		if r.URL.Path == "/missing" {
			http.Error(w, "sensitive body", http.StatusNotFound)
			return
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/ok", http.StatusFound)
			return
		}
		fmt.Fprint(w, manifestText("Namespace", "  name: root", "{}"))
	}))
	defer srv.Close()
	docs, err := Load(context.Background(), Options{Filenames: []string{srv.URL + "/redirect?token=secret"}})
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.NotContains(t, docs[0].Source, "token")
	_, err = Load(context.Background(), Options{Filenames: []string{srv.URL + "/missing?token=secret"}})
	require.ErrorContains(t, err, "HTTP 404")
	require.NotContains(t, err.Error(), "secret")
	require.NotContains(t, err.Error(), "sensitive body")
	_, err = Load(context.Background(), Options{Filenames: []string{strings.Replace(srv.URL, "://", "://user:password@", 1)}})
	require.ErrorContains(t, err, "credentials")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Load(ctx, Options{Filenames: []string{srv.URL}})
	require.ErrorIs(t, err, context.Canceled)
}

func TestKeyOutputRedacted(t *testing.T) {
	input := manifestText("Key", "  name: secret-key", "{keyData: {value: do-not-print-this}}")
	docs, err := loadText(t, input, "root")
	require.NoError(t, err)
	safe, err := docs[0].RedactedObject()
	require.NoError(t, err)
	data, err := json.Marshal(safe)
	require.NoError(t, err)
	require.NotContains(t, string(data), "do-not-print-this")
	require.Contains(t, string(data), "***")
	require.Contains(t, fmt.Sprint(docs[0].Object), "do-not-print-this")
	_, err = loadText(t, strings.Replace(input, "do-not-print-this", "'********'", 1), "root")
	require.ErrorContains(t, err, "redacted placeholders")
}

func TestNumericPresenceAndInvalidGeneration(t *testing.T) {
	input := manifestText("Connector", "  name: example\n  generation: 9007199254740993", "{}")
	docs, err := loadText(t, input, "root")
	require.NoError(t, err)
	safe, err := docs[0].RedactedObject()
	require.NoError(t, err)
	require.Equal(t, docs[0].Object, safe)
	for _, value := range []string{"0", "null", "-1"} {
		_, err := loadText(t, manifestText("Connector", "  name: example\n  generation: "+value, "{}"), "root")
		require.Error(t, err)
	}
}

func TestNestedSecretRedaction(t *testing.T) {
	cases := []struct{ kind, spec string }{
		{"Actor", "{signingKey: {sharedKey: {value: confidential-material}}}"},
		{"Connector", "{definition: {auth: {type: OAuth2, clientSecret: {value: confidential-material}}}}"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			docs, err := loadText(t, manifestText(tc.kind, "  name: example", tc.spec), "root")
			require.NoError(t, err)
			safe, err := docs[0].RedactedObject()
			require.NoError(t, err)
			encoded, err := json.Marshal(safe)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "confidential-material")
			require.Contains(t, string(encoded), "***")
		})
	}
}

func TestSourceSizeLimit(t *testing.T) {
	_, err := loadText(t, strings.Repeat(" ", maxSourceBytes+1), "")
	require.ErrorContains(t, err, "16 MiB")
}
