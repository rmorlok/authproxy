package core

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"

	"github.com/rmorlok/authproxy/internal/test_utils/connectorfixture"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apasynq"
	"github.com/rmorlok/authproxy/internal/apasynq/mock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	apredismock "github.com/rmorlok/authproxy/internal/apredis/mock"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	hmock "github.com/rmorlok/authproxy/internal/httpf/mock"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	cfgschema "github.com/rmorlok/authproxy/internal/schema/config"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configuredRateLimit(id, name, namespace string) rlschema.RateLimit {
	return rlschema.RateLimit{
		TypeMeta: meta.NewTypeMeta(rlschema.RateLimitKind),
		Metadata: meta.ObjectMeta{
			ID:        id,
			Name:      scommon.ResourceName(name),
			Namespace: namespace,
		},
		Spec: rlschema.RateLimitSpec{
			Selector: rlschema.Selector{},
			Bucket:   rlschema.Bucket{},
			Algorithm: rlschema.Algorithm{
				TokenBucket: &rlschema.TokenBucket{Capacity: 10, RefillRate: 1},
			},
		},
	}
}

func TestMigration(t *testing.T) {
	var cfg config.C
	var db database.DB
	var r apredis.Client
	var h httpf.F
	var rawDb *sql.DB
	var service iface.C
	var asynqClient apasynq.Client

	setupRoot := func(t *testing.T, root *cfgschema.Root) func() {
		root.DevSettings = &cfgschema.DevSettings{
			Enabled:                  true,
			FakeEncryption:           true,
			FakeEncryptionSkipBase64: true,
		}
		cfg = config.FromRoot(root)

		logger := slog.Default()
		cfg, db, rawDb = database.MustApplyBlankTestDbConfigRaw(t, cfg)
		ctrl := gomock.NewController(t)
		r = apredismock.NewMockClient(ctrl)
		e := encrypt.NewEncryptService(cfg, db, logger)

		asynqClient = mock.NewMockClient(ctrl)
		asynqClient.(*mock.MockClient).EXPECT().
			EnqueueContext(gomock.Any(), gomock.Any()).
			AnyTimes().
			Return(nil, nil)
		h = hmock.NewMockF(ctrl)

		service = NewCoreService(cfg, db, e, r, h, asynqClient, logger)

		return func() {
			ctrl.Finish()
			err := rawDb.Close()
			assert.NoError(t, err)
		}
	}

	t.Run("rate limits", func(t *testing.T) {
		ns := "root.acme"
		first := configuredRateLimit("rl_test0000000000001", "tenant-default", ns)
		second := configuredRateLimit("", "salesforce", ns)
		second.Spec.Scope = &rlschema.RateLimitScope{ConnectorRef: &meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1,
			Kind:       cschema.ConnectorKind,
			Name:       "salesforce",
			Namespace:  ns,
		}}
		root := &cfgschema.Root{
			RateLimits: &cfgschema.RateLimits{LoadFromList: []rlschema.RateLimit{first, second}},
		}
		cleanup := setupRoot(t, root)
		defer cleanup()

		connectorfixture.Seed(t, db, encrypt.NewEncryptService(cfg, db, slog.Default()), cschema.Connector{Metadata: meta.ObjectMeta{Name: "salesforce", Namespace: ns}, Spec: cschema.ConnectorSpec{Definition: cschema.ConnectorDefinition{DisplayName: "Salesforce"}}})
		require.NoError(t, service.Migrate(context.Background()))

		storedFirst, err := db.GetRateLimit(context.Background(), apid.MustParse("rl_test0000000000001"))
		require.NoError(t, err)
		require.Equal(t, scommon.ResourceName("tenant-default"), storedFirst.Name)
		require.Equal(t, "config", storedFirst.Labels[rateLimitSourceLabelKey])

		storedSecondPage := db.ListRateLimitsBuilder().ForNamespaceMatchers([]string{ns}).ForName("salesforce").FetchPage(context.Background())
		require.NoError(t, storedSecondPage.Error)
		require.Len(t, storedSecondPage.Results, 1)
		storedSecond := storedSecondPage.Results[0]
		require.NotNil(t, storedSecond.Definition.Scope)
		require.NotNil(t, storedSecond.Definition.Scope.ConnectorRef)
		require.NotEmpty(t, storedSecond.Definition.Scope.ConnectorRef.ID)
		require.Empty(t, storedSecond.Definition.Scope.ConnectorRef.Name)
		require.Zero(t, storedSecond.Definition.Scope.ConnectorRef.Generation)

		// Reconciliation updates the same explicit identity instead of creating
		// another row, and a removed config-owned resource is cleaned up.
		root.RateLimits.LoadFromList[0].Metadata.Name = "tenant-renamed"
		root.RateLimits.LoadFromList[0].Metadata.Labels = map[string]string{"team": "platform"}
		root.RateLimits.LoadFromList[0].Spec.Mode = rlschema.ModeObserve
		root.RateLimits.LoadFromList = root.RateLimits.LoadFromList[:1]

		apiID := apid.MustParse("rl_test0000000000099")
		require.NoError(t, db.CreateRateLimit(context.Background(), &database.RateLimit{
			Id:         apiID,
			Namespace:  ns,
			Name:       "api-owned",
			Definition: configuredRateLimit("", "", ns).Spec,
		}))
		require.NoError(t, service.Migrate(context.Background()))

		storedFirst, err = db.GetRateLimit(context.Background(), apid.MustParse("rl_test0000000000001"))
		require.NoError(t, err)
		require.Equal(t, scommon.ResourceName("tenant-renamed"), storedFirst.Name)
		require.Equal(t, rlschema.ModeObserve, storedFirst.Definition.Mode)
		require.Equal(t, "platform", storedFirst.Labels["team"])
		_, err = db.GetRateLimit(context.Background(), storedSecond.Id)
		require.ErrorIs(t, err, database.ErrNotFound)
		_, err = db.GetRateLimit(context.Background(), apiID)
		require.NoError(t, err, "API-owned rate limits must not be removed by config reconciliation")
	})

	t.Run("preserves formerly configured connectors", func(t *testing.T) {
		cleanup := setupRoot(t, &cfgschema.Root{})
		defer cleanup()
		id := apid.New(apid.PrefixConnector)
		connectorfixture.Seed(t, db, encrypt.NewEncryptService(cfg, db, slog.Default()), cschema.Connector{
			Metadata: meta.ObjectMeta{ID: id.String(), Name: "retained", Namespace: "root", Labels: map[string]string{"apxy/cxr/source": "config"}},
			Spec:     cschema.ConnectorSpec{Definition: cschema.ConnectorDefinition{DisplayName: "Retained"}},
		})
		before, err := db.GetConnectorGeneration(context.Background(), id, 1)
		require.NoError(t, err)
		require.NoError(t, service.Migrate(context.Background()))
		after, err := db.GetConnectorGeneration(context.Background(), id, 1)
		require.NoError(t, err)
		require.Equal(t, before, after)
	})

	t.Run("namespaces", func(t *testing.T) {
		t.Run("includes configured actor namespaces", func(t *testing.T) {
			cleanup := setupRoot(t, &cfgschema.Root{})
			defer cleanup()

			actor := actorschema.NewActor()
			actor.Metadata.Namespace = "root.smoke"
			actor.Spec.ExternalId = "smoke-user"
			actor.Spec.SigningKey = &cfgschema.Key{
				InnerVal: &cfgschema.KeyShared{
					SharedKey: &cfgschema.KeyData{
						InnerVal: &cfgschema.KeyDataBase64Val{Base64: "dGVzdA=="},
					},
				},
			}
			cfg.GetRoot().SystemAuth.Actors = &cfgschema.ConfiguredActors{
				InnerVal: cfgschema.ConfiguredActorsList{actor},
			}

			require.NoError(t, service.Migrate(context.Background()))
			_, err := db.GetNamespace(context.Background(), "root.smoke")
			require.NoError(t, err)
		})
	})
}
