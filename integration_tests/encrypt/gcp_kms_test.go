//go:build integration && gcp

package encrypt_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

const (
	gcpKMSTestEnv        = "AUTH_PROXY_GCP_KMS_TEST"
	gcpKMSKeyNameEnv     = "AUTH_PROXY_GCP_KMS_KEY_NAME"
	gcpKMSKeyNameV2Env   = "AUTH_PROXY_GCP_KMS_KEY_NAME_V2"
	gcpKMSLocationEnv    = "AUTH_PROXY_GCP_KMS_LOCATION"
	gcpKMSKeyRingEnv     = "AUTH_PROXY_GCP_KMS_KEY_RING"
	gcpKMSCryptoKeyEnv   = "AUTH_PROXY_GCP_KMS_CRYPTO_KEY"
	gcpKMSEndpointEnv    = "AUTH_PROXY_GCP_KMS_ENDPOINT"
	gcpKMSProviderString = string(sconfig.ProviderTypeGcpKMS)
)

func TestGcpKMSKeySyncAndReencrypt(t *testing.T) {
	if os.Getenv(gcpKMSTestEnv) != "1" {
		t.Skipf("%s is not set to 1", gcpKMSTestEnv)
	}

	keyName := gcpKMSKeyNameFromEnv(t, "")

	ctx := context.Background()
	env := helpers.Setup(t, helpers.SetupOptions{Service: helpers.ServiceTypeAPI})
	defer env.Cleanup()

	namespace := fmt.Sprintf("root.gcp-kms-test-%d", time.Now().UnixNano())
	keyID := apid.New(apid.PrefixKey)

	require.NoError(t, env.Db.CreateNamespace(ctx, &database.Namespace{
		Path: namespace,
	}))

	keyData := gcpKMSKeyData(keyName)
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
	require.Equal(t, gcpKMSProviderString, currentV1.Provider)
	require.Equal(t, keyName, currentV1.ProviderID)
	require.NotEmpty(t, currentV1.ProviderVersion)
	require.NotNil(t, currentV1.ProtectedData)
	require.Equal(t, gcpKMSProviderString, currentV1.ProtectedData.Type)
	require.NotEmpty(t, currentV1.ProtectedData.WrappedData)

	require.NoError(t, encrypt.SyncKeysToDatabase(ctx, env.Cfg, env.Db, env.Logger, nil))
	require.NoError(t, env.DM.GetEncryptService().SyncKeysFromDbToMemory(ctx))

	plaintext := "gcp-kms-test"
	encrypted, err := env.DM.GetEncryptService().EncryptStringForNamespace(ctx, namespace, plaintext)
	require.NoError(t, err)
	require.Equal(t, currentV1.Id, encrypted.ID)

	actorID := apid.New(apid.PrefixActor)
	require.NoError(t, env.Db.CreateActor(ctx, &database.Actor{
		Id:           actorID,
		Namespace:    namespace,
		ExternalId:   "gcp-kms-test-actor",
		EncryptedKey: &encrypted,
	}))

	keyNameV2 := gcpKMSKeyNameFromEnv(t, "_V2")
	if keyNameV2 != "" {
		keyDataV2 := gcpKMSKeyData(keyNameV2)
		keyDataJSONV2, err := json.Marshal(&keyDataV2)
		require.NoError(t, err)

		encKeyDataV2, err := env.DM.GetEncryptService().EncryptGlobal(ctx, keyDataJSONV2)
		require.NoError(t, err)
		_, err = env.Db.UpdateKey(ctx, keyID, map[string]interface{}{
			"encrypted_key_data": encKeyDataV2,
		})
		require.NoError(t, err)

		require.NoError(t, encrypt.SyncKeysToDatabase(ctx, env.Cfg, env.Db, env.Logger, nil))
		require.NoError(t, env.DM.GetEncryptService().SyncKeysFromDbToMemory(ctx))

		currentV2, err := env.Db.GetCurrentDataEncryptionKeyForKey(ctx, keyID)
		require.NoError(t, err)
		require.Equal(t, currentV1.Id, currentV2.Id)
		require.Equal(t, keyNameV2, currentV2.ProviderID)
		require.NotEqual(t, currentV1.ProviderVersion, currentV2.ProviderVersion)
		require.NotEqual(t, currentV1.ProtectedData.WrappedData, currentV2.ProtectedData.WrappedData)
	} else {
		t.Logf("%s is not set; skipping GCP KMS metadata advancement rewrap path", gcpKMSKeyNameV2Env)
	}

	require.NoError(t, runReencryptAll(ctx, env))

	updated, err := env.Db.GetActor(ctx, actorID)
	require.NoError(t, err)
	require.NotNil(t, updated.EncryptedKey)
	require.Equal(t, currentV1.Id, updated.EncryptedKey.ID)
	require.Equal(t, encrypted.Data, updated.EncryptedKey.Data)

	decrypted, err := env.DM.GetEncryptService().DecryptString(ctx, *updated.EncryptedKey)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}

func TestGcpKMSGlobalAESKeyStartup(t *testing.T) {
	if os.Getenv(gcpKMSTestEnv) != "1" {
		t.Skipf("%s is not set to 1", gcpKMSTestEnv)
	}

	keyName := gcpKMSKeyNameFromEnv(t, "")

	ctx := context.Background()
	keyData := gcpKMSKeyData(keyName)
	env := setupWithGlobalKeyDataIntegrationTest(t, &keyData)
	defer env.Cleanup()

	requireGlobalKeyProviderRoundTrip(t, ctx, env, sconfig.ProviderTypeGcpKMS, keyName)
}

func gcpKMSKeyData(keyName string) sconfig.KeyData {
	return sconfig.KeyData{
		InnerVal: &sconfig.KeyDataGcpKMS{
			GcpKMSKeyName:  keyName,
			GcpKMSEndpoint: os.Getenv(gcpKMSEndpointEnv),
		},
	}
}

func gcpKMSKeyNameFromEnv(t *testing.T, suffix string) string {
	t.Helper()

	if suffix == "_V2" {
		return os.Getenv(gcpKMSKeyNameV2Env)
	}

	keyName := os.Getenv(gcpKMSKeyNameEnv)
	if keyName != "" {
		return keyName
	}

	projectID := os.Getenv(gcpProjectIDEnv)
	location := os.Getenv(gcpKMSLocationEnv)
	keyRing := os.Getenv(gcpKMSKeyRingEnv)
	cryptoKey := os.Getenv(gcpKMSCryptoKeyEnv)
	if projectID == "" || location == "" || keyRing == "" || cryptoKey == "" {
		t.Skipf("set %s or %s, %s, %s, and %s", gcpKMSKeyNameEnv, gcpProjectIDEnv, gcpKMSLocationEnv, gcpKMSKeyRingEnv, gcpKMSCryptoKeyEnv)
	}

	return fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s", projectID, location, keyRing, cryptoKey)
}
