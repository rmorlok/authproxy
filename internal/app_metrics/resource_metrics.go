package app_metrics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
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

// ResourceMetricsQuery asks for a gauge time series over periodic resource snapshots. Each bucket of [Start, End)
// reports the counts from the most recent snapshot taken before the bucket ends, so values carry forward across
// buckets that contain no snapshot of their own.
type ResourceMetricsQuery struct {
	RefID             string
	Metric            ResourceMetric
	Start             time.Time
	End               time.Time
	Step              time.Duration
	NamespaceMatchers []string
	LabelSelector     string
	GroupBy           []ResourceGroupBy

	// SampleInterval is the cadence at which the resource snapshot task writes samples. It bounds how far back a
	// snapshot may be carried forward into a bucket; see maxSampleAge. Zero means the configured default
	// (appMetrics.resourceSnapshotInterval's default). StorageService fills it from configuration.
	SampleInterval time.Duration
}

// sampleInterval returns the snapshot cadence for the query, falling back to the configuration default.
func (q ResourceMetricsQuery) sampleInterval() time.Duration {
	if q.SampleInterval > 0 {
		return q.SampleInterval
	}
	return (&sconfig.AppMetrics{}).GetResourceSnapshotInterval()
}

// maxSampleAge is how old the latest snapshot may be, relative to the end of a bucket, and still supply that
// bucket's value. Snapshots are stamped with their run time truncated to the interval, and runs are scheduled every
// interval from worker start rather than on clock boundaries, so just before a run the newest snapshot's stamp can be
// nearly two intervals old. Half an interval more absorbs scheduling and execution delay. Beyond this age the bucket
// is reported as having no data (e.g. while the worker is down) rather than repeating a stale value.
func (q ResourceMetricsQuery) maxSampleAge() time.Duration {
	interval := q.sampleInterval()
	return 2*interval + interval/2
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
	if q.SampleInterval < 0 {
		return errors.New("sample interval must not be negative")
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
// distinct resources in one snapshot for one combination of group-by values.
type resourceMetricAggregate struct {
	// SampledAtMS identifies the snapshot (its sampled_at) in Unix milliseconds.
	SampledAtMS int64
	// Labels holds the values of the query's group-by dimensions for this row, keyed by dimension name.
	Labels map[string]string
	// Count is the number of distinct resources in this snapshot and group.
	Count int64
}

// resourceMetricFetcher is the provider-specific data access used to evaluate resource metrics queries.
type resourceMetricFetcher interface {
	// fetchSampleTimes returns the distinct sampled_at values (Unix ms, ascending) of the metric's sample table in
	// [from, to]. It deliberately ignores the query's namespace and label filters: a snapshot in which no resource
	// matches the filters must still count as a snapshot (with a value of zero) rather than as missing data.
	fetchSampleTimes(ctx context.Context, query ResourceMetricsQuery, from, to time.Time) ([]int64, error)
	// fetchAggregates returns distinct-resource counts per snapshot and group for the given snapshots, applying the
	// query's filters.
	fetchAggregates(ctx context.Context, query ResourceMetricsQuery, sampledAtMS []int64) ([]resourceMetricAggregate, error)
}

// executeResourceMetricsQueries validates each query, resolves which snapshot supplies each bucket, fetches counts for
// those snapshots through the provider, and converts them into time series. The first validation or fetch error
// aborts the batch.
func executeResourceMetricsQueries(
	ctx context.Context,
	queries []ResourceMetricsQuery,
	fetcher resourceMetricFetcher,
) ([]ResourceMetricSeries, error) {
	out := make([]ResourceMetricSeries, 0)
	for _, query := range queries {
		if err := validateResourceMetricsQuery(query); err != nil {
			return nil, err
		}

		// Look back far enough that the first buckets can carry forward a snapshot taken before Start.
		sampleTimes, err := fetcher.fetchSampleTimes(ctx, query, query.Start.Add(-query.maxSampleAge()), query.End)
		if err != nil {
			return nil, err
		}
		bucketSnapshots := selectResourceMetricBucketSnapshots(query, sampleTimes)

		var rows []resourceMetricAggregate
		if needed := distinctResourceMetricSnapshots(bucketSnapshots); len(needed) > 0 {
			rows, err = fetcher.fetchAggregates(ctx, query, needed)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, buildResourceMetricSeries(query, bucketSnapshots, rows)...)
	}
	return out, nil
}

// resourceMetricBucketCount is the number of Step-sized buckets needed to cover [Start, End); the last bucket may
// extend past End.
func resourceMetricBucketCount(query ResourceMetricsQuery) int {
	bucketCount := int(math.Ceil(float64(query.End.Sub(query.Start)) / float64(query.Step)))
	if bucketCount < 1 {
		bucketCount = 1
	}
	return bucketCount
}

// selectResourceMetricBucketSnapshots picks, for each bucket, the snapshot whose counts represent it: the latest
// snapshot taken before the bucket ends (or before End, for the final partial bucket) that is no older than
// maxSampleAge. Using the latest snapshot rather than every snapshot inside the bucket keeps the value a gauge reading,
// and allowing older snapshots fills buckets that are shorter than, or misaligned with, the snapshot cadence.
//
// sampleTimes must be sorted ascending. The result has one entry per bucket; -1 marks a bucket with no usable
// snapshot.
func selectResourceMetricBucketSnapshots(query ResourceMetricsQuery, sampleTimes []int64) []int64 {
	bucketCount := resourceMetricBucketCount(query)
	startMS := query.Start.UnixMilli()
	endMS := query.End.UnixMilli()
	stepMS := query.Step.Milliseconds()
	maxAgeMS := query.maxSampleAge().Milliseconds()

	selected := make([]int64, bucketCount)
	next := 0 // index of the first sample time not yet known to precede the current bucket's end
	for i := range bucketCount {
		bucketEndMS := min(startMS+int64(i+1)*stepMS, endMS)
		for next < len(sampleTimes) && sampleTimes[next] < bucketEndMS {
			next++
		}
		selected[i] = -1
		if next > 0 && bucketEndMS-sampleTimes[next-1] <= maxAgeMS {
			selected[i] = sampleTimes[next-1]
		}
	}
	return selected
}

// distinctResourceMetricSnapshots returns the distinct snapshot times referenced by bucketSnapshots, ignoring buckets
// without a snapshot. Adjacent buckets often share a snapshot, so this is usually much shorter than the bucket list.
func distinctResourceMetricSnapshots(bucketSnapshots []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0)
	for _, ts := range bucketSnapshots {
		if ts >= 0 && !seen[ts] {
			seen[ts] = true
			out = append(out, ts)
		}
	}
	return out
}

// buildResourceMetricSeries converts per-snapshot aggregate rows into one series per distinct label set. Each bucket
// with a selected snapshot gets a point holding that snapshot's count for the group, or zero when the group had no
// resources in that snapshot. Buckets without a usable snapshot get no point at all, so clients render a gap
// instead of a false drop to zero. An ungrouped query always yields one series, possibly with no points.
func buildResourceMetricSeries(query ResourceMetricsQuery, bucketSnapshots []int64, rows []resourceMetricAggregate) []ResourceMetricSeries {
	countsByGroup := map[string]map[int64]float64{}
	labelsByKey := map[string]map[string]string{}

	if len(query.GroupBy) == 0 {
		key := resourceMetricGroupKey(nil)
		countsByGroup[key] = map[int64]float64{}
		labelsByKey[key] = map[string]string{}
	}

	for _, row := range rows {
		key := resourceMetricGroupKey(row.Labels)
		if _, ok := countsByGroup[key]; !ok {
			countsByGroup[key] = map[int64]float64{}
			labelsByKey[key] = row.Labels
		}
		// Each (snapshot, group) row is already a distinct count, and the database returns at most one per key.
		countsByGroup[key][row.SampledAtMS] += float64(row.Count)
	}

	keys := make([]string, 0, len(countsByGroup))
	for key := range countsByGroup {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	series := make([]ResourceMetricSeries, 0, len(keys))
	for _, key := range keys {
		points := make([]ResourceMetricPoint, 0, len(bucketSnapshots))
		for i, snapshot := range bucketSnapshots {
			if snapshot < 0 {
				continue
			}
			points = append(points, ResourceMetricPoint{
				Timestamp: query.Start.Add(time.Duration(i) * query.Step),
				Value:     countsByGroup[key][snapshot],
			})
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

// resourceMetricsSampleQuery converts a metrics query's resource filters into a sample filter. It carries no time
// bounds: aggregates are fetched for an explicit list of snapshot times instead.
func resourceMetricsSampleQuery(query ResourceMetricsQuery) ResourceSampleQuery {
	return ResourceSampleQuery{
		NamespaceMatchers: query.NamespaceMatchers,
		LabelSelector:     query.LabelSelector,
	}
}
