package apply

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHistoryCommandsOnlyChangeSanitizedHistory(t *testing.T) {
	s, c := newBatchServer(t)
	ctx := context.Background()
	doc := batchDoc(t, "Actor", "example", "root", `{"externalId":"original","signingKey":{"sharedKey":{"value":"TOPSECRET"}}}`)
	live, err := c.Create(ctx, doc)
	require.NoError(t, err)
	targets, err := c.HistoryTargets(ctx, []Document{doc})
	require.NoError(t, err)
	_, err = targets[0].LastApplied()
	require.ErrorContains(t, err, "no last-applied")
	writes := len(s.writes)
	_, err = c.SetLastApplied(ctx, targets, []Document{doc}, false)
	require.ErrorContains(t, err, "create-annotation")
	require.Len(t, s.writes, writes)
	_, err = c.SetLastApplied(ctx, targets, []Document{doc}, true)
	require.NoError(t, err)
	targets, err = c.HistoryTargets(ctx, []Document{doc})
	require.NoError(t, err)
	desired, err := targets[0].LastApplied()
	require.NoError(t, err)
	data, err := json.Marshal(desired)
	require.NoError(t, err)
	require.NotContains(t, string(data), "TOPSECRET")
	require.NotContains(t, string(data), "signingKey")
	changed := batchDoc(t, "Actor", "example", "root", `{"externalId":"history-only"}`)
	_, err = c.SetLastApplied(ctx, targets, []Document{changed}, false)
	require.NoError(t, err)
	resource := s.objects["actors/"+live.Metadata.ID]
	require.Equal(t, "original", asSpec(resource)["externalId"])
	require.Contains(t, liveMeta(resource)["annotations"].(map[string]any)[LastAppliedAnnotation], "history-only")
	// A stale editor cannot overwrite newer history.
	writes = len(s.writes)
	_, err = c.SetLastApplied(ctx, targets, []Document{doc}, false)
	require.ErrorContains(t, err, "history changed")
	require.Len(t, s.writes, writes)
}

func TestHistoryRejectsIdentityEditsAndSecretBearingAnnotations(t *testing.T) {
	s, c := newBatchServer(t)
	ctx := context.Background()
	doc := batchDoc(t, "Actor", "example", "root", `{"externalId":"original"}`)
	batch, err := c.Prepare(ctx, []Document{doc}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	results, err := batch.Execute(ctx)
	require.NoError(t, err)
	targets, err := c.HistoryTargets(ctx, []Document{doc})
	require.NoError(t, err)
	writes := len(s.writes)
	_, err = c.SetLastApplied(ctx, targets, []Document{batchDoc(t, "Actor", "wrong", "root", `{}`)}, false)
	require.ErrorContains(t, err, "name mismatch")
	require.Len(t, s.writes, writes)
	_, err = c.SetLastApplied(ctx, targets, nil, false)
	require.Error(t, err)
	raw := liveMeta(s.objects["actors/"+results[0].Identity])["annotations"].(map[string]any)
	raw[LastAppliedAnnotation] = `{"version":1,"desired":{"apiVersion":"authproxy.net/v1alpha1","kind":"Actor","metadata":{"name":"example","namespace":"root"},"spec":{"signingKey":{"sharedKey":{"value":"TOPSECRET"}}}}}`
	targets, err = c.HistoryTargets(ctx, []Document{doc})
	require.NoError(t, err)
	_, err = targets[0].LastApplied()
	require.Error(t, err)
	require.NotContains(t, err.Error(), "TOPSECRET")
}
