package database

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
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
('cxr_testgmail0000001', 'root',                     1,       'active',   'gmail',        'encrypted-def',      'hash1', '2023-10-01 00:00:00', '2023-10-01 00:00:00', null),
('cxr_testgmail0000001', 'root',                     2,       'primary',  'gmail',        'encrypted-def',      'hash2', '2023-10-10 00:00:00', '2023-10-10 00:00:00', null),
('cxr_testgmail0000002', 'root.child',               1,       'archived', 'gmail',        'encrypted-def',      'hash3', '2023-10-02 00:00:00', '2023-10-02 00:00:00', null),
('cxr_testgmail0000002', 'root.child',               2,       'primary',  'gmail',        'encrypted-def',      'hash4', '2023-10-11 00:00:00', '2023-10-11 00:00:00', null),
('cxr_testslack0000001', 'root.child2',              1,       'active',   'outlook',      'encrypted-def',      'hash5', '2023-10-03 00:00:00', '2023-10-03 00:00:00', null),
('cxr_testslack0000001', 'root.child2',              2,       'primary',  'outlook',      'encrypted-def',      'hash6', '2023-10-12 00:00:00', '2023-10-12 00:00:00', null),
('cxr_testgmail0000003', 'root.child.grand',         1,       'archived', 'google_drive', 'encrypted-def',      'hash7', '2023-10-04 00:00:00', '2023-10-04 00:00:00', null),
('cxr_testgmail0000003', 'root.child.grand',         2,       'active',   'google_drive', 'encrypted-def',      'hash8', '2023-10-13 00:00:00', '2023-10-13 00:00:00', null),
('cxr_testgmail0000003', 'root.child.grand',         3,       'primary',  'google_drive', 'encrypted-def',      'hash9', '2023-10-14 00:00:00', '2023-10-14 00:00:00', null);
`
		_, err := rawDb.Exec(sql)
		require.NoError(t, err)

		pr := db.ListConnectorsBuilder().
			ForType("gmail").
			OrderBy(ConnectorOrderByCreatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 2)
		require.Equal(t, apid.MustParse("cxr_testgmail0000002"), pr.Results[0].Id)
		require.Equal(t, apid.MustParse("cxr_testgmail0000001"), pr.Results[1].Id)

		pr = db.ListConnectorsBuilder().
			ForNamespaceMatcher("root.child.**").
			OrderBy(ConnectorOrderByCreatedAt, pagination.OrderByAsc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 2)
		require.Equal(t, apid.MustParse("cxr_testgmail0000002"), pr.Results[0].Id)
		require.Equal(t, apid.MustParse("cxr_testgmail0000003"), pr.Results[1].Id)
	})

	t.Run("ForNamespaceMatchers", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		sql := `
INSERT INTO connector_versions
(id,                                     namespace,         version, state,      type,           encrypted_definition, hash,    created_at,            updated_at,            deleted_at) VALUES
('cxr_testgmail0000001', 'root.prod',       1,       'primary',  'gmail',        'encrypted-def',      'hash1', '2023-10-01 00:00:00', '2023-10-01 00:00:00', null),
('cxr_testgmail0000002', 'root.staging',    1,       'primary',  'gmail',        'encrypted-def',      'hash3', '2023-10-02 00:00:00', '2023-10-02 00:00:00', null),
('cxr_testslack0000001', 'root.dev',        1,       'primary',  'outlook',      'encrypted-def',      'hash5', '2023-10-03 00:00:00', '2023-10-03 00:00:00', null),
('cxr_testgmail0000003', 'root.prod.tenant1',1,      'primary',  'google_drive', 'encrypted-def',      'hash7', '2023-10-04 00:00:00', '2023-10-04 00:00:00', null);
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
			require.Equal(t, apid.MustParse("cxr_testgmail0000001"), pr.Results[0].Id)
		})

		t.Run("single wildcard matcher", func(t *testing.T) {
			pr := db.ListConnectorsBuilder().
				ForNamespaceMatchers([]string{"root.prod.**"}).
				OrderBy(ConnectorOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 2)
			require.Equal(t, apid.MustParse("cxr_testgmail0000001"), pr.Results[0].Id)
			require.Equal(t, apid.MustParse("cxr_testgmail0000003"), pr.Results[1].Id)
		})

		t.Run("multiple exact matchers (OR logic)", func(t *testing.T) {
			pr := db.ListConnectorsBuilder().
				ForNamespaceMatchers([]string{"root.prod", "root.staging"}).
				OrderBy(ConnectorOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 2)
			require.Equal(t, apid.MustParse("cxr_testgmail0000001"), pr.Results[0].Id)
			require.Equal(t, apid.MustParse("cxr_testgmail0000002"), pr.Results[1].Id)
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
