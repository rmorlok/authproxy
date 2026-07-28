package database

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

func testConnectorVersion(
	id apid.ID,
	namespace string,
	name scommon.ResourceName,
	version uint64,
) *ConnectorVersion {
	return &ConnectorVersion{
		Id:        id,
		Namespace: namespace,
		Name:      name,
		Version:   version,
		State:     ConnectorVersionStateDraft,
		Hash:      fmt.Sprintf("hash-%d", version),
		EncryptedDefinition: encfield.EncryptedField{
			ID:   apid.New(apid.PrefixDataEncryptionKey),
			Data: fmt.Sprintf("encrypted-%d", version),
		},
	}
}

func TestConnectorResourceNameDefaultsAndProjectsAcrossVersions(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	id := apid.New(apid.PrefixConnectorVersion)

	require.NoError(t, db.UpsertConnectorVersion(ctx, testConnectorVersion(id, "root", "", 1)))
	require.NoError(t, db.UpsertConnectorVersion(ctx, testConnectorVersion(id, "root", "", 2)))

	first, err := db.GetConnectorVersion(ctx, id, 1)
	require.NoError(t, err)
	second, err := db.GetConnectorVersion(ctx, id, 2)
	require.NoError(t, err)
	require.Equal(t, scommon.ResourceName(id.String()), first.Name)
	require.Equal(t, first.Name, second.Name)

	versions := db.ListConnectorVersionsBuilder().ForId(id).FetchPage(ctx)
	require.NoError(t, versions.Error)
	require.Len(t, versions.Results, 2)
	require.Equal(t, first.Name, versions.Results[0].Name)
	require.Equal(t, first.Name, versions.Results[1].Name)

	connectors := db.ListConnectorsBuilder().ForId(id).FetchPage(ctx)
	require.NoError(t, connectors.Error)
	require.Len(t, connectors.Results, 1)
	require.Equal(t, first.Name, connectors.Results[0].Name)
}

func TestConnectorResourceRenameDoesNotRewriteVersions(t *testing.T) {
	_, db, rawDB := MustApplyBlankTestDbConfigRaw(t, nil)
	ctx := context.Background()
	id := apid.New(apid.PrefixConnectorVersion)

	require.NoError(t, db.UpsertConnectorVersion(ctx, testConnectorVersion(id, "root", "original", 1)))
	require.NoError(t, db.UpsertConnectorVersion(ctx, testConnectorVersion(id, "root", "", 2)))

	originalHashes := connectorVersionHashes(t, rawDB, id)

	require.NoError(t, db.UpdateConnectorName(ctx, id, "renamed"))
	for _, version := range []uint64{1, 2} {
		projected, err := db.GetConnectorVersion(ctx, id, version)
		require.NoError(t, err)
		require.Equal(t, scommon.ResourceName("renamed"), projected.Name)
	}

	renamedHashes := connectorVersionHashes(t, rawDB, id)
	require.Equal(t, originalHashes, renamedHashes)
	require.Equal(t, 2, sqlhMustCountConnectorVersions(t, rawDB, id))

	err := db.UpsertConnectorVersion(ctx, testConnectorVersion(id, "root", "forked", 3))
	require.ErrorContains(t, err, "cannot modify connector name")
}

func TestConnectorResourceRejectsNamespaceFork(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	id := apid.New(apid.PrefixConnectorVersion)

	require.NoError(t, db.UpsertConnectorVersion(ctx, testConnectorVersion(id, "root", "connector", 1)))
	err := db.UpsertConnectorVersion(ctx, testConnectorVersion(id, "root.other", "", 2))
	require.ErrorContains(t, err, "cannot modify connector namespace")

	projected, err := db.GetConnectorVersion(ctx, id, 1)
	require.NoError(t, err)
	require.Equal(t, "root", projected.Namespace)
	require.Equal(t, scommon.ResourceName("connector"), projected.Name)
}

func TestConnectorResourceNameUniquenessAndDeleteReuse(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.other"}))

	firstID := apid.New(apid.PrefixConnectorVersion)
	require.NoError(t, db.UpsertConnectorVersion(ctx, testConnectorVersion(firstID, "root", "shared", 1)))

	conflictID := apid.New(apid.PrefixConnectorVersion)
	require.Error(t, db.UpsertConnectorVersion(ctx, testConnectorVersion(conflictID, "root", "shared", 1)))

	otherNamespaceID := apid.New(apid.PrefixConnectorVersion)
	require.NoError(t, db.UpsertConnectorVersion(ctx, testConnectorVersion(otherNamespaceID, "root.other", "shared", 1)))

	require.NoError(t, db.CreateActor(ctx, &Actor{
		Id:         apid.New(apid.PrefixActor),
		Namespace:  "root",
		Name:       "shared",
		ExternalId: "same-name-other-type",
	}))

	require.NoError(t, db.DeleteConnector(ctx, firstID))
	reusedID := apid.New(apid.PrefixConnectorVersion)
	require.NoError(t, db.UpsertConnectorVersion(ctx, testConnectorVersion(reusedID, "root", "shared", 1)))

	require.ErrorIs(t, db.UpdateConnectorName(ctx, firstID, "deleted"), ErrNotFound)
}

