//go:build integration

package encrypt_test

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
)

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()

	buf := make([]byte, n)
	_, err := rand.Read(buf)
	require.NoError(t, err)

	return buf
}

func createDataEncryptionKeyForIntegrationTest(
	t *testing.T,
	ctx context.Context,
	db database.DB,
	keyID apid.ID,
	keyData *sconfig.KeyData,
) *database.DataEncryptionKey {
	t.Helper()

	generated, err := keyData.GenerateDataEncryptionKey(ctx)
	require.NoError(t, err)

	dek := &database.DataEncryptionKey{
		KeyId:           keyID,
		Provider:        string(generated.Provider),
		ProviderID:      generated.ProviderID,
		ProviderVersion: generated.ProviderVersion,
		ProtectedData:   &generated.ProtectedData,
		IsCurrent:       true,
	}
	require.NoError(t, db.CreateDataEncryptionKey(ctx, dek))

	return dek
}

func runReencryptAll(ctx context.Context, env *helpers.IntegrationTestEnv) error {
	return env.Db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []database.ReEncryptionTarget, lastPage bool) (keepGoing pagination.KeepGoing, err error) {
		var updates []database.ReEncryptedFieldUpdate

		for _, target := range targets {
			newEF, reencryptErr := env.DM.GetEncryptService().ReEncryptField(ctx, target.EncryptedFieldValue, target.TargetDataEncryptionKeyId)
			if reencryptErr != nil {
				return pagination.Stop, reencryptErr
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
				return pagination.Stop, updateErr
			}
		}

		return pagination.Continue, nil
	})
}
