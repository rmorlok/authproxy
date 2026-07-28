package database

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/rmorlok/authproxy/internal/apid"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

type namedResourceHarness struct {
	resourceType string
	table        string
	prefix       apid.Prefix
	create       func(context.Context, DB, apid.ID, string, scommon.ResourceName) error
	getName      func(context.Context, DB, apid.ID) (scommon.ResourceName, error)
	delete       func(context.Context, DB, apid.ID) error
}

func namedResourceHarnesses() []namedResourceHarness {
	return []namedResourceHarness{
		{
			resourceType: "actor",
			table:        ActorTable,
			prefix:       apid.PrefixActor,
			create: func(ctx context.Context, db DB, id apid.ID, ns string, name scommon.ResourceName) error {
				return db.CreateActor(ctx, &Actor{
					Id:         id,
					Name:       name,
					Namespace:  ns,
					ExternalId: id.String(),
				})
			},
			getName: func(ctx context.Context, db DB, id apid.ID) (scommon.ResourceName, error) {
				value, err := db.GetActor(ctx, id)
				if err != nil {
					return "", err
				}
				return value.Name, nil
			},
			delete: func(ctx context.Context, db DB, id apid.ID) error {
				return db.DeleteActor(ctx, id)
			},
		},
		{
			resourceType: "connection",
			table:        ConnectionsTable,
			prefix:       apid.PrefixConnection,
			create: func(ctx context.Context, db DB, id apid.ID, ns string, name scommon.ResourceName) error {
				return db.CreateConnection(ctx, &Connection{
					Id:               id,
					Name:             name,
					Namespace:        ns,
					State:            ConnectionStateSetup,
					ConnectorId:      apid.New(apid.PrefixConnectorVersion),
					ConnectorVersion: 1,
				})
			},
			getName: func(ctx context.Context, db DB, id apid.ID) (scommon.ResourceName, error) {
				value, err := db.GetConnection(ctx, id)
				if err != nil {
					return "", err
				}
				return value.Name, nil
			},
			delete: func(ctx context.Context, db DB, id apid.ID) error {
				return db.DeleteConnection(ctx, id)
			},
		},
		{
			resourceType: "key",
			table:        KeysTable,
			prefix:       apid.PrefixKey,
			create: func(ctx context.Context, db DB, id apid.ID, ns string, name scommon.ResourceName) error {
				return db.CreateKey(ctx, &Key{
					Id:        id,
					Name:      name,
					Namespace: ns,
				})
			},
			getName: func(ctx context.Context, db DB, id apid.ID) (scommon.ResourceName, error) {
				value, err := db.GetKey(ctx, id)
				if err != nil {
					return "", err
				}
				return value.Name, nil
			},
			delete: func(ctx context.Context, db DB, id apid.ID) error {
				return db.DeleteKey(ctx, id)
			},
		},
		{
			resourceType: "rate limit",
			table:        RateLimitsTable,
			prefix:       apid.PrefixRateLimit,
			create: func(ctx context.Context, db DB, id apid.ID, ns string, name scommon.ResourceName) error {
				return db.CreateRateLimit(ctx, &RateLimit{
					Id:         id,
					Name:       name,
					Namespace:  ns,
					Definition: validDef(),
				})
			},
			getName: func(ctx context.Context, db DB, id apid.ID) (scommon.ResourceName, error) {
				value, err := db.GetRateLimit(ctx, id)
				if err != nil {
					return "", err
				}
				return value.Name, nil
			},
			delete: func(ctx context.Context, db DB, id apid.ID) error {
				return db.DeleteRateLimit(ctx, id)
			},
		},
	}
}

func TestResourceNamesDefaultToID(t *testing.T) {
	for _, harness := range namedResourceHarnesses() {
		t.Run(harness.resourceType, func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			ctx := context.Background()
			id := apid.New(harness.prefix)

			require.NoError(t, harness.create(ctx, db, id, "root", ""))
			name, err := harness.getName(ctx, db, id)
			require.NoError(t, err)
			require.Equal(t, scommon.ResourceName(id.String()), name)
		})
	}
}

func TestResourceNameLiveUniquenessAndReuse(t *testing.T) {
	for _, harness := range namedResourceHarnesses() {
		t.Run(harness.resourceType, func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			ctx := context.Background()
			require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.other"}))

			firstID := apid.New(harness.prefix)
			require.NoError(t, harness.create(ctx, db, firstID, "root", "shared"))

			require.Error(t, harness.create(ctx, db, apid.New(harness.prefix), "root", "shared"))
			require.NoError(t, harness.create(ctx, db, apid.New(harness.prefix), "root.other", "shared"))
			require.NoError(t, harness.create(ctx, db, apid.New(harness.prefix), "root", "Shared"))

			require.NoError(t, harness.delete(ctx, db, firstID))
			require.NoError(t, harness.create(ctx, db, apid.New(harness.prefix), "root", "shared"))
		})
	}
}

