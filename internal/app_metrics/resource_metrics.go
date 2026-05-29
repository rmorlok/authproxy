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

type ResourceMetric string

const (
	ResourceMetricConnectionsCount       ResourceMetric = "resources.connections.count"
	ResourceMetricActorsCount            ResourceMetric = "resources.actors.count"
	ResourceMetricConnectorsCount        ResourceMetric = "resources.connectors.count"
	ResourceMetricConnectorVersionsCount ResourceMetric = "resources.connector_versions.count"
	ResourceMetricNamespacesCount        ResourceMetric = "resources.namespaces.count"
	ResourceMetricRateLimitsCount        ResourceMetric = "resources.rate_limits.count"
)

type ResourceGroupBy string

const (
	ResourceGroupByState            ResourceGroupBy = "state"
	ResourceGroupByHealthState      ResourceGroupBy = "health_state"
	ResourceGroupByConnectorID      ResourceGroupBy = "connector_id"
	ResourceGroupByConnectorVersion ResourceGroupBy = "connector_version"
	ResourceGroupByNamespace        ResourceGroupBy = "namespace"
	ResourceGroupByMode             ResourceGroupBy = "mode"
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
	RefID  string                `json:"ref_id"`
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
		ResourceMetricConnectorVersionsCount,
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
			ResourceGroupByConnectorVersion:
			return true
		}
	case ResourceMetricActorsCount:
		return groupBy == ResourceGroupByNamespace
	case ResourceMetricConnectorsCount:
		switch groupBy {
		case ResourceGroupByState,
			ResourceGroupByConnectorVersion,
			ResourceGroupByNamespace:
			return true
		}
	case ResourceMetricConnectorVersionsCount:
		switch groupBy {
		case ResourceGroupByState,
			ResourceGroupByConnectorID,
			ResourceGroupByConnectorVersion,
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

func executeResourceMetricsQueries(
	ctx context.Context,
	queries []ResourceMetricsQuery,
	fetchConnections func(context.Context, ResourceMetricsQuery) ([]*ConnectionResourceSample, error),
	fetchActors func(context.Context, ResourceMetricsQuery) ([]*ActorResourceSample, error),
	fetchConnectors func(context.Context, ResourceMetricsQuery) ([]*ConnectorResourceSample, error),
	fetchConnectorVersions func(context.Context, ResourceMetricsQuery) ([]*ConnectorVersionResourceSample, error),
	fetchNamespaces func(context.Context, ResourceMetricsQuery) ([]*NamespaceResourceSample, error),
	fetchRateLimits func(context.Context, ResourceMetricsQuery) ([]*RateLimitResourceSample, error),
) ([]ResourceMetricSeries, error) {
	out := make([]ResourceMetricSeries, 0)
	for _, query := range queries {
		if err := validateResourceMetricsQuery(query); err != nil {
			return nil, err
		}
		switch query.Metric {
		case ResourceMetricConnectionsCount:
			samples, err := fetchConnections(ctx, query)
			if err != nil {
				return nil, err
			}
			out = append(out, buildConnectionResourceMetricSeries(query, samples)...)
		case ResourceMetricActorsCount:
			samples, err := fetchActors(ctx, query)
			if err != nil {
				return nil, err
			}
			out = append(out, buildActorResourceMetricSeries(query, samples)...)
		case ResourceMetricConnectorsCount:
			samples, err := fetchConnectors(ctx, query)
			if err != nil {
				return nil, err
			}
			out = append(out, buildConnectorResourceMetricSeries(query, samples)...)
		case ResourceMetricConnectorVersionsCount:
			samples, err := fetchConnectorVersions(ctx, query)
			if err != nil {
				return nil, err
			}
			out = append(out, buildConnectorVersionResourceMetricSeries(query, samples)...)
		case ResourceMetricNamespacesCount:
			samples, err := fetchNamespaces(ctx, query)
			if err != nil {
				return nil, err
			}
			out = append(out, buildNamespaceResourceMetricSeries(query, samples)...)
		case ResourceMetricRateLimitsCount:
			samples, err := fetchRateLimits(ctx, query)
			if err != nil {
				return nil, err
			}
			out = append(out, buildRateLimitResourceMetricSeries(query, samples)...)
		default:
			return nil, fmt.Errorf("unsupported resource metric %q", query.Metric)
		}
	}
	return out, nil
}

func buildConnectionResourceMetricSeries(query ResourceMetricsQuery, samples []*ConnectionResourceSample) []ResourceMetricSeries {
	return buildResourceMetricSeries(query, samples, func(sample *ConnectionResourceSample) time.Time {
		return sample.SampledAt
	}, func(sample *ConnectionResourceSample) string {
		return sample.ResourceID.String()
	}, func(sample *ConnectionResourceSample) map[string]string {
		return connectionResourceMetricLabels(sample, query.GroupBy)
	})
}

func buildActorResourceMetricSeries(query ResourceMetricsQuery, samples []*ActorResourceSample) []ResourceMetricSeries {
	return buildResourceMetricSeries(query, samples, func(sample *ActorResourceSample) time.Time {
		return sample.SampledAt
	}, func(sample *ActorResourceSample) string {
		return sample.ResourceID.String()
	}, func(sample *ActorResourceSample) map[string]string {
		return actorResourceMetricLabels(sample, query.GroupBy)
	})
}

func buildConnectorResourceMetricSeries(query ResourceMetricsQuery, samples []*ConnectorResourceSample) []ResourceMetricSeries {
	return buildResourceMetricSeries(query, samples, func(sample *ConnectorResourceSample) time.Time {
		return sample.SampledAt
	}, func(sample *ConnectorResourceSample) string {
		return sample.ResourceID.String()
	}, func(sample *ConnectorResourceSample) map[string]string {
		return connectorResourceMetricLabels(sample, query.GroupBy)
	})
}

func buildConnectorVersionResourceMetricSeries(query ResourceMetricsQuery, samples []*ConnectorVersionResourceSample) []ResourceMetricSeries {
	return buildResourceMetricSeries(query, samples, func(sample *ConnectorVersionResourceSample) time.Time {
		return sample.SampledAt
	}, func(sample *ConnectorVersionResourceSample) string {
		return sample.ResourceID.String() + ":" + strconv.FormatUint(sample.ConnectorVersion, 10)
	}, func(sample *ConnectorVersionResourceSample) map[string]string {
		return connectorVersionResourceMetricLabels(sample, query.GroupBy)
	})
}

func buildNamespaceResourceMetricSeries(query ResourceMetricsQuery, samples []*NamespaceResourceSample) []ResourceMetricSeries {
	return buildResourceMetricSeries(query, samples, func(sample *NamespaceResourceSample) time.Time {
		return sample.SampledAt
	}, func(sample *NamespaceResourceSample) string {
		return sample.ResourceID
	}, func(sample *NamespaceResourceSample) map[string]string {
		return namespaceResourceMetricLabels(sample, query.GroupBy)
	})
}

func buildRateLimitResourceMetricSeries(query ResourceMetricsQuery, samples []*RateLimitResourceSample) []ResourceMetricSeries {
	return buildResourceMetricSeries(query, samples, func(sample *RateLimitResourceSample) time.Time {
		return sample.SampledAt
	}, func(sample *RateLimitResourceSample) string {
		return sample.ResourceID.String()
	}, func(sample *RateLimitResourceSample) map[string]string {
		return rateLimitResourceMetricLabels(sample, query.GroupBy)
	})
}

func buildResourceMetricSeries[T any](
	query ResourceMetricsQuery,
	samples []*T,
	sampledAt func(*T) time.Time,
	resourceID func(*T) string,
	labelsFor func(*T) map[string]string,
) []ResourceMetricSeries {
	bucketCount := int(math.Ceil(float64(query.End.Sub(query.Start)) / float64(query.Step)))
	if bucketCount < 1 {
		bucketCount = 1
	}

	seenByGroup := map[string][]map[string]struct{}{}
	labelsByKey := map[string]map[string]string{}

	if len(query.GroupBy) == 0 {
		key := resourceMetricGroupKey(nil)
		seenByGroup[key] = makeResourceMetricBuckets(bucketCount)
		labelsByKey[key] = map[string]string{}
	}

	for _, sample := range samples {
		ts := sampledAt(sample)
		if ts.Before(query.Start) || !ts.Before(query.End) {
			continue
		}
		bucketIdx := int(ts.Sub(query.Start) / query.Step)
		if bucketIdx < 0 || bucketIdx >= bucketCount {
			continue
		}

		labels := labelsFor(sample)
		key := resourceMetricGroupKey(labels)
		if _, ok := seenByGroup[key]; !ok {
			seenByGroup[key] = makeResourceMetricBuckets(bucketCount)
			labelsByKey[key] = labels
		}
		seenByGroup[key][bucketIdx][resourceID(sample)] = struct{}{}
	}

	keys := make([]string, 0, len(seenByGroup))
	for key := range seenByGroup {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	series := make([]ResourceMetricSeries, 0, len(keys))
	for _, key := range keys {
		points := make([]ResourceMetricPoint, bucketCount)
		for i := range bucketCount {
			points[i] = ResourceMetricPoint{
				Timestamp: query.Start.Add(time.Duration(i) * query.Step),
				Value:     float64(len(seenByGroup[key][i])),
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

func makeResourceMetricBuckets(bucketCount int) []map[string]struct{} {
	buckets := make([]map[string]struct{}, bucketCount)
	for i := range buckets {
		buckets[i] = map[string]struct{}{}
	}
	return buckets
}

func connectionResourceMetricLabels(sample *ConnectionResourceSample, groupBy []ResourceGroupBy) map[string]string {
	labels := make(map[string]string, len(groupBy))
	for _, group := range groupBy {
		switch group {
		case ResourceGroupByState:
			labels[string(group)] = string(sample.State)
		case ResourceGroupByHealthState:
			labels[string(group)] = string(sample.HealthState)
		case ResourceGroupByConnectorID:
			labels[string(group)] = sample.ConnectorID.String()
		case ResourceGroupByConnectorVersion:
			labels[string(group)] = strconv.FormatUint(sample.ConnectorVersion, 10)
		}
	}
	return labels
}

func actorResourceMetricLabels(sample *ActorResourceSample, groupBy []ResourceGroupBy) map[string]string {
	labels := make(map[string]string, len(groupBy))
	for _, group := range groupBy {
		if group == ResourceGroupByNamespace {
			labels[string(group)] = sample.Namespace
		}
	}
	return labels
}

func connectorResourceMetricLabels(sample *ConnectorResourceSample, groupBy []ResourceGroupBy) map[string]string {
	labels := make(map[string]string, len(groupBy))
	for _, group := range groupBy {
		switch group {
		case ResourceGroupByState:
			labels[string(group)] = string(sample.State)
		case ResourceGroupByConnectorVersion:
			labels[string(group)] = strconv.FormatUint(sample.ConnectorVersion, 10)
		case ResourceGroupByNamespace:
			labels[string(group)] = sample.Namespace
		}
	}
	return labels
}

func connectorVersionResourceMetricLabels(sample *ConnectorVersionResourceSample, groupBy []ResourceGroupBy) map[string]string {
	labels := make(map[string]string, len(groupBy))
	for _, group := range groupBy {
		switch group {
		case ResourceGroupByState:
			labels[string(group)] = string(sample.State)
		case ResourceGroupByConnectorID:
			labels[string(group)] = sample.ResourceID.String()
		case ResourceGroupByConnectorVersion:
			labels[string(group)] = strconv.FormatUint(sample.ConnectorVersion, 10)
		case ResourceGroupByNamespace:
			labels[string(group)] = sample.Namespace
		}
	}
	return labels
}

func namespaceResourceMetricLabels(sample *NamespaceResourceSample, groupBy []ResourceGroupBy) map[string]string {
	labels := make(map[string]string, len(groupBy))
	for _, group := range groupBy {
		switch group {
		case ResourceGroupByState:
			labels[string(group)] = string(sample.State)
		case ResourceGroupByNamespace:
			labels[string(group)] = sample.Namespace
		}
	}
	return labels
}

func rateLimitResourceMetricLabels(sample *RateLimitResourceSample, groupBy []ResourceGroupBy) map[string]string {
	labels := make(map[string]string, len(groupBy))
	for _, group := range groupBy {
		switch group {
		case ResourceGroupByMode:
			labels[string(group)] = sample.Mode
		case ResourceGroupByNamespace:
			labels[string(group)] = sample.Namespace
		}
	}
	return labels
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
