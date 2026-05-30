import type {MetricsPoint, MetricsQueryResponse, MetricsSeries} from '@authproxy/api';

export interface MetricsSeriesSummary {
    refId: string;
    metric: string;
    aggregation: string;
    labels: Record<string, string>;
    latest: MetricsPoint | null;
    total: number;
    isEmpty: boolean;
}

export interface ChartMetricPoint {
    timestamp: string;
    [seriesKey: string]: string | number | null;
}

export const metricSeriesLabel = (series: Pick<MetricsSeries, 'ref_id' | 'labels'>): string => {
    const labels = series.labels || {};
    const entries = Object.entries(labels).sort(([a], [b]) => a.localeCompare(b));
    if (entries.length === 0) {
        return series.ref_id;
    }
    return entries.map(([key, value]) => `${key}: ${value}`).join(', ');
};

export const summarizeMetricSeries = (series: MetricsSeries): MetricsSeriesSummary => {
    const latest = findLatestMetricPoint(series.points || []);
    const total = (series.points || []).reduce((sum, point) => sum + point.value, 0);
    return {
        refId: series.ref_id,
        metric: series.metric,
        aggregation: series.aggregation,
        labels: series.labels || {},
        latest,
        total,
        isEmpty: !latest || total === 0,
    };
};

export const summarizeMetricsResponse = (response: MetricsQueryResponse | null | undefined): MetricsSeriesSummary[] => {
    return (response?.series || []).map(summarizeMetricSeries);
};

export const metricsResponseIsEmpty = (response: MetricsQueryResponse | null | undefined): boolean => {
    const series = response?.series || [];
    return series.length === 0 || series.every((item) => summarizeMetricSeries(item).isEmpty);
};

export const metricsSeriesByRef = (response: MetricsQueryResponse | null | undefined): Record<string, MetricsSeries[]> => {
    const grouped: Record<string, MetricsSeries[]> = {};
    for (const series of response?.series || []) {
        grouped[series.ref_id] = grouped[series.ref_id] || [];
        grouped[series.ref_id].push(series);
    }
    return grouped;
};

export const metricsSeriesToChartRows = (series: MetricsSeries[]): ChartMetricPoint[] => {
    const rowsByTimestamp = new Map<string, ChartMetricPoint>();
    for (const item of series) {
        const key = metricSeriesLabel(item);
        for (const point of item.points || []) {
            const row = rowsByTimestamp.get(point.timestamp) || {timestamp: point.timestamp};
            row[key] = point.value;
            rowsByTimestamp.set(point.timestamp, row);
        }
    }
    return Array.from(rowsByTimestamp.values()).sort((a, b) => a.timestamp.localeCompare(b.timestamp));
};

const findLatestMetricPoint = (points: MetricsPoint[]): MetricsPoint | null => {
    if (points.length === 0) {
        return null;
    }
    return points.reduce((latest, point) => {
        return point.timestamp > latest.timestamp ? point : latest;
    }, points[0]);
};