func TestConnectorResourceVersionLifecyclePreservesName(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	id := apid.New(apid.PrefixConnectorVersion)

	require.NoError(t, db.UpsertConnectorVersion(ctx, testConnectorVersion(id, "root", "lifecycle", 1)))
	require.NoError(t, db.SetConnectorVersionState(ctx, id, 1, ConnectorVersionStatePrimary))
	require.NoError(t, db.SetConnectorVersionState(ctx, id, 1, ConnectorVersionStateArchived))

	projected, err := db.GetConnectorVersion(ctx, id, 1)
	require.NoError(t, err)
	require.Equal(t, scommon.ResourceName("lifecycle"), projected.Name)
	require.Equal(t, ConnectorVersionStateArchived, projected.State)
}

func TestConnectorResourceMigrationBackfillsDeterministically(t *testing.T) {
	_, db, rawDB := MustApplyBlankTestDbConfigRaw(t, nil)
	service := db.(*service)
	migrateDatabaseToVersion(t, service, 15)

	liveID := apid.New(apid.PrefixConnectorVersion)
	deletedID := apid.New(apid.PrefixConnectorVersion)
	_, err := rawDB.Exec(fmt.Sprintf(`
		INSERT INTO connector_versions (
			id, version, namespace, state, hash, encrypted_definition,
			created_at, updated_at, deleted_at
		) VALUES
		('%s', 1, 'root.old', 'archived', 'old', '{"id":"dek_old","d":"old"}',
		 '2024-01-01T00:00:00Z', '2024-01-02T00:00:00Z', '2024-02-01T00:00:00Z'),
		('%s', 2, 'root.live', 'primary', 'live', '{"id":"dek_live","d":"live"}',
		 '2024-03-01T00:00:00Z', '2024-03-02T00:00:00Z', NULL),
		('%s', 1, 'root.deleted', 'archived', 'deleted', '{"id":"dek_deleted","d":"deleted"}',
		 '2024-04-01T00:00:00Z', '2024-04-02T00:00:00Z', '2024-05-01T00:00:00Z')
	`, liveID, liveID, deletedID))
	require.NoError(t, err)

	migrateDatabaseToVersion(t, service, 16)

	var liveNamespace, liveName string
	var liveDeletedAt any
	require.NoError(t, rawDB.QueryRow(fmt.Sprintf(
		"SELECT namespace, name, deleted_at FROM connectors WHERE id = '%s'",
		liveID,
	)).Scan(&liveNamespace, &liveName, &liveDeletedAt))
	require.Equal(t, "root.live", liveNamespace)
	require.Equal(t, liveID.String(), liveName)
	require.Nil(t, liveDeletedAt)

	var deletedName string
	var deletedAt any
	require.NoError(t, rawDB.QueryRow(fmt.Sprintf(
		"SELECT name, deleted_at FROM connectors WHERE id = '%s'",
		deletedID,
	)).Scan(&deletedName, &deletedAt))
	require.Equal(t, deletedID.String(), deletedName)
	require.NotNil(t, deletedAt)

	_, err = rawDB.Query("SELECT namespace FROM connector_versions")
	require.Error(t, err)

	projected, err := db.GetConnectorVersion(context.Background(), liveID, 2)
	require.NoError(t, err)
	require.Equal(t, "root.live", projected.Namespace)
	require.Equal(t, scommon.ResourceName(liveID.String()), projected.Name)

	migrateDatabaseToVersion(t, service, 15)
	rows, err := rawDB.Query(fmt.Sprintf(
		"SELECT DISTINCT namespace FROM connector_versions WHERE id = '%s'",
		liveID,
	))
	require.NoError(t, err)
	defer rows.Close()
	var namespaces []string
	for rows.Next() {
		var namespace string
		require.NoError(t, rows.Scan(&namespace))
		namespaces = append(namespaces, namespace)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []string{"root.live"}, namespaces)

	migrateDatabaseToVersion(t, service, 16)
}

func sqlhMustCountConnectorVersions(t *testing.T, rawDB *sql.DB, id apid.ID) int {
	t.Helper()
	var count int
	require.NoError(t, rawDB.QueryRow(fmt.Sprintf(
		"SELECT COUNT(*) FROM connector_versions WHERE id = '%s'",
		id,
	)).Scan(&count))
	return count
}

func connectorVersionHashes(t *testing.T, rawDB *sql.DB, id apid.ID) []string {
	t.Helper()
	rows, err := rawDB.Query(fmt.Sprintf(
		"SELECT hash FROM connector_versions WHERE id = '%s' ORDER BY version",
		id,
	))
	require.NoError(t, err)
	defer rows.Close()

	var hashes []string
	for rows.Next() {
		var hash string
		require.NoError(t, rows.Scan(&hash))
		hashes = append(hashes, hash)
	}
	require.NoError(t, rows.Err())
	return hashes
}
