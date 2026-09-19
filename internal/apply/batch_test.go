package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/registry"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"

	"github.com/stretchr/testify/require"
)

type batchServer struct {
	t          *testing.T
	objects    map[string]map[string]any
	writes     []string
	failName   string
	afterWrite func()
}

func newBatchServer(t *testing.T) (*batchServer, *Client) {
	s := &batchServer{
		t:       t,
		objects: map[string]map[string]any{},
	}
	s.objects["namespaces/root"] = map[string]any{
		"apiVersion": string(meta.APIVersionV1Alpha1),
		"kind":       "Namespace",
		"metadata": map[string]any{
			"id":   "root",
			"name": "root",
		}, "spec": map[string]any{},
	}
	return s, testClient(t, s.handle, false)
}

func (s *batchServer) handle(w http.ResponseWriter, r *http.Request) {
	t := s.t
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	collection, _, _ := strings.Cut(path, "/")
	kinds := map[string]string{
		"namespaces":  "Namespace",
		"actors":      "Actor",
		"keys":        "Key",
		"connectors":  "Connector",
		"rate-limits": "RateLimit",
		"connections": "Connection",
	}

	switch r.Method {
	case "GET":
		if object, ok := s.objects[path]; ok {
			require.NoError(t, json.NewEncoder(w).Encode(object))
			return
		}
		if strings.Contains(path, "/") {
			w.WriteHeader(404)
			return
		}
		var items []string
		paths := make([]string, 0, len(s.objects))
		for p := range s.objects {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			if !strings.HasPrefix(p, collection+"/") {
				continue
			}
			m := liveMeta(s.objects[p])
			if m["namespace"] != r.URL.Query().Get("namespace") || m["name"] != r.URL.Query().Get("name") {
				continue
			}
			data, err := json.Marshal(s.objects[p])
			require.NoError(t, err)
			items = append(items, string(data))
		}
		fmt.Fprint(w, listJSON(kinds[collection], items, ""))
	case "POST", "PATCH":
		var object map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&object))
		name, _ := liveMeta(object)["name"].(string)
		if r.Method == "PATCH" {
			name, _ = liveMeta(s.objects[path])["name"].(string)
		}
		s.writes = append(s.writes, r.Method+" "+collection+"/"+name)
		if name == s.failName {
			w.WriteHeader(500)
			fmt.Fprint(w, `{"message":"sensitive-server-detail"}`)
			return
		}
		if r.Method == "POST" {
			m := liveMeta(object)
			id := ""
			if collection == "namespaces" {
				id = fmt.Sprint(m["namespace"]) + "." + name
			} else {
				prefix := map[string]apid.Prefix{"actors": apid.PrefixActor, "keys": apid.PrefixKey, "connectors": apid.PrefixConnector, "rate-limits": apid.PrefixRateLimit}[collection]
				id = apid.New(prefix).String()
			}
			m["id"] = id
			path = collection + "/" + id
		} else {
			data, err := json.Marshal(s.objects[path])
			require.NoError(t, err)
			current, err := registry.NewResourceScheme().DecodeJSON(data)
			require.NoError(t, err)
			data, err = json.Marshal(object)
			require.NoError(t, err)
			descriptor, err := registry.TypeOf(current)
			require.NoError(t, err)
			patch, err := descriptor.DecodePatchJSON(data)
			require.NoError(t, err)
			updated, err := descriptor.ApplyPatch(current, patch)
			require.NoError(t, err)
			object, err = plainObject(updated)
			require.NoError(t, err)
		}
		// Ordinary reads omit write-only provider configuration.
		delete(asSpec(object), "keyData")
		delete(asSpec(object), "signingKey")
		s.objects[path] = object
		require.NoError(t, json.NewEncoder(w).Encode(object))
		if s.afterWrite != nil {
			s.afterWrite()
		}
	}
}

func asSpec(object map[string]any) map[string]any {
	m, _ := object["spec"].(map[string]any)
	return m
}

func batchDoc(t *testing.T, kind, name, ns, spec string) Document {
	return clientDoc(t, kind, "  name: "+name+"\n  namespace: "+ns, spec)
}

