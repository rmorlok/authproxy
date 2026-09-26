//go:build integration

package apply_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/cli/apply"
	"github.com/stretchr/testify/require"
)

type object = map[string]any

type cli struct {
	binary, root, config string
	env                  *helpers.IntegrationTestEnv
	admin                bool
}

func (c cli) run(t *testing.T, input string, exit int, extra ...string) ([]apply.Result, string) {
	t.Helper()
	stdout, stderr := c.runRaw(t, input, exit, extra...)
	var results []apply.Result
	if len(stdout) != 0 {
		require.NoError(t, json.Unmarshal(stdout, &results), "stdout: %s", stdout)
	}
	return results, stderr
}

func (c cli) runRaw(t *testing.T, input string, exit int, extra ...string) ([]byte, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"apply", "-f", "-", "-o", "json", "--config", c.config, "--actorId", "apply-operator", "--privateKeyPath", filepath.Join(c.root, "dev_config/keys/admin/bobdole")}
	if c.admin {
		args = append(args, "--admin", "--adminApiUrl", c.env.ServerURL, "--apiUrl", "http://127.0.0.1:1")
	} else {
		args = append(args, "--apiUrl", c.env.ServerURL)
	}
	args = append(args, extra...)
	cmd := exec.CommandContext(ctx, c.binary, args...)
	cmd.Dir = c.root
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	require.NoError(t, ctx.Err(), "CLI timed out")
	if exit == 0 {
		require.NoError(t, err, "stdout: %s\nstderr: %s", &stdout, &stderr)
	} else {
		var failure *exec.ExitError
		require.ErrorAs(t, err, &failure, "stdout: %s\nstderr: %s", &stdout, &stderr)
		require.Equal(t, exit, failure.ExitCode())
	}
	return stdout.Bytes(), stderr.String()
}

func manifest(kind, name, namespace string, spec object) object {
	metadata := object{}
	if name != "" {
		metadata["name"] = name
	}
	if namespace != "" {
		metadata["namespace"] = namespace
	}
	return object{"apiVersion": "authproxy.net/v1alpha1", "kind": kind, "metadata": metadata, "spec": spec}
}
func encode(t *testing.T, documents ...object) string {
	t.Helper()
	var parts []string
	for _, doc := range documents {
		b, err := json.Marshal(doc)
		require.NoError(t, err)
		parts = append(parts, string(b))
	}
	return strings.Join(parts, "\n---\n")
}
func metadata(doc object) object { return doc["metadata"].(map[string]any) }
func resultResource(t *testing.T, results []apply.Result, status string) object {
	t.Helper()
	require.Len(t, results, 1)
	require.Equal(t, status, results[0].Status)
	resource, ok := results[0].Resource.(map[string]any)
	require.True(t, ok)
	return resource
}
func (c cli) request(t *testing.T, method, path string, body object, status int) object {
	t.Helper()
	var input io.Reader
	if body != nil {
		input = strings.NewReader(encode(t, body))
	}
	req, err := http.NewRequest(method, c.env.ServerURL+"/api/v1/"+path, input)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+c.env.BearerToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, status, resp.StatusCode, "%s %s: %s", method, path, data)
	var resource object
	require.NoError(t, json.Unmarshal(data, &resource))
	return resource
}

