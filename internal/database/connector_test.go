package database

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

func testConnectorWithDefinition(
	id apid.ID,
	namespace string,
	name scommon.ResourceName,
	generation uint64,
) *ConnectorWithDefinition {
	return &ConnectorWithDefinition{
		Id:         id,
		Namespace:  namespace,
		Name:       name,
		Generation: generation,
		State:      ConnectorGenerationStateDraft,
		EncryptedDefinition: encfield.EncryptedField{
			ID:   apid.New(apid.PrefixDataEncryptionKey),
			Data: fmt.Sprintf("encrypted-%d", generation),
		},
	}
}

func TestConnectorNameDefaultsAndProjectsAcrossGenerations(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	id := apid.New(apid.PrefixConnector)

	require.NoError(t, db.UpsertConnectorGeneration(ctx, testConnectorWithDefinition(id, "root", "", 1)))
	require.NoError(t, db.UpsertConnectorGeneration(ctx, testConnectorWithDefinition(id, "root", "", 2)))

	first, err := db.GetConnectorGeneration(ctx, id, 1)
	require.NoError(t, err)
	second, err := db.GetConnectorGeneration(ctx, id, 2)
	require.NoError(t, err)
	require.Equal(t, scommon.ResourceName(id.String()), first.Name)
	require.Equal(t, first.Name, second.Name)
	require.Equal(t, id.String(), first.Labels["apxy/cxr/-/name"])

	generations := db.ListConnectorGenerationsBuilder().ForId(id).FetchPage(ctx)
	require.NoError(t, generations.Error)
	require.Len(t, generations.Results, 2)
	require.Equal(t, first.Name, generations.Results[0].Name)
	require.Equal(t, first.Name, generations.Results[1].Name)

	connectors := db.ListConnectorsBuilder().ForId(id).FetchPage(ctx)
	require.NoError(t, connectors.Error)
	require.Len(t, connectors.Results, 1)
	require.Equal(t, first.Name, connectors.Results[0].Name)
}

func TestConnectorRenameDoesNotRewriteGenerations(t *testing.T) {
	_, db, rawDB := MustApplyBlankTestDbConfigRaw(t, nil)
	ctx := context.Background()
	id := apid.New(apid.PrefixConnector)

	require.NoError(t, db.UpsertConnectorGeneration(ctx, testConnectorWithDefinition(id, "root", "original", 1)))
	require.NoError(t, db.UpsertConnectorGeneration(ctx, testConnectorWithDefinition(id, "root", "", 2)))

	originalDefinitions := connectorDefinitionPayloads(t, rawDB, id)

	require.NoError(t, db.UpdateConnectorName(ctx, id, "renamed"))
	for _, generation := range []uint64{1, 2} {
		projected, err := db.GetConnectorGeneration(ctx, id, generation)
		require.NoError(t, err)
		require.Equal(t, scommon.ResourceName("renamed"), projected.Name)
		require.Equal(t, "renamed", projected.Labels["apxy/cxr/-/name"])
	}

	renamedDefinitions := connectorDefinitionPayloads(t, rawDB, id)
	require.Equal(t, originalDefinitions, renamedDefinitions)
	require.Equal(t, 2, sqlhMustCountConnectorGenerations(t, rawDB, id))

	err := db.UpsertConnectorGeneration(ctx, testConnectorWithDefinition(id, "root", "forked", 3))
	require.ErrorContains(t, err, "cannot modify connector name")
}

func TestConnectorMetadataUpdatesDoNotRewriteGenerations(t *testing.T) {
	_, db, rawDB := MustApplyBlankTestDbConfigRaw(t, nil)
	ctx := context.Background()
	id := apid.New(apid.PrefixConnector)
	connector := testConnectorWithDefinition(id, "root", "configured", 1)
	connector.Labels = Labels{"environment": "demo"}
	connector.Annotations = Annotations{"example.com/owner": "integrations"}
	require.NoError(t, db.UpsertConnectorGeneration(ctx, connector))

	originalDefinitions := connectorDefinitionPayloads(t, rawDB, id)
	updated, err := db.UpdateConnectorLabels(ctx, id, map[string]string{"environment": "production"})
	require.NoError(t, err)
	userLabels, _ := SplitUserAndApxyLabels(updated.Labels)
	require.Equal(t, Labels{"environment": "production"}, userLabels)

	updated, err = db.UpdateConnectorAnnotations(ctx, id, map[string]string{"example.com/owner": "platform"})
	require.NoError(t, err)
	require.Equal(t, Annotations{"example.com/owner": "platform"}, updated.Annotations)

	require.Equal(t, originalDefinitions, connectorDefinitionPayloads(t, rawDB, id))
	require.Equal(t, 1, sqlhMustCountConnectorGenerations(t, rawDB, id))
}

