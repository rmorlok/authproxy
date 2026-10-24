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

type RequestEventMetric string

const (
	RequestEventMetricCount         RequestEventMetric = "request_events.count"
	RequestEventMetricErrorsCount   RequestEventMetric = "request_events.errors.count"
	RequestEventMetricDurationAvgMS RequestEventMetric = "request_events.duration_ms.avg"
	RequestEventMetricDurationP95MS RequestEventMetric = "request_events.duration_ms.p95"
)

type RequestEventGroupBy string

const (
	RequestEventGroupByType               RequestEventGroupBy = "type"
	RequestEventGroupByMethod             RequestEventGroupBy = "method"
	RequestEventGroupByResponseStatusCode RequestEventGroupBy = "response_status_code"
	RequestEventGroupByResponseSource     RequestEventGroupBy = "response_source"
	RequestEventGroupByConnectorID        RequestEventGroupBy = "connector_id"
)

type RequestEventMetricsQuery struct {
	RefID             string
	Metric            RequestEventMetric
	Start             time.Time
	End               time.Time
	Step              time.Duration
	NamespaceMatchers []string
	LabelSelector     string
	GroupBy           []RequestEventGroupBy
}

type RequestEventMetricPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type RequestEventMetricSeries struct {
	RefID  string                    `json:"refId"`
	Labels map[string]string         `json:"labels,omitempty"`
	Points []RequestEventMetricPoint `json:"points"`
}

