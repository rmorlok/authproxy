package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/require"
)

func pruneServer(t *testing.T) (*batchServer, *Client) {
	s, _ := newBatchServer(t)
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/")

		if r.Method == "DELETE" {
			s.writes = append(s.writes, "DELETE "+path)
			delete(s.objects, path)
			w.WriteHeader(204)
			return
		}

		if r.Method == "GET" &&
			!strings.Contains(path, "/") &&
			r.URL.Query().Get("name") == "" {
			var keys []string

			for key := range s.objects {
				if strings.HasPrefix(key, path+"/") {
					keys = append(keys, key)
				}
			}

			sort.Strings(keys)

			items := []string{}
			for _, key := range keys {
				data, err := json.Marshal(s.objects[key])
				require.NoError(t, err)
				items = append(items, string(data))
			}

			kind := map[string]string{
				"actors":      "Actor",
				"keys":        "Key",
				"rate-limits": "RateLimit",
				"namespaces":  "Namespace",
			}[path]

			fmt.Fprint(w, listJSON(kind, items, ""))

			return
		}

		s.handle(w, r)
	}, false)
	return s, c
}

// seedApply applies one or more documents to the server
func seedApply(t *testing.T, c *Client, docs ...Document) []Result {
	t.Helper()
	b, err := c.Prepare(
		context.Background(),
		docs,
		ReconcileOptions{Overwrite: true},
	)
	require.NoError(t, err)

	results, err := b.Execute(context.Background())
	require.NoError(t, err)

	return results
}

func pruneOptions() PruneOptions {
	return PruneOptions{
		Namespace: "root",
		All:       true,
		Allowlist: []string{"Actor"},
		Wait:      true,
		Timeout:   time.Second,
	}
}

func TestPruneExactScopeAndManagedCandidates(t *testing.T) {
	s, c := pruneServer(t)
	ctx := context.Background()
	keep := batchDoc(
		t,
		"Actor",
		"keep",
		"root",
		`{"externalId":"keep"}`,
	)
	obsolete := batchDoc(
		t,
		"Actor",
		"obsolete",
		"root",
		`{"externalId":"obsolete"}`,
	)

	// Seed two apply-managed actors in root. The next apply will keep only one,
	// leaving the other eligible for pruning.
	seeded := seedApply(t, c, keep, obsolete)

	// Create an actor without apply history in the same namespace. Pruning
	// must preserve resources that were never managed by apply.
	unmanaged, err := c.Create(
		ctx,
		batchDoc(
			t,
			"Actor",
			"unmanaged",
			"root",
			`{"externalId":"unmanaged"}`,
		),
	)
	require.NoError(t, err)

	// Create an actor in a descendant namespace to exercise exact namespace
	// scoping, even when the inventory includes descendants.
	child := batchDoc(
		t,
		"Actor",
		"descendant",
		"root.child",
		`{"externalId":"descendant"}`,
	)
	live, err := c.Create(ctx, child)
	require.NoError(t, err)

	// Build last-applied history for the child actor so it can be marked as
	// apply-managed without running another apply batch.
	h, err := newHistory(child)
	require.NoError(t, err)

	// Attach that history to the stored actor. It must now survive because it
	// is outside root, not because it lacks apply history.
	require.NoError(t, attachHistory(s.objects["actors/"+live.Metadata.ID], h))

	// Prepare the desired set containing only keep, intentionally omitting
	// obsolete so the previously applied actor becomes a prune candidate.
	b, err := c.Prepare(ctx, []Document{keep}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)

	// Inventory candidates before applying any writes. Only obsolete should
	// qualify: keep is desired, unmanaged has no history, and child is out of scope.
	p, err := b.PreparePrune(ctx, pruneOptions())
	require.NoError(t, err)
	require.Len(t, p.candidates, 1)

	// Complete the apply successfully before pruning; deletion is gated on
	// the associated batch succeeding.
	_, err = b.Execute(ctx)
	require.NoError(t, err)

	// Execute the prepared prune plan and verify that only obsolete was
	// deleted, preserving the desired, unmanaged, and child-namespace actors.
	results, err := p.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "pruned", results[0].Status)
	require.NotContains(t, s.objects, "actors/"+seeded[1].Identity)
	require.Contains(t, s.objects, "actors/"+seeded[0].Identity)
	require.Contains(t, s.objects, "actors/"+unmanaged.Metadata.ID)
	require.Contains(t, s.objects, "actors/"+live.Metadata.ID)

	// A prune plan is single-use, so a second execution must be rejected.
	_, err = p.Execute(ctx)
	require.ErrorContains(t, err, "already executed")
}

func TestPruneRejectsUnsafeScopes(t *testing.T) {
	for _, o := range []PruneOptions{
		{All: true, Allowlist: []string{"Actor"}},
		{Namespace: "root", Allowlist: []string{"Actor"}},
		{Namespace: "root", All: true, Selector: "team=ops", Allowlist: []string{"Actor"}},
		{Namespace: "root", All: true},
		{Namespace: "root", All: true, Allowlist: []string{"Namespace"}},
		{Namespace: "root", All: true, Allowlist: []string{"Connector"}},
		{Namespace: "root", All: true, Allowlist: []string{"Connection"}},
	} {
		require.Error(t, o.Validate())
	}
	require.NoError(t, (PruneOptions{Namespace: "root", Selector: "team=ops", Allowlist: []string{"authproxy.net/v1alpha1/Actor"}}).Validate())
}