func TestConnectorRejectsNamespaceFork(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	id := apid.New(apid.PrefixConnector)

	require.NoError(t, db.UpsertConnectorGeneration(ctx, testConnectorWithDefinition(id, "root", "connector", 1)))
	err := db.UpsertConnectorGeneration(ctx, testConnectorWithDefinition(id, "root.other", "", 2))
	require.ErrorContains(t, err, "cannot modify connector namespace")

	projected, err := db.GetConnectorGeneration(ctx, id, 1)
	require.NoError(t, err)
	require.Equal(t, "root", projected.Namespace)
	require.Equal(t, scommon.ResourceName("connector"), projected.Name)
}

func TestConnectorNameUniquenessAndDeleteReuse(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.other"}))

	firstID := apid.New(apid.PrefixConnector)
	require.NoError(t, db.UpsertConnectorGeneration(ctx, testConnectorWithDefinition(firstID, "root", "shared", 1)))

	conflictID := apid.New(apid.PrefixConnector)
	require.Error(t, db.UpsertConnectorGeneration(ctx, testConnectorWithDefinition(conflictID, "root", "shared", 1)))

	otherNamespaceID := apid.New(apid.PrefixConnector)
	require.NoError(t, db.UpsertConnectorGeneration(ctx, testConnectorWithDefinition(otherNamespaceID, "root.other", "shared", 1)))

	require.NoError(t, db.CreateActor(ctx, &Actor{
		Id:         apid.New(apid.PrefixActor),
		Namespace:  "root",
		Name:       "shared",
		ExternalId: "same-name-other-type",
	}))

	require.NoError(t, db.DeleteConnector(ctx, firstID))
	reusedID := apid.New(apid.PrefixConnector)
	require.NoError(t, db.UpsertConnectorGeneration(ctx, testConnectorWithDefinition(reusedID, "root", "shared", 1)))

	require.ErrorIs(t, db.UpdateConnectorName(ctx, firstID, "deleted"), ErrNotFound)
}

func TestConnectorNameExactListPaginationAndNamespaceRestrictions(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	require.NoError(t, db.EnsureNamespaceByPath(ctx, "root.allowed"))
	require.NoError(t, db.EnsureNamespaceByPath(ctx, "root.hidden"))

	allowedID := apid.New(apid.PrefixConnector)
	hiddenID := apid.New(apid.PrefixConnector)
	otherID := apid.New(apid.PrefixConnector)
	require.NoError(t, db.UpsertConnectorGeneration(ctx, testConnectorWithDefinition(allowedID, "root.allowed", "shared", 1)))
	require.NoError(t, db.UpsertConnectorGeneration(ctx, testConnectorWithDefinition(hiddenID, "root.hidden", "shared", 1)))
	require.NoError(t, db.UpsertConnectorGeneration(ctx, testConnectorWithDefinition(otherID, "root.allowed", "other", 1)))

	allowed := db.ListConnectorsBuilder().ForName("shared").ForNamespaceMatchers([]string{"root.allowed"}).FetchPage(ctx)
	require.NoError(t, allowed.Error)
	require.Len(t, allowed.Results, 1)
	require.Equal(t, allowedID, allowed.Results[0].Id)

	first := db.ListConnectorsBuilder().ForName("shared").ForNamespaceMatchers([]string{"root.**"}).Limit(1).FetchPage(ctx)
	require.NoError(t, first.Error)
	require.Len(t, first.Results, 1)
	require.NotEmpty(t, first.Cursor)
	executor, err := db.ListConnectorsFromCursor(ctx, first.Cursor)
	require.NoError(t, err)
	second := executor.FetchPage(ctx)
	require.NoError(t, second.Error)
	require.Len(t, second.Results, 1)
	require.ElementsMatch(t, []apid.ID{allowedID, hiddenID}, []apid.ID{first.Results[0].Id, second.Results[0].Id})

	generations := db.ListConnectorGenerationsBuilder().ForName("shared").ForNamespaceMatchers([]string{"root.**"}).FetchPage(ctx)
	require.NoError(t, generations.Error)
	require.Len(t, generations.Results, 2)
	for _, generation := range generations.Results {
		require.Equal(t, scommon.ResourceName("shared"), generation.Name)
	}

	err = db.UpdateConnectorName(ctx, otherID, "shared")
	require.ErrorIs(t, err, ErrDuplicate)
	unchanged, getErr := db.GetConnectorGeneration(ctx, otherID, 1)
	require.NoError(t, getErr)
	require.Equal(t, "other", unchanged.Labels["apxy/cxr/-/name"])
}

func TestConnectorGenerationLifecyclePreservesName(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := context.Background()
	id := apid.New(apid.PrefixConnector)

	require.NoError(t, db.UpsertConnectorGeneration(ctx, testConnectorWithDefinition(id, "root", "lifecycle", 1)))
	require.NoError(t, db.SetConnectorGenerationState(ctx, id, 1, ConnectorGenerationStatePrimary))
	require.NoError(t, db.SetConnectorGenerationState(ctx, id, 1, ConnectorGenerationStateArchived))

	projected, err := db.GetConnectorGeneration(ctx, id, 1)
	require.NoError(t, err)
	require.Equal(t, scommon.ResourceName("lifecycle"), projected.Name)
	require.Equal(t, ConnectorGenerationStateArchived, projected.State)
}

