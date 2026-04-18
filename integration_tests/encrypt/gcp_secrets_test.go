//go:build integration && gcp

package encrypt_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/joho/godotenv"
	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

const (
	gcpSecretsTestEnv = "AUTH_PROXY_GCP_SECRETS_TEST"
	gcpProjectIDEnv   = "GCP_PROJECT_ID"
)

func init() {
	// Best-effort load of a .env file so the test is runnable locally when
	// secrets are provided in a .env file. go test runs with CWD set to the
	// package directory (integration_tests/encrypt), so look in this dir, the
	// integration_tests dir, and the repo root.
	_ = godotenv.Load(".env", "../.env", "../../.env")
}

func TestGcpSecretManagerKeySyncAndReencrypt(t *testing.T) {
	if os.Getenv(gcpSecretsTestEnv) != "1" {
		t.Skipf("%s is not set to 1", gcpSecretsTestEnv)
	}

	projectID := os.Getenv(gcpProjectIDEnv)
	if projectID == "" {
		t.Skipf("%s is not set", gcpProjectIDEnv)
	}

	ctx := context.Background()
	sm := newGcpSecretManagerClient(t, ctx)
	defer sm.Close()

	env := helpers.Setup(t, helpers.SetupOptions{Service: helpers.ServiceTypeAPI})
	defer env.Cleanup()

	secretName := fmt.Sprintf("authproxy-gcp-sm-%d", time.Now().UnixNano())
	keyV1 := randomBytes(t, 32)

	createdSecret, err := sm.CreateSecret(ctx, &secretmanagerpb.CreateSecretRequest{
		Parent:   fmt.Sprintf("projects/%s", projectID),
		SecretId: secretName,
		Secret: &secretmanagerpb.Secret{
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			},
		},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = sm.DeleteSecret(cleanupCtx, &secretmanagerpb.DeleteSecretRequest{
			Name: createdSecret.Name,
		})
	})

	_, err = sm.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{
		Parent:  createdSecret.Name,
		Payload: &secretmanagerpb.SecretPayload{Data: keyV1},
	})
	require.NoError(t, err)

	namespace := fmt.Sprintf("root.gcp-sm-test-%d", time.Now().UnixNano())
	ekID := apid.New(apid.PrefixEncryptionKey)

	require.NoError(t, env.Db.CreateNamespace(ctx, &database.Namespace{
		Path:            namespace,
		EncryptionKeyId: &ekID,
	}))

	keyData := sconfig.KeyData{
		InnerVal: &sconfig.KeyDataGcpSecret{
			GcpSecretName: secretName,
			GcpProject:    projectID,
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

	plaintext := "gcp-secret-manager-test"
	encrypted, err := env.DM.GetEncryptService().EncryptStringForNamespace(ctx, namespace, plaintext)
	require.NoError(t, err)
	require.Equal(t, currentV1.Id, encrypted.ID)

	actorID := apid.New(apid.PrefixActor)
	require.NoError(t, env.Db.CreateActor(ctx, &database.Actor{
		Id:           actorID,
		Namespace:    namespace,
		ExternalId:   "gcp-sm-test-actor",
		EncryptedKey: &encrypted,
	}))

	keyV2 := randomBytes(t, 32)
	_, err = sm.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{
		Parent:  createdSecret.Name,
		Payload: &secretmanagerpb.SecretPayload{Data: keyV2},
	})
	require.NoError(t, err)

	require.NoError(t, encrypt.SyncKeysToDatabase(ctx, env.Cfg, env.Db, env.Logger, nil))
	require.NoError(t, env.DM.GetEncryptService().SyncKeysFromDbToMemory(ctx))

	currentV2, err := env.Db.GetCurrentEncryptionKeyVersionForNamespace(ctx, namespace)
	require.NoError(t, err)
	require.NotEqual(t, currentV1.Id, currentV2.Id)

	require.NoError(t, runReencryptAll(ctx, env))

	updated, err := env.Db.GetActor(ctx, actorID)
	require.NoError(t, err)
	require.NotNil(t, updated.EncryptedKey)
	require.Equal(t, currentV2.Id, updated.EncryptedKey.ID)

	decrypted, err := env.DM.GetEncryptService().DecryptString(ctx, *updated.EncryptedKey)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}

func newGcpSecretManagerClient(t *testing.T, ctx context.Context) *secretmanager.Client {
	t.Helper()

	client, err := secretmanager.NewClient(ctx)
	require.NoError(t, err)
	return client
}
