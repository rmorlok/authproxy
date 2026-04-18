//go:build integration && vault

package encrypt_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	vault "github.com/hashicorp/vault/api"
	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

const (
	vaultTestEnv    = "AUTH_PROXY_VAULT_TEST"
	vaultAddrEnv    = "VAULT_ADDR"
	vaultTokenEnv   = "VAULT_TOKEN"
	vaultKvMount    = "secret"
	vaultValueField = "value"
)

func init() {
	// Best-effort load of .env files walking up from the current working
	// directory so the test is runnable locally regardless of where the
	// user keeps their .env.
	util.LoadDotEnv()
}

// TestVaultKeySyncAndReencrypt verifies that encryption keys stored in a
// HashiCorp Vault KV v2 mount are correctly synced into the database, that
// new KV v2 versions are picked up on re-sync, and that the re-encryption
// pipeline rewrites data encrypted under a prior version to the new current
// version.
func TestVaultKeySyncAndReencrypt(t *testing.T) {
	if os.Getenv(vaultTestEnv) != "1" {
		t.Skipf("%s is not set to 1", vaultTestEnv)
	}

	vaultAddr := os.Getenv(vaultAddrEnv)
	if vaultAddr == "" {
		t.Skipf("%s is not set", vaultAddrEnv)
	}

	vaultToken := os.Getenv(vaultTokenEnv)
	if vaultToken == "" {
		t.Skipf("%s is not set", vaultTokenEnv)
	}

	ctx := context.Background()
	client := newVaultClient(t, vaultAddr, vaultToken)

	env := helpers.Setup(t, helpers.SetupOptions{Service: helpers.ServiceTypeAPI})
	defer env.Cleanup()

	secretPath := fmt.Sprintf("authproxy-vault-test-%d", time.Now().UnixNano())

	// Vault KV v2 stores string values, so we hex-encode 16 random bytes
	// (32 ASCII chars) so the resulting []byte is a valid 32-byte AES-256 key.
	keyV1 := hex.EncodeToString(randomBytes(t, 16))

	_, err := client.KVv2(vaultKvMount).Put(ctx, secretPath, map[string]interface{}{
		vaultValueField: keyV1,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = client.KVv2(vaultKvMount).DeleteMetadata(cleanupCtx, secretPath)
	})

	namespace := fmt.Sprintf("root.vault-test-%d", time.Now().UnixNano())
	ekID := apid.New(apid.PrefixEncryptionKey)

	require.NoError(t, env.Db.CreateNamespace(ctx, &database.Namespace{
		Path:            namespace,
		EncryptionKeyId: &ekID,
	}))

	keyData := sconfig.KeyData{
		InnerVal: &sconfig.KeyDataVault{
			VaultAddress: vaultAddr,
			VaultToken:   vaultToken,
			VaultPath:    fmt.Sprintf("%s/data/%s", vaultKvMount, secretPath),
			VaultKey:     vaultValueField,
		},
	}
	keyDataJSON, err := json.Marshal(&keyData)
	require.NoError(t, err)

	encKeyData, err := env.DM.GetEncryptService().EncryptGlobal(ctx, keyDataJSON)
	require.NoError(t, err)

	require.NoError(t, env.Db.CreateEncryptionKey(ctx, &database.EncryptionKey{
		Id:               ekID,
		Namespace:        namespace,
		EncryptedKeyData: &encKeyData,
		State:            database.EncryptionKeyStateActive,
	}))

	require.NoError(t, encrypt.SyncKeysToDatabase(ctx, env.Cfg, env.Db, env.Logger, nil))
	require.NoError(t, env.DM.GetEncryptService().SyncKeysFromDbToMemory(ctx))

	currentV1, err := env.Db.GetCurrentEncryptionKeyVersionForNamespace(ctx, namespace)
	require.NoError(t, err)
	require.Equal(t, "1", currentV1.ProviderVersion)

	plaintext := "vault-kv-test"
	encrypted, err := env.DM.GetEncryptService().EncryptStringForNamespace(ctx, namespace, plaintext)
	require.NoError(t, err)
	require.Equal(t, currentV1.Id, encrypted.ID)

	actorID := apid.New(apid.PrefixActor)
	require.NoError(t, env.Db.CreateActor(ctx, &database.Actor{
		Id:           actorID,
		Namespace:    namespace,
		ExternalId:   "vault-test-actor",
		EncryptedKey: &encrypted,
	}))

	// Rotate: write a new KV v2 version to the same path.
	keyV2 := hex.EncodeToString(randomBytes(t, 16))
	require.NotEqual(t, keyV1, keyV2)

	_, err = client.KVv2(vaultKvMount).Put(ctx, secretPath, map[string]interface{}{
		vaultValueField: keyV2,
	})
	require.NoError(t, err)

	require.NoError(t, encrypt.SyncKeysToDatabase(ctx, env.Cfg, env.Db, env.Logger, nil))
	require.NoError(t, env.DM.GetEncryptService().SyncKeysFromDbToMemory(ctx))

	currentV2, err := env.Db.GetCurrentEncryptionKeyVersionForNamespace(ctx, namespace)
	require.NoError(t, err)
	require.NotEqual(t, currentV1.Id, currentV2.Id)
	require.Equal(t, "2", currentV2.ProviderVersion)

	require.NoError(t, runReencryptAll(ctx, env))

	updated, err := env.Db.GetActor(ctx, actorID)
	require.NoError(t, err)
	require.NotNil(t, updated.EncryptedKey)
	require.Equal(t, currentV2.Id, updated.EncryptedKey.ID)

	decrypted, err := env.DM.GetEncryptService().DecryptString(ctx, *updated.EncryptedKey)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}

func newVaultClient(t *testing.T, addr, token string) *vault.Client {
	t.Helper()

	cfg := vault.DefaultConfig()
	cfg.Address = addr

	client, err := vault.NewClient(cfg)
	require.NoError(t, err)

	client.SetToken(token)
	return client
}
