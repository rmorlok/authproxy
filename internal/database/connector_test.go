package database

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestConnectors(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		sql := `
INSERT INTO connector_versions 
(id,                                     namespace,      version, state,      type,           encrypted_definition, hash,    created_at,            updated_at,            deleted_at) VALUES 
('6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11', 'root',                     1,       'active',   'gmail',        'encrypted-def',      'hash1', '2023-10-01 00:00:00', '2023-10-01 00:00:00', null),
('6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11', 'root',                     2,       'primary',  'gmail',        'encrypted-def',      'hash2', '2023-10-10 00:00:00', '2023-10-10 00:00:00', null),
('8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64', 'root.child',               1,       'archived', 'gmail',        'encrypted-def',      'hash3', '2023-10-02 00:00:00', '2023-10-02 00:00:00', null),
('8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64', 'root.child',               2,       'primary',  'gmail',        'encrypted-def',      'hash4', '2023-10-11 00:00:00', '2023-10-11 00:00:00', null),
('4a9f3c22-a8d5-423e-af53-e459f1d7c8da', 'root.child2',              1,       'active',   'outlook',      'encrypted-def',      'hash5', '2023-10-03 00:00:00', '2023-10-03 00:00:00', null),
('4a9f3c22-a8d5-423e-af53-e459f1d7c8da', 'root.child2',              2,       'primary',  'outlook',      'encrypted-def',      'hash6', '2023-10-12 00:00:00', '2023-10-12 00:00:00', null),
('c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa', 'root.child.grand',         1,       'archived', 'google_drive', 'encrypted-def',      'hash7', '2023-10-04 00:00:00', '2023-10-04 00:00:00', null),
('c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa', 'root.child.grand',         2,       'active',   'google_drive', 'encrypted-def',      'hash8', '2023-10-13 00:00:00', '2023-10-13 00:00:00', null),
('c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa', 'root.child.grand',         3,       'primary',  'google_drive', 'encrypted-def',      'hash9', '2023-10-14 00:00:00', '2023-10-14 00:00:00', null);
`
		_, err := rawDb.Exec(sql)
		require.NoError(t, err)

		pr := db.ListConnectorsBuilder().
			ForType("gmail").
			OrderBy(ConnectorOrderByCreatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 2)
		require.Equal(t, uuid.MustParse("8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64"), pr.Results[0].Id)
		require.Equal(t, uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), pr.Results[1].Id)

		pr = db.ListConnectorsBuilder().
			ForNamespaceMatcher("root.child.**").
			OrderBy(ConnectorOrderByCreatedAt, pagination.OrderByAsc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 2)
		require.Equal(t, uuid.MustParse("8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64"), pr.Results[0].Id)
		require.Equal(t, uuid.MustParse("c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa"), pr.Results[1].Id)
	})

	t.Run("ForNamespaceMatchers", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		sql := `
INSERT INTO connector_versions
(id,                                     namespace,         version, state,      type,           encrypted_definition, hash,    created_at,            updated_at,            deleted_at) VALUES
('6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11', 'root.prod',       1,       'primary',  'gmail',        'encrypted-def',      'hash1', '2023-10-01 00:00:00', '2023-10-01 00:00:00', null),
('8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64', 'root.staging',    1,       'primary',  'gmail',        'encrypted-def',      'hash3', '2023-10-02 00:00:00', '2023-10-02 00:00:00', null),
('4a9f3c22-a8d5-423e-af53-e459f1d7c8da', 'root.dev',        1,       'primary',  'outlook',      'encrypted-def',      'hash5', '2023-10-03 00:00:00', '2023-10-03 00:00:00', null),
('c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa', 'root.prod.tenant1',1,      'primary',  'google_drive', 'encrypted-def',      'hash7', '2023-10-04 00:00:00', '2023-10-04 00:00:00', null);
`
		_, err := rawDb.Exec(sql)
		require.NoError(t, err)

		t.Run("empty matchers returns all", func(t *testing.T) {
			pr := db.ListConnectorsBuilder().
				ForNamespaceMatchers([]string{}).
				OrderBy(ConnectorOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 4)
		})

		t.Run("single exact matcher", func(t *testing.T) {
			pr := db.ListConnectorsBuilder().
				ForNamespaceMatchers([]string{"root.prod"}).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 1)
			require.Equal(t, uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), pr.Results[0].Id)
		})

		t.Run("single wildcard matcher", func(t *testing.T) {
			pr := db.ListConnectorsBuilder().
				ForNamespaceMatchers([]string{"root.prod.**"}).
				OrderBy(ConnectorOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 2)
			require.Equal(t, uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), pr.Results[0].Id)
			require.Equal(t, uuid.MustParse("c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa"), pr.Results[1].Id)
		})

		t.Run("multiple exact matchers (OR logic)", func(t *testing.T) {
			pr := db.ListConnectorsBuilder().
				ForNamespaceMatchers([]string{"root.prod", "root.staging"}).
				OrderBy(ConnectorOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 2)
			require.Equal(t, uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), pr.Results[0].Id)
			require.Equal(t, uuid.MustParse("8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64"), pr.Results[1].Id)
		})

		t.Run("multiple wildcard matchers (OR logic)", func(t *testing.T) {
			pr := db.ListConnectorsBuilder().
				ForNamespaceMatchers([]string{"root.prod.**", "root.staging.**"}).
				OrderBy(ConnectorOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 3)
		})

		t.Run("no matching namespaces", func(t *testing.T) {
			pr := db.ListConnectorsBuilder().
				ForNamespaceMatchers([]string{"root.nonexistent"}).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 0)
		})
	})
}