func TestBatchNamespaceOrderingAndRepeatedApply(t *testing.T) {
	server, client := newBatchServer(t)
	docs := []Document{
		batchDoc(t, "Actor", "bob", "root.team", `{"externalId":"subject"}`),
		batchDoc(t, "Namespace", "team", "root", `{}`),
		batchDoc(t, "Actor", "independent", "root", `{"externalId":"independent"}`),
	}

	batch, err := client.Prepare(context.Background(), docs, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	require.Empty(t, server.writes)

	results, err := batch.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, results, 3)
	require.Equal(t, []string{"POST namespaces/team", "POST actors/bob", "POST actors/independent"}, server.writes)

	_, err = batch.Execute(context.Background())
	require.ErrorContains(t, err, "already been executed")
	require.Len(t, server.writes, 3)

	// First repeat may bind the newly allocated IDs into history; the next is a no-op.
	for iteration := 0; iteration < 2; iteration++ {
		batch, err = client.Prepare(context.Background(), docs, ReconcileOptions{Overwrite: true})
		require.NoError(t, err)
		results, err = batch.Execute(context.Background())
		require.NoError(t, err)
	}

	for _, result := range results {
		require.Equal(t, "unchanged", result.Status)
	}
}

func TestBatchMissingPrerequisitesAndCyclesPreventAllWrites(t *testing.T) {
	for _, tc := range []struct {
		name    string
		docs    func(*testing.T) []Document
		message string
	}{
		{
			"missing namespace",
			func(t *testing.T) []Document {
				return []Document{
					batchDoc(t, "Actor", "valid", "root", `{"externalId":"subject"}`),
					batchDoc(t, "Actor", "missing", "root.absent", `{"externalId":"subject"}`)}
			},
			"prerequisite",
		},
		{
			"missing explicit reference",
			func(t *testing.T) []Document {
				return []Document{
					batchDoc(t, "Namespace", "team", "root", `{"encryptionKeyRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Key","name":"missing","namespace":"root"}}`),
				}
			},
			"not found",
		},
		{
			"namespace key cycle",
			func(t *testing.T) []Document {
				return []Document{
					batchDoc(t, "Namespace", "team", "root", `{"encryptionKeyRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Key","name":"key","namespace":"root.team"}}`),
					batchDoc(t, "Key", "key", "root.team", `{"keyData":{"value":"secret"}}`),
				}
			},
			"cycle",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := newBatchServer(t)
			_, err := c.Prepare(context.Background(), tc.docs(t), ReconcileOptions{Overwrite: true})
			require.ErrorContains(t, err, tc.message)
			require.Empty(t, s.writes)
		})
	}
}

func TestBatchReferenceOrderingAndPartialFailure(t *testing.T) {
	server, client := newBatchServer(t)
	server.failName = "key"
	docs := []Document{
		batchDoc(t, "Namespace", "team", "root", `{"encryptionKeyRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Key","name":"key","namespace":"root"}}`),
		batchDoc(t, "Key", "key", "root", `{"keyData":{"value":"secret-material"}}`),
		batchDoc(t, "Actor", "dependent", "root.team", `{"externalId":"subject"}`),
		batchDoc(t, "Actor", "independent", "root", `{"externalId":"independent"}`),
	}
	batch, err := client.Prepare(context.Background(), docs, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	results, err := batch.Execute(context.Background())
	require.ErrorIs(t, err, ErrBatchFailed)
	require.Equal(t, []string{"POST keys/key", "POST actors/independent"}, server.writes)
	require.Equal(t, []string{"failed", "skipped", "skipped", "created"}, []string{results[0].Status, results[1].Status, results[2].Status, results[3].Status})
	data, err := json.Marshal(results)
	require.NoError(t, err)
	require.NotContains(t, string(data), "secret-material")
	require.NotContains(t, string(data), "sensitive-server-detail")
}

func TestBatchExistingNamespaceDoesNotCreateArtificialKeyCycle(t *testing.T) {
	server, client := newBatchServer(t)
	server.objects["namespaces/root.team"] = map[string]any{"apiVersion": string(meta.APIVersionV1Alpha1), "kind": "Namespace", "metadata": map[string]any{"id": "root.team", "name": "team", "namespace": "root"}, "spec": map[string]any{}}
	docs := []Document{
		batchDoc(t, "Namespace", "team", "root", `{"encryptionKeyRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Key","name":"key","namespace":"root.team"}}`),
		batchDoc(t, "Key", "key", "root.team", `{"keyData":{"value":"secret"}}`),
	}
	batch, err := client.Prepare(context.Background(), docs, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	_, err = batch.Execute(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"POST keys/key", "PATCH namespaces/team"}, server.writes)
}

func TestBatchCancellationStopsMutations(t *testing.T) {
	server, client := newBatchServer(t)
	docs := []Document{
		batchDoc(t, "Actor", "first", "root", `{"externalId":"first"}`),
		batchDoc(t, "Actor", "second", "root", `{"externalId":"second"}`),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	batch, err := client.Prepare(ctx, docs, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)

	server.afterWrite = cancel

	results, err := batch.Execute(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, server.writes, 1)
	require.Equal(t, "skipped", results[1].Status)

	_, err = client.Prepare(ctx, docs, ReconcileOptions{Overwrite: true})
	require.ErrorIs(t, err, context.Canceled)
}

func TestBatchConnectionCreationAndLateInvalidPlanPreventWrites(t *testing.T) {
	for _, last := range []Document{
		batchDoc(t, "Connection", "missing", "root", `{}`),
		batchDoc(t, "Actor", "invalid", "root", `{}`),
	} {
		server, client := newBatchServer(t)
		_, err := client.Prepare(
			context.Background(),
			[]Document{
				batchDoc(t, "Actor", "valid", "root", `{"externalId":"subject"}`),
				last,
			},
			ReconcileOptions{Overwrite: true},
		)
		require.Error(t, err)
		require.Empty(t, server.writes)
	}
}

func TestBatchExistingConnectionMetadataOnly(t *testing.T) {
	server, client := newBatchServer(t)
	id := apid.New(apid.PrefixConnection).String()
	connectorID := apid.New(apid.PrefixConnector).String()

	response := fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","metadata":{"id":%q,"name":"example","namespace":"root","createdAt":"2026-09-01T00:00:00Z","updatedAt":"2026-09-01T00:00:00Z"},"spec":{"connectorRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector","id":%q,"generation":1}},"status":{"lifecycle":{"state":"configured"},"health":{"state":"healthy"},"configuration":{"configured":true,"schema":{"type":"object"}}}}`, id, connectorID)
	var object map[string]any
	require.NoError(t, json.Unmarshal([]byte(response), &object))

	server.objects["connections/"+id] = object
	doc := clientDoc(t, "Connection", "  id: "+id+"\n  labels: {env: prod}", `{}`)

	batch, err := client.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)

	results, err := batch.Execute(context.Background())
	require.NoError(t, err)
	require.Equal(t, "configured", results[0].Status)
	require.Equal(t, []string{"PATCH connections/example"}, server.writes)
	require.Equal(t, "prod", liveMeta(server.objects["connections/"+id])["labels"].(map[string]any)["env"])
}

func TestDependencyOrderStableAndCycle(t *testing.T) {
	order, err := dependencyOrder([][]int{{2}, {}, {}, {1}})
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 0, 3}, order)

	_, err = dependencyOrder([][]int{{1}, {0}})
	require.ErrorContains(t, err, "cycle")

	_, err = dependencyOrder([][]int{{0}})
	require.ErrorContains(t, err, "cycle")
}

