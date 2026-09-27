package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/require"
)

func TestBatchRefreshPreservesConcurrentMetadata(t *testing.T) {
	for _, tc := range []struct {
		kind, collection, spec string
		prefix                 apid.Prefix
	}{
		{"Namespace", "namespaces", `{}`, ""},
		{"Actor", "actors", `{"externalId":"subject"}`, apid.PrefixActor},
		{"Key", "keys", `{}`, apid.PrefixKey},
		{"RateLimit", "rate-limits", `{"algorithm":{"tokenBucket":{"capacity":10,"refillRate":1}}}`, apid.PrefixRateLimit},
		{"Connection", "connections", `{}`, apid.PrefixConnection},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			s, c := newBatchServer(t)
			id := "root.example"
			if tc.prefix != "" {
				id = apid.New(tc.prefix).String()
			}
			path := tc.collection + "/" + id
			var obj map[string]any
			require.NoError(t, json.Unmarshal([]byte(liveJSON(tc.kind, id, "example", "root", tc.spec)), &obj))

			if tc.kind == "Connection" {
				// Connection metadata patches validate the full live contract.
				obj["metadata"].(map[string]any)["createdAt"] = "2026-09-01T00:00:00Z"
				obj["metadata"].(map[string]any)["updatedAt"] = "2026-09-01T00:00:00Z"
				obj["spec"] = map[string]any{
					"connectorRef": map[string]any{
						"apiVersion": "authproxy.net/v1alpha1",
						"kind":       "Connector",
						"id":         apid.New(apid.PrefixConnector).String(),
						"generation": 1,
					},
				}
				obj["status"] = map[string]any{
					"lifecycle": map[string]any{
						"state": "configured"},
					"health": map[string]any{
						"state": "healthy",
					},
					"configuration": map[string]any{
						"configured": true,
						"schema": map[string]any{
							"type": "object",
						},
					},
				}
			}

			s.objects[path] = obj

			doc := clientDoc(t, tc.kind, "  name: example\n  namespace: root\n  labels: {team: blue}", `{}`)
			b, err := c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: true})
			require.NoError(t, err)

			// Changed after all preparation reads, before Execute's refresh.
			liveMeta(obj)["labels"] = map[string]any{"audit": "external"}
			liveMeta(obj)["annotations"] = map[string]any{"example.com/external": "preserved"}

			result, err := b.Execute(context.Background())
			require.NoError(t, err)
			require.Equal(t, "configured", result[0].Status)

			m := liveMeta(s.objects[path])
			require.Equal(t, map[string]any{"audit": "external", "team": "blue"}, m["labels"])
			require.Equal(t, "preserved", m["annotations"].(map[string]any)["example.com/external"])
			require.Contains(t, m["annotations"], LastAppliedAnnotation)
		})
	}
}

