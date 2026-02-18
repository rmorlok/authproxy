package database

import (
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/apctx"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/sqlh"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
	"testing"
	"time"
)

func TestConnectorVersions(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		sql := `
INSERT INTO connector_versions
(id,                                     version, namespace,          labels,                      state,     encrypted_definition, hash,    created_at,            updated_at,            deleted_at) VALUES
('6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11', 1,       'root',             '{"type":"gmail"}',          'active',  'encrypted-def',      'hash1', '2023-10-01 00:00:00', '2023-10-01 00:00:00', null),
('6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11', 2,       'root',             '{"type":"gmail"}',          'primary', 'encrypted-def',      'hash2', '2023-10-10 00:00:00', '2023-10-10 00:00:00', null),
('8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64', 1,       'root.child',       '{"type":"gmail"}',          'archived','encrypted-def',      'hash3', '2023-10-02 00:00:00', '2023-10-02 00:00:00', null),
('8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64', 2,       'root.child',       '{"type":"gmail"}',          'primary', 'encrypted-def',      'hash4', '2023-10-11 00:00:00', '2023-10-11 00:00:00', null),
('4a9f3c22-a8d5-423e-af53-e459f1d7c8da', 1,       'root.child2',      '{"type":"outlook"}',        'active',  'encrypted-def',      'hash5', '2023-10-03 00:00:00', '2023-10-03 00:00:00', null),
('4a9f3c22-a8d5-423e-af53-e459f1d7c8da', 2,       'root.child2',      '{"type":"outlook"}',        'primary', 'encrypted-def',      'hash6', '2023-10-12 00:00:00', '2023-10-12 00:00:00', null),
('c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa', 1,       'root.child.grand', '{"type":"google_drive"}',   'archived','encrypted-def',      'hash7', '2023-10-04 00:00:00', '2023-10-04 00:00:00', null),
('c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa', 2,       'root.child.grand', '{"type":"google_drive"}',   'active',  'encrypted-def',      'hash8', '2023-10-13 00:00:00', '2023-10-13 00:00:00', null),
('c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa', 3,       'root.child.grand', '{"type":"google_drive"}',   'primary', 'encrypted-def',      'hash9', '2023-10-14 00:00:00', '2023-10-14 00:00:00', null);
`
		_, err := rawDb.Exec(sql)
		require.NoError(t, err)

		v, err := db.GetConnectorVersion(ctx, uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), 1)
		require.NoError(t, err)
		require.Equal(t, "gmail", v.Labels["type"])
		require.Equal(t, ConnectorVersionStateActive, v.State)

		results, err := db.GetConnectorVersions(ctx, []ConnectorVersionId{
			{uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), 1},
		})
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Equal(t, "gmail", results[ConnectorVersionId{uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), 1}].Labels["type"])
		require.Equal(t, ConnectorVersionStateActive, results[ConnectorVersionId{uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), 1}].State)

		results, err = db.GetConnectorVersions(ctx, []ConnectorVersionId{
			{uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), 1},
			{uuid.MustParse("8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64"), 2},
		})
		require.NoError(t, err)
		require.Len(t, results, 2)
		require.Equal(t, "gmail", results[ConnectorVersionId{uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), 1}].Labels["type"])
		require.Equal(t, ConnectorVersionStateActive, results[ConnectorVersionId{uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), 1}].State)
		require.Equal(t, "gmail", results[ConnectorVersionId{uuid.MustParse("8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64"), 2}].Labels["type"])
		require.Equal(t, ConnectorVersionStatePrimary, results[ConnectorVersionId{uuid.MustParse("8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64"), 2}].State)

		// Version doesn't exist
		v, err = db.GetConnectorVersion(ctx, uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), 99)
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, v)

		// UUID doesn't exist
		v, err = db.GetConnectorVersion(ctx, uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-999999999999"), 1)
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, v)

		v, err = db.GetConnectorVersionForState(ctx, uuid.MustParse("4a9f3c22-a8d5-423e-af53-e459f1d7c8da"), ConnectorVersionStatePrimary)
		require.NoError(t, err)
		require.Equal(t, "outlook", v.Labels["type"])
		require.Equal(t, ConnectorVersionStatePrimary, v.State)

		v, err = db.GetConnectorVersionForState(ctx, uuid.MustParse("4a9f3c22-a8d5-423e-af53-e459f1d7c8da"), ConnectorVersionStateArchived)
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, v)

		pr := db.ListConnectorVersionsBuilder().
			ForLabelSelector("type=gmail").
			OrderBy(ConnectorVersionOrderByCreatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 4)
		require.Equal(t, pr.Results[0].Id, uuid.MustParse("8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64"))
		require.Equal(t, uint64(2), pr.Results[0].Version)
		require.Equal(t, pr.Results[1].Id, uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"))
		require.Equal(t, uint64(2), pr.Results[1].Version)
		require.Equal(t, pr.Results[2].Id, uuid.MustParse("8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64"))
		require.Equal(t, uint64(1), pr.Results[2].Version)
		require.Equal(t, pr.Results[3].Id, uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"))
		require.Equal(t, uint64(1), pr.Results[3].Version)

		pr = db.ListConnectorVersionsBuilder().
			ForNamespaceMatcher("root.child.**").
			OrderBy(ConnectorVersionOrderByCreatedAt, pagination.OrderByAsc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 5)
		require.Equal(t, uuid.MustParse("8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64"), pr.Results[0].Id)
		require.Equal(t, uint64(1), pr.Results[0].Version)
		require.Equal(t, uuid.MustParse("c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa"), pr.Results[1].Id)
		require.Equal(t, uint64(1), pr.Results[1].Version)
		require.Equal(t, uuid.MustParse("8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64"), pr.Results[2].Id)
		require.Equal(t, uint64(2), pr.Results[2].Version)
		require.Equal(t, uuid.MustParse("c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa"), pr.Results[3].Id)
		require.Equal(t, uint64(2), pr.Results[3].Version)
		require.Equal(t, uuid.MustParse("c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa"), pr.Results[4].Id)
		require.Equal(t, uint64(3), pr.Results[4].Version)
	})

	t.Run("ForNamespaceMatchers", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		sql := `
INSERT INTO connector_versions
(id,                                     version, namespace,            labels,                      state,     encrypted_definition, hash,    created_at,            updated_at,            deleted_at) VALUES
('6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11', 1,       'root.prod',          '{"type":"gmail"}',          'primary', 'encrypted-def',      'hash1', '2023-10-01 00:00:00', '2023-10-01 00:00:00', null),
('8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64', 1,       'root.staging',       '{"type":"gmail"}',          'primary', 'encrypted-def',      'hash3', '2023-10-02 00:00:00', '2023-10-02 00:00:00', null),
('4a9f3c22-a8d5-423e-af53-e459f1d7c8da', 1,       'root.dev',           '{"type":"outlook"}',        'primary', 'encrypted-def',      'hash5', '2023-10-03 00:00:00', '2023-10-03 00:00:00', null),
('c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa', 1,       'root.prod.tenant1',  '{"type":"google_drive"}',   'primary', 'encrypted-def',      'hash7', '2023-10-04 00:00:00', '2023-10-04 00:00:00', null);
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
			require.Equal(t, uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), pr.Results[0].Id)
		})

		t.Run("single wildcard matcher", func(t *testing.T) {
			pr := db.ListConnectorVersionsBuilder().
				ForNamespaceMatchers([]string{"root.prod.**"}).
				OrderBy(ConnectorVersionOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 2)
			require.Equal(t, uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), pr.Results[0].Id)
			require.Equal(t, uuid.MustParse("c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa"), pr.Results[1].Id)
		})

		t.Run("multiple exact matchers (OR logic)", func(t *testing.T) {
			pr := db.ListConnectorVersionsBuilder().
				ForNamespaceMatchers([]string{"root.prod", "root.staging"}).
				OrderBy(ConnectorVersionOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 2)
			require.Equal(t, uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), pr.Results[0].Id)
			require.Equal(t, uuid.MustParse("8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64"), pr.Results[1].Id)
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
	t.Run("UpsertConnectorVersion", func(t *testing.T) {
		t.Run("creates a new connector version", func(t *testing.T) {
			// Setup
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create a new connector version
			connectorID := uuid.New()
			cv := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           "root.some-namespace",
				State:               ConnectorVersionStateDraft,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash",
				EncryptedDefinition: "test_encrypted_definition",
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
			assert.Equal(t, "test_hash", savedCV.Hash)
			assert.Equal(t, "test_encrypted_definition", savedCV.EncryptedDefinition)
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_versions"))
		})

		t.Run("refuses to create active and archived versions", func(t *testing.T) {
			// Setup
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create a new connector version
			connectorID := uuid.New()
			cv := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           "root.some-namespace",
				State:               ConnectorVersionStateActive,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash",
				EncryptedDefinition: "test_encrypted_definition",
			}

			// Test
			err := db.UpsertConnectorVersion(ctx, cv)
			require.Error(t, err) // Cannot create active directly (must be primary)
			require.Equal(t, 0, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_versions"))

			cv.State = ConnectorVersionStateArchived
			err = db.UpsertConnectorVersion(ctx, cv)
			require.Error(t, err) // Cannot create archived directly (must be primary)
			require.Equal(t, 0, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_versions"))
		})

		t.Run("updates an existing draft version", func(t *testing.T) {
			// Setup
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create a new connector version
			connectorID := uuid.New()
			cv := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStateDraft,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash",
				EncryptedDefinition: "test_encrypted_definition",
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
			assert.Equal(t, "test_hash", savedCV.Hash)
			assert.Equal(t, "test_encrypted_definition", savedCV.EncryptedDefinition)
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_versions"))
		})

		t.Run("refuses to change namespace for draft version", func(t *testing.T) {
			// Setup
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create a new connector version
			connectorID := uuid.New()
			cv := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStateDraft,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash",
				EncryptedDefinition: "test_encrypted_definition",
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
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_versions"))
		})

		t.Run("creates multiple versions of the same connector", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create connector Id
			connectorID := uuid.New()

			// Create version 1
			cv1 := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStateDraft,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash_v1",
				EncryptedDefinition: "test_encrypted_definition_v1",
			}

			err := db.UpsertConnectorVersion(ctx, cv1)
			require.NoError(t, err)

			// Create version 2
			cv2 := &ConnectorVersion{
				Id:                  connectorID,
				Version:             2,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStateDraft,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash_v2",
				EncryptedDefinition: "test_encrypted_definition_v2",
			}

			err = db.UpsertConnectorVersion(ctx, cv2)
			require.NoError(t, err)

			// Verify version 1
			savedCV1, err := db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV1)
			assert.Equal(t, uint64(1), savedCV1.Version)
			assert.Equal(t, "test_hash_v1", savedCV1.Hash)

			// Verify version 2
			savedCV2, err := db.GetConnectorVersion(ctx, connectorID, 2)
			require.NoError(t, err)
			require.NotNil(t, savedCV2)
			assert.Equal(t, uint64(2), savedCV2.Version)
			assert.Equal(t, "test_hash_v2", savedCV2.Hash)
		})

		t.Run("creates a primary connector version", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create connector Id
			connectorID := uuid.New()

			// Create a primary connector version
			cv := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStatePrimary,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash",
				EncryptedDefinition: "test_encrypted_definition",
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
			connectorID := uuid.New()

			// Create version 1 as primary
			cv1 := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStatePrimary,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash_v1",
				EncryptedDefinition: "test_encrypted_definition_v1",
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
				EncryptedDefinition: "test_encrypted_definition_v2",
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
		t.Run("refuses to skip version numbers", func(t *testing.T) {
			// This test simulates what UpsertConnectorVersion does when setting a new primary version

			// Setup
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create connector Id
			connectorID := uuid.New()

			// Create version 1 as primary
			cv1 := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStatePrimary,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash_v1",
				EncryptedDefinition: "test_encrypted_definition_v1",
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
				EncryptedDefinition: "test_encrypted_definition_v2",
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

			connectorID := uuid.New()
			cv := &ConnectorVersion{
				Id:                  connectorID,
				Version:             1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorVersionStateDraft,
				Labels:              Labels{"type": "test_connector"},
				Hash:                "test_hash",
				EncryptedDefinition: "test_encrypted_definition",
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

			err := db.SetConnectorVersionState(ctx, uuid.New(), 1, ConnectorVersionStatePrimary)
			require.ErrorIs(t, err, ErrNotFound)
		})

		t.Run("rejects invalid state", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.SetConnectorVersionState(ctx, uuid.New(), 1, ConnectorVersionState("invalid"))
			require.Error(t, err)
		})

		t.Run("demotes existing primary to active when forcing new primary", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := uuid.New()

			// Create v1 as primary
			err := db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: connectorID, Version: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStatePrimary, Labels: Labels{"type": "t"},
				Hash: "h1", EncryptedDefinition: "e1",
			})
			require.NoError(t, err)

			// Create v2 as draft
			err = db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: connectorID, Version: 2, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStateDraft, Labels: Labels{"type": "t"},
				Hash: "h2", EncryptedDefinition: "e2",
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

		t.Run("archives existing draft when forcing new draft", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := uuid.New()

			// Create v1 as primary
			err := db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: connectorID, Version: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStatePrimary, Labels: Labels{"type": "t"},
				Hash: "h1", EncryptedDefinition: "e1",
			})
			require.NoError(t, err)

			// Create v2 as draft
			err = db.UpsertConnectorVersion(ctx, &ConnectorVersion{
				Id: connectorID, Version: 2, Namespace: sconfig.RootNamespace,
				State: ConnectorVersionStateDraft, Labels: Labels{"type": "t"},
				Hash: "h2", EncryptedDefinition: "e2",
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
}
