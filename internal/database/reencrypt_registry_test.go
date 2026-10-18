package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestReEncryptRegistry(t *testing.T) {
	t.Run("registration validation", func(t *testing.T) {
		regs := GetEncryptedFieldRegistrations()
		require.GreaterOrEqual(t, len(regs), 4, "expected at least 4 registrations from init()")

		// Key provider configuration is intentionally not registered here:
		// namespace re-encryption can otherwise make a key depend on its own DEK.
		tableNames := make(map[string]bool)
		for _, r := range regs {
			tableNames[r.Table] = true
		}
		require.True(t, tableNames[ActorTable])
		require.True(t, tableNames[ConnectionsTable])
		require.True(t, tableNames[ConnectorGenerationsTable])
		require.True(t, tableNames[ConnectionCredentialsTable])
		require.True(t, tableNames[OAuth2TokensTable])
		require.False(t, tableNames[KeysTable])
	})

	t.Run("enumerate finds actor needing re-encryption", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		targetEKVId := apid.New(apid.PrefixDataEncryptionKey)
		oldEKVId := apid.New(apid.PrefixDataEncryptionKey)

		// Set up namespace with target EKV
		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET target_data_encryption_key_id = '%s' WHERE path = 'root'`, string(targetEKVId)))
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
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (keepGoing pagination.KeepGoing, err error) {
			allTargets = append(allTargets, targets...)
			return pagination.Continue, nil
		})
		require.NoError(t, err)

		// Find our actor target
		var found bool
		for _, tgt := range allTargets {
			if tgt.Table == ActorTable && tgt.FieldColumn == "encrypted_key" {
				// Check it's our actor
				if id, ok := tgt.PrimaryKeyValues[0].(string); ok && apid.ID(id) == actorId {
					found = true
					require.Equal(t, targetEKVId, tgt.TargetDataEncryptionKeyId)
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

		targetEKVId := apid.New(apid.PrefixDataEncryptionKey)

		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET target_data_encryption_key_id = '%s' WHERE path = 'root'`, string(targetEKVId)))
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
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (keepGoing pagination.KeepGoing, err error) {
			for _, tgt := range targets {
				if tgt.Table == ActorTable {
					if id, ok := tgt.PrimaryKeyValues[0].(string); ok && apid.ID(id) == actorId {
						actorTargets = append(actorTargets, tgt)
					}
				}
			}
			return pagination.Continue, nil
		})
		require.NoError(t, err)
		require.Empty(t, actorTargets, "actor at target EKV should not appear")
	})

	t.Run("oauth2 token one field mismatch yields 1 target", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		targetEKVId := apid.New(apid.PrefixDataEncryptionKey)
		oldEKVId := apid.New(apid.PrefixDataEncryptionKey)

		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET target_data_encryption_key_id = '%s' WHERE path = 'root'`, string(targetEKVId)))
		require.NoError(t, err)

		// Create connection in root namespace
		connId := apid.New(apid.PrefixConnection)
		connectorId := apid.New(apid.PrefixConnector)
		nowStr := now.Format(time.RFC3339)
		_, err = rawDb.Exec(fmt.Sprintf(
			`INSERT INTO connections (id, name, namespace, state, connector_id, connector_generation, created_at, updated_at) VALUES ('%s', '%s', 'root', 'ready', '%s', 1, '%s', '%s')`,
			string(connId), string(connId), string(connectorId), nowStr, nowStr,
		))
		require.NoError(t, err)

		// Create token: access token matches, refresh token mismatches
		accessToken := encfield.EncryptedField{ID: targetEKVId, Data: "YWNjZXNz"}
		refreshToken := encfield.EncryptedField{ID: oldEKVId, Data: "cmVmcmVzaA=="}
		token, err := db.InsertOAuth2Token(ctx, connId, nil, refreshToken, accessToken, nil, OAuth2RefreshTokenExpiry{}, "scope1", "scope1", nil)
		require.NoError(t, err)

		var tokenTargets []ReEncryptionTarget
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (keepGoing pagination.KeepGoing, err error) {
			for _, tgt := range targets {
				if tgt.Table == OAuth2TokensTable {
					if id, ok := tgt.PrimaryKeyValues[0].(string); ok && apid.ID(id) == token.Id {
						tokenTargets = append(tokenTargets, tgt)
					}
				}
			}
			return pagination.Continue, nil
		})
		require.NoError(t, err)
		require.Len(t, tokenTargets, 1, "only the mismatched field should appear")
		require.Equal(t, "encrypted_refresh_token", tokenTargets[0].FieldColumn)
	})

	t.Run("oauth2 token both fields mismatch yields 2 targets", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		targetEKVId := apid.New(apid.PrefixDataEncryptionKey)
		oldEKVId := apid.New(apid.PrefixDataEncryptionKey)

		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET target_data_encryption_key_id = '%s' WHERE path = 'root'`, string(targetEKVId)))
		require.NoError(t, err)

		connId := apid.New(apid.PrefixConnection)
		connectorId := apid.New(apid.PrefixConnector)
		nowStr := now.Format(time.RFC3339)
		_, err = rawDb.Exec(fmt.Sprintf(
			`INSERT INTO connections (id, name, namespace, state, connector_id, connector_generation, created_at, updated_at) VALUES ('%s', '%s', 'root', 'ready', '%s', 1, '%s', '%s')`,
			string(connId), string(connId), string(connectorId), nowStr, nowStr,
		))
		require.NoError(t, err)

		accessToken := encfield.EncryptedField{ID: oldEKVId, Data: "YWNjZXNz"}
		refreshToken := encfield.EncryptedField{ID: oldEKVId, Data: "cmVmcmVzaA=="}
		token, err := db.InsertOAuth2Token(ctx, connId, nil, refreshToken, accessToken, nil, OAuth2RefreshTokenExpiry{}, "scope1", "scope1", nil)
		require.NoError(t, err)

		var tokenTargets []ReEncryptionTarget
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (keepGoing pagination.KeepGoing, err error) {
			for _, tgt := range targets {
				if tgt.Table == OAuth2TokensTable {
					if id, ok := tgt.PrimaryKeyValues[0].(string); ok && apid.ID(id) == token.Id {
						tokenTargets = append(tokenTargets, tgt)
					}
				}
			}
			return pagination.Continue, nil
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

		targetEKVId := apid.New(apid.PrefixDataEncryptionKey)
		oldEKVId := apid.New(apid.PrefixDataEncryptionKey)

		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET target_data_encryption_key_id = '%s' WHERE path = 'root'`, string(targetEKVId)))
		require.NoError(t, err)

		connId := apid.New(apid.PrefixConnection)
		connectorId := apid.New(apid.PrefixConnector)
		nowStr := now.Format(time.RFC3339)
		_, err = rawDb.Exec(fmt.Sprintf(
			`INSERT INTO connections (id, name, namespace, state, connector_id, connector_generation, created_at, updated_at) VALUES ('%s', '%s', 'root', 'ready', '%s', 1, '%s', '%s')`,
			string(connId), string(connId), string(connectorId), nowStr, nowStr,
		))
		require.NoError(t, err)

		accessToken := encfield.EncryptedField{ID: oldEKVId, Data: "YWNjZXNz"}
		refreshToken := encfield.EncryptedField{ID: oldEKVId, Data: "cmVmcmVzaA=="}
		token, err := db.InsertOAuth2Token(ctx, connId, nil, refreshToken, accessToken, nil, OAuth2RefreshTokenExpiry{}, "", "", nil)
		require.NoError(t, err)

		var found bool
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (keepGoing pagination.KeepGoing, err error) {
			for _, tgt := range targets {
				if tgt.Table == OAuth2TokensTable {
					if id, ok := tgt.PrimaryKeyValues[0].(string); ok && apid.ID(id) == token.Id {
						found = true
						require.Equal(t, targetEKVId, tgt.TargetDataEncryptionKeyId)
					}
				}
			}
			return pagination.Continue, nil
		})
		require.NoError(t, err)
		require.True(t, found, "oauth2 token should resolve namespace via connections JOIN")
	})

	t.Run("connector generation public row ID", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		targetEKVId := apid.New(apid.PrefixDataEncryptionKey)
		oldEKVId := apid.New(apid.PrefixDataEncryptionKey)

		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET target_data_encryption_key_id = '%s' WHERE path = 'root'`, string(targetEKVId)))
		require.NoError(t, err)

		cvId := apid.New(apid.PrefixConnector)
		cv := &ConnectorWithDefinition{
			Id:                  cvId,
			Generation:          1,
			Namespace:           "root",
			State:               ConnectorGenerationStateDraft,
			EncryptedDefinition: encfield.EncryptedField{ID: oldEKVId, Data: "ZGVm"},
			Labels:              Labels{},
		}
		err = db.UpsertConnectorGeneration(ctx, cv)
		require.NoError(t, err)

		var cvTargets []ReEncryptionTarget
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (keepGoing pagination.KeepGoing, err error) {
			for _, tgt := range targets {
				if tgt.Table == ConnectorGenerationsTable {
					if id, ok := tgt.PrimaryKeyValues[0].(string); ok && apid.ID(id) == cv.DefinitionGenerationId {
						cvTargets = append(cvTargets, tgt)
					}
				}
			}
			return pagination.Continue, nil
		})
		require.NoError(t, err)
		require.Len(t, cvTargets, 1)
		require.Len(t, cvTargets[0].PrimaryKeyValues, 1)
		require.Equal(t, apid.PrefixConnectorGeneration, cv.DefinitionGenerationId.Prefix())

		require.NoError(t, db.DeleteConnector(ctx, cvId))
		cvTargets = nil
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (keepGoing pagination.KeepGoing, err error) {
			for _, tgt := range targets {
				if tgt.Table == ConnectorGenerationsTable {
					cvTargets = append(cvTargets, tgt)
				}
			}
			return pagination.Continue, nil
		})
		require.NoError(t, err)
		require.Empty(t, cvTargets)
	})

	t.Run("nullable encrypted field skipped", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		targetEKVId := apid.New(apid.PrefixDataEncryptionKey)

		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET target_data_encryption_key_id = '%s' WHERE path = 'root'`, string(targetEKVId)))
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
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (keepGoing pagination.KeepGoing, err error) {
			for _, tgt := range targets {
				if tgt.Table == ActorTable {
					if id, ok := tgt.PrimaryKeyValues[0].(string); ok && apid.ID(id) == actorId {
						actorTargets = append(actorTargets, tgt)
					}
				}
			}
			return pagination.Continue, nil
		})
		require.NoError(t, err)
		require.Empty(t, actorTargets, "actor with NULL encrypted_key should not appear")
	})

	t.Run("BatchUpdateReEncryptedFields", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		targetEKVId := apid.New(apid.PrefixDataEncryptionKey)
		oldEKVId := apid.New(apid.PrefixDataEncryptionKey)

		_, err := rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET target_data_encryption_key_id = '%s' WHERE path = 'root'`, string(targetEKVId)))
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
				ExpectedValue:    ef,
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
		err := db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (keepGoing pagination.KeepGoing, err error) {
			allTargets = append(allTargets, targets...)
			return pagination.Continue, nil
		})
		require.NoError(t, err)
		require.Empty(t, allTargets)
	})

	t.Run("namespace with NULL target_data_encryption_key_id skipped", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		oldEKVId := apid.New(apid.PrefixDataEncryptionKey)

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
		err = db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, lastPage bool) (keepGoing pagination.KeepGoing, err error) {
			for _, tgt := range targets {
				if tgt.Table == ActorTable {
					if id, ok := tgt.PrimaryKeyValues[0].(string); ok && apid.ID(id) == actorId {
						actorTargets = append(actorTargets, tgt)
					}
				}
			}
			return pagination.Continue, nil
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
				ExpectedValue:    encfield.EncryptedField{ID: "dek_old", Data: "old"},
				NewValue:         encfield.EncryptedField{ID: "dek_x", Data: "data"},
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not a registered encrypted field")
	})
}

