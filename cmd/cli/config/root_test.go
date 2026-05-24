package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshallYamlRoot(t *testing.T) {
	t.Run("minimal config", func(t *testing.T) {
		data := []byte(`
admin_username: bobdole
admin_private_key_path: /home/bob/.authproxy/admin.key
server:
  api: http://localhost:8081
`)
		root, err := UnmarshallYamlRoot(data)
		require.NoError(t, err)
		require.NotNil(t, root)

		assert.Equal(t, "bobdole", root.AdminUsername())
		assert.Equal(t, "/home/bob/.authproxy/admin.key", root.AdminPrivateKeyPath())
		assert.Equal(t, "", root.AdminSharedKeyPath())
		assert.Equal(t, "http://localhost:8081", root.ApiUrl())
		assert.Equal(t, "", root.AdminApiUrl())
		assert.Equal(t, "", root.AuthUrl())
		assert.Equal(t, "", root.MarketplaceUrl())
		assert.Equal(t, "", root.AdminUiUrl())
	})

	t.Run("full config with all server urls", func(t *testing.T) {
		data := []byte(`
admin_username: bobdole
admin_private_key_path: /home/bob/.authproxy/admin.key
admin_shared_key_path: /home/bob/.authproxy/shared.key
server:
  api: http://localhost:8081
  admin_api: http://localhost:8082
  auth: http://localhost:8080
  marketplace: http://localhost:5173
  admin_ui: http://localhost:5174
`)
		root, err := UnmarshallYamlRoot(data)
		require.NoError(t, err)

		assert.Equal(t, "bobdole", root.AdminUsername())
		assert.Equal(t, "/home/bob/.authproxy/admin.key", root.AdminPrivateKeyPath())
		assert.Equal(t, "/home/bob/.authproxy/shared.key", root.AdminSharedKeyPath())
		assert.Equal(t, "http://localhost:8081", root.ApiUrl())
		assert.Equal(t, "http://localhost:8082", root.AdminApiUrl())
		assert.Equal(t, "http://localhost:8080", root.AuthUrl())
		assert.Equal(t, "http://localhost:5173", root.MarketplaceUrl())
		assert.Equal(t, "http://localhost:5174", root.AdminUiUrl())
	})

	t.Run("admin_username via env var", func(t *testing.T) {
		t.Setenv("CLI_ROOT_TEST_USERNAME", "alice")
		data := []byte(`
admin_username:
  env_var: CLI_ROOT_TEST_USERNAME
`)
		root, err := UnmarshallYamlRoot(data)
		require.NoError(t, err)
		assert.Equal(t, "alice", root.AdminUsername())
	})

	t.Run("admin_username via env var with default", func(t *testing.T) {
		data := []byte(`
admin_username:
  env_var: CLI_ROOT_TEST_UNSET_USERNAME
  default: defaultuser
`)
		root, err := UnmarshallYamlRoot(data)
		require.NoError(t, err)
		assert.Equal(t, "defaultuser", root.AdminUsername())
	})

	t.Run("admin_username via env var unset returns empty (not error)", func(t *testing.T) {
		data := []byte(`
admin_username:
  env_var: CLI_ROOT_TEST_UNSET_USERNAME_NO_DEFAULT
`)
		root, err := UnmarshallYamlRoot(data)
		require.NoError(t, err)
		assert.Equal(t, "", root.AdminUsername())
	})

	t.Run("server api via templated env vars", func(t *testing.T) {
		t.Setenv("CLI_ROOT_TEST_HOST", "ap.example.com")
		t.Setenv("CLI_ROOT_TEST_PORT", "8081")
		data := []byte(`
server:
  api:
    template_env_vars: http://{{CLI_ROOT_TEST_HOST}}:{{CLI_ROOT_TEST_PORT}}
`)
		root, err := UnmarshallYamlRoot(data)
		require.NoError(t, err)
		assert.Equal(t, "http://ap.example.com:8081", root.ApiUrl())
	})

	t.Run("server api via templated env vars falls back to default when missing", func(t *testing.T) {
		data := []byte(`
server:
  api:
    template_env_vars: http://{{CLI_ROOT_TEST_MISSING}}:8081
    default: http://localhost:8081
`)
		root, err := UnmarshallYamlRoot(data)
		require.NoError(t, err)
		assert.Equal(t, "http://localhost:8081", root.ApiUrl())
	})

	t.Run("empty config yields empty values", func(t *testing.T) {
		root, err := UnmarshallYamlRoot([]byte(`{}`))
		require.NoError(t, err)
		require.NotNil(t, root)

		assert.Equal(t, "", root.AdminUsername())
		assert.Equal(t, "", root.AdminPrivateKeyPath())
		assert.Equal(t, "", root.AdminSharedKeyPath())
		assert.Equal(t, "", root.ApiUrl())
		assert.Equal(t, "", root.AdminApiUrl())
		assert.Equal(t, "", root.AuthUrl())
		assert.Equal(t, "", root.MarketplaceUrl())
		assert.Equal(t, "", root.AdminUiUrl())
	})

	t.Run("malformed yaml returns error", func(t *testing.T) {
		_, err := UnmarshallYamlRoot([]byte("admin_username: : :"))
		require.Error(t, err)
	})
}

func TestRoot_NilReceiverSafety(t *testing.T) {
	// All accessors must tolerate a nil receiver; the resolver relies on this when no
	// config file is present.
	var r *Root

	assert.Equal(t, "", r.AdminUsername())
	assert.Equal(t, "", r.AdminPrivateKeyPath())
	assert.Equal(t, "", r.AdminSharedKeyPath())
	assert.Equal(t, "", r.ApiUrl())
	assert.Equal(t, "", r.AdminApiUrl())
	assert.Equal(t, "", r.AuthUrl())
	assert.Equal(t, "", r.MarketplaceUrl())
	assert.Equal(t, "", r.AdminUiUrl())
}
