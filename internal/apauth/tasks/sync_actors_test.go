package tasks

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestSyncAdminUsersList(t *testing.T) {
	var db database.DB
	var enc encrypt.E
	var ctx context.Context
	now := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewFakeClock(now)

	setup := func(t *testing.T, adminUsers *sconfig.AdminUsers) config.C {
		var cfg config.C
		cfg, db = database.MustApplyBlankTestDbConfig(t.Name(), nil)
		cfg, enc = encrypt.NewTestEncryptService(cfg, db)
		ctx = apctx.NewBuilderBackground().WithClock(clk).Build()

		cfg.GetRoot().SystemAuth.AdminUsers = adminUsers
		cfg.GetRoot().SystemAuth.AdminEmailDomain = "example.com"

		return cfg
	}

	t.Run("syncs admin users from list", func(t *testing.T) {
		adminUsers := &sconfig.AdminUsers{
			InnerVal: sconfig.AdminUsersList{
				{
					Username: "alice",
					Email:    "alice@custom.com",
					Key: &sconfig.Key{
						InnerVal: &sconfig.KeyShared{
							SharedKey: &sconfig.KeyData{
								InnerVal: &sconfig.KeyDataBase64Val{Base64: "dGVzdGtleTEyMw=="},
							},
						},
					},
					Permissions: []aschema.Permission{
						{Namespace: "root", Resources: []string{"*"}, Verbs: []string{"*"}},
					},
				},
				{
					Username: "bob",
					Key: &sconfig.Key{
						InnerVal: &sconfig.KeyShared{
							SharedKey: &sconfig.KeyData{
								InnerVal: &sconfig.KeyDataBase64Val{Base64: "dGVzdGtleTQ1Ng=="},
							},
						},
					},
				},
			},
		}

		cfg := setup(t, adminUsers)
		svc := NewService(cfg, db, enc, cfg.GetRootLogger())

		err := svc.SyncActorList(ctx)
		require.NoError(t, err)

		// Verify alice was created
		alice, err := db.GetActorByExternalId(ctx, "root", "admin/alice")
		require.NoError(t, err)
		require.Equal(t, "alice@custom.com", alice.Email)
		require.True(t, alice.Admin)
		require.NotNil(t, alice.EncryptedKey)
		require.Equal(t, LabelValueConfigList, alice.Labels[LabelAdminSyncSource])

		// Verify bob was created with generated email
		bob, err := db.GetActorByExternalId(ctx, "root", "admin/bob")
		require.NoError(t, err)
		require.Equal(t, "bob@example.com", bob.Email)
		require.True(t, bob.Admin)
		require.NotNil(t, bob.EncryptedKey)
	})

	t.Run("deletes stale admin users", func(t *testing.T) {
		adminUsers := &sconfig.AdminUsers{
			InnerVal: sconfig.AdminUsersList{
				{Username: "alice", Key: &sconfig.Key{InnerVal: &sconfig.KeyShared{SharedKey: &sconfig.KeyData{InnerVal: &sconfig.KeyDataBase64Val{Base64: "dGVzdA=="}}}}},
				{Username: "bob", Key: &sconfig.Key{InnerVal: &sconfig.KeyShared{SharedKey: &sconfig.KeyData{InnerVal: &sconfig.KeyDataBase64Val{Base64: "dGVzdA=="}}}}},
			},
		}

		cfg := setup(t, adminUsers)
		svc := NewService(cfg, db, enc, cfg.GetRootLogger())

		// Initial sync
		err := svc.SyncActorList(ctx)
		require.NoError(t, err)

		// Remove bob from config
		cfg.GetRoot().SystemAuth.AdminUsers = &sconfig.AdminUsers{
			InnerVal: sconfig.AdminUsersList{
				{Username: "alice", Key: &sconfig.Key{InnerVal: &sconfig.KeyShared{SharedKey: &sconfig.KeyData{InnerVal: &sconfig.KeyDataBase64Val{Base64: "dGVzdA=="}}}}},
			},
		}

		// Sync again
		err = svc.SyncActorList(ctx)
		require.NoError(t, err)

		// Alice should still exist
		_, err = db.GetActorByExternalId(ctx, "root", "admin/alice")
		require.NoError(t, err)

		// Bob should be deleted
		_, err = db.GetActorByExternalId(ctx, "root", "admin/bob")
		require.ErrorIs(t, err, database.ErrNotFound)
	})

	t.Run("encrypted key can be decrypted", func(t *testing.T) {
		adminUsers := &sconfig.AdminUsers{
			InnerVal: sconfig.AdminUsersList{
				{
					Username: "alice",
					Key: &sconfig.Key{
						InnerVal: &sconfig.KeyShared{
							SharedKey: &sconfig.KeyData{
								InnerVal: &sconfig.KeyDataBase64Val{Base64: "dGVzdGtleTEyMzQ1Njc4OTAxMjM0NTY="},
							},
						},
					},
				},
			},
		}

		cfg := setup(t, adminUsers)
		svc := NewService(cfg, db, enc, cfg.GetRootLogger())

		err := svc.SyncActorList(ctx)
		require.NoError(t, err)

		// Get alice and decrypt her key
		alice, err := db.GetActorByExternalId(ctx, "root", "admin/alice")
		require.NoError(t, err)
		require.NotNil(t, alice.EncryptedKey)

		// Decrypt the key
		decrypted, err := enc.DecryptStringGlobal(ctx, *alice.EncryptedKey)
		require.NoError(t, err)

		// Parse the key JSON
		var key sconfig.Key
		err = json.Unmarshal([]byte(decrypted), &key)
		require.NoError(t, err)

		// Verify it's the right type
		_, ok := key.InnerVal.(*sconfig.KeyShared)
		require.True(t, ok)
	})

	t.Run("skips sync for non-list admin users", func(t *testing.T) {
		adminUsers := &sconfig.AdminUsers{
			InnerVal: &sconfig.AdminUsersExternalSource{
				KeysPath: "/tmp/keys",
			},
		}

		cfg := setup(t, adminUsers)
		svc := NewService(cfg, db, enc, cfg.GetRootLogger())

		// Should not error, just skip
		err := svc.SyncActorList(ctx)
		require.NoError(t, err)
	})

	t.Run("handles nil admin users", func(t *testing.T) {
		cfg := setup(t, nil)
		svc := NewService(cfg, db, enc, cfg.GetRootLogger())

		err := svc.SyncActorList(ctx)
		require.NoError(t, err)
	})
}
