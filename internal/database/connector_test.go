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

func testConnectorWithDefinition(
	id apid.ID,
	namespace string,
	name scommon.ResourceName,
	version uint64,
) *ConnectorWithDefinition {
	return &ConnectorWithDefinition{
		Id:        id,
		Namespace: namespace,
		Name:      name,
		Version:   version,
		State:     ConnectorDefinitionVersionStateDraft,
		EncryptedDefinition: encfield.EncryptedField{
			ID:   apid.New(apid.PrefixDataEncryptionKey),
			Data: fmt.Sprintf("encrypted-%d", version),
		},
	}
}

func TestConnectorNameDefaultsAndProjectsAcrossVersions(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	id := apid.New(apid.PrefixConnectorVersion)

	require.NoError(t, db.UpsertConnectorDefinitionVersion(ctx, testConnectorWithDefinition(id, "root", "", 1)))
	require.NoError(t, db.UpsertConnectorDefinitionVersion(ctx, testConnectorWithDefinition(id, "root", "", 2)))

	first, err := db.GetConnectorDefinitionVersion(ctx, id, 1)
	require.NoError(t, err)
	second, err := db.GetConnectorDefinitionVersion(ctx, id, 2)
	require.NoError(t, err)
	require.Equal(t, scommon.ResourceName(id.String()), first.Name)
	require.Equal(t, first.Name, second.Name)

	versions := db.ListConnectorDefinitionVersionsBuilder().ForId(id).FetchPage(ctx)
	require.NoError(t, versions.Error)
	require.Len(t, versions.Results, 2)
	require.Equal(t, first.Name, versions.Results[0].Name)
	require.Equal(t, first.Name, versions.Results[1].Name)

	connectors := db.ListConnectorsBuilder().ForId(id).FetchPage(ctx)
	require.NoError(t, connectors.Error)
	require.Len(t, connectors.Results, 1)
	require.Equal(t, first.Name, connectors.Results[0].Name)
}

func TestConnectorRenameDoesNotRewriteVersions(t *testing.T) {
	_, db, rawDB := MustApplyBlankTestDbConfigRaw(t, nil)
	ctx := context.Background()
	id := apid.New(apid.PrefixConnectorVersion)

	require.NoError(t, db.UpsertConnectorDefinitionVersion(ctx, testConnectorWithDefinition(id, "root", "original", 1)))
	require.NoError(t, db.UpsertConnectorDefinitionVersion(ctx, testConnectorWithDefinition(id, "root", "", 2)))

	originalDefinitions := connectorDefinitionPayloads(t, rawDB, id)

	require.NoError(t, db.UpdateConnectorName(ctx, id, "renamed"))
	for _, version := range []uint64{1, 2} {
		projected, err := db.GetConnectorDefinitionVersion(ctx, id, version)
		require.NoError(t, err)
		require.Equal(t, scommon.ResourceName("renamed"), projected.Name)
	}

	renamedDefinitions := connectorDefinitionPayloads(t, rawDB, id)
	require.Equal(t, originalDefinitions, renamedDefinitions)
	require.Equal(t, 2, sqlhMustCountConnectorDefinitionVersions(t, rawDB, id))

	err := db.UpsertConnectorDefinitionVersion(ctx, testConnectorWithDefinition(id, "root", "forked", 3))
	require.ErrorContains(t, err, "cannot modify connector name")
}

func TestConnectorRejectsNamespaceFork(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	id := apid.New(apid.PrefixConnectorVersion)

	require.NoError(t, db.UpsertConnectorDefinitionVersion(ctx, testConnectorWithDefinition(id, "root", "connector", 1)))
	err := db.UpsertConnectorDefinitionVersion(ctx, testConnectorWithDefinition(id, "root.other", "", 2))
	require.ErrorContains(t, err, "cannot modify connector namespace")

	projected, err := db.GetConnectorDefinitionVersion(ctx, id, 1)
	require.NoError(t, err)
	require.Equal(t, "root", projected.Namespace)
	require.Equal(t, scommon.ResourceName("connector"), projected.Name)
}