func TestBatchPrevalidatesOverwriteConflicts(t *testing.T) {
	server, client := newBatchServer(t)
	old := batchDoc(t, "Actor", "example", "root", `{"externalId":"subject"}`)
	live := withHistory(
		t,
		reconcileLive(
			t,
			"Actor",
			apid.New(apid.PrefixActor).String(),
			`{"externalId":"subject"}`,
			map[string]string{"managed": "changed"},
		),
		clientDoc(
			t,
			"Actor",
			"  name: example\n  namespace: root\n  annotations: {managed: original}",
			`{"externalId":"subject"}`,
		),
	)

	object, err := plainObject(live.Resource)
	require.NoError(t, err)

	server.objects["actors/"+live.Metadata.ID] = object
	_, err = client.Prepare(context.Background(), []Document{batchDoc(t, "Actor", "new", "root", `{"externalId":"new"}`), old}, ReconcileOptions{Overwrite: false})
	require.ErrorContains(t, err, "conflict")
	require.Empty(t, server.writes)
}

func TestBatchExternalReferenceAndConflictingAlias(t *testing.T) {
	for _, mismatched := range []bool{false, true} {
		server, client := newBatchServer(t)
		id := apid.New(apid.PrefixKey).String()

		var key map[string]any
		require.NoError(t, json.Unmarshal([]byte(liveJSON("Key", id, "existing", "root", `{"usage":"data_encryption","materialType":"symmetric"}`)), &key))

		server.objects["keys/"+id] = key
		refName := "existing"

		if mismatched {
			refName = "different"
		}

		ref := fmt.Sprintf(`{"encryptionKeyRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Key","id":%q,"name":%q,"namespace":"root"}}`, id, refName)

		docs := []Document{
			batchDoc(t, "Namespace", "team", "root", ref),
		}

		if mismatched {
			docs = append(docs, clientDoc(t, "Key", "  id: "+id, `{}`))
		}

		batch, err := client.Prepare(context.Background(), docs, ReconcileOptions{Overwrite: true})

		if mismatched {
			require.ErrorContains(t, err, "identity")
			require.Empty(t, server.writes)
			continue
		}

		require.NoError(t, err)

		_, err = batch.Execute(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"POST namespaces/team"}, server.writes)
	}
}
