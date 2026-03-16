package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestReEncryptRegistry(t *testing.T) {
	t.Run("registration validation", func(t *testing.T) {
		regs := GetEncryptedFieldRegistrations()
		require.GreaterOrEqual(t, len(regs), 4, "expected at least 4 registrations from init()")

		// Verify the four expected registrations exist
		tableNames := make(map[string]bool)
		for _, r := range regs {
			tableNames[r.Table] = true
		}
		require.True(t, tableNames[ActorTable])
		require.True(t, tableNames[ConnectorVersionsTable])
		require.True(t, tableNames[OAuth2TokensTable])
		require.True(t, tableNames[EncryptionKeysTable])
	})

	t.Run("enumerate finds actor needing re-encryption", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		targetEKVId := apid.New(apid.PrefixEncryptionKeyVersion)
		oldEKVId := apid.New(apid.PrefixEncryptionKeyVersion)

		// Set up namespace with target EKV
		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET target_encryption_key_version_id = '%s' WHERE path = 'root'`, string(targetEKVId)))
		require.NoError(t, err)

		// Create actor with mismatched EKV
		actorId := apid.New(apid.PrefixActor)
		ef := encfield.EncryptedField{ID: oldEKVId, Data: "dGVzdA=="}
		err = db.CreateActor(ctx, &Actor{
			Id:           actorId,
			Namespace:    "root",
			ExternalId:   "user1",
			EncryptedKey: &ef,
		})
		require.NoError(t, err)

		var allTargets []ReEncryptionTarget
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (stop bool, err error) {
			allTargets = append(allTargets, targets...)
			return false, nil
		})
		require.NoError(t, err)

		// Find our actor target
		var found bool
		for _, tgt := range allTargets {
			if tgt.Table == ActorTable && tgt.FieldColumn == "encrypted_key" {
				// Check it's our actor
				if id, ok := tgt.PrimaryKeyValues[0].(string); ok && apid.ID(id) == actorId {
					found = true
					require.Equal(t, targetEKVId, tgt.TargetEncryptionKeyVersionId)
					require.Equal(t, oldEKVId, tgt.EncryptedFieldValue.ID)
				}
			}
		}
		require.True(t, found, "expected to find actor target needing re-encryption")
	})

	t.Run("enumerate skips actor already at target", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		targetEKVId := apid.New(apid.PrefixEncryptionKeyVersion)

		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET target_encryption_key_version_id = '%s' WHERE path = 'root'`, string(targetEKVId)))
		require.NoError(t, err)

		// Create actor with matching EKV
		actorId := apid.New(apid.PrefixActor)
		ef := encfield.EncryptedField{ID: targetEKVId, Data: "dGVzdA=="}
		err = db.CreateActor(ctx, &Actor{
			Id:           actorId,
			Namespace:    "root",
			ExternalId:   "user1",
			EncryptedKey: &ef,
		})
		require.NoError(t, err)

		var actorTargets []ReEncryptionTarget
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (stop bool, err error) {
			for _, tgt := range targets {
				if tgt.Table == ActorTable {
					if id, ok := tgt.PrimaryKeyValues[0].(string); ok && apid.ID(id) == actorId {
						actorTargets = append(actorTargets, tgt)
					}
				}
			}
			return false, nil
		})
		require.NoError(t, err)
		require.Empty(t, actorTargets, "actor at target EKV should not appear")
	})

	t.Run("oauth2 token one field mismatch yields 1 target", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		targetEKVId := apid.New(apid.PrefixEncryptionKeyVersion)
		oldEKVId := apid.New(apid.PrefixEncryptionKeyVersion)

		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET target_encryption_key_version_id = '%s' WHERE path = 'root'`, string(targetEKVId)))
		require.NoError(t, err)

		// Create connection in root namespace
		connId := apid.New(apid.PrefixConnection)
		connectorId := apid.New(apid.PrefixConnectorVersion)
		nowStr := now.Format(time.RFC3339)
		_, err = rawDb.Exec(fmt.Sprintf(
			`INSERT INTO connections (id, namespace, state, connector_id, connector_version, created_at, updated_at) VALUES ('%s', 'root', 'ready', '%s', 1, '%s', '%s')`,
			string(connId), string(connectorId), nowStr, nowStr,
		))
		require.NoError(t, err)

		// Create token: access token matches, refresh token mismatches
		accessToken := encfield.EncryptedField{ID: targetEKVId, Data: "YWNjZXNz"}
		refreshToken := encfield.EncryptedField{ID: oldEKVId, Data: "cmVmcmVzaA=="}
		token, err := db.InsertOAuth2Token(ctx, connId, nil, refreshToken, accessToken, nil, "scope1")
		require.NoError(t, err)

		var tokenTargets []ReEncryptionTarget
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (stop bool, err error) {
			for _, tgt := range targets {
				if tgt.Table == OAuth2TokensTable {
					if id, ok := tgt.PrimaryKeyValues[0].(string); ok && apid.ID(id) == token.Id {
						tokenTargets = append(tokenTargets, tgt)
					}
				}
			}
			return false, nil
		})
		require.NoError(t, err)
		require.Len(t, tokenTargets, 1, "only the mismatched field should appear")
		require.Equal(t, "encrypted_refresh_token", tokenTargets[0].FieldColumn)
	})

	t.Run("oauth2 token both fields mismatch yields 2 targets", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		targetEKVId := apid.New(apid.PrefixEncryptionKeyVersion)
		oldEKVId := apid.New(apid.PrefixEncryptionKeyVersion)

		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET target_encryption_key_version_id = '%s' WHERE path = 'root'`, string(targetEKVId)))
		require.NoError(t, err)

		connId := apid.New(apid.PrefixConnection)
		connectorId := apid.New(apid.PrefixConnectorVersion)
		nowStr := now.Format(time.RFC3339)
		_, err = rawDb.Exec(fmt.Sprintf(
			`INSERT INTO connections (id, namespace, state, connector_id, connector_version, created_at, updated_at) VALUES ('%s', 'root', 'ready', '%s', 1, '%s', '%s')`,
			string(connId), string(connectorId), nowStr, nowStr,
		))
		require.NoError(t, err)

		accessToken := encfield.EncryptedField{ID: oldEKVId, Data: "YWNjZXNz"}
		refreshToken := encfield.EncryptedField{ID: oldEKVId, Data: "cmVmcmVzaA=="}
		token, err := db.InsertOAuth2Token(ctx, connId, nil, refreshToken, accessToken, nil, "scope1")
		require.NoError(t, err)

		var tokenTargets []ReEncryptionTarget
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (stop bool, err error) {
			for _, tgt := range targets {
				if tgt.Table == OAuth2TokensTable {
					if id, ok := tgt.PrimaryKeyValues[0].(string); ok && apid.ID(id) == token.Id {
						tokenTargets = append(tokenTargets, tgt)
					}
				}
			}
			return false, nil
		})
		require.NoError(t, err)
		require.Len(t, tokenTargets, 2, "both mismatched fields should appear")

		fieldCols := map[string]bool{}
		for _, tgt := range tokenTargets {
			fieldCols[tgt.FieldColumn] = true
		}
		require.True(t, fieldCols["encrypted_access_token"])
		require.True(t, fieldCols["encrypted_refresh_token"])
	})

	t.Run("indirect JOIN resolves namespace through connections", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		targetEKVId := apid.New(apid.PrefixEncryptionKeyVersion)
		oldEKVId := apid.New(apid.PrefixEncryptionKeyVersion)

		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET target_encryption_key_version_id = '%s' WHERE path = 'root'`, string(targetEKVId)))
		require.NoError(t, err)

		connId := apid.New(apid.PrefixConnection)
		connectorId := apid.New(apid.PrefixConnectorVersion)
		nowStr := now.Format(time.RFC3339)
		_, err = rawDb.Exec(fmt.Sprintf(
			`INSERT INTO connections (id, namespace, state, connector_id, connector_version, created_at, updated_at) VALUES ('%s', 'root', 'ready', '%s', 1, '%s', '%s')`,
			string(connId), string(connectorId), nowStr, nowStr,
		))
		require.NoError(t, err)

		accessToken := encfield.EncryptedField{ID: oldEKVId, Data: "YWNjZXNz"}
		refreshToken := encfield.EncryptedField{ID: oldEKVId, Data: "cmVmcmVzaA=="}
		token, err := db.InsertOAuth2Token(ctx, connId, nil, refreshToken, accessToken, nil, "")
		require.NoError(t, err)

		var found bool
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (stop bool, err error) {
			for _, tgt := range targets {
				if tgt.Table == OAuth2TokensTable {
					if id, ok := tgt.PrimaryKeyValues[0].(string); ok && apid.ID(id) == token.Id {
						found = true
						require.Equal(t, targetEKVId, tgt.TargetEncryptionKeyVersionId)
					}
				}
			}
			return false, nil
		})
		require.NoError(t, err)
		require.True(t, found, "oauth2 token should resolve namespace via connections JOIN")
	})

	t.Run("composite PK connector_versions", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		targetEKVId := apid.New(apid.PrefixEncryptionKeyVersion)
		oldEKVId := apid.New(apid.PrefixEncryptionKeyVersion)

		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET target_encryption_key_version_id = '%s' WHERE path = 'root'`, string(targetEKVId)))
		require.NoError(t, err)

		cvId := apid.New(apid.PrefixConnectorVersion)
		cv := &ConnectorVersion{
			Id:                  cvId,
			Version:             1,
			Namespace:           "root",
			State:               ConnectorVersionStateDraft,
			Hash:                "abc123",
			EncryptedDefinition: encfield.EncryptedField{ID: oldEKVId, Data: "ZGVm"},
			Labels:              Labels{},
		}
		err = db.UpsertConnectorVersion(ctx, cv)
		require.NoError(t, err)

		var cvTargets []ReEncryptionTarget
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (stop bool, err error) {
			for _, tgt := range targets {
				if tgt.Table == ConnectorVersionsTable {
					if id, ok := tgt.PrimaryKeyValues[0].(string); ok && apid.ID(id) == cvId {
						cvTargets = append(cvTargets, tgt)
					}
				}
			}
			return false, nil
		})
		require.NoError(t, err)
		require.Len(t, cvTargets, 1)
		require.Len(t, cvTargets[0].PrimaryKeyValues, 2, "composite PK should have 2 values")
	})

	t.Run("nullable encrypted field skipped", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		targetEKVId := apid.New(apid.PrefixEncryptionKeyVersion)

		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET target_encryption_key_version_id = '%s' WHERE path = 'root'`, string(targetEKVId)))
		require.NoError(t, err)

		// Create actor with NULL encrypted_key
		actorId := apid.New(apid.PrefixActor)
		err = db.CreateActor(ctx, &Actor{
			Id:         actorId,
			Namespace:  "root",
			ExternalId: "user_no_key",
		})
		require.NoError(t, err)

		var actorTargets []ReEncryptionTarget
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (stop bool, err error) {
			for _, tgt := range targets {
				if tgt.Table == ActorTable {
					if id, ok := tgt.PrimaryKeyValues[0].(string); ok && apid.ID(id) == actorId {
						actorTargets = append(actorTargets, tgt)
					}
				}
			}
			return false, nil
		})
		require.NoError(t, err)
		require.Empty(t, actorTargets, "actor with NULL encrypted_key should not appear")
	})

	t.Run("BatchUpdateReEncryptedFields", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		targetEKVId := apid.New(apid.PrefixEncryptionKeyVersion)
		oldEKVId := apid.New(apid.PrefixEncryptionKeyVersion)

		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET target_encryption_key_version_id = '%s' WHERE path = 'root'`, string(targetEKVId)))
		require.NoError(t, err)

		actorId := apid.New(apid.PrefixActor)
		ef := encfield.EncryptedField{ID: oldEKVId, Data: "b2xk"}
		err = db.CreateActor(ctx, &Actor{
			Id:           actorId,
			Namespace:    "root",
			ExternalId:   "update_test",
			EncryptedKey: &ef,
		})
		require.NoError(t, err)

		// Perform batch update
		newEf := encfield.EncryptedField{ID: targetEKVId, Data: "bmV3"}
		later := now.Add(time.Hour)
		ctx = apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(later)).Build()

		err = db.BatchUpdateReEncryptedFields(ctx, []ReEncryptedFieldUpdate{
			{
				Table:            ActorTable,
				PrimaryKeyCols:   []string{"id"},
				PrimaryKeyValues: []any{string(actorId)},
				FieldColumn:      "encrypted_key",
				NewValue:         newEf,
			},
		})
		require.NoError(t, err)

		// Verify the update
		actor, err := db.GetActor(ctx, actorId)
		require.NoError(t, err)
		require.NotNil(t, actor.EncryptedKey)
		require.Equal(t, targetEKVId, actor.EncryptedKey.ID)
		require.Equal(t, "bmV3", actor.EncryptedKey.Data)
		require.NotNil(t, actor.EncryptedAt)
		require.True(t, later.Equal(*actor.EncryptedAt), "encrypted_at should match the clock time")
	})

	t.Run("empty results when nothing needs re-encryption", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// No target EKV set on any namespace, so nothing should be returned
		var allTargets []ReEncryptionTarget
		err := db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (stop bool, err error) {
			allTargets = append(allTargets, targets...)
			return false, nil
		})
		require.NoError(t, err)
		require.Empty(t, allTargets)
	})

	t.Run("namespace with NULL target_encryption_key_version_id skipped", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		oldEKVId := apid.New(apid.PrefixEncryptionKeyVersion)

		// Create actor with encrypted key but namespace has no target EKV (NULL by default)
		actorId := apid.New(apid.PrefixActor)
		ef := encfield.EncryptedField{ID: oldEKVId, Data: "dGVzdA=="}
		err := db.CreateActor(ctx, &Actor{
			Id:           actorId,
			Namespace:    "root",
			ExternalId:   "user_null_target",
			EncryptedKey: &ef,
		})
		require.NoError(t, err)

		var actorTargets []ReEncryptionTarget
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (stop bool, err error) {
			for _, tgt := range targets {
				if tgt.Table == ActorTable {
					if id, ok := tgt.PrimaryKeyValues[0].(string); ok && apid.ID(id) == actorId {
						actorTargets = append(actorTargets, tgt)
					}
				}
			}
			return false, nil
		})
		require.NoError(t, err)
		require.Empty(t, actorTargets, "namespace with NULL target should not produce targets")
	})

	t.Run("BatchUpdateReEncryptedFields rejects unregistered field", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		err := db.BatchUpdateReEncryptedFields(ctx, []ReEncryptedFieldUpdate{
			{
				Table:            "fake_table",
				PrimaryKeyCols:   []string{"id"},
				PrimaryKeyValues: []any{"fake_id"},
				FieldColumn:      "fake_col",
				NewValue:         encfield.EncryptedField{ID: "ekv_x", Data: "data"},
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not a registered encrypted field")
	})
}