func validateRequestEventMetricsQuery(q RequestEventMetricsQuery) error {
	if q.RefID == "" {
		return errors.New("ref_id is required")
	}
	if !isValidRequestEventMetric(q.Metric) {
		return fmt.Errorf("unsupported request event metric %q", q.Metric)
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
		if !isValidRequestEventGroupBy(groupBy) {
			return fmt.Errorf("unsupported request event group_by dimension %q", groupBy)
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

func isValidRequestEventMetric(metric RequestEventMetric) bool {
	switch metric {
	case RequestEventMetricCount,
		RequestEventMetricErrorsCount,
		RequestEventMetricDurationAvgMS,
		RequestEventMetricDurationP95MS:
		return true
	default:
		return false
	}
}

func isValidRequestEventGroupBy(groupBy RequestEventGroupBy) bool {
	switch groupBy {
	case RequestEventGroupByType,
		RequestEventGroupByMethod,
		RequestEventGroupByResponseStatusCode,
		RequestEventGroupByResponseSource,
		RequestEventGroupByConnectorID:
		return true
	default:
		return false
	}
}

// requestEventMetricAggregate is one row of pre-aggregated request-event data returned by a provider for a single
// metrics query. Providers aggregate in the database (grouped by time bucket and the query's group-by dimensions) so
// that only a handful of rows cross the wire instead of every raw request event in the range.
//
// For count/error/average metrics a row summarizes every event in its bucket+group. For percentile metrics, which
// cannot be computed portably in SQL, providers return one row per event with Count=1 and DurationSumMS set to that
// event's duration, and the percentile is computed in Go.
type requestEventMetricAggregate struct {
	// BucketStartMS is the start of the time bucket in Unix milliseconds, aligned to the query's Start and Step.
	BucketStartMS int64
	// Labels holds the values of the query's group-by dimensions for this row, keyed by dimension name.
	Labels map[string]string
	// Count is the number of request events in this row.
	Count int64
	// ErrorCount is the number of those events that are errors (see requestEventErrorSQL).
	ErrorCount int64
	// DurationSumMS is the summed duration of those events in milliseconds.
	DurationSumMS int64
}

// requestEventMetricAccumulator folds aggregate rows for one bucket of one series into a metric value.
type requestEventMetricAccumulator struct {
	count         int64
	errorCount    int64
	durationSumMS int64
	// durations collects individual event durations (ms); only populated for percentile metrics.
	durations []float64
}

// add merges an aggregate row into the accumulator. Rows can be merged because count, error count, and duration sum
// are all additive; for percentile metrics each row is a single event whose duration is retained.
func (a *requestEventMetricAccumulator) add(metric RequestEventMetric, row requestEventMetricAggregate) {
	a.count += row.Count
	a.errorCount += row.ErrorCount
	a.durationSumMS += row.DurationSumMS
	if metric == RequestEventMetricDurationP95MS {
		a.durations = append(a.durations, float64(row.DurationSumMS))
	}
}

// value returns the accumulated value of the requested metric for the bucket.
func (a *requestEventMetricAccumulator) value(metric RequestEventMetric) float64 {
	switch metric {
	case RequestEventMetricCount:
		return float64(a.count)
	case RequestEventMetricErrorsCount:
		return float64(a.errorCount)
	case RequestEventMetricDurationAvgMS:
		if a.count == 0 {
			return 0
		}
		return float64(a.durationSumMS) / float64(a.count)
	case RequestEventMetricDurationP95MS:
		return percentileNearestRank(a.durations, 0.95)
	default:
		return 0
	}
}

// requestEventErrorSQL is the SQL predicate for an errored request event. It mirrors the error definition used by the
// request-event list/detail views: an HTTP error status, a transport error, an internal timeout, or a cancellation.
const requestEventErrorSQL = "response_status_code >= 400 OR response_error <> '' OR internal_timeout OR request_cancelled"

// executeRequestEventMetricsQueries validates each query, fetches its aggregate rows through the provider-specific
// fetch function, and converts them into zero-filled time series. Queries are executed sequentially and the first
// validation or fetch error aborts the whole batch.
func executeRequestEventMetricsQueries(
	ctx context.Context,
	queries []RequestEventMetricsQuery,
	fetch func(context.Context, RequestEventMetricsQuery) ([]requestEventMetricAggregate, error),
) ([]RequestEventMetricSeries, error) {
	out := make([]RequestEventMetricSeries, 0)
	for _, query := range queries {
		if err := validateRequestEventMetricsQuery(query); err != nil {
			return nil, err
		}
		rows, err := fetch(ctx, query)
		if err != nil {
			return nil, err
		}
		out = append(out, buildRequestEventMetricSeries(query, rows)...)
	}
	return out, nil
}

// buildRequestEventMetricSeries converts aggregate rows into one series per distinct label set, with a point for every
// bucket in [Start, End) (empty buckets are zero). Rows whose bucket falls outside that range are dropped; this covers
// events stamped exactly at End, which the inclusive timestamp filter still returns.
func buildRequestEventMetricSeries(query RequestEventMetricsQuery, rows []requestEventMetricAggregate) []RequestEventMetricSeries {
	bucketCount := int(math.Ceil(float64(query.End.Sub(query.Start)) / float64(query.Step)))
	if bucketCount < 1 {
		bucketCount = 1
	}
	startMS := query.Start.UnixMilli()
	stepMS := query.Step.Milliseconds()

	accumulators := map[string][]requestEventMetricAccumulator{}
	labelsByKey := map[string]map[string]string{}

	if len(query.GroupBy) == 0 {
		key := requestEventMetricGroupKey(nil)
		accumulators[key] = make([]requestEventMetricAccumulator, bucketCount)
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
		key := requestEventMetricGroupKey(row.Labels)
		if _, ok := accumulators[key]; !ok {
			accumulators[key] = make([]requestEventMetricAccumulator, bucketCount)
			labelsByKey[key] = row.Labels
		}
		accumulators[key][bucketIdx].add(query.Metric, row)
	}

	keys := make([]string, 0, len(accumulators))
	for key := range accumulators {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	series := make([]RequestEventMetricSeries, 0, len(keys))
	for _, key := range keys {
		points := make([]RequestEventMetricPoint, bucketCount)
		for i := range bucketCount {
			points[i] = RequestEventMetricPoint{
				Timestamp: query.Start.Add(time.Duration(i) * query.Step),
				Value:     accumulators[key][i].value(query.Metric),
			}
		}
		series = append(series, RequestEventMetricSeries{
			RefID:  query.RefID,
			Labels: labelsByKey[key],
			Points: points,
		})
	}
	return series
}

func requestEventMetricGroupKey(labels map[string]string) string {
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

func percentileNearestRank(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	rank := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

func requestEventMetricsFilters(query RequestEventMetricsQuery) ListFilters {
	filters := ListFilters{}
	filters.SetTimestampRange(query.Start, query.End)
	_ = filters.SetNamespaceMatchers(query.NamespaceMatchers)
	if query.LabelSelector != "" {
		filters.SetLabelSelector(query.LabelSelector)
	}
	return filters
}