func TestCLIApply(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	directory := t.TempDir()
	binary := filepath.Join(directory, "ap")
	build := exec.Command("go", "build", "-o", binary, "./cmd/cli")
	build.Dir = root
	output, err := build.CombinedOutput()
	require.NoError(t, err, "%s", output)
	publicKey, err := os.ReadFile(filepath.Join(root, "dev_config/keys/admin/bobdole.pub"))
	require.NoError(t, err)
	config := filepath.Join(directory, "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte("{}"), 0600))
	for _, admin := range []bool{false, true} {
		service := helpers.ServiceTypeAPI
		if admin {
			service = helpers.ServiceTypeAdminAPI
		}
		t.Run(string(service), func(t *testing.T) {
			env := helpers.Setup(t, helpers.SetupOptions{Service: service, StartHTTPServer: true})
			t.Cleanup(env.Cleanup)
			c := cli{binary: binary, root: root, config: config, env: env, admin: admin}
			c.request(t, "POST", "actors", manifest("Actor", "apply-operator", "root", object{
				"externalId": "apply-operator", "signingKey": object{"publicKey": object{"value": string(publicKey)}},
				"permissions": []any{object{"namespace": "root.**", "resources": []string{"*"}, "verbs": []string{"*"}}},
			}), 201)
			t.Run("Identity", c.identity)
			if !admin {
				return
			}
			t.Run("ResourceLifecycles", c.lifecycles)
			t.Run("ConnectorGenerations", c.generations)
			t.Run("DependenciesAndFailures", c.dependencies)
			t.Run("ReferenceDependencies", c.references)
			t.Run("Prune", c.prune)
			t.Run("HistoryCommands", c.historyCommands)
		})
	}
}

func (c cli) identity(t *testing.T) {
	doc := manifest("Actor", "identity", "", object{"externalId": "apply-identity"})
	results, _ := c.run(t, encode(t, doc), 1)
	require.Empty(t, results)
	results, _ = c.run(t, encode(t, doc), 0, "--namespace=root")
	resource := resultResource(t, results, "created")
	require.Equal(t, "root", metadata(resource)["namespace"])
	id := metadata(resource)["id"].(string)
	metadata(doc)["namespace"] = "root"
	results, _ = c.run(t, encode(t, doc), 0, "--namespace=root.nonexistent")
	require.Equal(t, id, metadata(resultResource(t, results, "configured"))["id"])
	doc["metadata"] = object{"id": id}
	results, _ = c.run(t, encode(t, doc), 0)
	require.Equal(t, id, metadata(results[0].Resource.(map[string]any))["id"])
	metadata(doc)["name"] = "wrong"
	c.run(t, encode(t, doc), 1)
	doc["metadata"] = object{"id": apid.New(apid.PrefixActor).String()}
	c.run(t, encode(t, doc), 1)
	if !c.admin {
		c.run(t, encode(t, manifest("Key", "api-key", "root", object{"keyData": object{"numBytes": 32}})), 0)
	}
}

func (c cli) lifecycles(t *testing.T) {
	cases := []struct {
		kind, path string
		spec       object
	}{
		{"Namespace", "namespaces", object{}},
		{"Actor", "actors", object{"externalId": "apply-lifecycle", "signingKey": object{"sharedKey": object{"value": "apply-test-signing-secret"}}}},
		{"Key", "keys", object{"keyData": object{"value": "0123456789abcdef0123456789abcdef"}}},
		{"RateLimit", "rate-limits", object{"algorithm": object{"tokenBucket": object{"capacity": 10, "refillRate": 1}}}},
		{"Connector", "connectors", object{"definition": object{"displayName": "Apply", "auth": object{"type": "no-auth"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			doc := manifest(tc.kind, "lifecycle-"+strings.ToLower(tc.kind), "root", tc.spec)
			metadata(doc)["labels"] = object{"managed": "initial", "remove": "yes"}
			results, _ := c.run(t, encode(t, doc), 0)
			resource := resultResource(t, results, "created")
			path := tc.path + "/" + metadata(resource)["id"].(string)
			live := c.request(t, "GET", path, nil, 200)
			history := metadata(live)["annotations"].(map[string]any)[apply.LastAppliedAnnotation].(string)
			require.NotContains(t, history, "apply-test-signing-secret")
			require.NotContains(t, history, "0123456789abcdef")
			require.NotContains(t, fmt.Sprint(results), "apply-test-signing-secret")
			require.NotContains(t, fmt.Sprint(results), "0123456789abcdef")
			// Omitted secrets must survive subsequent metadata-only applies.
			delete(tc.spec, "signingKey")
			delete(tc.spec, "keyData")
			c.run(t, encode(t, doc), 0) // Bind the server-assigned identity into history.
			results, _ = c.run(t, encode(t, doc), 0)
			resultResource(t, results, "unchanged")
			patch := manifest(tc.kind, "", "", object{})
			metadata(patch)["labels"] = object{"unmanaged": "keep"}
			c.request(t, "PATCH", path, patch, 200)
			metadata(doc)["labels"] = object{"managed": "updated"}
			results, _ = c.run(t, encode(t, doc), 0)
			resultResource(t, results, "configured")
			live = c.request(t, "GET", path, nil, 200)
			require.Subset(t, metadata(live)["labels"], map[string]any{"managed": "updated", "unmanaged": "keep"})
			require.NotContains(t, metadata(live)["labels"], "remove")
			// A managed field changed outside apply conflicts when overwrite is disabled.
			metadata(patch)["labels"] = object{"managed": "drifted", "unmanaged": "keep"}
			c.request(t, "PATCH", path, patch, 200)
			_, diagnostic := c.run(t, encode(t, doc), 1, "--overwrite=false")
			require.Contains(t, diagnostic, "conflict")
			require.Equal(t, "drifted", metadata(c.request(t, "GET", path, nil, 200))["labels"].(map[string]any)["managed"])
			c.run(t, encode(t, doc), 0)
			// Remove history through the API and exercise explicit adoption of this ID.
			metadata(patch)["annotations"] = object{}
			delete(metadata(patch), "labels")
			c.request(t, "PATCH", path, patch, 200)
			doc["metadata"] = object{"id": metadata(resource)["id"], "labels": object{"adopted": "yes"}}
			results, warning := c.run(t, encode(t, doc), 0)
			resultResource(t, results, "configured")
			require.Contains(t, warning, "adopt")
			if tc.kind == "Actor" {
				actor, err := c.env.Db.GetActor(context.Background(), apid.ID(metadata(resource)["id"].(string)))
				require.NoError(t, err)
				require.NotNil(t, actor.EncryptedKey)
				plaintext, err := c.env.DM.GetEncryptService().DecryptString(context.Background(), *actor.EncryptedKey)
				require.NoError(t, err)
				require.Contains(t, plaintext, "apply-test-signing-secret")
			}
			if tc.kind == "Key" {
				key, err := c.env.Db.GetKey(context.Background(), apid.ID(metadata(resource)["id"].(string)))
				require.NoError(t, err)
				require.NotNil(t, key.EncryptedKeyData)
				plaintext, err := c.env.DM.GetEncryptService().DecryptString(context.Background(), *key.EncryptedKeyData)
				require.NoError(t, err)
				require.Contains(t, plaintext, "0123456789abcdef0123456789abcdef")
			}
			if tc.kind == "Connector" {
				connectorID, err := apid.Parse(metadata(resource)["id"].(string))
				require.NoError(t, err)
				connectionID := c.env.CreateConnection(t, connectorID, 1)
				connection := manifest("Connection", "", "", object{})
				metadata(connection)["id"] = connectionID
				metadata(connection)["labels"] = object{"managed": "yes"}
				results, warning = c.run(t, encode(t, connection), 0)
				resultResource(t, results, "configured")
				require.Contains(t, warning, "adopt")
				results, _ = c.run(t, encode(t, connection), 0)
				resultResource(t, results, "unchanged")
				metadata(connection)["labels"] = object{}
				c.run(t, encode(t, connection), 0)
				require.NotContains(t, metadata(c.request(t, "GET", "connections/"+connectionID, nil, 200))["labels"], "managed")
				c.run(t, encode(t, manifest("Connection", "missing", "root", object{})), 1)
			}
		})
	}
}

func (c cli) generations(t *testing.T) {
	spec := object{"definition": object{"displayName": "First", "auth": object{"type": "no-auth"}}, "release": object{"desiredState": "primary"}}
	doc := manifest("Connector", "generations", "root", spec)
	results, _ := c.run(t, encode(t, doc), 0)
	resource := resultResource(t, results, "created")
	path := "connectors/" + metadata(resource)["id"].(string)
	c.run(t, encode(t, doc), 0)
	results, _ = c.run(t, encode(t, doc), 0)
	resultResource(t, results, "unchanged")
	spec["definition"].(map[string]any)["displayName"] = "Second"
	spec["release"] = object{"desiredState": "draft"}
	c.run(t, encode(t, doc), 0)
	generations := c.request(t, "GET", path+"/generations", nil, 200)["items"].([]any)
	require.Len(t, generations, 2)
	c.run(t, encode(t, doc), 0)
	require.Len(t, c.request(t, "GET", path+"/generations", nil, 200)["items"], 2)
	spec["release"] = object{"desiredState": "primary"}
	c.run(t, encode(t, doc), 0)
	live := c.request(t, "GET", path, nil, 200)
	require.Equal(t, float64(2), metadata(live)["generation"])
	require.Equal(t, "primary", live["status"].(map[string]any)["release"].(map[string]any)["state"])
	metadata(doc)["generation"] = 2
	spec["definition"].(map[string]any)["displayName"] = "Cannot edit published generation"
	c.run(t, encode(t, doc), 1)
	require.Len(t, c.request(t, "GET", path+"/generations", nil, 200)["items"], 2)
}

func (c cli) dependencies(t *testing.T) {
	child := manifest("Actor", "child", "root.applydeps", object{"externalId": "apply-child"})
	parent := manifest("Namespace", "applydeps", "root", object{})
	directory := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, "nested"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "child.json"), []byte(encode(t, child)), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "nested", "namespace.yaml"), []byte("apiVersion: authproxy.net/v1alpha1\nkind: Namespace\nmetadata: {name: applydeps, namespace: root}\nspec: {}\n"), 0600))
	results, _ := c.run(t, "", 0, "-f", directory, "--recursive")
	require.Len(t, results, 2)
	for _, result := range results {
		require.Equal(t, "created", result.Status)
	}
	// A missing prerequisite prevents even independent creates during preparation.
	independent := manifest("Actor", "independent", "root", object{"externalId": "apply-independent"})
	metadata(child)["namespace"] = "root.missing"
	results, _ = c.run(t, encode(t, independent, child), 1)
	require.Empty(t, results)
	// Fault injection is limited to one namespace mutation; all other requests
	// reach the real API. This exercises partial success and skipped dependents.
	target, err := url.Parse(c.env.ServerURL)
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(target)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/v1/namespaces" {
			w.WriteHeader(503)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	defer server.Close()
	metadata(parent)["name"] = "applyfailed"
	metadata(child)["namespace"] = "root.applyfailed"
	results, _ = c.run(t, encode(t, child, parent, independent), 1, "--adminApiUrl", server.URL)
	require.Len(t, results, 3)
	statuses := map[string]string{}
	for _, result := range results {
		statuses[string(result.Kind)] = result.Status
	}
	require.Equal(t, "failed", statuses["Namespace"])
	require.Equal(t, "skipped", results[1].Status)
	require.Equal(t, "created", results[2].Status)
}

