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
	vaultTestEnv      = "AUTH_PROXY_VAULT_TEST"
	vaultAddrEnv      = "VAULT_ADDR"
	vaultTokenEnv     = "VAULT_TOKEN"
	vaultKvMount      = "secret"
	vaultTransitMount = "transit"
	vaultValueField   = "value"
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
	ekID := apid.New(apid.PrefixKey)

	require.NoError(t, env.Db.CreateNamespace(ctx, &database.Namespace{
		Path: namespace,
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

	require.NoError(t, env.Db.CreateKey(ctx, &database.Key{
		Id:               ekID,
		Namespace:        namespace,
		EncryptedKeyData: &encKeyData,
		State:            database.KeyStateActive,
	}))
	_, err = env.Db.SetNamespaceKeyId(ctx, namespace, &ekID)
	require.NoError(t, err)
	currentV1 := createDataEncryptionKeyForIntegrationTest(t, ctx, env.Db, ekID, &keyData)

	require.NoError(t, encrypt.SyncKeysToDatabase(ctx, env.Cfg, env.Db, env.Logger, nil))
	require.NoError(t, env.DM.GetEncryptService().SyncKeysFromDbToMemory(ctx))

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

	currentV2, err := env.Db.GetCurrentDataEncryptionKeyForKey(ctx, ekID)
	require.NoError(t, err)
	require.Equal(t, currentV1.Id, currentV2.Id)
	require.Equal(t, "2", currentV2.ProviderVersion)

	require.NoError(t, runReencryptAll(ctx, env))

	updated, err := env.Db.GetActor(ctx, actorID)
	require.NoError(t, err)
	require.NotNil(t, updated.EncryptedKey)
	require.Equal(t, currentV2.Id, updated.EncryptedKey.ID)
	require.Equal(t, encrypted.Data, updated.EncryptedKey.Data)

	decrypted, err := env.DM.GetEncryptService().DecryptString(ctx, *updated.EncryptedKey)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}

func TestVaultTransitKeySyncAndReencrypt(t *testing.T) {
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
	ensureVaultTransitMount(t, ctx, client, vaultTransitMount)

	env := helpers.Setup(t, helpers.SetupOptions{Service: helpers.ServiceTypeAPI})
	defer env.Cleanup()

	transitKeyName := fmt.Sprintf("authproxy-transit-test-%d", time.Now().UnixNano())
	_, err := client.Logical().WriteWithContext(ctx, fmt.Sprintf("%s/keys/%s", vaultTransitMount, transitKeyName), map[string]interface{}{
		"type": "aes256-gcm96",
	})
	require.NoError(t, err)

	namespace := fmt.Sprintf("root.vault-transit-test-%d", time.Now().UnixNano())
	keyID := apid.New(apid.PrefixKey)

	require.NoError(t, env.Db.CreateNamespace(ctx, &database.Namespace{
		Path: namespace,
	}))

	keyData := sconfig.KeyData{
		InnerVal: &sconfig.KeyDataVaultTransit{
			VaultAddress:          vaultAddr,
			VaultToken:            vaultToken,
			VaultTransitMountPath: vaultTransitMount,
			VaultTransitKeyName:   transitKeyName,
		},
	}
	keyDataJSON, err := json.Marshal(&keyData)
	require.NoError(t, err)

	encKeyData, err := env.DM.GetEncryptService().EncryptGlobal(ctx, keyDataJSON)
	require.NoError(t, err)

	require.NoError(t, env.Db.CreateKey(ctx, &database.Key{
		Id:               keyID,
		Namespace:        namespace,
		MaterialType:     database.KeyMaterialTypeExternal,
		EncryptedKeyData: &encKeyData,
		State:            database.KeyStateActive,
	}))
	_, err = env.Db.SetNamespaceKeyId(ctx, namespace, &keyID)
	require.NoError(t, err)

	currentV1 := createDataEncryptionKeyForIntegrationTest(t, ctx, env.Db, keyID, &keyData)
	require.Equal(t, string(sconfig.ProviderTypeHashicorpVaultTransit), currentV1.Provider)
	require.Equal(t, fmt.Sprintf("%s/%s", vaultTransitMount, transitKeyName), currentV1.ProviderID)
	require.Equal(t, "1", currentV1.ProviderVersion)
	require.NotNil(t, currentV1.ProtectedData)
	require.Equal(t, string(sconfig.ProviderTypeHashicorpVaultTransit), currentV1.ProtectedData.Type)
	require.NotEmpty(t, currentV1.ProtectedData.WrappedData)

	require.NoError(t, encrypt.SyncKeysToDatabase(ctx, env.Cfg, env.Db, env.Logger, nil))
	require.NoError(t, env.DM.GetEncryptService().SyncKeysFromDbToMemory(ctx))

	plaintext := "vault-transit-test"
	encrypted, err := env.DM.GetEncryptService().EncryptStringForNamespace(ctx, namespace, plaintext)
	require.NoError(t, err)
	require.Equal(t, currentV1.Id, encrypted.ID)

	actorID := apid.New(apid.PrefixActor)
	require.NoError(t, env.Db.CreateActor(ctx, &database.Actor{
		Id:           actorID,
		Namespace:    namespace,
		ExternalId:   "vault-transit-test-actor",
		EncryptedKey: &encrypted,
	}))

	_, err = client.Logical().WriteWithContext(ctx, fmt.Sprintf("%s/keys/%s/rotate", vaultTransitMount, transitKeyName), map[string]interface{}{})
	require.NoError(t, err)

	require.NoError(t, encrypt.SyncKeysToDatabase(ctx, env.Cfg, env.Db, env.Logger, nil))
	require.NoError(t, env.DM.GetEncryptService().SyncKeysFromDbToMemory(ctx))

	currentV2, err := env.Db.GetCurrentDataEncryptionKeyForKey(ctx, keyID)
	require.NoError(t, err)
	require.Equal(t, currentV1.Id, currentV2.Id)
	require.Equal(t, "2", currentV2.ProviderVersion)
	require.NotEqual(t, currentV1.ProtectedData.WrappedData, currentV2.ProtectedData.WrappedData)

	require.NoError(t, runReencryptAll(ctx, env))

	updated, err := env.Db.GetActor(ctx, actorID)
	require.NoError(t, err)
	require.NotNil(t, updated.EncryptedKey)
	require.Equal(t, currentV2.Id, updated.EncryptedKey.ID)
	require.Equal(t, encrypted.Data, updated.EncryptedKey.Data)

	freshEncryptService := encrypt.NewEncryptService(env.Cfg, env.Db, env.Logger)
	freshEncryptService.Start()
	defer freshEncryptService.Shutdown()

	decrypted, err := freshEncryptService.DecryptString(ctx, *updated.EncryptedKey)
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

func ensureVaultTransitMount(t *testing.T, ctx context.Context, client *vault.Client, mount string) {
	t.Helper()

	mounts, err := client.Sys().ListMountsWithContext(ctx)
	require.NoError(t, err)
	if _, ok := mounts[mount+"/"]; ok {
		return
	}

	require.NoError(t, client.Sys().MountWithContext(ctx, mount, &vault.MountInput{
		Type: "transit",
	}))
}
