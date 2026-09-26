package apply

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKustomizeAuthProxyNamespacePaths(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "kustomization.yaml"), []byte("resources: [resources.yaml]\nnamespace: root.company\nnamePrefix: test-\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "resources.yaml"), []byte(`apiVersion: authproxy.net/v1alpha1
kind: Namespace
metadata: {name: team, namespace: root}
spec: {}
---
apiVersion: authproxy.net/v1alpha1
kind: Actor
metadata: {name: worker, namespace: root}
spec: {externalId: worker}
`), 0600))
	docs, err := LoadKustomize(context.Background(), directory, Options{Namespace: "root.fallback"})
	require.NoError(t, err)
	require.Len(t, docs, 2)
	for _, doc := range docs {
		require.Equal(t, "root.company", doc.Metadata.Namespace)
		if doc.Kind == "Namespace" {
			require.Equal(t, "team", string(doc.Metadata.Name))
		} else {
			require.Equal(t, "test-worker", string(doc.Metadata.Name))
		}
	}
	for _, options := range []Options{{Filenames: []string{"anything"}}, {Recursive: true}} {
		_, err = LoadKustomize(context.Background(), directory, options)
		require.Error(t, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = LoadKustomize(ctx, directory, Options{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestKustomizeFallbackAndBuildErrors(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "kustomization.yaml"), []byte("resources: [actor.yaml]\npatches:\n- target: {kind: Actor}\n  patch: |-\n    - op: add\n      path: /metadata/labels\n      value: {team: operations}\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "actor.yaml"), []byte("apiVersion: authproxy.net/v1alpha1\nkind: Actor\nmetadata: {name: worker}\nspec: {}\n"), 0600))
	docs, err := LoadKustomize(context.Background(), directory, Options{Namespace: "root.team"})
	require.NoError(t, err)
	require.Equal(t, "root.team", docs[0].Metadata.Namespace)
	require.Equal(t, "operations", docs[0].Metadata.Labels["team"])
	require.NoError(t, os.Remove(filepath.Join(directory, "actor.yaml")))
	_, err = LoadKustomize(context.Background(), directory, Options{})
	require.ErrorContains(t, err, "build failed")
}
