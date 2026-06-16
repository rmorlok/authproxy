//go:build integration && aws

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
	awsKMSTestEnv        = "AUTH_PROXY_AWS_KMS_TEST"
	awsKMSKeyIDEnv       = "AUTH_PROXY_AWS_KMS_KEY_ID"
	awsKMSKeyIDV2Env     = "AUTH_PROXY_AWS_KMS_KEY_ID_V2"
	awsKMSEndpointEnv    = "AUTH_PROXY_AWS_KMS_ENDPOINT"
	awsKMSProviderString = string(sconfig.ProviderTypeAwsKMS)
)

func TestAwsKMSKeySyncAndReencrypt(t *testing.T) {
	if os.Getenv(awsKMSTestEnv) != "1" {
		t.Skipf("%s is not set to 1", awsKMSTestEnv)
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		t.Skip("AWS_REGION is not set")
	}
	keyID := os.Getenv(awsKMSKeyIDEnv)
	if keyID == "" {
		t.Skipf("%s is not set", awsKMSKeyIDEnv)
	}

	ctx := context.Background()
	env := helpers.Setup(t, helpers.SetupOptions{Service: helpers.ServiceTypeAPI})
	defer env.Cleanup()

	namespace := fmt.Sprintf("root.aws-kms-test-%d", time.Now().UnixNano())
	keyIDAuthProxy := apid.New(apid.PrefixKey)

	require.NoError(t, env.Db.CreateNamespace(ctx, &database.Namespace{
		Path:  namespace,
		KeyId: &keyIDAuthProxy,
	}))

	keyData := awsKMSKeyData(keyID, region)
	keyDataJSON, err := json.Marshal(&keyData)
	require.NoError(t, err)

	encKeyData, err := env.DM.GetEncryptService().EncryptGlobal(ctx, keyDataJSON)
	require.NoError(t, err)

	require.NoError(t, env.Db.CreateKey(ctx, &database.Key{
		Id:               keyIDAuthProxy,
		Namespace:        namespace,
		MaterialType:     database.KeyMaterialTypeExternal,
		EncryptedKeyData: &encKeyData,
		State:            database.KeyStateActive,
	}))

	currentV1 := createDataEncryptionKeyForIntegrationTest(t, ctx, env.Db, keyIDAuthProxy, &keyData)
	require.Equal(t, awsKMSProviderString, currentV1.Provider)
	require.Equal(t, keyID, currentV1.ProviderID)
	require.NotEmpty(t, currentV1.ProviderVersion)
	require.NotNil(t, currentV1.ProtectedData)
	require.Equal(t, awsKMSProviderString, currentV1.ProtectedData.Type)
	require.NotEmpty(t, currentV1.ProtectedData.WrappedData)

	require.NoError(t, encrypt.SyncKeysToDatabase(ctx, env.Cfg, env.Db, env.Logger, nil))
	require.NoError(t, env.DM.GetEncryptService().SyncKeysFromDbToMemory(ctx))

	plaintext := "aws-kms-test"
	encrypted, err := env.DM.GetEncryptService().EncryptStringForNamespace(ctx, namespace, plaintext)
	require.NoError(t, err)
	require.Equal(t, currentV1.Id, encrypted.ID)

	actorID := apid.New(apid.PrefixActor)
	require.NoError(t, env.Db.CreateActor(ctx, &database.Actor{
		Id:           actorID,
		Namespace:    namespace,
		ExternalId:   "aws-kms-test-actor",
		EncryptedKey: &encrypted,
	}))

	keyIDV2 := os.Getenv(awsKMSKeyIDV2Env)
	if keyIDV2 != "" {
		keyDataV2 := awsKMSKeyData(keyIDV2, region)
		keyDataJSONV2, err := json.Marshal(&keyDataV2)
		require.NoError(t, err)

		encKeyDataV2, err := env.DM.GetEncryptService().EncryptGlobal(ctx, keyDataJSONV2)
		require.NoError(t, err)
		_, err = env.Db.UpdateKey(ctx, keyIDAuthProxy, map[string]interface{}{
			"encrypted_key_data": encKeyDataV2,
		})
		require.NoError(t, err)

		require.NoError(t, encrypt.SyncKeysToDatabase(ctx, env.Cfg, env.Db, env.Logger, nil))
		require.NoError(t, env.DM.GetEncryptService().SyncKeysFromDbToMemory(ctx))

		currentV2, err := env.Db.GetCurrentDataEncryptionKeyForKey(ctx, keyIDAuthProxy)
		require.NoError(t, err)
		require.Equal(t, currentV1.Id, currentV2.Id)
		require.Equal(t, keyIDV2, currentV2.ProviderID)
		require.NotEqual(t, currentV1.ProviderVersion, currentV2.ProviderVersion)
		require.NotEqual(t, currentV1.ProtectedData.WrappedData, currentV2.ProtectedData.WrappedData)
	} else {
		t.Logf("%s is not set; skipping AWS KMS metadata advancement rewrap path", awsKMSKeyIDV2Env)
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

func awsKMSKeyData(keyID string, region string) sconfig.KeyData {
	return sconfig.KeyData{
		InnerVal: &sconfig.KeyDataAwsKMS{
			AwsKMSKeyID:    keyID,
			AwsRegion:      region,
			AwsKMSEndpoint: os.Getenv(awsKMSEndpointEnv),
		},
	}
}
