package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/stretchr/testify/require"
)

func TestConfiguredActorsExternalSources(t *testing.T) {
	t.Run("decodes namespaced sources from JSON", func(t *testing.T) {
		var actors ConfiguredActors
		err := json.Unmarshal([]byte(`{
  "root": {"keysPath": "/keys/root"},
  "root.smoke": {
    "keysPath": "/keys/smoke",
    "permissions": [{"namespace": "root.smoke.{{external_id}}", "resources": ["connections"], "verbs": ["create"]}]
  },
  "syncCronSchedule": "* * * * *"
}`), &actors)
		require.NoError(t, err)

		sources, ok := actors.InnerVal.(*ConfiguredActorsExternalSources)
		require.True(t, ok)
		require.Equal(t, "* * * * *", sources.SyncCronSchedule)
		require.Equal(t, "/keys/root", sources.Sources["root"].KeysPath)
		require.Equal(t, "root.smoke.{{external_id}}", sources.Sources["root.smoke"].Permissions[0].Namespace)
	})

	t.Run("rejects invalid source namespace", func(t *testing.T) {
		var actors ConfiguredActors
		err := json.Unmarshal([]byte(`{"smoke": {"keysPath": "/keys/smoke"}}`), &actors)
		require.ErrorContains(t, err, "invalid actor source namespace")
	})

	t.Run("rejects legacy single source", func(t *testing.T) {
		var actors ConfiguredActors
		err := json.Unmarshal([]byte(`{"keysPath": "/keys/root"}`), &actors)
		require.ErrorContains(t, err, "invalid actor source namespace")
	})

	t.Run("loads every source into its namespace", func(t *testing.T) {
		rootKeys := t.TempDir()
		smokeKeys := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(rootKeys, "admin.pub"), []byte("root-key"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(smokeKeys, "smoke-user.pub"), []byte("smoke-key"), 0o600))

		sources := &ConfiguredActorsExternalSources{
			Sources: map[string]*ConfiguredActorsExternalSource{
				"root": {KeysPath: rootKeys},
				"root.smoke": {
					KeysPath: smokeKeys,
					Permissions: []aschema.Permission{{
						Namespace: "root.smoke.{{external_id}}",
						Resources: []string{"connections"},
						Verbs:     []string{"create"},
					}},
				},
			},
		}

		loaded := sources.All()
		require.Len(t, loaded, 2)
		require.Equal(t, "admin", loaded[0].ExternalId)
		require.Equal(t, "root", loaded[0].Namespace)
		require.Equal(t, "smoke-user", loaded[1].ExternalId)
		require.Equal(t, "root.smoke", loaded[1].Namespace)
		require.Equal(t, "root.smoke.{{external_id}}", loaded[1].Permissions[0].Namespace)
	})
}
