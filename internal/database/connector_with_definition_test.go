package database

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
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
INSERT INTO connectors
(id,                         namespace,          name,                       labels,                    created_at,            updated_at,            deleted_at) VALUES
('cxr_testgmail0000001',     'root',             'cxr_testgmail0000001',     '{"type":"gmail"}',        '2023-10-01 00:00:00', '2023-10-10 00:00:00', null),
('cxr_testgmail0000002',     'root.child',       'cxr_testgmail0000002',     '{"type":"gmail"}',        '2023-10-02 00:00:00', '2023-10-11 00:00:00', null),
('cxr_testslack0000001',     'root.child2',      'cxr_testslack0000001',     '{"type":"outlook"}',      '2023-10-03 00:00:00', '2023-10-12 00:00:00', null),
('cxr_testgmail0000003',     'root.child.grand', 'cxr_testgmail0000003',     '{"type":"google_drive"}', '2023-10-04 00:00:00', '2023-10-14 00:00:00', null);

INSERT INTO connector_definition_versions
(id,                         connector_id,                version, state,      encrypted_definition,                       created_at,       updated_at) VALUES
('cvd_testgmail0000011',     'cxr_testgmail0000001',      1,       'active',   '{"id":"dek_test","d":"encrypted-def"}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('cvd_testgmail0000012',     'cxr_testgmail0000001',      2,       'primary',  '{"id":"dek_test","d":"encrypted-def"}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('cvd_testgmail0000021',     'cxr_testgmail0000002',      1,       'archived', '{"id":"dek_test","d":"encrypted-def"}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('cvd_testgmail0000022',     'cxr_testgmail0000002',      2,       'primary',  '{"id":"dek_test","d":"encrypted-def"}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('cvd_testslack0000011',     'cxr_testslack0000001',      1,       'active',   '{"id":"dek_test","d":"encrypted-def"}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('cvd_testslack0000012',     'cxr_testslack0000001',      2,       'primary',  '{"id":"dek_test","d":"encrypted-def"}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('cvd_testgmail0000031',     'cxr_testgmail0000003',      1,       'archived', '{"id":"dek_test","d":"encrypted-def"}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('cvd_testgmail0000032',     'cxr_testgmail0000003',      2,       'active',   '{"id":"dek_test","d":"encrypted-def"}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('cvd_testgmail0000033',     'cxr_testgmail0000003',      3,       'primary',  '{"id":"dek_test","d":"encrypted-def"}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
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
INSERT INTO connectors
(id,                         namespace,           name,                       labels,                    created_at,            updated_at,            deleted_at) VALUES
('cxr_testgmail0000001',     'root.prod',         'cxr_testgmail0000001',     '{"type":"gmail"}',        '2023-10-01 00:00:00', '2023-10-01 00:00:00', null),
('cxr_testgmail0000002',     'root.staging',      'cxr_testgmail0000002',     '{"type":"gmail"}',        '2023-10-02 00:00:00', '2023-10-02 00:00:00', null),
('cxr_testslack0000001',     'root.dev',          'cxr_testslack0000001',     '{"type":"outlook"}',      '2023-10-03 00:00:00', '2023-10-03 00:00:00', null),
('cxr_testgmail0000003',     'root.prod.tenant1', 'cxr_testgmail0000003',     '{"type":"google_drive"}', '2023-10-04 00:00:00', '2023-10-04 00:00:00', null);

INSERT INTO connector_definition_versions
(id,                         connector_id,                version, state,     encrypted_definition,                       created_at,       updated_at) VALUES
('cvd_testgmail0000011',     'cxr_testgmail0000001',      1,       'primary', '{"id":"dek_test","d":"encrypted-def"}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('cvd_testgmail0000021',     'cxr_testgmail0000002',      1,       'primary', '{"id":"dek_test","d":"encrypted-def"}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('cvd_testslack0000011',     'cxr_testslack0000001',      1,       'primary', '{"id":"dek_test","d":"encrypted-def"}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('cvd_testgmail0000031',     'cxr_testgmail0000003',      1,       'primary', '{"id":"dek_test","d":"encrypted-def"}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
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
