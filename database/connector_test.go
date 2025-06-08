package database

import (
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/apctx"
	"github.com/rmorlok/authproxy/sqlh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
	"testing"
	"time"
)

func TestConnectors(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw("connection_round_trip", nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		sql := `
INSERT INTO connector_versions 
(id, version, state, type, encrypted_definition, hash, created_at, updated_at, deleted_at) VALUES 
('6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11', 1, 'active', 'gmail', 'encrypted-def', 'hash1', '2023-10-01 00:00:00', '2023-10-01 00:00:00', null),
('6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11', 2, 'primary', 'gmail', 'encrypted-def', 'hash2', '2023-10-10 00:00:00', '2023-10-10 00:00:00', null),
('8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64', 1, 'archived', 'gmail', 'encrypted-def', 'hash3', '2023-10-02 00:00:00', '2023-10-02 00:00:00', null),
('8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64', 2, 'primary', 'gmail', 'encrypted-def', 'hash4', '2023-10-11 00:00:00', '2023-10-11 00:00:00', null),
('4a9f3c22-a8d5-423e-af53-e459f1d7c8da', 1, 'active', 'outlook', 'encrypted-def', 'hash5', '2023-10-03 00:00:00', '2023-10-03 00:00:00', null),
('4a9f3c22-a8d5-423e-af53-e459f1d7c8da', 2, 'primary', 'outlook', 'encrypted-def', 'hash6', '2023-10-12 00:00:00', '2023-10-12 00:00:00', null),
('c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa', 1, 'archived', 'google_drive', 'encrypted-def', 'hash7', '2023-10-04 00:00:00', '2023-10-04 00:00:00', null),
('c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa', 2, 'active', 'google_drive', 'encrypted-def', 'hash8', '2023-10-13 00:00:00', '2023-10-13 00:00:00', null),
('c5e6a111-e2bc-4cb8-9f00-df68e4ab71aa', 3, 'primary', 'google_drive', 'encrypted-def', 'hash9', '2023-10-14 00:00:00', '2023-10-14 00:00:00', null);
`
		_, err := rawDb.Exec(sql)
		require.NoError(t, err)

		v, err := db.GetConnectorVersion(ctx, uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), 1)
		require.NoError(t, err)
		require.Equal(t, "gmail", v.Type)
		require.Equal(t, ConnectorVersionStateActive, v.State)

		// Version doesn't exist
		v, err = db.GetConnectorVersion(ctx, uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"), 99)
		require.NoError(t, err)
		require.Nil(t, v)

		// UUID doesn't exist
		v, err = db.GetConnectorVersion(ctx, uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-999999999999"), 1)
		require.NoError(t, err)
		require.Nil(t, v)

		pr := db.ListConnectorsBuilder().
			ForType("gmail").
			OrderBy(ConnectorOrderByCreatedAt, OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 2)
		require.Equal(t, pr.Results[0].ID, uuid.MustParse("8e9a7d67-3b4c-512d-9fb4-fd2d381bfa64"))
		require.Equal(t, pr.Results[1].ID, uuid.MustParse("6f1f9c15-1a2b-4d0a-b3d8-966c073a1a11"))
	})
	t.Run("UpsertConnectorVersion", func(t *testing.T) {
		t.Run("creates a new connector version", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig("create_connector_version", nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create a new connector version
			connectorID := uuid.New()
			cv := &ConnectorVersion{
				ID:                  connectorID,
				Version:             1,
				State:               ConnectorVersionStateDraft,
				Type:                "test_connector",
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
			assert.Equal(t, connectorID, savedCV.ID)
			assert.Equal(t, int64(1), savedCV.Version)
			assert.Equal(t, ConnectorVersionStateDraft, savedCV.State)
			assert.Equal(t, "test_connector", savedCV.Type)
			assert.Equal(t, "test_hash", savedCV.Hash)
			assert.Equal(t, "test_encrypted_definition", savedCV.EncryptedDefinition)
		})

		t.Run("refuses to create active and archived versions", func(t *testing.T) {
			// Setup
			_, db, rawDb := MustApplyBlankTestDbConfigRaw("create_connector_version", nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create a new connector version
			connectorID := uuid.New()
			cv := &ConnectorVersion{
				ID:                  connectorID,
				Version:             1,
				State:               ConnectorVersionStateActive,
				Type:                "test_connector",
				Hash:                "test_hash",
				EncryptedDefinition: "test_encrypted_definition",
			}

			// Test
			err := db.UpsertConnectorVersion(ctx, cv)
			require.Error(t, err)
			require.Equal(t, 0, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_versions"))

			cv.State = ConnectorVersionStateArchived
			err = db.UpsertConnectorVersion(ctx, cv)
			require.Error(t, err)
			require.Equal(t, 0, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_versions"))
		})

		t.Run("updates an existing draft version", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig("create_connector_version", nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create a new connector version
			connectorID := uuid.New()
			cv := &ConnectorVersion{
				ID:                  connectorID,
				Version:             1,
				State:               ConnectorVersionStateDraft,
				Type:                "test_connector",
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
			assert.Equal(t, connectorID, savedCV.ID)
			assert.Equal(t, int64(1), savedCV.Version)
			assert.Equal(t, ConnectorVersionStateDraft, savedCV.State)
			assert.Equal(t, "test_connector", savedCV.Type)
			assert.Equal(t, "test_hash", savedCV.Hash)
			assert.Equal(t, "test_encrypted_definition", savedCV.EncryptedDefinition)
		})

		t.Run("creates multiple versions of the same connector", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig("create_connector_version_multiple", nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create connector ID
			connectorID := uuid.New()

			// Create version 1
			cv1 := &ConnectorVersion{
				ID:                  connectorID,
				Version:             1,
				State:               ConnectorVersionStateDraft,
				Type:                "test_connector",
				Hash:                "test_hash_v1",
				EncryptedDefinition: "test_encrypted_definition_v1",
			}

			err := db.UpsertConnectorVersion(ctx, cv1)
			require.NoError(t, err)

			// Create version 2
			cv2 := &ConnectorVersion{
				ID:                  connectorID,
				Version:             2,
				State:               ConnectorVersionStateDraft,
				Type:                "test_connector",
				Hash:                "test_hash_v2",
				EncryptedDefinition: "test_encrypted_definition_v2",
			}

			err = db.UpsertConnectorVersion(ctx, cv2)
			require.NoError(t, err)

			// Verify version 1
			savedCV1, err := db.GetConnectorVersion(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV1)
			assert.Equal(t, int64(1), savedCV1.Version)
			assert.Equal(t, "test_hash_v1", savedCV1.Hash)

			// Verify version 2
			savedCV2, err := db.GetConnectorVersion(ctx, connectorID, 2)
			require.NoError(t, err)
			require.NotNil(t, savedCV2)
			assert.Equal(t, int64(2), savedCV2.Version)
			assert.Equal(t, "test_hash_v2", savedCV2.Hash)
		})

		t.Run("creates a primary connector version", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig("create_connector_version_primary", nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create connector ID
			connectorID := uuid.New()

			// Create a primary connector version
			cv := &ConnectorVersion{
				ID:                  connectorID,
				Version:             1,
				State:               ConnectorVersionStatePrimary,
				Type:                "test_connector",
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
			_, db := MustApplyBlankTestDbConfig("create_connector_version_multiple_primary", nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create connector ID
			connectorID := uuid.New()

			// Create version 1 as primary
			cv1 := &ConnectorVersion{
				ID:                  connectorID,
				Version:             1,
				State:               ConnectorVersionStatePrimary,
				Type:                "test_connector",
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
				ID:                  connectorID,
				Version:             2,
				State:               ConnectorVersionStatePrimary,
				Type:                "test_connector",
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
			_, db := MustApplyBlankTestDbConfig("create_connector_version_multiple_primary", nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create connector ID
			connectorID := uuid.New()

			// Create version 1 as primary
			cv1 := &ConnectorVersion{
				ID:                  connectorID,
				Version:             1,
				State:               ConnectorVersionStatePrimary,
				Type:                "test_connector",
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
				ID:                  connectorID,
				Version:             3,
				State:               ConnectorVersionStatePrimary,
				Type:                "test_connector",
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
			require.NoError(t, err)
			require.Nil(t, savedCV1)
		})
	})
}