// TestReEncryptCompareAndSwap compares encrypted payload values rather than
// serialized JSON, preserving later writes, cleared credentials, and deletions.
func TestReEncryptCompareAndSwap(t *testing.T) {
	old := encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "b2xk"}
	replacement := encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "cm90YXRlZA=="}
	differentData := encfield.EncryptedField{ID: old.ID, Data: "bmV3LWRhdGE="}
	differentKey := encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: old.Data}
	for _, test := range []struct {
		name      string
		current   *encfield.EncryptedField
		deleted   bool
		formatted bool
	}{
		{name: "same key with changed ciphertext", current: &differentData},
		{name: "changed key with same ciphertext", current: &differentKey},
		{name: "cleared field"},
		{name: "deleted row", current: &old, deleted: true},
		{name: "equivalent JSON formatting", current: &old, formatted: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, db, rawDB := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
			ctx := apctx.WithFixedClock(context.Background(), now)
			id := apid.New(apid.PrefixActor)
			require.NoError(t, db.CreateActor(ctx, &Actor{
				Id:           id,
				Namespace:    "root",
				ExternalId:   "cas",
				EncryptedKey: &old,
			}))

			// Simulate a write after the rotation job captured old. Reordered,
			// spaced JSON must still match semantically on SQLite and Postgres.
			var stored any
			if test.current != nil {
				stored = *test.current
			}
			if test.formatted {
				stored = fmt.Sprintf("{\n  \"d\": %q, \"id\": %q\n}", old.Data, old.ID)
			}
			_, err := db.(*service).sq.
				Update(ActorTable).
				Set("encrypted_key", stored).
				Set("encrypted_at", now).
				Where(sq.Eq{"id": id}).
				RunWith(rawDB).
				ExecContext(ctx)
			require.NoError(t, err)
			if test.deleted {
				require.NoError(t, db.DeleteActor(ctx, id))
			}

			later := now.Add(time.Hour)
			require.NoError(t, db.BatchUpdateReEncryptedFields(apctx.WithFixedClock(ctx, later), []ReEncryptedFieldUpdate{{
				Table:            ActorTable,
				PrimaryKeyCols:   []string{"id"},
				PrimaryKeyValues: []any{id.String()},
				FieldColumn:      "encrypted_key",
				ExpectedValue:    old,
				NewValue:         replacement,
			}}))
			var actual *encfield.EncryptedField
			var encryptedAt time.Time
			err = db.(*service).sq.
				Select("encrypted_key", "encrypted_at").
				From(ActorTable).
				Where(sq.Eq{"id": id}).
				RunWith(rawDB).
				QueryRowContext(ctx).
				Scan(&actual, &encryptedAt)
			require.NoError(t, err)
			if test.formatted {
				require.Equal(t, &replacement, actual)
				require.True(t, later.Equal(encryptedAt))
			} else {
				require.Equal(t, test.current, actual)
				require.True(t, now.Equal(encryptedAt), "skipped rotations must not touch encrypted_at")
			}
		})
	}
}

