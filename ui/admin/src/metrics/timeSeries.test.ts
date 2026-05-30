import {describe, expect, it} from 'vitest';
import {
    metricSeriesLabel,
    metricsResponseIsEmpty,
    metricsSeriesByRef,
    metricsSeriesToChartRows,
    summarizeMetricSeries,
} from './timeSeries';
import type {MetricsQueryResponse, MetricsSeries} from '@authproxy/api';

const series = (overrides: Partial<MetricsSeries>): MetricsSeries => ({
    ref_id: 'connections',
    metric: 'resources.connections',
    aggregation: 'count',
    labels: {},
    points: [],
    ...overrides,
});

describe('metrics time-series helpers', () => {
    it('formats labels in a stable order', () => {
        expect(metricSeriesLabel(series({
            labels: {state: 'configured', health_state: 'healthy'},
        }))).toEqual('health_state: healthy, state: configured');
        expect(metricSeriesLabel(series({labels: {}}))).toEqual('connections');
    });

    it('summarizes latest point and total value', () => {
        const summary = summarizeMetricSeries(series({
            points: [
                {timestamp: '2026-05-29T12:00:00Z', value: 2},
                {timestamp: '2026-05-29T12:15:00Z', value: 3},
            ],
        }));
        expect(summary.latest?.value).toEqual(3);
        expect(summary.total).toEqual(5);
        expect(summary.isEmpty).toEqual(false);
    });

    it('detects empty responses', () => {
        expect(metricsResponseIsEmpty(null)).toEqual(true);
        expect(metricsResponseIsEmpty({series: []})).toEqual(true);
        expect(metricsResponseIsEmpty({series: [series({
            points: [{timestamp: '2026-05-29T12:00:00Z', value: 0}],
        })]})).toEqual(true);
    });

    it('groups series by ref and converts chart rows', () => {
        const response: MetricsQueryResponse = {
            series: [
                series({
                    labels: {state: 'configured'},
                    points: [{timestamp: '2026-05-29T12:00:00Z', value: 2}],
                }),
                series({
                    labels: {state: 'setup'},
                    points: [{timestamp: '2026-05-29T12:00:00Z', value: 1}],
                }),
            ],
        };

        expect(metricsSeriesByRef(response).connections).toHaveLength(2);
        expect(metricsSeriesToChartRows(response.series)).toEqual([
            {
                timestamp: '2026-05-29T12:00:00Z',
                'state: configured': 2,
                'state: setup': 1,
            },
        ]);
    });
});
