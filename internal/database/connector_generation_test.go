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

func TestConnectorGenerations(t *testing.T) {
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

INSERT INTO connector_generations
(id,                         connector_id,                generation, state,      encrypted_definition,                       created_at,            updated_at) VALUES
('cvd_testgmail0000011',     'cxr_testgmail0000001',      1,       'active',   '{"id":"dek_test","d":"encrypted-def"}', '2023-10-01 00:00:00', '2023-10-10 00:00:00'),
('cvd_testgmail0000012',     'cxr_testgmail0000001',      2,       'primary',  '{"id":"dek_test","d":"encrypted-def"}', '2023-10-01 01:00:00', '2023-10-10 01:00:00'),
('cvd_testgmail0000021',     'cxr_testgmail0000002',      1,       'archived', '{"id":"dek_test","d":"encrypted-def"}', '2023-10-02 00:00:00', '2023-10-11 00:00:00'),
('cvd_testgmail0000022',     'cxr_testgmail0000002',      2,       'primary',  '{"id":"dek_test","d":"encrypted-def"}', '2023-10-02 01:00:00', '2023-10-11 01:00:00'),
('cvd_testslack0000011',     'cxr_testslack0000001',      1,       'active',   '{"id":"dek_test","d":"encrypted-def"}', '2023-10-03 00:00:00', '2023-10-12 00:00:00'),
('cvd_testslack0000012',     'cxr_testslack0000001',      2,       'primary',  '{"id":"dek_test","d":"encrypted-def"}', '2023-10-03 01:00:00', '2023-10-12 01:00:00'),
('cvd_testgmail0000031',     'cxr_testgmail0000003',      1,       'archived', '{"id":"dek_test","d":"encrypted-def"}', '2023-10-04 00:00:00', '2023-10-14 00:00:00'),
('cvd_testgmail0000032',     'cxr_testgmail0000003',      2,       'active',   '{"id":"dek_test","d":"encrypted-def"}', '2023-10-04 01:00:00', '2023-10-14 01:00:00'),
('cvd_testgmail0000033',     'cxr_testgmail0000003',      3,       'primary',  '{"id":"dek_test","d":"encrypted-def"}', '2023-10-04 02:00:00', '2023-10-14 02:00:00');
`
		_, err := rawDb.Exec(sql)
		require.NoError(t, err)

		v, err := db.GetConnectorGeneration(ctx, apid.MustParse("cxr_testgmail0000001"), 1)
		require.NoError(t, err)
		require.Equal(t, "gmail", v.Labels["type"])
		require.Equal(t, ConnectorGenerationStateActive, v.State)

		results, err := db.GetConnectorGenerations(ctx, []ConnectorGenerationId{
			{apid.MustParse("cxr_testgmail0000001"), 1},
		})
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Equal(t, "gmail", results[ConnectorGenerationId{apid.MustParse("cxr_testgmail0000001"), 1}].Labels["type"])
		require.Equal(t, ConnectorGenerationStateActive, results[ConnectorGenerationId{apid.MustParse("cxr_testgmail0000001"), 1}].State)

		results, err = db.GetConnectorGenerations(ctx, []ConnectorGenerationId{
			{apid.MustParse("cxr_testgmail0000001"), 1},
			{apid.MustParse("cxr_testgmail0000002"), 2},
		})
		require.NoError(t, err)
		require.Len(t, results, 2)
		require.Equal(t, "gmail", results[ConnectorGenerationId{apid.MustParse("cxr_testgmail0000001"), 1}].Labels["type"])
		require.Equal(t, ConnectorGenerationStateActive, results[ConnectorGenerationId{apid.MustParse("cxr_testgmail0000001"), 1}].State)
		require.Equal(t, "gmail", results[ConnectorGenerationId{apid.MustParse("cxr_testgmail0000002"), 2}].Labels["type"])
		require.Equal(t, ConnectorGenerationStatePrimary, results[ConnectorGenerationId{apid.MustParse("cxr_testgmail0000002"), 2}].State)

		// Generation doesn't exist
		v, err = db.GetConnectorGeneration(ctx, apid.MustParse("cxr_testgmail0000001"), 99)
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, v)

		// UUID doesn't exist
		v, err = db.GetConnectorGeneration(ctx, apid.MustParse("cxr_testnotfound0001"), 1)
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, v)

		v, err = db.GetConnectorGenerationForState(ctx, apid.MustParse("cxr_testslack0000001"), ConnectorGenerationStatePrimary)
		require.NoError(t, err)
		require.Equal(t, "outlook", v.Labels["type"])
		require.Equal(t, ConnectorGenerationStatePrimary, v.State)

		v, err = db.GetConnectorGenerationForState(ctx, apid.MustParse("cxr_testslack0000001"), ConnectorGenerationStateArchived)
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, v)

		pr := db.ListConnectorGenerationsBuilder().
			ForLabelSelector("type=gmail").
			OrderBy(ConnectorGenerationOrderByCreatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 4)
		require.Equal(t, pr.Results[0].Id, apid.MustParse("cxr_testgmail0000002"))
		require.Equal(t, uint64(2), pr.Results[0].Generation)
		require.Equal(t, pr.Results[1].Id, apid.MustParse("cxr_testgmail0000002"))
		require.Equal(t, uint64(1), pr.Results[1].Generation)
		require.Equal(t, pr.Results[2].Id, apid.MustParse("cxr_testgmail0000001"))
		require.Equal(t, uint64(2), pr.Results[2].Generation)
		require.Equal(t, pr.Results[3].Id, apid.MustParse("cxr_testgmail0000001"))
		require.Equal(t, uint64(1), pr.Results[3].Generation)

		pr = db.ListConnectorGenerationsBuilder().
			ForNamespaceMatcher("root.child.**").
			OrderBy(ConnectorGenerationOrderByCreatedAt, pagination.OrderByAsc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 5)
		require.Equal(t, apid.MustParse("cxr_testgmail0000002"), pr.Results[0].Id)
		require.Equal(t, uint64(1), pr.Results[0].Generation)
		require.Equal(t, apid.MustParse("cxr_testgmail0000002"), pr.Results[1].Id)
		require.Equal(t, uint64(2), pr.Results[1].Generation)
		require.Equal(t, apid.MustParse("cxr_testgmail0000003"), pr.Results[2].Id)
		require.Equal(t, uint64(1), pr.Results[2].Generation)
		require.Equal(t, apid.MustParse("cxr_testgmail0000003"), pr.Results[3].Id)
		require.Equal(t, uint64(2), pr.Results[3].Generation)
		require.Equal(t, apid.MustParse("cxr_testgmail0000003"), pr.Results[4].Id)
		require.Equal(t, uint64(3), pr.Results[4].Generation)
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

INSERT INTO connector_generations
(id,                         connector_id,                generation, state,     encrypted_definition,                       created_at,            updated_at) VALUES
('cvd_testgmail0000011',     'cxr_testgmail0000001',      1,       'primary', '{"id":"dek_test","d":"encrypted-def"}', '2023-10-01 00:00:00', '2023-10-01 00:00:00'),
('cvd_testgmail0000021',     'cxr_testgmail0000002',      1,       'primary', '{"id":"dek_test","d":"encrypted-def"}', '2023-10-02 00:00:00', '2023-10-02 00:00:00'),
('cvd_testslack0000011',     'cxr_testslack0000001',      1,       'primary', '{"id":"dek_test","d":"encrypted-def"}', '2023-10-03 00:00:00', '2023-10-03 00:00:00'),
('cvd_testgmail0000031',     'cxr_testgmail0000003',      1,       'primary', '{"id":"dek_test","d":"encrypted-def"}', '2023-10-04 00:00:00', '2023-10-04 00:00:00');
`
		_, err := rawDb.Exec(sql)
		require.NoError(t, err)

		t.Run("empty matchers returns all", func(t *testing.T) {
			pr := db.ListConnectorGenerationsBuilder().
				ForNamespaceMatchers([]string{}).
				OrderBy(ConnectorGenerationOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 4)
		})

		t.Run("single exact matcher", func(t *testing.T) {
			pr := db.ListConnectorGenerationsBuilder().
				ForNamespaceMatchers([]string{"root.prod"}).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 1)
			require.Equal(t, apid.MustParse("cxr_testgmail0000001"), pr.Results[0].Id)
		})

		t.Run("single wildcard matcher", func(t *testing.T) {
			pr := db.ListConnectorGenerationsBuilder().
				ForNamespaceMatchers([]string{"root.prod.**"}).
				OrderBy(ConnectorGenerationOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 2)
			require.Equal(t, apid.MustParse("cxr_testgmail0000001"), pr.Results[0].Id)
			require.Equal(t, apid.MustParse("cxr_testgmail0000003"), pr.Results[1].Id)
		})

		t.Run("multiple exact matchers (OR logic)", func(t *testing.T) {
			pr := db.ListConnectorGenerationsBuilder().
				ForNamespaceMatchers([]string{"root.prod", "root.staging"}).
				OrderBy(ConnectorGenerationOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 2)
			require.Equal(t, apid.MustParse("cxr_testgmail0000001"), pr.Results[0].Id)
			require.Equal(t, apid.MustParse("cxr_testgmail0000002"), pr.Results[1].Id)
		})

		t.Run("multiple wildcard matchers (OR logic)", func(t *testing.T) {
			pr := db.ListConnectorGenerationsBuilder().
				ForNamespaceMatchers([]string{"root.prod.**", "root.staging.**"}).
				OrderBy(ConnectorGenerationOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 3)
		})

		t.Run("no matching namespaces", func(t *testing.T) {
			pr := db.ListConnectorGenerationsBuilder().
				ForNamespaceMatchers([]string{"root.nonexistent"}).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 0)
		})
	})
	t.Run("validation rejects wrong prefix on id", func(t *testing.T) {
		cv := &ConnectorWithDefinition{
			Id:                  apid.New(apid.PrefixActor), // wrong prefix
			Generation:          1,
			Namespace:           "root",
			State:               ConnectorGenerationStateDraft,
			EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "def"},
		}
		require.Error(t, cv.Validate())
	})
	t.Run("UpsertConnectorGeneration", func(t *testing.T) {
		t.Run("creates a new connector generation", func(t *testing.T) {
			// Setup
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create a new connector generation
			connectorID := apid.New(apid.PrefixConnector)
			cv := &ConnectorWithDefinition{
				Id:                  connectorID,
				Generation:          1,
				Namespace:           "root.some-namespace",
				State:               ConnectorGenerationStateDraft,
				Labels:              Labels{"type": "test_connector"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"},
			}

			// Test
			err := db.UpsertConnectorGeneration(ctx, cv)
			require.NoError(t, err)

			// Verify
			savedCV, err := db.GetConnectorGeneration(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV)
			assert.Equal(t, connectorID, savedCV.Id)
			assert.Equal(t, uint64(1), savedCV.Generation)
			assert.Equal(t, ConnectorGenerationStateDraft, savedCV.State)
			assert.Equal(t, "test_connector", savedCV.Labels["type"])
			assert.Equal(t, "root.some-namespace", savedCV.Namespace)
			require.True(t, savedCV.DefinitionGenerationId.HasPrefix(apid.PrefixConnectorGeneration))
			assert.Equal(t, encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"}, savedCV.EncryptedDefinition)
			require.True(t, now.Equal(savedCV.DefinitionCreatedAt))
			require.True(t, now.Equal(savedCV.DefinitionUpdatedAt))
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_generations"))
		})

		t.Run("refuses to create active and archived generations", func(t *testing.T) {
			// Setup
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create a new connector generation
			connectorID := apid.New(apid.PrefixConnector)
			cv := &ConnectorWithDefinition{
				Id:                  connectorID,
				Generation:          1,
				Namespace:           "root.some-namespace",
				State:               ConnectorGenerationStateActive,
				Labels:              Labels{"type": "test_connector"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"},
			}

			// Test
			err := db.UpsertConnectorGeneration(ctx, cv)
			require.Error(t, err) // Cannot create active directly (must be primary)
			require.Equal(t, 0, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_generations"))

			cv.State = ConnectorGenerationStateArchived
			err = db.UpsertConnectorGeneration(ctx, cv)
			require.Error(t, err) // Cannot create archived directly (must be primary)
			require.Equal(t, 0, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_generations"))
		})

		t.Run("updates an existing draft generation", func(t *testing.T) {
			// Setup
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			clk := clock.NewFakeClock(now)
			ctx := apctx.NewBuilderBackground().WithClock(clk).Build()

			// Create a new connector generation
			connectorID := apid.New(apid.PrefixConnector)
			cv := &ConnectorWithDefinition{
				Id:                  connectorID,
				Generation:          1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorGenerationStateDraft,
				Labels:              Labels{"type": "test_connector"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"},
			}

			// Test
			err := db.UpsertConnectorGeneration(ctx, cv)
			require.NoError(t, err)

			clk.Step(time.Hour)
			cv.EncryptedDefinition.Data = "updated_encrypted_definition"
			err = db.UpsertConnectorGeneration(ctx, cv)
			require.NoError(t, err)

			// Verify
			savedCV, err := db.GetConnectorGeneration(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV)
			assert.Equal(t, connectorID, savedCV.Id)
			assert.Equal(t, uint64(1), savedCV.Generation)
			assert.Equal(t, ConnectorGenerationStateDraft, savedCV.State)
			assert.Equal(t, "test_connector", savedCV.Labels["type"])
			assert.Equal(t, encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "updated_encrypted_definition"}, savedCV.EncryptedDefinition)
			require.True(t, now.Equal(savedCV.DefinitionCreatedAt))
			require.True(t, now.Add(time.Hour).Equal(savedCV.DefinitionUpdatedAt))
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_generations"))
		})

		t.Run("refuses to change namespace for draft generation", func(t *testing.T) {
			// Setup
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create a new connector generation
			connectorID := apid.New(apid.PrefixConnector)
			cv := &ConnectorWithDefinition{
				Id:                  connectorID,
				Generation:          1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorGenerationStateDraft,
				Labels:              Labels{"type": "test_connector"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"},
			}

			// Test
			err := db.UpsertConnectorGeneration(ctx, cv)
			require.NoError(t, err)

			// Verify
			savedCV, err := db.GetConnectorGeneration(ctx, connectorID, 1)
			require.NoError(t, err)
			assert.Equal(t, sconfig.RootNamespace, savedCV.Namespace)

			// Try to change namespace
			cv.Namespace = "root.some-other-namespace"
			err = db.UpsertConnectorGeneration(ctx, cv)
			require.Error(t, err)

			// Verify unchanged
			savedCV, err = db.GetConnectorGeneration(ctx, connectorID, 1)
			require.NoError(t, err)
			assert.Equal(t, sconfig.RootNamespace, savedCV.Namespace)
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_generations"))
		})

		t.Run("creates multiple generations of the same connector", func(t *testing.T) {
			// Setup
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create connector Id
			connectorID := apid.New(apid.PrefixConnector)

			// Create generation 1
			cv1 := &ConnectorWithDefinition{
				Id:                  connectorID,
				Generation:          1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorGenerationStateDraft,
				Labels:              Labels{"type": "test_connector_v1"},
				Annotations:         Annotations{"owner": "v1"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition_v1"},
			}

			err := db.UpsertConnectorGeneration(ctx, cv1)
			require.NoError(t, err)

			// Create generation 2
			cv2 := &ConnectorWithDefinition{
				Id:                  connectorID,
				Generation:          2,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorGenerationStateDraft,
				Labels:              Labels{"type": "test_connector_v2"},
				Annotations:         Annotations{"owner": "v2"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition_v2"},
			}

			err = db.UpsertConnectorGeneration(ctx, cv2)
			require.NoError(t, err)

			// Verify generation 1
			savedCV1, err := db.GetConnectorGeneration(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV1)
			assert.Equal(t, uint64(1), savedCV1.Generation)
			assert.Equal(t, "test_connector_v2", savedCV1.Labels["type"])
			assert.Equal(t, "v2", savedCV1.Annotations["owner"])

			// Verify generation 2
			savedCV2, err := db.GetConnectorGeneration(ctx, connectorID, 2)
			require.NoError(t, err)
			require.NotNil(t, savedCV2)
			assert.Equal(t, uint64(2), savedCV2.Generation)
			assert.Equal(t, savedCV1.Labels, savedCV2.Labels)
			assert.Equal(t, savedCV1.Annotations, savedCV2.Annotations)
			require.NotEqual(t, savedCV1.DefinitionGenerationId, savedCV2.DefinitionGenerationId)
			require.True(t, savedCV1.DefinitionGenerationId.HasPrefix(apid.PrefixConnectorGeneration))
			require.True(t, savedCV2.DefinitionGenerationId.HasPrefix(apid.PrefixConnectorGeneration))
			require.Equal(t, 2, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_generations"))
		})

		t.Run("creates a primary connector generation", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create connector Id
			connectorID := apid.New(apid.PrefixConnector)

			// Create a primary connector generation
			cv := &ConnectorWithDefinition{
				Id:                  connectorID,
				Generation:          1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorGenerationStatePrimary,
				Labels:              Labels{"type": "test_connector"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"},
			}

			err := db.UpsertConnectorGeneration(ctx, cv)
			require.NoError(t, err)

			// Verify
			savedCV, err := db.GetConnectorGeneration(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV)
			assert.Equal(t, ConnectorGenerationStatePrimary, savedCV.State)
		})

		t.Run("creates multiple primary generations and updates previous primary to active", func(t *testing.T) {
			// This test simulates what UpsertConnectorGeneration does when setting a new primary generation

			// Setup
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create connector Id
			connectorID := apid.New(apid.PrefixConnector)

			// Create generation 1 as primary
			cv1 := &ConnectorWithDefinition{
				Id:                  connectorID,
				Generation:          1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorGenerationStatePrimary,
				Labels:              Labels{"type": "test_connector"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition_v1"},
			}

			err := db.UpsertConnectorGeneration(ctx, cv1)
			require.NoError(t, err)

			// Verify generation 1 is primary
			savedCV1, err := db.GetConnectorGeneration(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV1)
			assert.Equal(t, ConnectorGenerationStatePrimary, savedCV1.State)

			// Create generation 2 as primary
			cv2 := &ConnectorWithDefinition{
				Id:                  connectorID,
				Generation:          2,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorGenerationStatePrimary,
				Labels:              Labels{"type": "test_connector"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition_v2"},
			}

			err = db.UpsertConnectorGeneration(ctx, cv2)
			require.NoError(t, err)

			// Verify generation 2 is primary
			savedCV2, err := db.GetConnectorGeneration(ctx, connectorID, 2)
			require.NoError(t, err)
			require.NotNil(t, savedCV2)
			assert.Equal(t, ConnectorGenerationStatePrimary, savedCV2.State)

			// Verify generation 1 is now active
			savedCV1, err = db.GetConnectorGeneration(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV1)
			assert.Equal(t, ConnectorGenerationStateActive, savedCV1.State)
		})
		t.Run("upsert does not resurrect a soft-deleted connector", func(t *testing.T) {
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnector)
			cv := &ConnectorWithDefinition{
				Id:                  connectorID,
				Generation:          1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorGenerationStateDraft,
				Labels:              Labels{"type": "test_connector"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"},
			}

			err := db.UpsertConnectorGeneration(ctx, cv)
			require.NoError(t, err)

			require.NoError(t, db.DeleteConnector(ctx, connectorID))

			cv2 := &ConnectorWithDefinition{
				Id:                  connectorID,
				Generation:          1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorGenerationStateDraft,
				Labels:              Labels{"type": "updated_connector"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "new_encrypted_definition"},
			}

			err = db.UpsertConnectorGeneration(ctx, cv2)
			require.Error(t, err)

			var encrypted encfield.EncryptedField
			err = rawDb.QueryRow(fmt.Sprintf(
				"SELECT encrypted_definition FROM connector_generations WHERE connector_id = '%s' AND generation = 1",
				connectorID,
			)).Scan(&encrypted)
			require.NoError(t, err)
			require.Equal(t, cv.EncryptedDefinition, encrypted)
		})

		t.Run("refuses to skip generation numbers", func(t *testing.T) {
			// This test simulates what UpsertConnectorGeneration does when setting a new primary generation

			// Setup
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			// Create connector Id
			connectorID := apid.New(apid.PrefixConnector)

			// Create generation 1 as primary
			cv1 := &ConnectorWithDefinition{
				Id:                  connectorID,
				Generation:          1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorGenerationStatePrimary,
				Labels:              Labels{"type": "test_connector"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition_v1"},
			}

			err := db.UpsertConnectorGeneration(ctx, cv1)
			require.NoError(t, err)

			// Verify generation 1 is primary
			savedCV1, err := db.GetConnectorGeneration(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV1)
			assert.Equal(t, ConnectorGenerationStatePrimary, savedCV1.State)

			// Create generation 2 as primary
			cv2 := &ConnectorWithDefinition{
				Id:                  connectorID,
				Generation:          3,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorGenerationStatePrimary,
				Labels:              Labels{"type": "test_connector"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition_v2"},
			}

			err = db.UpsertConnectorGeneration(ctx, cv2)
			require.Error(t, err)

			// Verify generation 1 is primary
			savedCV2, err := db.GetConnectorGeneration(ctx, connectorID, 1)
			require.NoError(t, err)
			require.NotNil(t, savedCV2)
			assert.Equal(t, ConnectorGenerationStatePrimary, savedCV2.State)

			// Verify generation wasn't created
			savedCV1, err = db.GetConnectorGeneration(ctx, connectorID, 3)
			require.ErrorIs(t, err, ErrNotFound)
			require.Nil(t, savedCV1)
		})
	})

	t.Run("SetConnectorGenerationState", func(t *testing.T) {
		t.Run("sets state successfully", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnector)
			cv := &ConnectorWithDefinition{
				Id:                  connectorID,
				Generation:          1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorGenerationStateDraft,
				Labels:              Labels{"type": "test_connector"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"},
			}

			err := db.UpsertConnectorGeneration(ctx, cv)
			require.NoError(t, err)

			err = db.SetConnectorGenerationState(ctx, connectorID, 1, ConnectorGenerationStatePrimary)
			require.NoError(t, err)

			saved, err := db.GetConnectorGeneration(ctx, connectorID, 1)
			require.NoError(t, err)
			assert.Equal(t, ConnectorGenerationStatePrimary, saved.State)
		})

		t.Run("returns not found for nonexistent generation", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.SetConnectorGenerationState(ctx, apid.New(apid.PrefixConnector), 1, ConnectorGenerationStatePrimary)
			require.ErrorIs(t, err, ErrNotFound)
		})

		t.Run("rejects invalid state", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.SetConnectorGenerationState(ctx, apid.New(apid.PrefixConnector), 1, ConnectorGenerationState("invalid"))
			require.Error(t, err)
		})

		t.Run("demotes existing primary to active when forcing new primary", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnector)

			// Create v1 as primary
			err := db.UpsertConnectorGeneration(ctx, &ConnectorWithDefinition{
				Id: connectorID, Generation: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorGenerationStatePrimary, Labels: Labels{"type": "t"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e1"},
			})
			require.NoError(t, err)

			// Create v2 as draft
			err = db.UpsertConnectorGeneration(ctx, &ConnectorWithDefinition{
				Id: connectorID, Generation: 2, Namespace: sconfig.RootNamespace,
				State: ConnectorGenerationStateDraft, Labels: Labels{"type": "t"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e2"},
			})
			require.NoError(t, err)

			// Force v2 to primary
			err = db.SetConnectorGenerationState(ctx, connectorID, 2, ConnectorGenerationStatePrimary)
			require.NoError(t, err)

			// v2 should be primary
			v2, err := db.GetConnectorGeneration(ctx, connectorID, 2)
			require.NoError(t, err)
			assert.Equal(t, ConnectorGenerationStatePrimary, v2.State)

			// v1 should have been demoted to active
			v1, err := db.GetConnectorGeneration(ctx, connectorID, 1)
			require.NoError(t, err)
			assert.Equal(t, ConnectorGenerationStateActive, v1.State)
		})

		t.Run("returns not found for a generation of a soft-deleted connector", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnector)
			cv := &ConnectorWithDefinition{
				Id:                  connectorID,
				Generation:          1,
				Namespace:           sconfig.RootNamespace,
				State:               ConnectorGenerationStateDraft,
				Labels:              Labels{"type": "test_connector"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "test_encrypted_definition"},
			}

			err := db.UpsertConnectorGeneration(ctx, cv)
			require.NoError(t, err)

			require.NoError(t, db.DeleteConnector(ctx, connectorID))

			err = db.SetConnectorGenerationState(ctx, connectorID, 1, ConnectorGenerationStatePrimary)
			require.ErrorIs(t, err, ErrNotFound)
		})

		t.Run("archives existing draft when forcing new draft", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnector)

			// Create v1 as primary
			err := db.UpsertConnectorGeneration(ctx, &ConnectorWithDefinition{
				Id: connectorID, Generation: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorGenerationStatePrimary, Labels: Labels{"type": "t"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e1"},
			})
			require.NoError(t, err)

			// Create v2 as draft
			err = db.UpsertConnectorGeneration(ctx, &ConnectorWithDefinition{
				Id: connectorID, Generation: 2, Namespace: sconfig.RootNamespace,
				State: ConnectorGenerationStateDraft, Labels: Labels{"type": "t"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e2"},
			})
			require.NoError(t, err)

			// Force v1 to draft
			err = db.SetConnectorGenerationState(ctx, connectorID, 1, ConnectorGenerationStateDraft)
			require.NoError(t, err)

			// v1 should be draft
			v1, err := db.GetConnectorGeneration(ctx, connectorID, 1)
			require.NoError(t, err)
			assert.Equal(t, ConnectorGenerationStateDraft, v1.State)

			// v2 should have been archived
			v2, err := db.GetConnectorGeneration(ctx, connectorID, 2)
			require.NoError(t, err)
			assert.Equal(t, ConnectorGenerationStateArchived, v2.State)
		})
	})

	t.Run("DeleteConnector", func(t *testing.T) {
		t.Run("soft-deletes the connector while retaining definition history", func(t *testing.T) {
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			defer rawDb.Close()
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnector)

			err := db.UpsertConnectorGeneration(ctx, &ConnectorWithDefinition{
				Id: connectorID, Generation: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorGenerationStatePrimary, Labels: Labels{"type": "t"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e1"},
			})
			require.NoError(t, err)

			err = db.UpsertConnectorGeneration(ctx, &ConnectorWithDefinition{
				Id: connectorID, Generation: 2, Namespace: sconfig.RootNamespace,
				State: ConnectorGenerationStateDraft, Labels: Labels{"type": "t"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e2"},
			})
			require.NoError(t, err)

			require.Equal(t, 2, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_generations"))
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connectors WHERE deleted_at IS NULL"))

			err = db.DeleteConnector(ctx, connectorID)
			require.NoError(t, err)

			require.Equal(t, 2, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_generations"))
			require.Equal(t, 2, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_generations WHERE deleted_at IS NOT NULL"))
			require.Equal(t, 0, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connectors WHERE deleted_at IS NULL"))
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connectors WHERE deleted_at IS NOT NULL"))

			rows, err := rawDb.Query("SELECT deleted_at FROM connector_generations ORDER BY generation")
			require.NoError(t, err)
			defer rows.Close()
			for rows.Next() {
				var deletedAt time.Time
				require.NoError(t, rows.Scan(&deletedAt))
				require.True(t, now.Equal(deletedAt))
			}
			require.NoError(t, rows.Err())

			_, err = db.GetConnectorGeneration(ctx, connectorID, 1)
			require.ErrorIs(t, err, ErrNotFound)
			_, err = db.GetConnectorGeneration(ctx, connectorID, 2)
			require.ErrorIs(t, err, ErrNotFound)
		})

		t.Run("returns ErrNotFound when no live generations exist", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.DeleteConnector(ctx, apid.New(apid.PrefixConnector))
			require.ErrorIs(t, err, ErrNotFound)
		})

		t.Run("returns ErrNotFound when the connector is already soft-deleted", func(t *testing.T) {
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			defer rawDb.Close()
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnector)
			err := db.UpsertConnectorGeneration(ctx, &ConnectorWithDefinition{
				Id: connectorID, Generation: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorGenerationStatePrimary, Labels: Labels{"type": "t"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e1"},
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

			targetID := apid.New(apid.PrefixConnector)
			survivorID := apid.New(apid.PrefixConnector)

			err := db.UpsertConnectorGeneration(ctx, &ConnectorWithDefinition{
				Id: targetID, Generation: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorGenerationStatePrimary, Labels: Labels{"type": "t1"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e1"},
			})
			require.NoError(t, err)

			err = db.UpsertConnectorGeneration(ctx, &ConnectorWithDefinition{
				Id: survivorID, Generation: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorGenerationStatePrimary, Labels: Labels{"type": "t2"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e2"},
			})
			require.NoError(t, err)

			err = db.DeleteConnector(ctx, targetID)
			require.NoError(t, err)

			survivor, err := db.GetConnectorGeneration(ctx, survivorID, 1)
			require.NoError(t, err)
			require.Equal(t, survivorID, survivor.Id)

			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connectors WHERE deleted_at IS NULL"))
			require.Equal(t, 2, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_generations"))
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_generations WHERE deleted_at IS NULL"))
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM connector_generations WHERE deleted_at IS NOT NULL"))
		})

		t.Run("rejects nil id", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			ctx := apctx.NewBuilderBackground().Build()
			require.Error(t, db.DeleteConnector(ctx, apid.Nil))
		})
	})

	t.Run("UpsertConnectorGeneration apxy label semantics", func(t *testing.T) {
		t.Run("caller-supplied apxy labels persist on insert", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnector)
			err := db.UpsertConnectorGeneration(ctx, &ConnectorWithDefinition{
				Id: connectorID, Generation: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorGenerationStatePrimary,
				Labels: Labels{
					"type":            "t",
					"apxy/cxr/source": "config",
					"apxy/cxr/-/name": "caller-cannot-override",
				},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e1"},
			})
			require.NoError(t, err)

			saved, err := db.GetConnectorGeneration(ctx, connectorID, 1)
			require.NoError(t, err)
			require.Equal(t, "config", saved.Labels["apxy/cxr/source"])
			require.Equal(t, "t", saved.Labels["type"])
			// self-implicit labels still injected
			require.Equal(t, string(connectorID), saved.Labels["apxy/cxr/-/id"])
			require.Equal(t, string(connectorID), saved.Labels["apxy/cxr/-/name"])
		})

		t.Run("caller-supplied apxy labels override stored apxy on update", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnector)

			// Insert as draft with one apxy value.
			err := db.UpsertConnectorGeneration(ctx, &ConnectorWithDefinition{
				Id: connectorID, Generation: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorGenerationStateDraft,
				Labels: Labels{
					"type":            "t",
					"apxy/cxr/source": "api",
				},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e1"},
			})
			require.NoError(t, err)

			// Update the draft with a different apxy value for the same key.
			err = db.UpsertConnectorGeneration(ctx, &ConnectorWithDefinition{
				Id: connectorID, Generation: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorGenerationStateDraft,
				Labels: Labels{
					"type":            "t",
					"apxy/cxr/source": "config",
				},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e2"},
			})
			require.NoError(t, err)

			saved, err := db.GetConnectorGeneration(ctx, connectorID, 1)
			require.NoError(t, err)
			require.Equal(t, "config", saved.Labels["apxy/cxr/source"], "caller's apxy value must override stored")
			// self-implicit labels stay intact
			require.Equal(t, string(connectorID), saved.Labels["apxy/cxr/-/id"])
			require.Equal(t, string(connectorID), saved.Labels["apxy/cxr/-/name"])
		})

		t.Run("stored apxy labels not in caller are preserved on update", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			connectorID := apid.New(apid.PrefixConnector)

			// Insert as draft with an apxy value the caller will not pass on update.
			err := db.UpsertConnectorGeneration(ctx, &ConnectorWithDefinition{
				Id: connectorID, Generation: 1, Namespace: sconfig.RootNamespace,
				State: ConnectorGenerationStateDraft,
				Labels: Labels{
					"type":            "t",
					"apxy/cxr/source": "config",
				},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e1"},
			})
			require.NoError(t, err)

			// Update with only user labels — stored apxy/cxr/source must survive.
			err = db.UpsertConnectorGeneration(ctx, &ConnectorWithDefinition{
				Id: connectorID, Generation: 1, Namespace: sconfig.RootNamespace,
				State:               ConnectorGenerationStateDraft,
				Labels:              Labels{"type": "t-changed"},
				EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "e2"},
			})
			require.NoError(t, err)

			saved, err := db.GetConnectorGeneration(ctx, connectorID, 1)
			require.NoError(t, err)
			require.Equal(t, "config", saved.Labels["apxy/cxr/source"], "stored apxy label must survive an update that omits it")
			require.Equal(t, "t-changed", saved.Labels["type"])
		})
	})
}
