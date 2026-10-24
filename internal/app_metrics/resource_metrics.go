package app_metrics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/rmorlok/authproxy/internal/database"
)

type ResourceMetric string

const (
	ResourceMetricConnectionsCount          ResourceMetric = "resources.connections.count"
	ResourceMetricActorsCount               ResourceMetric = "resources.actors.count"
	ResourceMetricConnectorsCount           ResourceMetric = "resources.connectors.count"
	ResourceMetricConnectorGenerationsCount ResourceMetric = "resources.connector_generations.count"
	ResourceMetricNamespacesCount           ResourceMetric = "resources.namespaces.count"
	ResourceMetricRateLimitsCount           ResourceMetric = "resources.rate_limits.count"
)

type ResourceGroupBy string

const (
	ResourceGroupByState               ResourceGroupBy = "state"
	ResourceGroupByHealthState         ResourceGroupBy = "health_state"
	ResourceGroupByConnectorID         ResourceGroupBy = "connector_id"
	ResourceGroupByConnectorGeneration ResourceGroupBy = "connector_generation"
	ResourceGroupByNamespace           ResourceGroupBy = "namespace"
	ResourceGroupByMode                ResourceGroupBy = "mode"
)

type ResourceMetricsQuery struct {
	RefID             string
	Metric            ResourceMetric
	Start             time.Time
	End               time.Time
	Step              time.Duration
	NamespaceMatchers []string
	LabelSelector     string
	GroupBy           []ResourceGroupBy
}

type ResourceMetricPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type ResourceMetricSeries struct {
	RefID  string                `json:"refId"`
	Labels map[string]string     `json:"labels,omitempty"`
	Points []ResourceMetricPoint `json:"points"`
}

func validateResourceMetricsQuery(q ResourceMetricsQuery) error {
	if q.RefID == "" {
		return errors.New("ref_id is required")
	}
	if !isValidResourceMetric(q.Metric) {
		return fmt.Errorf("unsupported resource metric %q", q.Metric)
	}
	if q.Start.IsZero() || q.End.IsZero() {
		return errors.New("start and end are required")
	}
	if !q.Start.Before(q.End) {
		return errors.New("start must be before end")
	}
	if q.Step <= 0 {
		return errors.New("step must be greater than zero")
	}
	// Buckets are computed in SQL on millisecond timestamps, so the step must be expressible in whole milliseconds.
	if q.Step%time.Millisecond != 0 {
		return errors.New("step must be a whole number of milliseconds")
	}
	for _, groupBy := range q.GroupBy {
		if !isValidResourceGroupBy(q.Metric, groupBy) {
			return fmt.Errorf("unsupported resource group_by dimension %q for metric %q", groupBy, q.Metric)
		}
	}
	if q.LabelSelector != "" {
		if _, err := database.ParseLabelSelector(q.LabelSelector); err != nil {
			return err
		}
	}
	filters := ListFilters{}
	if len(q.NamespaceMatchers) > 0 {
		if err := filters.SetNamespaceMatchers(q.NamespaceMatchers); err != nil {
			return err
		}
	}
	return nil
}

func isValidResourceMetric(metric ResourceMetric) bool {
	switch metric {
	case ResourceMetricConnectionsCount,
		ResourceMetricActorsCount,
		ResourceMetricConnectorsCount,
		ResourceMetricConnectorGenerationsCount,
		ResourceMetricNamespacesCount,
		ResourceMetricRateLimitsCount:
		return true
	default:
		return false
	}
}

func isValidResourceGroupBy(metric ResourceMetric, groupBy ResourceGroupBy) bool {
	return IsValidResourceGroupBy(metric, groupBy)
}

