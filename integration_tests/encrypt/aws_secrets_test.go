//go:build integration && aws

package encrypt_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

const awsSecretsTestEnv = "AUTH_PROXY_AWS_SECRETS_TEST"

func TestAwsSecretsManagerKeySyncAndReencrypt(t *testing.T) {
	if os.Getenv(awsSecretsTestEnv) != "1" {
		t.Skipf("%s is not set to 1", awsSecretsTestEnv)
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		t.Skip("AWS_REGION is not set")
	}

	ctx := context.Background()
	sm := newSecretsManagerClient(t, ctx, region)

	env := helpers.Setup(t, helpers.SetupOptions{Service: helpers.ServiceTypeAPI})
	defer env.Cleanup()

	secretName := fmt.Sprintf("authproxy-aws-sm-%d", time.Now().UnixNano())
	keyV1 := randomBytes(t, 32)

	createOut, err := sm.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String(secretName),
		SecretBinary: keyV1,
	})
	require.NoError(t, err)

	secretID := secretName
	if createOut.ARN != nil {
		secretID = *createOut.ARN
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = sm.DeleteSecret(cleanupCtx, &secretsmanager.DeleteSecretInput{
			SecretId:                   aws.String(secretID),
			ForceDeleteWithoutRecovery: aws.Bool(true),
		})
	})

	namespace := fmt.Sprintf("root.aws-sm-test-%d", time.Now().UnixNano())
	ekID := apid.New(apid.PrefixEncryptionKey)

	require.NoError(t, env.Db.CreateNamespace(ctx, &database.Namespace{
		Path:            namespace,
		EncryptionKeyId: &ekID,
	}))

	keyData := sconfig.KeyData{
		InnerVal: &sconfig.KeyDataAwsSecret{
			AwsSecretID: secretID,
			AwsRegion:   region,
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

	plaintext := "aws-secrets-manager-test"
	encrypted, err := env.DM.GetEncryptService().EncryptStringForNamespace(ctx, namespace, plaintext)
	require.NoError(t, err)
	require.Equal(t, currentV1.Id, encrypted.ID)

	actorID := apid.New(apid.PrefixActor)
	require.NoError(t, env.Db.CreateActor(ctx, &database.Actor{
		Id:           actorID,
		Namespace:    namespace,
		ExternalId:   "aws-sm-test-actor",
		EncryptedKey: &encrypted,
	}))

	keyV2 := randomBytes(t, 32)
	_, err = sm.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     aws.String(secretID),
		SecretBinary: keyV2,
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

func newSecretsManagerClient(t *testing.T, ctx context.Context, region string) *secretsmanager.Client {
	t.Helper()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	require.NoError(t, err)

	return secretsmanager.NewFromConfig(cfg)
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()

	buf := make([]byte, n)
	_, err := rand.Read(buf)
	require.NoError(t, err)

	return buf
}

func runReencryptAll(ctx context.Context, env *helpers.IntegrationTestEnv) error {
	return env.Db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []database.ReEncryptionTarget, lastPage bool) (stop bool, err error) {
		var updates []database.ReEncryptedFieldUpdate

		for _, target := range targets {
			newEF, reencryptErr := env.DM.GetEncryptService().ReEncryptField(ctx, target.EncryptedFieldValue, target.TargetEncryptionKeyVersionId)
			if reencryptErr != nil {
				return true, reencryptErr
			}

			if newEF.ID == target.EncryptedFieldValue.ID {
				continue
			}

			updates = append(updates, database.ReEncryptedFieldUpdate{
				Table:            target.Table,
				PrimaryKeyCols:   target.PrimaryKeyCols,
				PrimaryKeyValues: target.PrimaryKeyValues,
				FieldColumn:      target.FieldColumn,
				NewValue:         newEF,
			})
		}

		if len(updates) > 0 {
			if updateErr := env.Db.BatchUpdateReEncryptedFields(ctx, updates); updateErr != nil {
				return true, updateErr
			}
		}

		return false, nil
	})
}