func TestPruneNeverDeletesAfterFailedApplyOrCandidateChange(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) (*batchServer, *Batch, *PrunePlan, string) {
		t.Helper()
		s, c := pruneServer(t)
		keep := batchDoc(t, "Actor", "keep", "root", `{"externalId":"keep"}`)
		old := batchDoc(t, "Actor", "old", "root", `{"externalId":"old"}`)
		seeded := seedApply(t, c, old)

		// Omit the previously applied actor so the plan has a deletion candidate.
		b, err := c.Prepare(ctx, []Document{keep}, ReconcileOptions{Overwrite: true})
		require.NoError(t, err)
		p, err := b.PreparePrune(ctx, pruneOptions())
		require.NoError(t, err)
		require.Len(t, p.candidates, 1)

		return s, b, p, "actors/" + seeded[0].Identity
	}
	assertNoDeletion := func(t *testing.T, s *batchServer, candidatePath string) {
		t.Helper()
		for _, write := range s.writes {
			require.NotContains(t, write, "DELETE")
		}
		require.Contains(t, s.objects, candidatePath)
	}

	t.Run("rejects prune after a failed apply", func(t *testing.T) {
		s, b, p, candidatePath := setup(t)
		s.failName = "keep"

		_, err := b.Execute(ctx)
		require.Error(t, err)

		_, err = p.Execute(ctx)
		require.Error(t, err)
		assertNoDeletion(t, s, candidatePath)
	})

	t.Run("rejects prune when a candidate changed after planning", func(t *testing.T) {
		s, b, p, candidatePath := setup(t)
		_, err := b.Execute(ctx)
		require.NoError(t, err)

		// Simulate a concurrent edit after the deletion candidate was captured.
		liveMeta(s.objects[candidatePath])["labels"] = map[string]any{"changed": "yes"}

		_, err = p.Execute(ctx)
		require.Error(t, err)
		assertNoDeletion(t, s, candidatePath)
	})

	t.Run("rejects prune before apply has executed", func(t *testing.T) {
		s, _, p, candidatePath := setup(t)

		_, err := p.Execute(ctx)
		require.Error(t, err)
		assertNoDeletion(t, s, candidatePath)
	})
}

func TestPruneRejectsIncompleteInventoryAndReferences(t *testing.T) {
	t.Run("inventory", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"apiVersion":"authproxy.net/v1alpha1","kind":"ActorList","metadata":{"remainingItemCount":1},"items":[]}`)
		}, false)
		_, err := c.listAll(context.Background(), "Actor", "root")
		require.ErrorContains(t, err, "incomplete")
	})
	t.Run("reference", func(t *testing.T) {
		s, c := pruneServer(t)
		ctx := context.Background()
		key := batchDoc(t, "Key", "oldkey", "root", `{"keyData":{"numBytes":32}}`)
		seeded := seedApply(t, c, key)
		s.objects["namespaces/root"]["spec"] = map[string]any{"encryptionKeyRef": map[string]any{"apiVersion": "authproxy.net/v1alpha1", "kind": "Key", "id": seeded[0].Identity}}
		keep := batchDoc(t, "Actor", "keep", "root", `{"externalId":"keep"}`)
		b, err := c.Prepare(ctx, []Document{keep}, ReconcileOptions{Overwrite: true})
		require.NoError(t, err)
		options := pruneOptions()
		options.Allowlist = []string{"Key"}
		p, err := b.PreparePrune(ctx, options)
		require.NoError(t, err)
		_, err = b.Execute(ctx)
		require.NoError(t, err)
		_, err = p.Execute(ctx)
		require.ErrorContains(t, err, "referenced")
		require.Contains(t, s.objects, "keys/"+seeded[0].Identity)
	})
}

func TestPruneWaitTimeoutAndMissingGuardedEndpoint(t *testing.T) {
	id := apid.New(apid.PrefixActor).String()
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, actorJSON(id, "old", "root")) }, false)
	p := &PrunePlan{client: c, options: PruneOptions{Timeout: time.Millisecond}}
	require.ErrorIs(t, p.waitDeleted(context.Background(), "Actor", "actors/"+id), context.DeadlineExceeded)
	keyID := apid.New(apid.PrefixKey).String()
	deletes := []string{}
	c = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			deletes = append(deletes, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if r.URL.Path == "/api/v1/namespaces" {
			fmt.Fprint(w, listJSON("Namespace", nil, ""))
			return
		}
		fmt.Fprint(w, liveJSON("Key", keyID, "old", "root", `{}`))
	}, false)
	live, err := c.get(context.Background(), "Key", "keys/"+keyID)
	require.NoError(t, err)
	p = &PrunePlan{client: c, options: pruneOptions(), candidates: []*LiveResource{live}, batch: &Batch{}}
	p.batch.completedSuccessfully.Store(true)
	_, err = p.Execute(context.Background())
	require.Error(t, err)
	require.Equal(t, []string{"/api/v1/keys/" + keyID + "/unused"}, deletes, "older servers must never fall back to destructive ordinary Key deletion")
}