// TestReEncryptBatchPreservesPublishedDefinition reproduces a draft edit and
// publication after capture, while allowing unrelated current rows to rotate.
// Replaying the captured batch must also preserve the first rotation timestamp.
func TestReEncryptBatchPreservesPublishedDefinition(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	ctx := apctx.WithFixedClock(context.Background(), now)
	old := encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "b2xk"}
	rotated := encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "cm90YXRlZA=="}
	connector := testConnectorWithDefinition(apid.New(apid.PrefixConnector), "root", "published", 1)
	connector.EncryptedDefinition = old
	require.NoError(t, db.UpsertConnectorGeneration(ctx, connector))
	actorID := apid.New(apid.PrefixActor)
	require.NoError(t, db.CreateActor(ctx, &Actor{
		Id:           actorID,
		Namespace:    "root",
		ExternalId:   "batch",
		EncryptedKey: &old,
	}))
	actorUpdate := ReEncryptedFieldUpdate{
		Table:            ActorTable,
		PrimaryKeyCols:   []string{"id"},
		PrimaryKeyValues: []any{actorID.String()},
		FieldColumn:      "encrypted_key",
		ExpectedValue:    old,
		NewValue:         rotated,
	}
	missingExpectation := actorUpdate
	missingExpectation.ExpectedValue = encfield.EncryptedField{}
	require.ErrorContains(t, db.BatchUpdateReEncryptedFields(ctx, []ReEncryptedFieldUpdate{missingExpectation}), "expected encrypted value is required")
	actor, err := db.GetActor(ctx, actorID)
	require.NoError(t, err)
	require.Equal(t, &old, actor.EncryptedKey)

	editedAt := now.Add(time.Minute)
	ctx = apctx.WithFixedClock(ctx, editedAt)
	newDefinition := encfield.EncryptedField{ID: old.ID, Data: "bmV3LWRlZmluaXRpb24="}
	connector.EncryptedDefinition = newDefinition
	connector.EncryptedAt = &editedAt
	require.NoError(t, db.UpsertConnectorGeneration(ctx, connector))
	require.NoError(t, db.SetConnectorGenerationState(ctx, connector.Id, connector.Generation, ConnectorGenerationStatePrimary))
	updates := []ReEncryptedFieldUpdate{
		{
			Table:            ConnectorGenerationsTable,
			PrimaryKeyCols:   []string{"id"},
			PrimaryKeyValues: []any{connector.DefinitionGenerationId.String()},
			FieldColumn:      "encrypted_definition",
			ExpectedValue:    old,
			NewValue:         rotated,
		},
		actorUpdate, actorUpdate,
	}
	rotatedAt := now.Add(time.Hour)
	for _, attemptAt := range []time.Time{rotatedAt, rotatedAt.Add(time.Hour)} {
		require.NoError(t, db.BatchUpdateReEncryptedFields(apctx.WithFixedClock(ctx, attemptAt), updates))
		published, err := db.GetConnectorGeneration(ctx, connector.Id, connector.Generation)
		require.NoError(t, err)
		require.Equal(t, ConnectorGenerationStatePrimary, published.State)
		require.Equal(t, newDefinition, published.EncryptedDefinition)
		require.NotNil(t, published.EncryptedAt)
		require.True(t, editedAt.Equal(*published.EncryptedAt))
		actor, err := db.GetActor(ctx, actorID)
		require.NoError(t, err)
		require.Equal(t, &rotated, actor.EncryptedKey)
		require.NotNil(t, actor.EncryptedAt)
		require.True(t, rotatedAt.Equal(*actor.EncryptedAt))
	}
}
