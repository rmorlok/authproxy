package database

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/require"
)

func TestSearchResourcesRanksAndBoundsActorLabelMatches(t *testing.T) {
	_, db, raw := MustApplyBlankTestDbConfigRaw(t, nil)

	exactID := apid.New(apid.PrefixActor)
	prefixID := apid.New(apid.PrefixActor)
	substringID := apid.New(apid.PrefixActor)
	systemOnlyID := apid.New(apid.PrefixActor)
	deletedID := apid.New(apid.PrefixActor)

	inserts := []string{
		fmt.Sprintf(`INSERT INTO actors (id, namespace, labels, external_id, created_at, updated_at) VALUES ('%s', 'root', '{"team":"acme","literal":"has_value","percent":"rate%%value","slash":"path\\value","apxy/act/-/id":"%s"}', 'exact', CURRENT_TIMESTAMP, '2024-01-01T00:00:00Z')`, exactID, exactID),
		fmt.Sprintf(`INSERT INTO actors (id, namespace, labels, external_id, created_at, updated_at) VALUES ('%s', 'root', '{"team":"acme-prod"}', 'prefix', CURRENT_TIMESTAMP, '2025-01-01T00:00:00Z')`, prefixID),
		fmt.Sprintf(`INSERT INTO actors (id, namespace, labels, external_id, created_at, updated_at) VALUES ('%s', 'root', '{"team":"west-acme"}', 'substring', CURRENT_TIMESTAMP, '2026-01-01T00:00:00Z')`, substringID),
		fmt.Sprintf(`INSERT INTO actors (id, namespace, labels, external_id, created_at, updated_at) VALUES ('%s', 'root', '{"apxy/ns/-/ns":"root.acme"}', 'system-only', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, systemOnlyID),
		fmt.Sprintf(`INSERT INTO actors (id, namespace, labels, external_id, created_at, updated_at, deleted_at) VALUES ('%s', 'root', '{"team":"acme"}', 'deleted', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, deletedID),
	}
	for _, statement := range inserts {
		_, err := raw.Exec(statement)
		require.NoError(t, err)
	}

	result, err := db.SearchResources(t.Context(), SearchResourcesParams{
		ResourceType:      SearchResourceTypeActor,
		Query:             "AcMe",
		NamespaceMatchers: []string{"root.**"},
		Limit:             2,
	})
	require.NoError(t, err)
	require.True(t, result.Truncated)
	require.Len(t, result.Items, 2)
	require.Equal(t, exactID.String(), result.Items[0].ResourceID)
	require.Equal(t, 0, result.Items[0].MatchRank)
	require.Equal(t, prefixID.String(), result.Items[1].ResourceID)
	require.Equal(t, 1, result.Items[1].MatchRank)
	require.NotContains(t, result.Items[0].Labels, "apxy/act/-/id")

	for _, literalQuery := range []string{"_", "%", `\`} {
		t.Run("literal "+literalQuery, func(t *testing.T) {
			literal, err := db.SearchResources(t.Context(), SearchResourcesParams{
				ResourceType:      SearchResourceTypeActor,
				Query:             literalQuery,
				NamespaceMatchers: []string{"root.**"},
				Limit:             10,
			})
			require.NoError(t, err)
			require.Len(t, literal.Items, 1)
			require.Equal(t, exactID.String(), literal.Items[0].ResourceID)
		})
	}
}

func TestSearchResourcesCombinesTextSelectorAndNamespace(t *testing.T) {
	_, db, raw := MustApplyBlankTestDbConfigRaw(t, nil)

	matchID := apid.New(apid.PrefixConnection)
	wrongLabelID := apid.New(apid.PrefixConnection)
	wrongNamespaceID := apid.New(apid.PrefixConnection)
	for _, statement := range []string{
		fmt.Sprintf(`INSERT INTO connections (id, namespace, labels, state, connector_id, connector_version, created_at, updated_at) VALUES ('%s', 'root.team', '{"name":"payments-api","env":"prod"}', 'configured', 'cxr_test', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, matchID),
		fmt.Sprintf(`INSERT INTO connections (id, namespace, labels, state, connector_id, connector_version, created_at, updated_at) VALUES ('%s', 'root.team', '{"name":"payments-api","env":"dev"}', 'configured', 'cxr_test', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, wrongLabelID),
		fmt.Sprintf(`INSERT INTO connections (id, namespace, labels, state, connector_id, connector_version, created_at, updated_at) VALUES ('%s', 'root.other', '{"name":"payments-api","env":"prod"}', 'configured', 'cxr_test', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, wrongNamespaceID),
	} {
		_, err := raw.Exec(statement)
		require.NoError(t, err)
	}

	result, err := db.SearchResources(t.Context(), SearchResourcesParams{
		ResourceType:      SearchResourceTypeConnection,
		Query:             "payments",
		LabelSelector:     "env=prod",
		NamespaceMatchers: []string{"root.team.**"},
		Limit:             10,
	})
	require.NoError(t, err)
	require.False(t, result.Truncated)
	require.Len(t, result.Items, 1)
	require.Equal(t, matchID.String(), result.Items[0].ResourceID)
	require.ElementsMatch(t, []SearchLabelMatch{{Key: "env", Value: "prod"}, {Key: "name", Value: "payments-api"}}, result.Items[0].MatchedLabels)
}

func TestSearchResourcesCollapsesConnectorVersionsDeterministically(t *testing.T) {
	_, db, raw := MustApplyBlankTestDbConfigRaw(t, nil)

	connectorID := apid.New(apid.PrefixConnectorVersion)
	for _, statement := range []string{
		fmt.Sprintf(`INSERT INTO connector_versions (id, version, namespace, labels, state, type, created_at, updated_at) VALUES ('%s', 1, 'root', '{"name":"old-primary"}', 'primary', 'test', CURRENT_TIMESTAMP, '2024-01-01T00:00:00Z')`, connectorID),
		fmt.Sprintf(`INSERT INTO connector_versions (id, version, namespace, labels, state, type, created_at, updated_at) VALUES ('%s', 2, 'root', '{"name":"new-primary"}', 'primary', 'test', CURRENT_TIMESTAMP, '2025-01-01T00:00:00Z')`, connectorID),
		fmt.Sprintf(`INSERT INTO connector_versions (id, version, namespace, labels, state, type, created_at, updated_at) VALUES ('%s', 3, 'root', '{"name":"draft-match"}', 'draft', 'test', CURRENT_TIMESTAMP, '2026-01-01T00:00:00Z')`, connectorID),
	} {
		_, err := raw.Exec(statement)
		require.NoError(t, err)
	}

	result, err := db.SearchResources(t.Context(), SearchResourcesParams{
		ResourceType:      SearchResourceTypeConnector,
		Query:             "new-primary",
		NamespaceMatchers: []string{"root.**"},
		Limit:             10,
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, connectorID.String(), result.Items[0].ResourceID)

	notRepresentative, err := db.SearchResources(t.Context(), SearchResourcesParams{
		ResourceType:      SearchResourceTypeConnector,
		Query:             "draft-match",
		NamespaceMatchers: []string{"root.**"},
		Limit:             10,
	})
	require.NoError(t, err)
	require.Empty(t, notRepresentative.Items)
}

func TestSearchResourcesEmptyNamespaceMatchersNeverMeansUnrestricted(t *testing.T) {
	_, db, raw := MustApplyBlankTestDbConfigRaw(t, nil)
	id := apid.New(apid.PrefixActor)
	_, err := raw.Exec(fmt.Sprintf(`INSERT INTO actors (id, namespace, labels, external_id, created_at, updated_at) VALUES ('%s', 'root', '{"name":"visible"}', 'actor', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, id))
	require.NoError(t, err)

	result, err := db.SearchResources(t.Context(), SearchResourcesParams{
		ResourceType: SearchResourceTypeActor,
		Query:        "visible",
		Limit:        10,
	})
	require.NoError(t, err)
	require.Empty(t, result.Items)
}

func TestSearchResourcesSeedCoversNamespaceKeyAndRateLimit(t *testing.T) {
	_, db, raw := MustApplyBlankTestDbConfigRaw(t, nil)

	namespacePath := "root.seed"
	keyID := apid.New(apid.PrefixKey)
	rateLimitID := apid.New(apid.PrefixRateLimit)
	for _, statement := range []string{
		`INSERT INTO namespaces (path, depth, state, labels, created_at, updated_at) VALUES ('root.seed', 1, 'active', '{"name":"seed namespace","apxy/ns/-/ns":"root.seed"}', CURRENT_TIMESTAMP, '2024-01-01T00:00:00Z')`,
		fmt.Sprintf(`INSERT INTO keys (id, namespace, usage, material_type, state, labels, created_at, updated_at) VALUES ('%s', 'root.seed', 'data_encryption', 'symmetric', 'active', '{"name":"seed key","apxy/key/-/id":"%s"}', CURRENT_TIMESTAMP, '2025-01-01T00:00:00Z')`, keyID, keyID),
		fmt.Sprintf(`INSERT INTO rate_limits (id, namespace, definition, labels, created_at, updated_at) VALUES ('%s', 'root.seed', '{}', '{"name":"seed rate limit","apxy/rl/-/id":"%s"}', CURRENT_TIMESTAMP, '2026-01-01T00:00:00Z')`, rateLimitID, rateLimitID),
	} {
		_, err := raw.Exec(statement)
		require.NoError(t, err)
	}

	tests := []struct {
		resourceType SearchResourceType
		resourceID   string
		labels       Labels
	}{
		{resourceType: SearchResourceTypeNamespace, resourceID: namespacePath, labels: Labels{"name": "seed namespace"}},
		{resourceType: SearchResourceTypeKey, resourceID: keyID.String(), labels: Labels{"name": "seed key"}},
		{resourceType: SearchResourceTypeRateLimit, resourceID: rateLimitID.String(), labels: Labels{"name": "seed rate limit"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.resourceType), func(t *testing.T) {
			result, err := db.SearchResources(t.Context(), SearchResourcesParams{
				ResourceType:      tt.resourceType,
				NamespaceMatchers: []string{"root.**"},
				Limit:             50,
			})
			require.NoError(t, err)
			require.False(t, result.Truncated)

			var found *SearchResource
			for i := range result.Items {
				if result.Items[i].ResourceID == tt.resourceID {
					found = &result.Items[i]
					break
				}
			}
			require.NotNil(t, found)
			require.Equal(t, tt.resourceType, found.ResourceType)
			require.Equal(t, namespacePath, found.Namespace)
			require.Equal(t, tt.labels, found.Labels)
			require.Empty(t, found.MatchedLabels)
		})
	}
}

func TestParseLabelSelectorAllowsLongApxyValues(t *testing.T) {
	value := "root." + strings.Repeat("segment.", 12) + "tail"
	require.Greater(t, len(value), LabelValueMaxLength)

	tests := []struct {
		operator string
		expected LabelOperator
	}{
		{operator: "=", expected: LabelOperatorEqual},
		{operator: "==", expected: LabelOperatorEqual},
		{operator: "!=", expected: LabelOperatorNotEqual},
	}
	for _, tt := range tests {
		t.Run(tt.operator, func(t *testing.T) {
			selector, err := ParseLabelSelector("apxy/ns/-/ns" + tt.operator + value)
			require.NoError(t, err)
			require.Equal(t, LabelSelector{{Key: "apxy/ns/-/ns", Operator: tt.expected, Value: value}}, selector)

			_, err = ParseLabelSelector("ordinary" + tt.operator + value)
			require.Error(t, err)
		})
	}
}