func TestConnectorNameUniquenessAndDeleteReuse(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.other"}))

	firstID := apid.New(apid.PrefixConnectorVersion)
	require.NoError(t, db.UpsertConnectorDefinitionVersion(ctx, testConnectorWithDefinition(firstID, "root", "shared", 1)))

	conflictID := apid.New(apid.PrefixConnectorVersion)
	require.Error(t, db.UpsertConnectorDefinitionVersion(ctx, testConnectorWithDefinition(conflictID, "root", "shared", 1)))

	otherNamespaceID := apid.New(apid.PrefixConnectorVersion)
	require.NoError(t, db.UpsertConnectorDefinitionVersion(ctx, testConnectorWithDefinition(otherNamespaceID, "root.other", "shared", 1)))

	require.NoError(t, db.CreateActor(ctx, &Actor{
		Id:         apid.New(apid.PrefixActor),
		Namespace:  "root",
		Name:       "shared",
		ExternalId: "same-name-other-type",
	}))

	require.NoError(t, db.DeleteConnector(ctx, firstID))
	reusedID := apid.New(apid.PrefixConnectorVersion)
	require.NoError(t, db.UpsertConnectorDefinitionVersion(ctx, testConnectorWithDefinition(reusedID, "root", "shared", 1)))

	require.ErrorIs(t, db.UpdateConnectorName(ctx, firstID, "deleted"), ErrNotFound)
}

func TestConnectorDefinitionVersionLifecyclePreservesName(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	id := apid.New(apid.PrefixConnectorVersion)

	require.NoError(t, db.UpsertConnectorDefinitionVersion(ctx, testConnectorWithDefinition(id, "root", "lifecycle", 1)))
	require.NoError(t, db.SetConnectorDefinitionVersionState(ctx, id, 1, ConnectorDefinitionVersionStatePrimary))
	require.NoError(t, db.SetConnectorDefinitionVersionState(ctx, id, 1, ConnectorDefinitionVersionStateArchived))

	projected, err := db.GetConnectorDefinitionVersion(ctx, id, 1)
	require.NoError(t, err)
	require.Equal(t, scommon.ResourceName("lifecycle"), projected.Name)
	require.Equal(t, ConnectorDefinitionVersionStateArchived, projected.State)
}