func TestConnectorMigrationBackfillsDeterministically(t *testing.T) {
	_, db, rawDB := MustApplyBlankTestDbConfigRaw(t, nil)
	service := db.(*service)
	migrateDatabaseToVersion(t, service, 15)

	liveID := apid.New(apid.PrefixConnector)
	deletedID := apid.New(apid.PrefixConnector)
	_, err := rawDB.Exec(fmt.Sprintf(`
		INSERT INTO connector_generations (
			id, generation, namespace, labels, annotations, state, hash, encrypted_definition,
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

	_, err = rawDB.Query("SELECT namespace FROM connector_generations")
	require.Error(t, err)
	_, err = rawDB.Query("SELECT labels FROM connector_generations")
	require.Error(t, err)
	_, err = rawDB.Query("SELECT annotations FROM connector_generations")
	require.Error(t, err)
	_, err = rawDB.Query("SELECT hash FROM connector_generations")
	require.Error(t, err)
	_, err = rawDB.Query("SELECT type FROM connector_generations")
	require.Error(t, err)
	var liveDefinitionCreatedAt, liveDefinitionUpdatedAt time.Time
	require.NoError(t, rawDB.QueryRow(fmt.Sprintf(
		"SELECT created_at, updated_at FROM connector_generations WHERE connector_id = '%s' AND generation = 2",
		liveID,
	)).Scan(&liveDefinitionCreatedAt, &liveDefinitionUpdatedAt))
	require.True(t, time.Date(2024, time.March, 1, 0, 0, 0, 0, time.UTC).Equal(liveDefinitionCreatedAt))
	require.True(t, time.Date(2024, time.March, 2, 0, 0, 0, 0, time.UTC).Equal(liveDefinitionUpdatedAt))
	var liveDefinitionDeletedAt any
	require.NoError(t, rawDB.QueryRow(fmt.Sprintf(
		"SELECT deleted_at FROM connector_generations WHERE connector_id = '%s' LIMIT 1",
		liveID,
	)).Scan(&liveDefinitionDeletedAt))
	require.Nil(t, liveDefinitionDeletedAt)

	var deletedDefinitionDeletedAt any
	require.NoError(t, rawDB.QueryRow(fmt.Sprintf(
		"SELECT deleted_at FROM connector_generations WHERE connector_id = '%s' LIMIT 1",
		deletedID,
	)).Scan(&deletedDefinitionDeletedAt))
	require.NotNil(t, deletedDefinitionDeletedAt)

	rows, err := rawDB.Query(fmt.Sprintf(
		"SELECT id FROM connector_generations WHERE connector_id = '%s' ORDER BY generation",
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
		require.Equal(t, apid.PrefixConnectorGeneration, definitionID.Prefix())
	}

	_, err = rawDB.Exec(fmt.Sprintf(`
		INSERT INTO connector_generations (
			id, connector_id, generation, state, encrypted_definition, created_at, updated_at
		) VALUES (
			'cvd_duplicate', '%s', 2, 'draft', '{"id":"dek_duplicate","d":"duplicate"}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)
	`, liveID))
	require.Error(t, err, "(connector_id, generation) must be unique")

	projected, err := db.GetConnectorGeneration(context.Background(), liveID, 2)
	require.NoError(t, err)
	require.Equal(t, "root.live", projected.Namespace)
	require.Equal(t, scommon.ResourceName(liveID.String()), projected.Name)

	// A connector that was already soft-deleted before the table split must
	// release its backfilled name after the upgrade. Reusing the name creates a
	// new logical connector without reviving or rewriting the old generations.
	replacementID := apid.New(apid.PrefixConnector)
	require.NoError(t, db.UpsertConnectorGeneration(
		context.Background(),
		testConnectorWithDefinition(replacementID, "root.deleted", scommon.ResourceName(deletedID.String()), 1),
	))
	replacement, err := db.GetConnectorGeneration(context.Background(), replacementID, 1)
	require.NoError(t, err)
	require.Equal(t, scommon.ResourceName(deletedID.String()), replacement.Name)
	require.Equal(t, 2, sqlhMustCountConnectorGenerations(t, rawDB, liveID))
	require.Equal(t, 1, sqlhMustCountConnectorGenerations(t, rawDB, replacementID))

	migrateDatabaseToVersion(t, service, 15)
	rows, err = rawDB.Query(fmt.Sprintf(
		"SELECT DISTINCT namespace FROM connector_generations WHERE id = '%s'",
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

func sqlhMustCountConnectorGenerations(t *testing.T, rawDB *sql.DB, id apid.ID) int {
	t.Helper()
	var count int
	require.NoError(t, rawDB.QueryRow(fmt.Sprintf(
		"SELECT COUNT(*) FROM connector_generations WHERE connector_id = '%s'",
		id,
	)).Scan(&count))
	return count
}

func connectorDefinitionPayloads(t *testing.T, rawDB *sql.DB, id apid.ID) []string {
	t.Helper()
	rows, err := rawDB.Query(fmt.Sprintf(
		"SELECT encrypted_definition FROM connector_generations WHERE connector_id = '%s' ORDER BY generation",
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
