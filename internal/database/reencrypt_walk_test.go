package database

import (
	"context"
	"fmt"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
)

// reEncryptionWalkPageSize is small so the walk tests span several pages.
const reEncryptionWalkPageSize = 3

// rotateTableForTest walks the fields requiring re-encryption the way the
// re-encryption task does: each page is rewritten to the target key before the next
// page is read, so rewritten rows leave the walk's filter while it runs. It returns
// how many times each row of table was visited, keyed by its primary key values.
func rotateTableForTest(t *testing.T, ctx context.Context, db DB, table string) map[string]int {
	t.Helper()
	s := db.(*service)
	visits := make(map[string]int)
	err := s.enumerateFieldsRequiringReEncryption(ctx, reEncryptionWalkPageSize, func(targets []ReEncryptionTarget, _ bool) (pagination.KeepGoing, error) {
		var updates []ReEncryptedFieldUpdate
		for _, target := range targets {
			if target.Table != table {
				continue
			}
			visits[fmt.Sprint(target.PrimaryKeyValues)]++
			updates = append(updates, ReEncryptedFieldUpdate{
				Table:            target.Table,
				PrimaryKeyCols:   target.PrimaryKeyCols,
				PrimaryKeyValues: target.PrimaryKeyValues,
				FieldColumn:      target.FieldColumn,
				ExpectedValue:    target.EncryptedFieldValue,
				NewValue:         encfield.EncryptedField{ID: target.TargetDataEncryptionKeyId, Data: "cm90YXRlZA=="},
			})
		}
		require.NoError(t, db.BatchUpdateReEncryptedFields(ctx, updates))
		return pagination.Continue, nil
	})
	require.NoError(t, err)
	return visits
}

// remainingTargetsForTest counts the fields of table that still require
// re-encryption.
func remainingTargetsForTest(t *testing.T, ctx context.Context, db DB, table string) int {
	t.Helper()
	remaining := 0
	err := db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []ReEncryptionTarget, _ bool) (pagination.KeepGoing, error) {
		for _, target := range targets {
			if target.Table == table {
				remaining++
			}
		}
		return pagination.Continue, nil
	})
	require.NoError(t, err)
	return remaining
}

// setNamespaceTargetKeyForTest points namespace at a new target data encryption key
// and returns it, so fields encrypted under any other key need re-encryption.
func setNamespaceTargetKeyForTest(t *testing.T, ctx context.Context, db DB, namespace string) apid.ID {
	t.Helper()
	s := db.(*service)
	target := apid.New(apid.PrefixDataEncryptionKey)
	_, err := s.sq.Update(NamespacesTable).
		Set("target_data_encryption_key_id", target).
		Where(sq.Eq{"path": namespace}).
		RunWith(s.db).
		ExecContext(ctx)
	require.NoError(t, err)
	return target
}

// TestReEncryptionWalkVisitsEveryRowWhileRotating covers a walk whose callback
// re-encrypts each page before the next one is read, as the reencrypt_all task does.
// Rewritten rows leave the walk's filter, so an offset walk skips a page of rows for
// each page it rewrites.
func TestReEncryptionWalkVisitsEveryRowWhileRotating(t *testing.T) {
	t.Run("single-column key", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := reconcileTestCtx()
		setNamespaceTargetKeyForTest(t, ctx, db, "root")

		// Actors are encrypted under an old key, spanning several pages.
		const actorCount = 10
		oldKey := apid.New(apid.PrefixDataEncryptionKey)
		for i := 0; i < actorCount; i++ {
			id := apid.New(apid.PrefixActor)
			require.NoError(t, db.CreateActor(ctx, &Actor{
				Id:           id,
				Namespace:    "root",
				ExternalId:   id.String(),
				EncryptedKey: &encfield.EncryptedField{ID: oldKey, Data: "dGVzdA=="},
			}))
		}

		// One walk visits every actor once and leaves none behind.
		visits := rotateTableForTest(t, ctx, db, ActorTable)
		require.Len(t, visits, actorCount)
		for key, count := range visits {
			require.Equal(t, 1, count, key)
		}
		require.Zero(t, remainingTargetsForTest(t, ctx, db, ActorTable))
	})

	t.Run("composite key", func(t *testing.T) {
		// Two tools share a namespace; each has several revisions, so pages break both
		// within one tool's revisions and across the two tools.
		ctx, db, first, _ := standaloneToolForTest(t)
		connection := standaloneToolConnectionForTest(t, ctx, db, first.Namespace)
		second := &Tool{
			Id:           apid.New(apid.PrefixTool),
			Namespace:    connection.Namespace,
			ConnectionId: connection.Id,
			Description:  "Read another example resource.",
			Verbs:        []string{"example:read"},
		}
		definition := encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "ZGVmaW5pdGlvbg=="}
		_, err := db.CreateStandaloneTool(ctx, second, definition, toolDefinitionDigestForTest)
		require.NoError(t, err)

		const revisionsPerTool = 4
		for _, tool := range []*Tool{first, second} {
			for revision := uint64(1); revision < revisionsPerTool; revision++ {
				_, err := db.UpdateStandaloneTool(ctx, tool, revision, definition, toolDefinitionDigestForTest)
				require.NoError(t, err)
			}
		}

		// Every revision is encrypted under a key other than the new target.
		setNamespaceTargetKeyForTest(t, ctx, db, first.Namespace)

		// One walk visits every (tool, revision) once and leaves none behind.
		visits := rotateTableForTest(t, ctx, db, ToolRevisionsTable)
		require.Len(t, visits, 2*revisionsPerTool)
		for key, count := range visits {
			require.Equal(t, 1, count, key)
		}
		require.Zero(t, remainingTargetsForTest(t, ctx, db, ToolRevisionsTable))
	})
}

// TestPrimaryKeyAfter covers the SQL that continues a re-encryption walk after a
// row, for single-column and composite primary keys.
func TestPrimaryKeyAfter(t *testing.T) {
	// A single column compares directly.
	sqlStr, args, err := primaryKeyAfter([]string{"actors.id"}, []any{"act_b"}).ToSql()
	require.NoError(t, err)
	require.Equal(t, "actors.id > ?", sqlStr)
	require.Equal(t, []any{"act_b"}, args)

	// A composite key compares as a row value, ordering by its columns in turn.
	sqlStr, args, err = primaryKeyAfter(
		[]string{"tool_revisions.tool_id", "tool_revisions.revision"},
		[]any{"tol_b", int64(2)},
	).ToSql()
	require.NoError(t, err)
	require.Equal(t, "(tool_revisions.tool_id, tool_revisions.revision) > (?, ?)", sqlStr)
	require.Equal(t, []any{"tol_b", int64(2)}, args)
}