func (c cli) references(t *testing.T) {
	key := manifest("Key", "reference-key", "root.references", object{"keyData": object{"numBytes": 32}})
	namespace := manifest("Namespace", "references", "root", object{})
	ref := object{"apiVersion": "authproxy.net/v1alpha1", "kind": "Key", "name": "reference-key", "namespace": "root.references"}
	namespace["spec"] = object{"encryptionKeyRef": ref}
	_, diagnostic := c.run(t, encode(t, namespace, key), 1)
	require.Contains(t, diagnostic, "cycle")
	namespace["spec"] = object{}
	c.run(t, encode(t, namespace), 0)
	namespace["spec"] = object{"encryptionKeyRef": ref}
	results, _ := c.run(t, encode(t, namespace, key), 0)
	require.Len(t, results, 2)
	require.Equal(t, "Key", string(results[0].Kind))
	require.Equal(t, "created", results[0].Status)
	require.Equal(t, "configured", results[1].Status)
	live := c.request(t, "GET", "namespaces/root.references", nil, 200)
	require.Equal(t, results[0].Identity, live["spec"].(map[string]any)["encryptionKeyRef"].(map[string]any)["id"])
	// Explicit null clears the complete reference, including its resolved ID.
	namespace["spec"] = object{"encryptionKeyRef": nil}
	c.run(t, encode(t, namespace), 0)
	live = c.request(t, "GET", "namespaces/root.references", nil, 200)
	require.Empty(t, live["spec"].(map[string]any)["encryptionKeyRef"])
}

