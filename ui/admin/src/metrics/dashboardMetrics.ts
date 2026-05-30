import type {MetricsPoint, MetricsSeries} from '@authproxy/api';

export type MetricTrend = 'up' | 'down' | 'neutral';

export interface LabeledMetricValue {
    label: string;
    value: number;
}

export const latestSeriesValue = (series: MetricsSeries | undefined): number => {
    const point = latestPoint(series?.points || []);
    return point?.value || 0;
};

export const latestTotalValue = (series: MetricsSeries[] | undefined): number => {
    return (series || []).reduce((sum, item) => sum + latestSeriesValue(item), 0);
};

export const sumSeriesValue = (series: MetricsSeries | undefined): number => {
    return (series?.points || []).reduce((sum, point) => sum + point.value, 0);
};

export const sumTotalValue = (series: MetricsSeries[] | undefined): number => {
    return (series || []).reduce((sum, item) => sum + sumSeriesValue(item), 0);
};

export const seriesValues = (series: MetricsSeries | undefined, minimumLength = 2): number[] => {
    const values = (series?.points || [])
        .slice()
        .sort((a, b) => a.timestamp.localeCompare(b.timestamp))
        .map((point) => point.value);

    if (values.length >= minimumLength) {
        return values;
    }
    return [...values, ...Array(minimumLength - values.length).fill(0)];
};

export const totalValuesByTimestamp = (series: MetricsSeries[] | undefined): number[] => {
    const totals = new Map<string, number>();
    for (const item of series || []) {
        for (const point of item.points || []) {
            totals.set(point.timestamp, (totals.get(point.timestamp) || 0) + point.value);
        }
    }

    const values = Array.from(totals.entries())
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([, value]) => value);
    return values.length >= 2 ? values : [...values, ...Array(2 - values.length).fill(0)];
};

export const chartTimestamps = (series: MetricsSeries[] | undefined): string[] => {
    const timestamps = new Set<string>();
    for (const item of series || []) {
        for (const point of item.points || []) {
            timestamps.add(point.timestamp);
        }
    }
    return Array.from(timestamps).sort();
};

export const valuesForTimestamps = (series: MetricsSeries, timestamps: string[]): number[] => {
    const byTimestamp = new Map((series.points || []).map((point) => [point.timestamp, point.value]));
    return timestamps.map((timestamp) => byTimestamp.get(timestamp) || 0);
};

export const latestValuesByLabel = (
    series: MetricsSeries[] | undefined,
    labelKey: string,
): LabeledMetricValue[] => {
    return (series || [])
        .map((item) => ({
            label: item.labels?.[labelKey] || 'unknown',
            value: latestSeriesValue(item),
        }))
        .filter((item) => item.value > 0)
        .sort((a, b) => b.value - a.value || a.label.localeCompare(b.label));
};

export const metricTrend = (values: number[]): MetricTrend => {
    if (values.length < 2) {
        return 'neutral';
    }

    const first = values[0];
    const last = values[values.length - 1];
    if (last > first) {
        return 'up';
    }
    if (last < first) {
        return 'down';
    }
    return 'neutral';
};

export const formatMetricValue = (value: number): string => {
    return new Intl.NumberFormat('en-US', {
        notation: value >= 10000 ? 'compact' : 'standard',
        maximumFractionDigits: value >= 10000 ? 1 : 0,
    }).format(value);
};

const latestPoint = (points: MetricsPoint[]): MetricsPoint | null => {
    if (points.length === 0) {
        return null;
    }
    return points.reduce((latest, point) => (
        point.timestamp > latest.timestamp ? point : latest
    ), points[0]);
};
