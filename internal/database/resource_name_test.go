package database

import (
	"context"
	"fmt"
	"testing"

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
	rename       func(context.Context, DB, apid.ID, scommon.ResourceName) error
	list         func(context.Context, DB, scommon.ResourceName, []string, int32) namedResourcePage
	listCursor   func(context.Context, DB, string) namedResourcePage
	delete       func(context.Context, DB, apid.ID) error
}

type namedResourcePage struct {
	ids    []apid.ID
	cursor string
	err    error
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
			rename: func(ctx context.Context, db DB, id apid.ID, name scommon.ResourceName) error {
				_, err := db.UpdateActorName(ctx, id, name)
				return err
			},
			list: func(ctx context.Context, db DB, name scommon.ResourceName, matchers []string, limit int32) namedResourcePage {
				page := db.ListActorsBuilder().ForName(name).ForNamespaceMatchers(matchers).Limit(limit).FetchPage(ctx)
				ids := make([]apid.ID, len(page.Results))
				for i := range page.Results {
					ids[i] = page.Results[i].Id
				}
				return namedResourcePage{ids: ids, cursor: page.Cursor, err: page.Error}
			},
			listCursor: func(ctx context.Context, db DB, cursor string) namedResourcePage {
				executor, err := db.ListActorsFromCursor(ctx, cursor)
				if err != nil {
					return namedResourcePage{err: err}
				}
				page := executor.FetchPage(ctx)
				ids := make([]apid.ID, len(page.Results))
				for i := range page.Results {
					ids[i] = page.Results[i].Id
				}
				return namedResourcePage{ids: ids, cursor: page.Cursor, err: page.Error}
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
			rename: func(ctx context.Context, db DB, id apid.ID, name scommon.ResourceName) error {
				_, err := db.UpdateConnectionName(ctx, id, name)
				return err
			},
			list: func(ctx context.Context, db DB, name scommon.ResourceName, matchers []string, limit int32) namedResourcePage {
				page := db.ListConnectionsBuilder().ForName(name).ForNamespaceMatchers(matchers).Limit(limit).FetchPage(ctx)
				ids := make([]apid.ID, len(page.Results))
				for i := range page.Results {
					ids[i] = page.Results[i].Id
				}
				return namedResourcePage{ids: ids, cursor: page.Cursor, err: page.Error}
			},
			listCursor: func(ctx context.Context, db DB, cursor string) namedResourcePage {
				executor, err := db.ListConnectionsFromCursor(ctx, cursor)
				if err != nil {
					return namedResourcePage{err: err}
				}
				page := executor.FetchPage(ctx)
				ids := make([]apid.ID, len(page.Results))
				for i := range page.Results {
					ids[i] = page.Results[i].Id
				}
				return namedResourcePage{ids: ids, cursor: page.Cursor, err: page.Error}
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
			rename: func(ctx context.Context, db DB, id apid.ID, name scommon.ResourceName) error {
				_, err := db.UpdateKey(ctx, id, map[string]interface{}{"name": name})
				return err
			},
			list: func(ctx context.Context, db DB, name scommon.ResourceName, matchers []string, limit int32) namedResourcePage {
				page := db.ListKeysBuilder().ForName(name).ForNamespaceMatchers(matchers).Limit(limit).FetchPage(ctx)
				ids := make([]apid.ID, len(page.Results))
				for i := range page.Results {
					ids[i] = page.Results[i].Id
				}
				return namedResourcePage{ids: ids, cursor: page.Cursor, err: page.Error}
			},
			listCursor: func(ctx context.Context, db DB, cursor string) namedResourcePage {
				executor, err := db.ListKeysFromCursor(ctx, cursor)
				if err != nil {
					return namedResourcePage{err: err}
				}
				page := executor.FetchPage(ctx)
				ids := make([]apid.ID, len(page.Results))
				for i := range page.Results {
					ids[i] = page.Results[i].Id
				}
				return namedResourcePage{ids: ids, cursor: page.Cursor, err: page.Error}
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
			rename: func(ctx context.Context, db DB, id apid.ID, name scommon.ResourceName) error {
				_, err := db.UpdateRateLimitName(ctx, id, name)
				return err
			},
			list: func(ctx context.Context, db DB, name scommon.ResourceName, matchers []string, limit int32) namedResourcePage {
				page := db.ListRateLimitsBuilder().ForName(name).ForNamespaceMatchers(matchers).Limit(limit).FetchPage(ctx)
				ids := make([]apid.ID, len(page.Results))
				for i := range page.Results {
					ids[i] = page.Results[i].Id
				}
				return namedResourcePage{ids: ids, cursor: page.Cursor, err: page.Error}
			},
			listCursor: func(ctx context.Context, db DB, cursor string) namedResourcePage {
				executor, err := db.ListRateLimitsFromCursor(ctx, cursor)
				if err != nil {
					return namedResourcePage{err: err}
				}
				page := executor.FetchPage(ctx)
				ids := make([]apid.ID, len(page.Results))
				for i := range page.Results {
					ids[i] = page.Results[i].Id
				}
				return namedResourcePage{ids: ids, cursor: page.Cursor, err: page.Error}
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

func TestResourceNameRenameAndExactListBehavior(t *testing.T) {
	for _, harness := range namedResourceHarnesses() {
		t.Run(harness.resourceType, func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			ctx := context.Background()
			require.NoError(t, db.EnsureNamespaceByPath(ctx, "root.allowed"))
			require.NoError(t, db.EnsureNamespaceByPath(ctx, "root.hidden"))

			firstID := apid.New(harness.prefix)
			secondID := apid.New(harness.prefix)
			hiddenID := apid.New(harness.prefix)
			conflictID := apid.New(harness.prefix)
			require.NoError(t, harness.create(ctx, db, firstID, "root.allowed", "first"))
			require.NoError(t, harness.create(ctx, db, secondID, "root.allowed", "shared"))
			require.NoError(t, harness.create(ctx, db, hiddenID, "root.hidden", "shared"))
			require.NoError(t, harness.create(ctx, db, conflictID, "root.allowed", "conflict"))

			require.NoError(t, harness.rename(ctx, db, firstID, "renamed"))
			name, err := harness.getName(ctx, db, firstID)
			require.NoError(t, err)
			require.Equal(t, scommon.ResourceName("renamed"), name)

			err = harness.rename(ctx, db, conflictID, "shared")
			require.ErrorIs(t, err, ErrDuplicate)
			name, getErr := harness.getName(ctx, db, conflictID)
			require.NoError(t, getErr)
			require.Equal(t, scommon.ResourceName("conflict"), name)

			allowed := harness.list(ctx, db, "shared", []string{"root.allowed"}, 10)
			require.NoError(t, allowed.err)
			require.Equal(t, []apid.ID{secondID}, allowed.ids)

			firstPage := harness.list(ctx, db, "shared", []string{"root.**"}, 1)
			require.NoError(t, firstPage.err)
			require.Len(t, firstPage.ids, 1)
			require.NotEmpty(t, firstPage.cursor)

			secondPage := harness.listCursor(ctx, db, firstPage.cursor)
			require.NoError(t, secondPage.err)
			require.Len(t, secondPage.ids, 1)
			require.ElementsMatch(t, []apid.ID{secondID, hiddenID}, append(firstPage.ids, secondPage.ids...))

			broad := harness.list(ctx, db, "shared", []string{"root.**"}, 10)
			require.NoError(t, broad.err)
			require.ElementsMatch(t, []apid.ID{secondID, hiddenID}, broad.ids)
		})
	}
}

func TestNamespaceFinalSegmentExactNameListAndPagination(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	for _, path := range []string{"root.allowed", "root.allowed.shared", "root.hidden", "root.hidden.shared", "root.allowed.other", "root.allowed._shared-"} {
		require.NoError(t, db.EnsureNamespaceByPath(ctx, path))
	}

	allowed := db.ListNamespacesBuilder().ForName("shared").ForNamespaceMatchers([]string{"root.allowed.**"}).FetchPage(ctx)
	require.NoError(t, allowed.Error)
	require.Len(t, allowed.Results, 1)
	require.Equal(t, "root.allowed.shared", allowed.Results[0].Path)

	first := db.ListNamespacesBuilder().ForName("shared").ForNamespaceMatchers([]string{"root.**"}).Limit(1).FetchPage(ctx)
	require.NoError(t, first.Error)
	require.Len(t, first.Results, 1)
	require.NotEmpty(t, first.Cursor)
	executor, err := db.ListNamespacesFromCursor(ctx, first.Cursor)
	require.NoError(t, err)
	second := executor.FetchPage(ctx)
	require.NoError(t, second.Error)
	require.Len(t, second.Results, 1)
	require.ElementsMatch(t, []string{"root.allowed.shared", "root.hidden.shared"}, []string{first.Results[0].Path, second.Results[0].Path})

	escapedWildcard := db.ListNamespacesBuilder().ForName("_shared-").FetchPage(ctx)
	require.NoError(t, escapedWildcard.Error)
	require.Len(t, escapedWildcard.Results, 1)
	require.Equal(t, "root.allowed._shared-", escapedWildcard.Results[0].Path)
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
	migrateDatabaseToVersion(t, service, 14)

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

	migrateDatabaseToVersion(t, service, 15)

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

	migrateDatabaseToVersion(t, service, 16)
}