func (c cli) prune(t *testing.T) {
	namespace := manifest("Namespace", "prune", "root", object{})
	c.run(t, encode(t, namespace), 0)
	keep := manifest("Actor", "keep", "root.prune", object{"externalId": "keep"})
	obsolete := manifest("Actor", "obsolete", "root.prune", object{"externalId": "obsolete"})
	key := manifest("Key", "obsolete", "root.prune", object{"keyData": object{"numBytes": 32}})
	limit := manifest("RateLimit", "obsolete", "root.prune", object{"algorithm": object{"tokenBucket": object{"capacity": 10, "refillRate": 1}}})
	seeded, _ := c.run(t, encode(t, keep, obsolete, key, limit), 0)
	results, _ := c.run(t, encode(t, keep), 0, "--namespace=root.prune", "--prune", "--all", "--prune-allowlist=Actor,Key,RateLimit")
	require.Len(t, results, 4)
	for _, result := range results[1:] {
		require.Equal(t, "pruned", result.Status)
	}
	for i, collection := range []string{"actors", "keys", "rate-limits"} {
		c.request(t, "GET", collection+"/"+seeded[i+1].Identity, nil, 404)
	}
	c.request(t, "GET", "actors/"+seeded[0].Identity, nil, 200)
}

func (c cli) historyCommands(t *testing.T) {
	doc := manifest("Actor", "history", "root", object{"externalId": "original", "signingKey": object{"sharedKey": object{"value": "history-test-secret"}}})
	results, _ := c.run(t, encode(t, doc), 0)
	id := results[0].Identity
	data, _ := c.runRaw(t, encode(t, doc), 0, "view-last-applied")
	require.NotContains(t, string(data), "history-test-secret")
	var desired []object
	require.NoError(t, json.Unmarshal(data, &desired))
	require.Equal(t, "original", desired[0]["spec"].(map[string]any)["externalId"])
	doc["spec"] = object{"externalId": "history-only"}
	c.run(t, encode(t, doc), 0, "set-last-applied")
	require.Equal(t, "original", c.request(t, "GET", "actors/"+id, nil, 200)["spec"].(map[string]any)["externalId"])
	doc["spec"] = object{"externalId": "edited-history"}
	editor := filepath.Join(t.TempDir(), "editor")
	require.NoError(t, os.WriteFile(editor, []byte("#!/bin/sh\ncat > \"$1\" <<'MANIFEST'\n"+encode(t, doc)+"\nMANIFEST\n"), 0700))
	t.Setenv("KUBE_EDITOR", editor)
	c.run(t, encode(t, doc), 0, "edit-last-applied")
	data, _ = c.runRaw(t, encode(t, doc), 0, "view-last-applied")
	require.Contains(t, string(data), "edited-history")
	require.Equal(t, "original", c.request(t, "GET", "actors/"+id, nil, 200)["spec"].(map[string]any)["externalId"])
}