func TestResourceNameCanBeChangedByIDWithoutConflicts(t *testing.T) {
	for _, harness := range namedResourceHarnesses() {
		t.Run(harness.resourceType, func(t *testing.T) {
			_, db, rawDB := MustApplyBlankTestDbConfigRaw(t, nil)
			ctx := context.Background()
			firstID := apid.New(harness.prefix)
			secondID := apid.New(harness.prefix)
			require.NoError(t, harness.create(ctx, db, firstID, "root", "first"))
			require.NoError(t, harness.create(ctx, db, secondID, "root", "second"))

			_, err := rawDB.Exec(fmt.Sprintf(
				"UPDATE %s SET name = 'renamed' WHERE id = '%s'",
				harness.table,
				firstID.String(),
			))
			require.NoError(t, err)
			name, err := harness.getName(ctx, db, firstID)
			require.NoError(t, err)
			require.Equal(t, scommon.ResourceName("renamed"), name)

			_, err = rawDB.Exec(fmt.Sprintf(
				"UPDATE %s SET name = 'renamed' WHERE id = '%s'",
				harness.table,
				secondID.String(),
			))
			require.Error(t, err)
			name, err = harness.getName(ctx, db, secondID)
			require.NoError(t, err)
			require.Equal(t, scommon.ResourceName("second"), name)
		})
	}
}

func TestResourceNamesDoNotConflictAcrossTypes(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()

	for _, harness := range namedResourceHarnesses() {
		require.NoError(t, harness.create(ctx, db, apid.New(harness.prefix), "root", "shared"))
	}
}

func TestResourceNameValidationAppliesOnCreate(t *testing.T) {
	for _, harness := range namedResourceHarnesses() {
		t.Run(harness.resourceType, func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			err := harness.create(context.Background(), db, apid.New(harness.prefix), "root", "not valid")
			require.Error(t, err)
			require.Contains(t, err.Error(), "name")
		})
	}
}

func TestResourceNameDatabaseRejectsEmptyValues(t *testing.T) {
	for _, harness := range namedResourceHarnesses() {
		t.Run(harness.resourceType, func(t *testing.T) {
			_, db, rawDB := MustApplyBlankTestDbConfigRaw(t, nil)
			ctx := context.Background()
			id := apid.New(harness.prefix)
			require.NoError(t, harness.create(ctx, db, id, "root", "valid"))

			_, err := rawDB.Exec(fmt.Sprintf(
				"UPDATE %s SET name = '' WHERE id = '%s'",
				harness.table,
				id.String(),
			))
			require.Error(t, err)

			_, err = rawDB.Exec(fmt.Sprintf(
				"UPDATE %s SET name = NULL WHERE id = '%s'",
				harness.table,
				id.String(),
			))
			require.Error(t, err)
		})
	}
}

func TestResourceNameMigrationBackfillsExistingRows(t *testing.T) {
	_, db, rawDB := MustApplyBlankTestDbConfigRaw(t, nil)
	service := db.(*service)
	migrateResourceNamesBySteps(t, service, -1)

	_, err := rawDB.Exec(`
		INSERT INTO actors (id, namespace, external_id)
		VALUES ('act_migration', 'root', 'migration')
	`)
	require.NoError(t, err)
	_, err = rawDB.Exec(`
		INSERT INTO connections (id, namespace, state, connector_id, connector_version)
		VALUES ('cxn_migration', 'root', 'setup', 'cxr_migration', 1)
	`)
	require.NoError(t, err)
	_, err = rawDB.Exec(`
		INSERT INTO keys (
			id, namespace, usage, material_type, state, created_at, updated_at
		)
		VALUES (
			'key_migration', 'root', 'data_encryption', 'symmetric', 'active',
			'2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'
		)
	`)
	require.NoError(t, err)
	_, err = rawDB.Exec(`
		INSERT INTO rate_limits (
			id, namespace, definition, created_at, updated_at
		)
		VALUES (
			'rl_migration', 'root', '{}',
			'2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'
		)
	`)
	require.NoError(t, err)

	migrateResourceNamesBySteps(t, service, 1)

	for table, id := range map[string]string{
		ActorTable:       "act_migration",
		ConnectionsTable: "cxn_migration",
		KeysTable:        "key_migration",
		RateLimitsTable:  "rl_migration",
	} {
		var name string
		err := rawDB.QueryRow(fmt.Sprintf(
			"SELECT name FROM %s WHERE id = '%s'",
			table,
			id,
		)).Scan(&name)
		require.NoError(t, err)
		require.Equal(t, id, name)
	}

	var globalKeyName string
	require.NoError(t, rawDB.QueryRow(
		"SELECT name FROM keys WHERE id = 'key_global'",
	).Scan(&globalKeyName))
	require.Equal(t, "key_global", globalKeyName)
}

func migrateResourceNamesBySteps(t *testing.T, service *service, steps int) {
	t.Helper()

	source, err := iofs.New(
		migrationsFs,
		fmt.Sprintf("migrations/%s", service.cfg.GetProvider()),
	)
	require.NoError(t, err)

	migrator, err := migrate.NewWithSourceInstance("iofs", source, service.cfg.GetUri())
	require.NoError(t, err)
	defer func() {
		sourceErr, databaseErr := migrator.Close()
		require.NoError(t, sourceErr)
		require.NoError(t, databaseErr)
	}()

	require.NoError(t, migrator.Steps(steps))
}