func IsValidResourceGroupBy(metric ResourceMetric, groupBy ResourceGroupBy) bool {
	switch metric {
	case ResourceMetricConnectionsCount:
		switch groupBy {
		case ResourceGroupByState,
			ResourceGroupByHealthState,
			ResourceGroupByConnectorID,
			ResourceGroupByConnectorGeneration:
			return true
		}
	case ResourceMetricActorsCount:
		return groupBy == ResourceGroupByNamespace
	case ResourceMetricConnectorsCount:
		switch groupBy {
		case ResourceGroupByState,
			ResourceGroupByConnectorGeneration,
			ResourceGroupByNamespace:
			return true
		}
	case ResourceMetricConnectorGenerationsCount:
		switch groupBy {
		case ResourceGroupByState,
			ResourceGroupByConnectorID,
			ResourceGroupByConnectorGeneration,
			ResourceGroupByNamespace:
			return true
		}
	case ResourceMetricNamespacesCount:
		switch groupBy {
		case ResourceGroupByState,
			ResourceGroupByNamespace:
			return true
		}
	case ResourceMetricRateLimitsCount:
		switch groupBy {
		case ResourceGroupByMode,
			ResourceGroupByNamespace:
			return true
		}
	}
	return false
}

// resourceMetricSpec describes where a resource metric's samples live and how its group-by dimensions map to columns.
// It is the single source of truth used to build aggregate SQL for every provider; only identifiers from these specs
// are ever interpolated into queries.
type resourceMetricSpec struct {
	// table is the sample table queried for the metric.
	table string
	// identityColumns identify one distinct resource. A resource sampled several times inside a bucket is counted
	// once. Connector generations are identified by (resource_id, connector_generation).
	identityColumns []string
	// groupByColumns maps each group-by dimension supported by the metric to the column holding its value.
	groupByColumns map[ResourceGroupBy]string
}

// resourceMetricSpecs holds the spec for every supported resource metric.
var resourceMetricSpecs = map[ResourceMetric]resourceMetricSpec{
	ResourceMetricConnectionsCount: {
		table:           connectionResourceSamplesTable,
		identityColumns: []string{"resource_id"},
		groupByColumns: map[ResourceGroupBy]string{
			ResourceGroupByState:               "state",
			ResourceGroupByHealthState:         "health_state",
			ResourceGroupByConnectorID:         "connector_id",
			ResourceGroupByConnectorGeneration: "connector_generation",
		},
	},
	ResourceMetricActorsCount: {
		table:           actorResourceSamplesTable,
		identityColumns: []string{"resource_id"},
		groupByColumns: map[ResourceGroupBy]string{
			ResourceGroupByNamespace: "namespace",
		},
	},
	ResourceMetricConnectorsCount: {
		table:           connectorResourceSamplesTable,
		identityColumns: []string{"resource_id"},
		groupByColumns: map[ResourceGroupBy]string{
			ResourceGroupByState:               "state",
			ResourceGroupByConnectorGeneration: "connector_generation",
			ResourceGroupByNamespace:           "namespace",
		},
	},
	ResourceMetricConnectorGenerationsCount: {
		table:           connectorGenerationSamplesTable,
		identityColumns: []string{"resource_id", "connector_generation"},
		groupByColumns: map[ResourceGroupBy]string{
			ResourceGroupByState: "state",
			// A connector generation's resource_id is the connector's id.
			ResourceGroupByConnectorID:         "resource_id",
			ResourceGroupByConnectorGeneration: "connector_generation",
			ResourceGroupByNamespace:           "namespace",
		},
	},
	ResourceMetricNamespacesCount: {
		table:           namespaceResourceSamplesTable,
		identityColumns: []string{"resource_id"},
		groupByColumns: map[ResourceGroupBy]string{
			ResourceGroupByState:     "state",
			ResourceGroupByNamespace: "namespace",
		},
	},
	ResourceMetricRateLimitsCount: {
		table:           rateLimitResourceSamplesTable,
		identityColumns: []string{"resource_id"},
		groupByColumns: map[ResourceGroupBy]string{
			ResourceGroupByMode:      "mode",
			ResourceGroupByNamespace: "namespace",
		},
	},
}

