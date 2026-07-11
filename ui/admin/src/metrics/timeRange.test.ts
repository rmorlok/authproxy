import dayjs from 'dayjs';
import {describe, expect, it} from 'vitest';
import {
    calendarRangeFromDashboardRange,
    DEFAULT_DASHBOARD_TIME_RANGE,
    describeDashboardTimeRange,
    formatCalendarRangeEnd,
    formatCalendarRangeStart,
    formatStepLabel,
    parseGrafanaTimeRange,
    resolveDashboardTimeRange,
    serializeGrafanaTimeRange,
} from './timeRange';

describe('dashboard time range helpers', () => {
    it('serializes Grafana-compatible clipboard JSON', () => {
        expect(serializeGrafanaTimeRange({from: 'now-1h', to: 'now'})).toEqual(
            '{"from":"now-1h","to":"now"}',
        );
    });

    it('parses Grafana clipboard JSON', () => {
        expect(parseGrafanaTimeRange('{"from":"now-1h","to":"now"}')).toEqual({
            from: 'now-1h',
            to: 'now',
        });
        expect(parseGrafanaTimeRange('{"from":"2026-07-17 00:00:00","to":"2026-07-17 23:59:59"}')).toEqual({
            from: '2026-07-17 00:00:00',
            to: '2026-07-17 23:59:59',
        });
    });

    it('also accepts Grafana URL time parameters', () => {
        expect(parseGrafanaTimeRange('from=now-6h&to=now')).toEqual({
            from: 'now-6h',
            to: 'now',
        });
    });

    it('resolves relative ranges using the supplied reference date', () => {
        const resolved = resolveDashboardTimeRange(
            {from: 'now-1h', to: 'now'},
            new Date('2026-07-17T12:00:00.000Z'),
        );

        expect(resolved.range).toEqual({
            start: '2026-07-17T11:00:00.000Z',
            end: '2026-07-17T12:00:00.000Z',
            step: '5m',
        });
    });

    it('resolves Grafana absolute ranges as browser-local time', () => {
        const resolved = resolveDashboardTimeRange({
            from: '2026-07-17 00:00:00',
            to: '2026-07-17 23:59:59',
        });

        expect(dayjs(resolved.range.start).format('YYYY-MM-DD HH:mm:ss')).toEqual('2026-07-17 00:00:00');
        expect(dayjs(resolved.range.end).format('YYYY-MM-DD HH:mm:ss')).toEqual('2026-07-17 23:59:59');
        expect(resolved.range.step).toEqual('15m');
    });

    it('formats selected calendar ranges as full-day Grafana absolute values', () => {
        const [from, to] = calendarRangeFromDashboardRange(
            {from: 'now-1d', to: 'now'},
            new Date('2026-07-17T12:00:00.000Z'),
        );

        expect(from).not.toBeNull();
        expect(to).not.toBeNull();
        expect(formatCalendarRangeStart(from!)).toEqual('2026-07-16 00:00:00');
        expect(formatCalendarRangeEnd(to!)).toEqual('2026-07-17 23:59:59');
    });

    it('describes quick ranges and step labels', () => {
        expect(describeDashboardTimeRange(DEFAULT_DASHBOARD_TIME_RANGE)).toEqual('Last 24 hours');
        expect(formatStepLabel('15m')).toEqual('15-minute intervals');
        expect(formatStepLabel('1h')).toEqual('1-hour interval');
    });

    it('rejects inverted ranges', () => {
        expect(() => resolveDashboardTimeRange(
            {from: 'now', to: 'now-1h'},
            new Date('2026-07-17T12:00:00.000Z'),
        )).toThrow('To must be after From');
    });
});
