package app_metrics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
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
	RefID  string                    `json:"ref_id"`
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

type requestEventMetricAccumulator struct {
	count     int
	sum       float64
	durations []float64
}

func (a *requestEventMetricAccumulator) add(metric RequestEventMetric, record *LogRecord) {
	switch metric {
	case RequestEventMetricCount:
		a.count++
	case RequestEventMetricErrorsCount:
		if isRequestEventError(record) {
			a.count++
		}
	case RequestEventMetricDurationAvgMS:
		a.count++
		a.sum += float64(record.MillisecondDuration.Duration().Milliseconds())
	case RequestEventMetricDurationP95MS:
		a.durations = append(a.durations, float64(record.MillisecondDuration.Duration().Milliseconds()))
	}
}

func (a *requestEventMetricAccumulator) value(metric RequestEventMetric) float64 {
	switch metric {
	case RequestEventMetricCount, RequestEventMetricErrorsCount:
		return float64(a.count)
	case RequestEventMetricDurationAvgMS:
		if a.count == 0 {
			return 0
		}
		return a.sum / float64(a.count)
	case RequestEventMetricDurationP95MS:
		return percentileNearestRank(a.durations, 0.95)
	default:
		return 0
	}
}

func isRequestEventError(record *LogRecord) bool {
	return record.ResponseStatusCode >= 400 ||
		record.ResponseError != "" ||
		record.InternalTimeout ||
		record.RequestCancelled
}

func executeRequestEventMetricsQueries(
	ctx context.Context,
	queries []RequestEventMetricsQuery,
	fetch func(context.Context, RequestEventMetricsQuery) ([]*LogRecord, error),
) ([]RequestEventMetricSeries, error) {
	out := make([]RequestEventMetricSeries, 0)
	for _, query := range queries {
		if err := validateRequestEventMetricsQuery(query); err != nil {
			return nil, err
		}
		records, err := fetch(ctx, query)
		if err != nil {
			return nil, err
		}
		out = append(out, buildRequestEventMetricSeries(query, records)...)
	}
	return out, nil
}

func buildRequestEventMetricSeries(query RequestEventMetricsQuery, records []*LogRecord) []RequestEventMetricSeries {
	bucketCount := int(math.Ceil(float64(query.End.Sub(query.Start)) / float64(query.Step)))
	if bucketCount < 1 {
		bucketCount = 1
	}

	accumulators := map[string][]requestEventMetricAccumulator{}
	labelsByKey := map[string]map[string]string{}

	if len(query.GroupBy) == 0 {
		key := requestEventMetricGroupKey(nil)
		accumulators[key] = make([]requestEventMetricAccumulator, bucketCount)
		labelsByKey[key] = map[string]string{}
	}

	for _, record := range records {
		if record.Timestamp.Before(query.Start) || !record.Timestamp.Before(query.End) {
			continue
		}
		bucketIdx := int(record.Timestamp.Sub(query.Start) / query.Step)
		if bucketIdx < 0 || bucketIdx >= bucketCount {
			continue
		}
		labels := requestEventMetricLabels(record, query.GroupBy)
		key := requestEventMetricGroupKey(labels)
		if _, ok := accumulators[key]; !ok {
			accumulators[key] = make([]requestEventMetricAccumulator, bucketCount)
			labelsByKey[key] = labels
		}
		accumulators[key][bucketIdx].add(query.Metric, record)
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

func requestEventMetricLabels(record *LogRecord, groupBy []RequestEventGroupBy) map[string]string {
	labels := make(map[string]string, len(groupBy))
	for _, group := range groupBy {
		switch group {
		case RequestEventGroupByType:
			labels[string(group)] = string(record.Type)
		case RequestEventGroupByMethod:
			labels[string(group)] = record.Method
		case RequestEventGroupByResponseStatusCode:
			labels[string(group)] = strconv.Itoa(record.ResponseStatusCode)
		case RequestEventGroupByResponseSource:
			source := record.ResponseSource
			if source == "" {
				source = ResponseSourceUpstream
			}
			labels[string(group)] = string(source)
		case RequestEventGroupByConnectorID:
			labels[string(group)] = record.ConnectorId.String()
		}
	}
	return labels
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
