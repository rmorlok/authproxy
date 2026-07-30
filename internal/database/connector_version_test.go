package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/sqlh"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestConnectorVersions(t *testing.T) {
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
(id,                         connector_id,                version, state,      encrypted_definition) VALUES
('cvd_testgmail0000011',     'cxr_testgmail0000001',      1,       'active',   '{"id":"dek_test","d":"encrypted-def"}'),
('cvd_testgmail0000012',     'cxr_testgmail0000001',      2,       'primary',  '{"id":"dek_test","d":"encrypted-def"}'),
('cvd_testgmail0000021',     'cxr_testgmail0000002',      1,       'archived', '{"id":"dek_test","d":"encrypted-def"}'),
('cvd_testgmail0000022',     'cxr_testgmail0000002',      2,       'primary',  '{"id":"dek_test","d":"encrypted-def"}'),
('cvd_testslack0000011',     'cxr_testslack0000001',      1,       'active',   '{"id":"dek_test","d":"encrypted-def"}'),
('cvd_testslack0000012',     'cxr_testslack0000001',      2,       'primary',  '{"id":"dek_test","d":"encrypted-def"}'),
('cvd_testgmail0000031',     'cxr_testgmail0000003',      1,       'archived', '{"id":"dek_test","d":"encrypted-def"}'),
('cvd_testgmail0000032',     'cxr_testgmail0000003',      2,       'active',   '{"id":"dek_test","d":"encrypted-def"}'),
('cvd_testgmail0000033',     'cxr_testgmail0000003',      3,       'primary',  '{"id":"dek_test","d":"encrypted-def"}');
`
		_, err := rawDb.Exec(sql)
		require.NoError(t, err)

		v, err := db.GetConnectorVersion(ctx, apid.MustParse("cxr_testgmail0000001"), 1)
		require.NoError(t, err)
		require.Equal(t, "gmail", v.Labels["type"])
		require.Equal(t, ConnectorVersionStateActive, v.State)

		results, err := db.GetConnectorVersions(ctx, []ConnectorVersionId{
			{apid.MustParse("cxr_testgmail0000001"), 1},
		})
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Equal(t, "gmail", results[ConnectorVersionId{apid.MustParse("cxr_testgmail0000001"), 1}].Labels["type"])
		require.Equal(t, ConnectorVersionStateActive, results[ConnectorVersionId{apid.MustParse("cxr_testgmail0000001"), 1}].State)

		results, err = db.GetConnectorVersions(ctx, []ConnectorVersionId{
			{apid.MustParse("cxr_testgmail0000001"), 1},
			{apid.MustParse("cxr_testgmail0000002"), 2},
		})
		require.NoError(t, err)
		require.Len(t, results, 2)
		require.Equal(t, "gmail", results[ConnectorVersionId{apid.MustParse("cxr_testgmail0000001"), 1}].Labels["type"])
		require.Equal(t, ConnectorVersionStateActive, results[ConnectorVersionId{apid.MustParse("cxr_testgmail0000001"), 1}].State)
		require.Equal(t, "gmail", results[ConnectorVersionId{apid.MustParse("cxr_testgmail0000002"), 2}].Labels["type"])
		require.Equal(t, ConnectorVersionStatePrimary, results[ConnectorVersionId{apid.MustParse("cxr_testgmail0000002"), 2}].State)

		// Version doesn't exist
		v, err = db.GetConnectorVersion(ctx, apid.MustParse("cxr_testgmail0000001"), 99)
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, v)

		// UUID doesn't exist
		v, err = db.GetConnectorVersion(ctx, apid.MustParse("cxr_testnotfound0001"), 1)
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, v)

		v, err = db.GetConnectorVersionForState(ctx, apid.MustParse("cxr_testslack0000001"), ConnectorVersionStatePrimary)
		require.NoError(t, err)
		require.Equal(t, "outlook", v.Labels["type"])
		require.Equal(t, ConnectorVersionStatePrimary, v.State)

		v, err = db.GetConnectorVersionForState(ctx, apid.MustParse("cxr_testslack0000001"), ConnectorVersionStateArchived)
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, v)

		pr := db.ListConnectorVersionsBuilder().
			ForLabelSelector("type=gmail").
			OrderBy(ConnectorVersionOrderByCreatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 4)
		require.Equal(t, pr.Results[0].Id, apid.MustParse("cxr_testgmail0000002"))
		require.Equal(t, uint64(2), pr.Results[0].Version)
		require.Equal(t, pr.Results[1].Id, apid.MustParse("cxr_testgmail0000002"))
		require.Equal(t, uint64(1), pr.Results[1].Version)
		require.Equal(t, pr.Results[2].Id, apid.MustParse("cxr_testgmail0000001"))
		require.Equal(t, uint64(2), pr.Results[2].Version)
		require.Equal(t, pr.Results[3].Id, apid.MustParse("cxr_testgmail0000001"))
		require.Equal(t, uint64(1), pr.Results[3].Version)

		pr = db.ListConnectorVersionsBuilder().
			ForNamespaceMatcher("root.child.**").
			OrderBy(ConnectorVersionOrderByCreatedAt, pagination.OrderByAsc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 5)
		require.Equal(t, apid.MustParse("cxr_testgmail0000002"), pr.Results[0].Id)
		require.Equal(t, uint64(1), pr.Results[0].Version)
		require.Equal(t, apid.MustParse("cxr_testgmail0000002"), pr.Results[1].Id)
		require.Equal(t, uint64(2), pr.Results[1].Version)
		require.Equal(t, apid.MustParse("cxr_testgmail0000003"), pr.Results[2].Id)
		require.Equal(t, uint64(1), pr.Results[2].Version)
		require.Equal(t, apid.MustParse("cxr_testgmail0000003"), pr.Results[3].Id)
		require.Equal(t, uint64(2), pr.Results[3].Version)
		require.Equal(t, apid.MustParse("cxr_testgmail0000003"), pr.Results[4].Id)
		require.Equal(t, uint64(3), pr.Results[4].Version)
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
(id,                         connector_id,                version, state,     encrypted_definition) VALUES
('cvd_testgmail0000011',     'cxr_testgmail0000001',      1,       'primary', '{"id":"dek_test","d":"encrypted-def"}'),
('cvd_testgmail0000021',     'cxr_testgmail0000002',      1,       'primary', '{"id":"dek_test","d":"encrypted-def"}'),
('cvd_testslack0000011',     'cxr_testslack0000001',      1,       'primary', '{"id":"dek_test","d":"encrypted-def"}'),
('cvd_testgmail0000031',     'cxr_testgmail0000003',      1,       'primary', '{"id":"dek_test","d":"encrypted-def"}');
`
		_, err := rawDb.Exec(sql)
		require.NoError(t, err)

		t.Run("empty matchers returns all", func(t *testing.T) {
			pr := db.ListConnectorVersionsBuilder().
				ForNamespaceMatchers([]string{}).
				OrderBy(ConnectorVersionOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 4)
		})

		t.Run("single exact matcher", func(t *testing.T) {
			pr := db.ListConnectorVersionsBuilder().
				ForNamespaceMatchers([]string{"root.prod"}).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 1)
			require.Equal(t, apid.MustParse("cxr_testgmail0000001"), pr.Results[0].Id)
		})

		t.Run("single wildcard matcher", func(t *testing.T) {
			pr := db.ListConnectorVersionsBuilder().
				ForNamespaceMatchers([]string{"root.prod.**"}).
				OrderBy(ConnectorVersionOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 2)
			require.Equal(t, apid.MustParse("cxr_testgmail0000001"), pr.Results[0].Id)
			require.Equal(t, apid.MustParse("cxr_testgmail0000003"), pr.Results[1].Id)
		})

		t.Run("multiple exact matchers (OR logic)", func(t *testing.T) {
			pr := db.ListConnectorVersionsBuilder().
				ForNamespaceMatchers([]string{"root.prod", "root.staging"}).
				OrderBy(ConnectorVersionOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 2)
			require.Equal(t, apid.MustParse("cxr_testgmail0000001"), pr.Results[0].Id)
			require.Equal(t, apid.MustParse("cxr_testgmail0000002"), pr.Results[1].Id)
		})

		t.Run("multiple wildcard matchers (OR logic)", func(t *testing.T) {
			pr := db.ListConnectorVersionsBuilder().
				ForNamespaceMatchers([]string{"root.prod.**", "root.staging.**"}).
				OrderBy(ConnectorVersionOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 3)
		})

		t.Run("no matching namespaces", func(t *testing.T) {
			pr := db.ListConnectorVersionsBuilder().
				ForNamespaceMatchers([]string{"root.nonexistent"}).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 0)
		})
	})
	t.Run("validation rejects wrong prefix on id", func(t *testing.T) {
		cv := &ConnectorVersion{
			Id:                  apid.New(apid.PrefixActor), // wrong prefix
			Version:             1,
			Namespace:           "root",
			State:               ConnectorVersionStateDraft,
			Hash:                "hash",
			EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "def"},
		}
		require.Error(t, cv.Validate())
	})
	t.Run("UpsertConnectorVersion", func(t *testing.T) {
		t.Run("creates a new connector version", func(t *testing.T) {
			// Setup
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create a new connector version
			connectorID := apid.New(apid.PrefixConnectorVersion)
			cv := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           "root.some-namespace",
				State:               ConnectorVersionStateDraft,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"},
			}

			// Test
			err := db.UpsertConnectorVersion(ctx, cv)
			require.NoError(t, err)

			// Verify
			savedCV, err := db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV)
			assert.Equal(t, connectorID, savedCV.Id)
			assert.Equal(t, uint64(1), savedCV.Version)
			assert.Equal(t, ConnectorVersionStateDraft, savedCV.State)
			assert.Equal(t, "test_connector", savedCV.Labels["type"])
			assert.Equal(t, "root.some-namespace", savedCV.Namespace)
			require.True(t, savedCV.DefinitionVersionId.HasPrefix(apid.PrefixConnectorDefinitionVersion))
			assert.Equal(t, encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"}, savedCV.EncryptedDefinition)
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_definition_versions"))
		})

		t.Run("refuses to create active and archived versions", func(t *testing.T) {
			// Setup
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create a new connector version
			connectorID := apid.New(apid.PrefixConnectorVersion)
			cv := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           "root.some-namespace",
				State:               ConnectorVersionStateActive,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"},
			}

			// Test
			err := db.UpsertConnectorVersion(ctx, cv)
			require.Error(t, err) // Cannot create active directly (must be primary)
			require.Equal(t, 0, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_definition_versions"))

			cv.State = ConnectorVersionStateArchived
			err = db.UpsertConnectorVersion(ctx, cv)
			require.Error(t, err) // Cannot create archived directly (must be primary)
			require.Equal(t, 0, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_definition_versions"))
		})

		t.Run("updates an existing draft version", func(t *testing.T) {
			// Setup
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create a new connector version
			connectorID := apid.New(apid.PrefixConnectorVersion)
			cv := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStateDraft,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"},
			}

			// Test
			err := db.UpsertConnectorVersion(ctx, cv)
			require.NoError(t, err)

			// Verify
			savedCV, err := db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV)
			assert.Equal(t, connectorID, savedCV.Id)
			assert.Equal(t, uint64(1), savedCV.Version)
			assert.Equal(t, ConnectorVersionStateDraft, savedCV.State)
			assert.Equal(t, "test_connector", savedCV.Labels["type"])
			assert.Equal(t, encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"}, savedCV.EncryptedDefinition)
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_definition_versions"))
		})

		t.Run("refuses to change namespace for draft version", func(t *testing.T) {
			// Setup
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create a new connector version
			connectorID := apid.New(apid.PrefixConnectorVersion)
			cv := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStateDraft,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"},
			}

			// Test
			err := db.UpsertConnectorVersion(ctx, cv)
			require.NoError(t, err)

			// Verify
			savedCV, err := db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			assert.Equal(t, sconfig.RootNamespace, savedCV.Namespace)

			// Try to change namespace
			cv.Namespace = "root.some-other-namespace"
			err = db.UpsertConnectorVersion(ctx, cv)
			require.Error(t, err)

			// Verify unchanged
			savedCV, err = db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			assert.Equal(t, sconfig.RootNamespace, savedCV.Namespace)
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_definition_versions"))
		})

		t.Run("creates multiple versions of the same connector", func(t *testing.T) {
			// Setup
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create connector Id
			connectorID := apid.New(apid.PrefixConnectorVersion)

			// Create version 1
			cv1 := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStateDraft,
				Labels:              Labels{"type": "test_connector_v1"},
				Annotations:         Annotations{"owner": "v1"},
				Hash:                "test_hash_v1",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition_v1"},
			}

			err := db.UpsertConnectorVersion(ctx, cv1)
			require.NoError(t, err)

			// Create version 2
			cv2 := &ConnectorVersion{
				Id:                  connectorID,
				Version:             2,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStateDraft,
				Labels:              Labels{"type": "test_connector_v2"},
				Annotations:         Annotations{"owner": "v2"},
				Hash:                "test_hash_v2",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition_v2"},
			}

			err = db.UpsertConnectorVersion(ctx, cv2)
			require.NoError(t, err)

			// Verify version 1
			savedCV1, err := db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV1)
			assert.Equal(t, uint64(1), savedCV1.Version)
			assert.Equal(t, "test_connector_v2", savedCV1.Labels["type"])
			assert.Equal(t, "v2", savedCV1.Annotations["owner"])

			// Verify version 2
			savedCV2, err := db.GetConnectorVersion(ctx, connectorID, 2)
			require.NoError(t, err)
			require.NotNil(t, savedCV2)
			assert.Equal(t, uint64(2), savedCV2.Version)
			assert.Equal(t, savedCV1.Labels, savedCV2.Labels)
			assert.Equal(t, savedCV1.Annotations, savedCV2.Annotations)
			require.NotEqual(t, savedCV1.DefinitionVersionId, savedCV2.DefinitionVersionId)
			require.True(t, savedCV1.DefinitionVersionId.HasPrefix(apid.PrefixConnectorDefinitionVersion))
			require.True(t, savedCV2.DefinitionVersionId.HasPrefix(apid.PrefixConnectorDefinitionVersion))
			require.Equal(t, 2, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_definition_versions"))
		})

		t.Run("creates a primary connector version", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create connector Id
			connectorID := apid.New(apid.PrefixConnectorVersion)

			// Create a primary connector version
			cv := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStatePrimary,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"},
			}

			err := db.UpsertConnectorVersion(ctx, cv)
			require.NoError(t, err)

			// Verify
			savedCV, err := db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV)
			assert.Equal(t, ConnectorVersionStatePrimary, savedCV.State)
		})

		t.Run("creates multiple primary versions and updates previous primary to active", func(t *testing.T) {
			// This test simulates what UpsertConnectorVersion does when setting a new primary version

			// Setup
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create connector Id
			connectorID := apid.New(apid.PrefixConnectorVersion)

			// Create version 1 as primary
			cv1 := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStatePrimary,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash_v1",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition_v1"},
			}

			err := db.UpsertConnectorVersion(ctx, cv1)
			require.NoError(t, err)

			// Verify version 1 is primary
			savedCV1, err := db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV1)
			assert.Equal(t, ConnectorVersionStatePrimary, savedCV1.State)

			// Create version 2 as primary
			cv2 := &ConnectorVersion{
				Id:                  connectorID,
				Version:             2,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStatePrimary,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash_v2",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition_v2"},
			}

			err = db.UpsertConnectorVersion(ctx, cv2)
			require.NoError(t, err)

			// Verify version 2 is primary
			savedCV2, err := db.GetConnectorVersion(ctx, connectorID, 2)
			require.NoError(t, err)
			require.NotNil(t, savedCV2)
			assert.Equal(t, ConnectorVersionStatePrimary, savedCV2.State)

			// Verify version 1 is now active
			savedCV1, err = db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV1)
			assert.Equal(t, ConnectorVersionStateActive, savedCV1.State)
		})
		t.Run("upsert does not resurrect a soft-deleted connector", func(t *testing.T) {
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnectorVersion)
			cv := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStateDraft,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"},
			}

			err := db.UpsertConnectorVersion(ctx, cv)
			require.NoError(t, err)

			require.NoError(t, db.DeleteConnector(ctx, connectorID))

			cv2 := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStateDraft,
				Labels:              Labels{"type": "updated_connector"},
				Hash:                "new_hash",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "new_encrypted_definition"},
			}

			err = db.UpsertConnectorVersion(ctx, cv2)
			require.Error(t, err)

			var encrypted encfield.EncryptedField
			err = rawDb.QueryRow(fmt.Sprintf(
				"SELECT encrypted_definition FROM connector_definition_versions WHERE connector_id = '%s' AND version = 1",
				connectorID,
			)).Scan(&encrypted)
			require.NoError(t, err)
			require.Equal(t, cv.EncryptedDefinition, encrypted)
		})

		t.Run("refuses to skip version numbers", func(t *testing.T) {
			// This test simulates what UpsertConnectorVersion does when setting a new primary version

			// Setup
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create connector Id
			connectorID := apid.New(apid.PrefixConnectorVersion)

			// Create version 1 as primary
			cv1 := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStatePrimary,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash_v1",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition_v1"},
			}

			err := db.UpsertConnectorVersion(ctx, cv1)
			require.NoError(t, err)

			// Verify version 1 is primary
			savedCV1, err := db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV1)
			assert.Equal(t, ConnectorVersionStatePrimary, savedCV1.State)

			// Create version 2 as primary
			cv2 := &ConnectorVersion{
				Id:                  connectorID,
				Version:             3,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStatePrimary,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash_v2",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition_v2"},
			}

			err = db.UpsertConnectorVersion(ctx, cv2)
			require.Error(t, err)

			// Verify version 1 is primary
			savedCV2, err := db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV2)
			assert.Equal(t, ConnectorVersionStatePrimary, savedCV2.State)

			// Verify version wasn't created
			savedCV1, err = db.GetConnectorVersion(ctx, connectorID, 3)
			require.ErrorIs(t, err, ErrNotFound)
			require.Nil(t, savedCV1)
		})
	})

	t.Run("SetConnectorVersionState", func(t *testing.T) {
		t.Run("sets state successfully", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnectorVersion)
			cv := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStateDraft,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"},
			}

			err := db.UpsertConnectorVersion(ctx, cv)
			require.NoError(t, err)

			err = db.SetConnectorVersionState(ctx, connectorID, 1, ConnectorVersionStatePrimary)
			require.NoError(t, err)

			saved, err := db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			assert.Equal(t, ConnectorVersionStatePrimary, saved.State)
		})

		t.Run("returns not found for nonexistent version", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.SetConnectorVersionState(ctx, apid.New(apid.PrefixConnectorVersion), 1, ConnectorVersionStatePrimary)
			require.ErrorIs(t, err, ErrNotFound)
		})

		t.Run("rejects invalid state", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.SetConnectorVersionState(ctx, apid.New(apid.PrefixConnectorVersion), 1, ConnectorVersionState("invalid"))
			require.Error(t, err)
		})

		t.Run("demotes existing primary to active when forcing new primary", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnectorVersion)

			// Create v1 as primary
			err := db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: connectorID, Version: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStatePrimary, Labels: Labels{"type": "t"},
				Hash: "h1", EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e1"},
			})
			require.NoError(t, err)

			// Create v2 as draft
			err = db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: connectorID, Version: 2, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStateDraft, Labels: Labels{"type": "t"},
				Hash: "h2", EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e2"},
			})
			require.NoError(t, err)

			// Force v2 to primary
			err = db.SetConnectorVersionState(ctx, connectorID, 2, ConnectorVersionStatePrimary)
			require.NoError(t, err)

			// v2 should be primary
			v2, err := db.GetConnectorVersion(ctx, connectorID, 2)
			require.NoError(t, err)
			assert.Equal(t, ConnectorVersionStatePrimary, v2.State)

			// v1 should have been demoted to active
			v1, err := db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			assert.Equal(t, ConnectorVersionStateActive, v1.State)
		})

		t.Run("returns not found for a version of a soft-deleted connector", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnectorVersion)
			cv := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStateDraft,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"},
			}

			err := db.UpsertConnectorVersion(ctx, cv)
			require.NoError(t, err)

			require.NoError(t, db.DeleteConnector(ctx, connectorID))

			err = db.SetConnectorVersionState(ctx, connectorID, 1, ConnectorVersionStatePrimary)
			require.ErrorIs(t, err, ErrNotFound)
		})

		t.Run("archives existing draft when forcing new draft", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnectorVersion)

			// Create v1 as primary
			err := db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: connectorID, Version: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStatePrimary, Labels: Labels{"type": "t"},
				Hash: "h1", EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e1"},
			})
			require.NoError(t, err)

			// Create v2 as draft
			err = db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: connectorID, Version: 2, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStateDraft, Labels: Labels{"type": "t"},
				Hash: "h2", EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e2"},
			})
			require.NoError(t, err)

			// Force v1 to draft
			err = db.SetConnectorVersionState(ctx, connectorID, 1, ConnectorVersionStateDraft)
			require.NoError(t, err)

			// v1 should be draft
			v1, err := db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			assert.Equal(t, ConnectorVersionStateDraft, v1.State)

			// v2 should have been archived
			v2, err := db.GetConnectorVersion(ctx, connectorID, 2)
			require.NoError(t, err)
			assert.Equal(t, ConnectorVersionStateArchived, v2.State)
		})
	})

	t.Run("DeleteConnector", func(t *testing.T) {
		t.Run("soft-deletes the connector while retaining definition history", func(t *testing.T) {
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			defer rawDb.Close()
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnectorVersion)

			err := db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: connectorID, Version: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStatePrimary, Labels: Labels{"type": "t"},
				Hash: "h1", EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e1"},
			})
			require.NoError(t, err)

			err = db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: connectorID, Version: 2, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStateDraft, Labels: Labels{"type": "t"},
				Hash: "h2", EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e2"},
			})
			require.NoError(t, err)

			require.Equal(t, 2, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_definition_versions"))
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connectors WHERE deleted_at IS NULL"))

			err = db.DeleteConnector(ctx, connectorID)
			require.NoError(t, err)

			require.Equal(t, 2, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_definition_versions"))
			require.Equal(t, 0, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connectors WHERE deleted_at IS NULL"))
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connectors WHERE deleted_at IS NOT NULL"))

			_, err = db.GetConnectorVersion(ctx, connectorID, 1)
			require.ErrorIs(t, err, ErrNotFound)
			_, err = db.GetConnectorVersion(ctx, connectorID, 2)
			require.ErrorIs(t, err, ErrNotFound)
		})

		t.Run("returns ErrNotFound when no live versions exist", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.DeleteConnector(ctx, apid.New(apid.PrefixConnectorVersion))
			require.ErrorIs(t, err, ErrNotFound)
		})

		t.Run("returns ErrNotFound when the connector is already soft-deleted", func(t *testing.T) {
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			defer rawDb.Close()
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnectorVersion)
			err := db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: connectorID, Version: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStatePrimary, Labels: Labels{"type": "t"},
				Hash: "h1", EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e1"},
			})
			require.NoError(t, err)

			err = db.DeleteConnector(ctx, connectorID)
			require.NoError(t, err)

			err = db.DeleteConnector(ctx, connectorID)
			require.ErrorIs(t, err, ErrNotFound)
		})

		t.Run("does not affect other connectors", func(t *testing.T) {
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			defer rawDb.Close()
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			targetID := apid.New(apid.PrefixConnectorVersion)
			survivorID := apid.New(apid.PrefixConnectorVersion)

			err := db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: targetID, Version: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStatePrimary, Labels: Labels{"type": "t1"},
				Hash: "h1", EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e1"},
			})
			require.NoError(t, err)

			err = db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: survivorID, Version: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStatePrimary, Labels: Labels{"type": "t2"},
				Hash: "h2", EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e2"},
			})
			require.NoError(t, err)

			err = db.DeleteConnector(ctx, targetID)
			require.NoError(t, err)

			survivor, err := db.GetConnectorVersion(ctx, survivorID, 1)
			require.NoError(t, err)
			require.Equal(t, survivorID, survivor.Id)

			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connectors WHERE deleted_at IS NULL"))
			require.Equal(t, 2, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_definition_versions"))
		})

		t.Run("rejects nil id", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			ctx := apctx.NewBuilderBackground().Build()
			require.Error(t, db.DeleteConnector(ctx, apid.Nil))
		})
	})

	t.Run("UpsertConnectorVersion apxy label semantics", func(t *testing.T) {
		t.Run("caller-supplied apxy labels persist on insert", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnectorVersion)
			err := db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: connectorID, Version: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStatePrimary,
				Labels: Labels{
					"type":            "t",
					"apxy/cxr/source": "config",
				},
				Hash:                "h1",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e1"},
			})
			require.NoError(t, err)

			saved, err := db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			require.Equal(t, "config", saved.Labels["apxy/cxr/source"])
			require.Equal(t, "t", saved.Labels["type"])
			// self-implicit labels still injected
			require.Equal(t, string(connectorID), saved.Labels["apxy/cxr/-/id"])
		})

		t.Run("caller-supplied apxy labels override stored apxy on update", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnectorVersion)

			// Insert as draft with one apxy value.
			err := db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: connectorID, Version: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStateDraft,
				Labels: Labels{
					"type":            "t",
					"apxy/cxr/source": "api",
				},
				Hash:                "h1",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e1"},
			})
			require.NoError(t, err)

			// Update the draft with a different apxy value for the same key.
			err = db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: connectorID, Version: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStateDraft,
				Labels: Labels{
					"type":            "t",
					"apxy/cxr/source": "config",
				},
				Hash:                "h2",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e2"},
			})
			require.NoError(t, err)

			saved, err := db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			require.Equal(t, "config", saved.Labels["apxy/cxr/source"], "caller's apxy value must override stored")
			// self-implicit labels stay intact
			require.Equal(t, string(connectorID), saved.Labels["apxy/cxr/-/id"])
		})

		t.Run("stored apxy labels not in caller are preserved on update", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnectorVersion)

			// Insert as draft with an apxy value the caller will not pass on update.
			err := db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: connectorID, Version: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStateDraft,
				Labels: Labels{
					"type":            "t",
					"apxy/cxr/source": "config",
				},
				Hash:                "h1",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e1"},
			})
			require.NoError(t, err)

			// Update with only user labels — stored apxy/cxr/source must survive.
			err = db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: connectorID, Version: 1, Namespace: sconfig.RootNamespace,
				State:               ConnectorVersionStateDraft,
				Labels:              Labels{"type": "t-changed"},
				Hash:                "h2",
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e2"},
			})
			require.NoError(t, err)

			saved, err := db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			require.Equal(t, "config", saved.Labels["apxy/cxr/source"], "stored apxy label must survive an update that omits it")
			require.Equal(t, "t-changed", saved.Labels["type"])
		})
	})
}
