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
	tu "github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestSyncActorsList(t *testing.T) {
	var db database.DB
	var enc encrypt.E
	var ctx context.Context
	now := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewFakeClock(now)

	setup := func(t *testing.T, actors *sconfig.ConfiguredActors) config.C {
		var cfg config.C
		cfg, db = database.MustApplyBlankTestDbConfig(t.Name(), nil)
		cfg, enc = encrypt.NewTestEncryptService(cfg, db)
		ctx = apctx.NewBuilderBackground().WithClock(clk).Build()

		cfg.GetRoot().SystemAuth.Actors = actors

		return cfg
	}

	t.Run("syncs actors from list", func(t *testing.T) {
		actors := &sconfig.ConfiguredActors{
			InnerVal: sconfig.ConfiguredActorsList{
				{
					ExternalId: "alice",
					Labels:     map[string]string{"authproxy.io/team": "engineering"},
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
					ExternalId: "bob",
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

		cfg := setup(t, actors)
		svc := NewService(cfg, db, enc, cfg.GetRootLogger())

		err := svc.SyncActorList(ctx)
		require.NoError(t, err)

		// Verify alice was created
		alice, err := db.GetActorByExternalId(ctx, "root", "alice")
		require.NoError(t, err)
		require.Equal(t, "engineering", alice.Labels["authproxy.io/team"])
		require.NotNil(t, alice.EncryptedKey)
		require.Equal(t, LabelValueConfigList, alice.Labels[LabelConfiguredActorSyncSource])

		// Verify bob was created
		bob, err := db.GetActorByExternalId(ctx, "root", "bob")
		require.NoError(t, err)
		require.NotNil(t, bob.EncryptedKey)
	})

	t.Run("deletes stale actors", func(t *testing.T) {
		actors := &sconfig.ConfiguredActors{
			InnerVal: sconfig.ConfiguredActorsList{
				{ExternalId: "alice", Key: &sconfig.Key{InnerVal: &sconfig.KeyShared{SharedKey: &sconfig.KeyData{InnerVal: &sconfig.KeyDataBase64Val{Base64: "dGVzdA=="}}}}},
				{ExternalId: "bob", Key: &sconfig.Key{InnerVal: &sconfig.KeyShared{SharedKey: &sconfig.KeyData{InnerVal: &sconfig.KeyDataBase64Val{Base64: "dGVzdA=="}}}}},
			},
		}

		cfg := setup(t, actors)
		svc := NewService(cfg, db, enc, cfg.GetRootLogger())

		// Initial sync
		err := svc.SyncActorList(ctx)
		require.NoError(t, err)

		// Remove bob from config
		cfg.GetRoot().SystemAuth.Actors = &sconfig.ConfiguredActors{
			InnerVal: sconfig.ConfiguredActorsList{
				{ExternalId: "alice", Key: &sconfig.Key{InnerVal: &sconfig.KeyShared{SharedKey: &sconfig.KeyData{InnerVal: &sconfig.KeyDataBase64Val{Base64: "dGVzdA=="}}}}},
			},
		}

		// Sync again
		err = svc.SyncActorList(ctx)
		require.NoError(t, err)

		// Alice should still exist
		_, err = db.GetActorByExternalId(ctx, "root", "alice")
		require.NoError(t, err)

		// Bob should be deleted
		_, err = db.GetActorByExternalId(ctx, "root", "bob")
		require.ErrorIs(t, err, database.ErrNotFound)
	})

	t.Run("encrypted key can be decrypted", func(t *testing.T) {
		actors := &sconfig.ConfiguredActors{
			InnerVal: sconfig.ConfiguredActorsList{
				{
					ExternalId: "alice",
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

		cfg := setup(t, actors)
		svc := NewService(cfg, db, enc, cfg.GetRootLogger())

		err := svc.SyncActorList(ctx)
		require.NoError(t, err)

		// Get alice and decrypt her key
		alice, err := db.GetActorByExternalId(ctx, "root", "alice")
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

	t.Run("skips sync for non-list actors", func(t *testing.T) {
		actors := &sconfig.ConfiguredActors{
			InnerVal: &sconfig.ConfiguredActorsExternalSource{
				KeysPath: "/tmp/keys",
			},
		}

		cfg := setup(t, actors)
		svc := NewService(cfg, db, enc, cfg.GetRootLogger())

		// Should not error, just skip
		err := svc.SyncActorList(ctx)
		require.NoError(t, err)
	})

	t.Run("handles nil actors", func(t *testing.T) {
		cfg := setup(t, nil)
		svc := NewService(cfg, db, enc, cfg.GetRootLogger())

		err := svc.SyncActorList(ctx)
		require.NoError(t, err)
	})
}

func TestSyncConfiguredActorsExternalSource(t *testing.T) {
	var db database.DB
	var enc encrypt.E
	var ctx context.Context
	now := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewFakeClock(now)

	setup := func(t *testing.T, actors *sconfig.ConfiguredActors) config.C {
		var cfg config.C
		cfg, db = database.MustApplyBlankTestDbConfig(t.Name(), nil)
		cfg, enc = encrypt.NewTestEncryptService(cfg, db)
		ctx = apctx.NewBuilderBackground().WithClock(clk).Build()

		cfg.GetRoot().SystemAuth.Actors = actors

		return cfg
	}

	t.Run("syncs actors from external source", func(t *testing.T) {
		actors := &sconfig.ConfiguredActors{
			InnerVal: &sconfig.ConfiguredActorsExternalSource{
				KeysPath: tu.TestDataPath("admin_user_keys"),
				Permissions: []aschema.Permission{
					{Namespace: "root", Resources: []string{"connections"}, Verbs: []string{"list", "get"}},
				},
			},
		}

		cfg := setup(t, actors)
		svc := NewService(cfg, db, enc, cfg.GetRootLogger())

		err := svc.SyncConfiguredActorsExternalSource(ctx)
		require.NoError(t, err)

		// Verify bobdole was created (one of the .pub files in test data)
		bobdole, err := db.GetActorByExternalId(ctx, "root", "bobdole")
		require.NoError(t, err)
		require.NotNil(t, bobdole.EncryptedKey)
		require.Equal(t, LabelValuePublicKeyDir, bobdole.Labels[LabelConfiguredActorSyncSource])

		// Verify billclinton was created
		billclinton, err := db.GetActorByExternalId(ctx, "root", "billclinton")
		require.NoError(t, err)
		require.NotNil(t, billclinton.EncryptedKey)
		require.Equal(t, LabelValuePublicKeyDir, billclinton.Labels[LabelConfiguredActorSyncSource])
	})

	t.Run("deletes stale actors from external source", func(t *testing.T) {
		// First sync with external source to create actors
		actors := &sconfig.ConfiguredActors{
			InnerVal: &sconfig.ConfiguredActorsExternalSource{
				KeysPath: tu.TestDataPath("admin_user_keys"),
			},
		}

		cfg := setup(t, actors)
		svc := NewService(cfg, db, enc, cfg.GetRootLogger())

		// Initial sync
		err := svc.SyncConfiguredActorsExternalSource(ctx)
		require.NoError(t, err)

		// Manually create an actor that looks like it was from external source but isn't in the directory
		_, err = db.UpsertActor(ctx, &configuredActorData{
			namespace:  "root",
			externalId: "stale-actor",
			labels: database.Labels{
				LabelConfiguredActorSyncSource: LabelValuePublicKeyDir,
			},
		})
		require.NoError(t, err)

		// Verify stale actor exists
		_, err = db.GetActorByExternalId(ctx, "root", "stale-actor")
		require.NoError(t, err)

		// Sync again
		err = svc.SyncConfiguredActorsExternalSource(ctx)
		require.NoError(t, err)

		// Stale actor should be deleted
		_, err = db.GetActorByExternalId(ctx, "root", "stale-actor")
		require.ErrorIs(t, err, database.ErrNotFound)

		// Real actors should still exist
		_, err = db.GetActorByExternalId(ctx, "root", "bobdole")
		require.NoError(t, err)
	})

	t.Run("does not delete actors from different sync source", func(t *testing.T) {
		actors := &sconfig.ConfiguredActors{
			InnerVal: &sconfig.ConfiguredActorsExternalSource{
				KeysPath: tu.TestDataPath("admin_user_keys"),
			},
		}

		cfg := setup(t, actors)
		svc := NewService(cfg, db, enc, cfg.GetRootLogger())

		// Create an actor that was synced from config-list (different source)
		_, err := db.UpsertActor(ctx, &configuredActorData{
			namespace:  "root",
			externalId: "config-list-actor",
			labels: database.Labels{
				LabelConfiguredActorSyncSource: LabelValueConfigList,
			},
		})
		require.NoError(t, err)

		// Sync from external source
		err = svc.SyncConfiguredActorsExternalSource(ctx)
		require.NoError(t, err)

		// Config-list actor should still exist (different source, not deleted)
		_, err = db.GetActorByExternalId(ctx, "root", "config-list-actor")
		require.NoError(t, err)
	})

	t.Run("encrypted key can be decrypted", func(t *testing.T) {
		actors := &sconfig.ConfiguredActors{
			InnerVal: &sconfig.ConfiguredActorsExternalSource{
				KeysPath: tu.TestDataPath("admin_user_keys"),
			},
		}

		cfg := setup(t, actors)
		svc := NewService(cfg, db, enc, cfg.GetRootLogger())

		err := svc.SyncConfiguredActorsExternalSource(ctx)
		require.NoError(t, err)

		// Get bobdole and decrypt the key
		bobdole, err := db.GetActorByExternalId(ctx, "root", "bobdole")
		require.NoError(t, err)
		require.NotNil(t, bobdole.EncryptedKey)

		// Decrypt the key
		decrypted, err := enc.DecryptStringGlobal(ctx, *bobdole.EncryptedKey)
		require.NoError(t, err)

		// Parse the key JSON
		var key sconfig.Key
		err = json.Unmarshal([]byte(decrypted), &key)
		require.NoError(t, err)

		// Verify it's a public/private key type (from .pub file)
		_, ok := key.InnerVal.(*sconfig.KeyPublicPrivate)
		require.True(t, ok)
	})

	t.Run("skips sync for non-external-source actors", func(t *testing.T) {
		actors := &sconfig.ConfiguredActors{
			InnerVal: sconfig.ConfiguredActorsList{
				{
					ExternalId: "alice",
					Key: &sconfig.Key{
						InnerVal: &sconfig.KeyShared{
							SharedKey: &sconfig.KeyData{
								InnerVal: &sconfig.KeyDataBase64Val{Base64: "dGVzdA=="},
							},
						},
					},
				},
			},
		}

		cfg := setup(t, actors)
		svc := NewService(cfg, db, enc, cfg.GetRootLogger())

		// Should not error, just skip
		err := svc.SyncConfiguredActorsExternalSource(ctx)
		require.NoError(t, err)

		// Alice should not exist (sync was skipped)
		_, err = db.GetActorByExternalId(ctx, "root", "alice")
		require.ErrorIs(t, err, database.ErrNotFound)
	})

	t.Run("handles nil actors", func(t *testing.T) {
		cfg := setup(t, nil)
		svc := NewService(cfg, db, enc, cfg.GetRootLogger())

		err := svc.SyncConfiguredActorsExternalSource(ctx)
		require.NoError(t, err)
	})
}