// resourceMetricAggregate is one pre-aggregated row returned by a provider for a resource metrics query: the number of
// distinct resources sampled in one time bucket for one combination of group-by values.
type resourceMetricAggregate struct {
	// BucketStartMS is the start of the time bucket in Unix milliseconds, aligned to the query's Start and Step.
	BucketStartMS int64
	// Labels holds the values of the query's group-by dimensions for this row, keyed by dimension name.
	Labels map[string]string
	// Count is the number of distinct resources in this bucket and group.
	Count int64
}

// executeResourceMetricsQueries validates each query, fetches its aggregate rows through the provider-specific fetch
// function, and converts them into zero-filled time series. The first validation or fetch error aborts the batch.
func executeResourceMetricsQueries(
	ctx context.Context,
	queries []ResourceMetricsQuery,
	fetch func(context.Context, ResourceMetricsQuery) ([]resourceMetricAggregate, error),
) ([]ResourceMetricSeries, error) {
	out := make([]ResourceMetricSeries, 0)
	for _, query := range queries {
		if err := validateResourceMetricsQuery(query); err != nil {
			return nil, err
		}
		rows, err := fetch(ctx, query)
		if err != nil {
			return nil, err
		}
		out = append(out, buildResourceMetricSeries(query, rows)...)
	}
	return out, nil
}

// buildResourceMetricSeries converts aggregate rows into one series per distinct label set, with a point for every
// bucket in [Start, End) (empty buckets are zero). Rows whose bucket falls outside that range are dropped; this covers
// samples taken exactly at End, which the inclusive sampled_at filter still returns.
func buildResourceMetricSeries(query ResourceMetricsQuery, rows []resourceMetricAggregate) []ResourceMetricSeries {
	bucketCount := int(math.Ceil(float64(query.End.Sub(query.Start)) / float64(query.Step)))
	if bucketCount < 1 {
		bucketCount = 1
	}
	startMS := query.Start.UnixMilli()
	stepMS := query.Step.Milliseconds()

	countsByGroup := map[string][]float64{}
	labelsByKey := map[string]map[string]string{}

	if len(query.GroupBy) == 0 {
		key := resourceMetricGroupKey(nil)
		countsByGroup[key] = make([]float64, bucketCount)
		labelsByKey[key] = map[string]string{}
	}

	for _, row := range rows {
		if row.BucketStartMS < startMS || row.BucketStartMS >= query.End.UnixMilli() {
			continue
		}
		bucketIdx := int((row.BucketStartMS - startMS) / stepMS)
		if bucketIdx < 0 || bucketIdx >= bucketCount {
			continue
		}
		key := resourceMetricGroupKey(row.Labels)
		if _, ok := countsByGroup[key]; !ok {
			countsByGroup[key] = make([]float64, bucketCount)
			labelsByKey[key] = row.Labels
		}
		// Each (bucket, group) row is already a distinct count, and the database returns at most one per key.
		countsByGroup[key][bucketIdx] += float64(row.Count)
	}

	keys := make([]string, 0, len(countsByGroup))
	for key := range countsByGroup {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	series := make([]ResourceMetricSeries, 0, len(keys))
	for _, key := range keys {
		points := make([]ResourceMetricPoint, bucketCount)
		for i := range bucketCount {
			points[i] = ResourceMetricPoint{
				Timestamp: query.Start.Add(time.Duration(i) * query.Step),
				Value:     countsByGroup[key][i],
			}
		}
		series = append(series, ResourceMetricSeries{
			RefID:  query.RefID,
			Labels: labelsByKey[key],
			Points: points,
		})
	}
	return series
}

func resourceMetricGroupKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := ""
	for _, key := range keys {
		out += key + "=" + labels[key] + "\x00"
	}
	return out
}

func resourceMetricsSampleQuery(query ResourceMetricsQuery) ResourceSampleQuery {
	return ResourceSampleQuery{
		Start:             &query.Start,
		End:               &query.End,
		NamespaceMatchers: query.NamespaceMatchers,
		LabelSelector:     query.LabelSelector,
	}
}