func TestConnectorMigrationBackfillsDeterministically(t *testing.T) {
	_, db, rawDB := MustApplyBlankTestDbConfigRaw(t, nil)
	service := db.(*service)
	migrateDatabaseToVersion(t, service, 15)

	liveID := apid.New(apid.PrefixConnectorVersion)
	deletedID := apid.New(apid.PrefixConnectorVersion)
	_, err := rawDB.Exec(fmt.Sprintf(`
		INSERT INTO connector_versions (
			id, version, namespace, labels, annotations, state, hash, encrypted_definition,
			created_at, updated_at, deleted_at
		) VALUES
		('%s', 1, 'root.old', '{"selected":"no"}', '{"selected":"no"}', 'archived', 'old', '{"id":"dek_old","d":"old"}',
		 '2024-01-01T00:00:00Z', '2024-01-02T00:00:00Z', '2024-02-01T00:00:00Z'),
		('%s', 2, 'root.live', '{"selected":"yes"}', '{"selected":"yes"}', 'primary', 'live', '{"id":"dek_live","d":"live"}',
		 '2024-03-01T00:00:00Z', '2024-03-02T00:00:00Z', NULL),
		('%s', 1, 'root.deleted', '{"deleted":"yes"}', '{"deleted":"yes"}', 'archived', 'deleted', '{"id":"dek_deleted","d":"deleted"}',
		 '2024-04-01T00:00:00Z', '2024-04-02T00:00:00Z', '2024-05-01T00:00:00Z')
	`, liveID, liveID, deletedID))
	require.NoError(t, err)

	migrateDatabaseToVersion(t, service, 16)

	var liveNamespace, liveName, liveLabels, liveAnnotations string
	var liveDeletedAt any
	require.NoError(t, rawDB.QueryRow(fmt.Sprintf(
		"SELECT namespace, name, labels, annotations, deleted_at FROM connectors WHERE id = '%s'",
		liveID,
	)).Scan(&liveNamespace, &liveName, &liveLabels, &liveAnnotations, &liveDeletedAt))
	require.Equal(t, "root.live", liveNamespace)
	require.Equal(t, liveID.String(), liveName)
	require.JSONEq(t, `{"selected":"yes"}`, liveLabels)
	require.JSONEq(t, `{"selected":"yes"}`, liveAnnotations)
	require.Nil(t, liveDeletedAt)

	var deletedName string
	var deletedAt any
	require.NoError(t, rawDB.QueryRow(fmt.Sprintf(
		"SELECT name, deleted_at FROM connectors WHERE id = '%s'",
		deletedID,
	)).Scan(&deletedName, &deletedAt))
	require.Equal(t, deletedID.String(), deletedName)
	require.NotNil(t, deletedAt)

	_, err = rawDB.Query("SELECT namespace FROM connector_definition_versions")
	require.Error(t, err)
	_, err = rawDB.Query("SELECT labels FROM connector_definition_versions")
	require.Error(t, err)
	_, err = rawDB.Query("SELECT annotations FROM connector_definition_versions")
	require.Error(t, err)
	_, err = rawDB.Query("SELECT hash FROM connector_definition_versions")
	require.Error(t, err)
	_, err = rawDB.Query("SELECT type FROM connector_definition_versions")
	require.Error(t, err)
	_, err = rawDB.Query("SELECT created_at FROM connector_definition_versions")
	require.Error(t, err)
	var liveDefinitionDeletedAt any
	require.NoError(t, rawDB.QueryRow(fmt.Sprintf(
		"SELECT deleted_at FROM connector_definition_versions WHERE connector_id = '%s' LIMIT 1",
		liveID,
	)).Scan(&liveDefinitionDeletedAt))
	require.Nil(t, liveDefinitionDeletedAt)

	var deletedDefinitionDeletedAt any
	require.NoError(t, rawDB.QueryRow(fmt.Sprintf(
		"SELECT deleted_at FROM connector_definition_versions WHERE connector_id = '%s' LIMIT 1",
		deletedID,
	)).Scan(&deletedDefinitionDeletedAt))
	require.NotNil(t, deletedDefinitionDeletedAt)

	rows, err := rawDB.Query(fmt.Sprintf(
		"SELECT id FROM connector_definition_versions WHERE connector_id = '%s' ORDER BY version",
		liveID,
	))
	require.NoError(t, err)
	var definitionIDs []apid.ID
	for rows.Next() {
		var definitionID apid.ID
		require.NoError(t, rows.Scan(&definitionID))
		definitionIDs = append(definitionIDs, definitionID)
	}
	require.NoError(t, rows.Close())
	require.Len(t, definitionIDs, 2)
	for _, definitionID := range definitionIDs {
		require.Equal(t, apid.PrefixConnectorDefinitionVersion, definitionID.Prefix())
	}

	_, err = rawDB.Exec(fmt.Sprintf(`
		INSERT INTO connector_definition_versions (
			id, connector_id, version, state, encrypted_definition
		) VALUES (
			'cvd_duplicate', '%s', 2, 'draft', '{"id":"dek_duplicate","d":"duplicate"}'
		)
	`, liveID))
	require.Error(t, err, "(connector_id, version) must be unique")

	projected, err := db.GetConnectorDefinitionVersion(context.Background(), liveID, 2)
	require.NoError(t, err)
	require.Equal(t, "root.live", projected.Namespace)
	require.Equal(t, scommon.ResourceName(liveID.String()), projected.Name)

	migrateDatabaseToVersion(t, service, 15)
	rows, err = rawDB.Query(fmt.Sprintf(
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

func sqlhMustCountConnectorDefinitionVersions(t *testing.T, rawDB *sql.DB, id apid.ID) int {
	t.Helper()
	var count int
	require.NoError(t, rawDB.QueryRow(fmt.Sprintf(
		"SELECT COUNT(*) FROM connector_definition_versions WHERE connector_id = '%s'",
		id,
	)).Scan(&count))
	return count
}

func connectorDefinitionPayloads(t *testing.T, rawDB *sql.DB, id apid.ID) []string {
	t.Helper()
	rows, err := rawDB.Query(fmt.Sprintf(
		"SELECT encrypted_definition FROM connector_definition_versions WHERE connector_id = '%s' ORDER BY version",
		id,
	))
	require.NoError(t, err)
	defer rows.Close()

	var definitions []string
	for rows.Next() {
		var definition string
		require.NoError(t, rows.Scan(&definition))
		definitions = append(definitions, definition)
	}
	require.NoError(t, rows.Err())
	return definitions
}