func TestBatchRefreshDriftAndIdentity(t *testing.T) {
	for _, overwrite := range []bool{false, true} {
		t.Run(fmt.Sprintf("overwrite=%t", overwrite), func(t *testing.T) {
			s, c := newBatchServer(t)
			doc := clientDoc(t, "Actor", "  name: example\n  namespace: root\n  labels: {team: blue}", `{"externalId":"subject"}`)
			b, err := c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: overwrite})
			require.NoError(t, err)
			results, err := b.Execute(context.Background())
			require.NoError(t, err)
			path := "actors/" + results[0].Identity
			// Bind assigned identity into history so preparation yields unchanged.
			b, err = c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: overwrite})
			require.NoError(t, err)
			_, err = b.Execute(context.Background())
			require.NoError(t, err)
			b, err = c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: overwrite})
			require.NoError(t, err)
			require.Equal(t, OperationUnchanged, b.plans[0].Operation)
			liveMeta(s.objects[path])["labels"].(map[string]any)["team"] = "external"
			before := len(s.writes)
			results, err = b.Execute(context.Background())
			if overwrite {
				require.NoError(t, err)
				require.Equal(t, OperationUpdate, results[0].Operation)
				require.Equal(t, "blue", liveMeta(s.objects[path])["labels"].(map[string]any)["team"])
			} else {
				require.ErrorIs(t, err, ErrBatchFailed)
				require.Contains(t, results[0].Error, "conflict")
				require.Len(t, s.writes, before)
			}
		})
	}
	t.Run("deleted existing target is not recreated or replaced by name", func(t *testing.T) {
		s, c := newBatchServer(t)
		id := apid.New(apid.PrefixActor).String()
		var obj map[string]any
		require.NoError(t, json.Unmarshal([]byte(actorJSON(id, "example", "root")), &obj))
		s.objects["actors/"+id] = obj
		b, err := c.Prepare(context.Background(), []Document{batchDoc(t, "Actor", "example", "root", `{}`)}, ReconcileOptions{Overwrite: true})
		require.NoError(t, err)
		delete(s.objects, "actors/"+id)
		replacement := apid.New(apid.PrefixActor).String()
		liveMeta(obj)["id"] = replacement
		s.objects["actors/"+replacement] = obj
		_, err = b.Execute(context.Background())
		require.ErrorIs(t, err, ErrBatchFailed)
		require.Empty(t, s.writes)
	})
	t.Run("a resource appearing before a create is not silently adopted", func(t *testing.T) {
		s, c := newBatchServer(t)
		doc := batchDoc(t, "Actor", "example", "root", `{"externalId":"subject"}`)
		b, err := c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: true})
		require.NoError(t, err)
		id := apid.New(apid.PrefixActor).String()
		var obj map[string]any
		require.NoError(t, json.Unmarshal([]byte(actorJSON(id, "example", "root")), &obj))
		s.objects["actors/"+id] = obj
		results, err := b.Execute(context.Background())
		require.ErrorIs(t, err, ErrBatchFailed)
		require.Contains(t, results[0].Error, "appeared after preparation")
		require.Empty(t, s.writes)
	})
}

func TestBatchNeverRetriesMutationFailures(t *testing.T) {
	for _, method := range []string{"POST", "PATCH"} {
		for _, status := range []int{http.StatusConflict, http.StatusPreconditionFailed, http.StatusInternalServerError, http.StatusOK} {
			t.Run(fmt.Sprintf("%s/%d", method, status), func(t *testing.T) {
				s, _ := newBatchServer(t)
				attempts := 0
				if method == "PATCH" {
					id := apid.New(apid.PrefixActor).String()
					var obj map[string]any
					require.NoError(t, json.Unmarshal([]byte(actorJSON(id, "example", "root")), &obj))
					s.objects["actors/"+id] = obj
				}
				c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
					if r.Method == "GET" {
						s.handle(w, r)
						return
					}
					attempts++
					require.Equal(t, method, r.Method)
					w.WriteHeader(status)
					fmt.Fprint(w, "invalid sensitive response")
				}, false)
				doc := batchDoc(t, "Actor", "example", "root", `{"externalId":"external-bob"}`)
				b, err := c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: true})
				require.NoError(t, err)
				results, err := b.Execute(context.Background())
				require.ErrorIs(t, err, ErrBatchFailed)
				require.Equal(t, 1, attempts)
				require.NotContains(t, results[0].Error, "sensitive")
				require.True(t, strings.Contains(results[0].Error, "uncertain"))
				_, err = b.Execute(context.Background())
				require.Error(t, err)
				require.Equal(t, 1, attempts)
			})
		}
	}
}

func TestBatchRefreshRejectsHistoryRemoval(t *testing.T) {
	s, c := newBatchServer(t)
	doc := batchDoc(t, "Actor", "example", "root", `{"externalId":"subject"}`)
	b, err := c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	results, err := b.Execute(context.Background())
	require.NoError(t, err)
	b, err = c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	liveMeta(s.objects["actors/"+results[0].Identity])["annotations"] = map[string]any{}
	before := len(s.writes)
	results, err = b.Execute(context.Background())
	require.ErrorIs(t, err, ErrBatchFailed)
	require.Contains(t, results[0].Error, "history was removed")
	require.Len(t, s.writes, before)
}
