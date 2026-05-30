import {describe, expect, it} from 'vitest';
import type {MetricsSeries} from '@authproxy/api';
import {
    chartTimestamps,
    formatMetricValue,
    latestSeriesValue,
    latestTotalValue,
    latestValuesByLabel,
    metricTrend,
    seriesValues,
    sumSeriesValue,
    sumTotalValue,
    totalValuesByTimestamp,
    valuesForTimestamps,
} from './dashboardMetrics';

const series = (overrides: Partial<MetricsSeries>): MetricsSeries => ({
    ref_id: 'connections',
    metric: 'resources.connections',
    aggregation: 'count',
    labels: {},
    points: [],
    ...overrides,
});

describe('dashboard metrics helpers', () => {
    it('uses the latest point for resource snapshots', () => {
        expect(latestSeriesValue(series({
            points: [
                {timestamp: '2026-05-29T12:15:00Z', value: 3},
                {timestamp: '2026-05-29T12:00:00Z', value: 1},
            ],
        }))).toEqual(3);

        expect(latestTotalValue([
            series({points: [{timestamp: '2026-05-29T12:15:00Z', value: 3}]}),
            series({points: [{timestamp: '2026-05-29T12:15:00Z', value: 2}]}),
        ])).toEqual(5);
    });

    it('sums event counters across the selected range', () => {
        expect(sumSeriesValue(series({
            points: [
                {timestamp: '2026-05-29T12:00:00Z', value: 4},
                {timestamp: '2026-05-29T12:15:00Z', value: 6},
            ],
        }))).toEqual(10);

        expect(sumTotalValue([
            series({points: [{timestamp: '2026-05-29T12:00:00Z', value: 4}]}),
            series({points: [{timestamp: '2026-05-29T12:00:00Z', value: 6}]}),
        ])).toEqual(10);
    });

    it('builds stable chart axes and fills missing values', () => {
        const grouped = [
            series({
                labels: {state: 'configured'},
                points: [
                    {timestamp: '2026-05-29T12:00:00Z', value: 1},
                    {timestamp: '2026-05-29T12:30:00Z', value: 3},
                ],
            }),
            series({
                labels: {state: 'setup'},
                points: [{timestamp: '2026-05-29T12:15:00Z', value: 2}],
            }),
        ];

        const timestamps = chartTimestamps(grouped);
        expect(timestamps).toEqual([
            '2026-05-29T12:00:00Z',
            '2026-05-29T12:15:00Z',
            '2026-05-29T12:30:00Z',
        ]);
        expect(valuesForTimestamps(grouped[1], timestamps)).toEqual([0, 2, 0]);
        expect(totalValuesByTimestamp(grouped)).toEqual([1, 2, 3]);
    });

    it('formats display values and trend direction', () => {
        expect(seriesValues(series({points: [{timestamp: '2026-05-29T12:00:00Z', value: 7}]}))).toEqual([7, 0]);
        expect(metricTrend([1, 2])).toEqual('up');
        expect(metricTrend([2, 1])).toEqual('down');
        expect(metricTrend([2, 2])).toEqual('neutral');
        expect(formatMetricValue(1234)).toEqual('1,234');
        expect(formatMetricValue(12345)).toEqual('12.3K');
    });

    it('extracts latest grouped values by label', () => {
        expect(latestValuesByLabel([
            series({labels: {state: 'setup'}, points: [{timestamp: '2026-05-29T12:00:00Z', value: 1}]}),
            series({labels: {state: 'configured'}, points: [{timestamp: '2026-05-29T12:00:00Z', value: 3}]}),
            series({labels: {state: 'deleted'}, points: [{timestamp: '2026-05-29T12:00:00Z', value: 0}]}),
        ], 'state')).toEqual([
            {label: 'configured', value: 3},
            {label: 'setup', value: 1},
        ]);
    });
});
