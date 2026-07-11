import dayjs from 'dayjs';
import customParseFormat from 'dayjs/plugin/customParseFormat';
import type {ManipulateType, OpUnitType} from 'dayjs';
import type {MetricsRange} from '@authproxy/api';

dayjs.extend(customParseFormat);

export interface DashboardTimeRange {
    from: string;
    to: string;
}

export interface QuickDashboardTimeRange {
    label: string;
    range: DashboardTimeRange;
}

export interface ResolvedDashboardTimeRange {
    range: MetricsRange;
    durationMs: number;
}

export const DEFAULT_DASHBOARD_TIME_RANGE: DashboardTimeRange = {
    from: 'now-24h',
    to: 'now',
};

export const QUICK_DASHBOARD_TIME_RANGES: QuickDashboardTimeRange[] = [
    {label: 'Last 5 minutes', range: {from: 'now-5m', to: 'now'}},
    {label: 'Last 15 minutes', range: {from: 'now-15m', to: 'now'}},
    {label: 'Last 30 minutes', range: {from: 'now-30m', to: 'now'}},
    {label: 'Last 1 hour', range: {from: 'now-1h', to: 'now'}},
    {label: 'Last 3 hours', range: {from: 'now-3h', to: 'now'}},
    {label: 'Last 6 hours', range: {from: 'now-6h', to: 'now'}},
    {label: 'Last 12 hours', range: {from: 'now-12h', to: 'now'}},
    {label: 'Last 24 hours', range: DEFAULT_DASHBOARD_TIME_RANGE},
    {label: 'Last 2 days', range: {from: 'now-2d', to: 'now'}},
    {label: 'Last 7 days', range: {from: 'now-7d', to: 'now'}},
    {label: 'Last 30 days', range: {from: 'now-30d', to: 'now'}},
];

const ABSOLUTE_TIME_FORMATS = [
    'YYYY-MM-DD HH:mm:ss',
    'YYYY-MM-DD HH:mm',
    'YYYY-MM-DD',
    'YYYY-MM-DDTHH:mm:ss.SSSZ',
    'YYYY-MM-DDTHH:mm:ss.SSS',
    'YYYY-MM-DDTHH:mm:ssZ',
    'YYYY-MM-DDTHH:mm:ss',
    'YYYY-MM-DDTHH:mm',
];

const RELATIVE_TIME_RE = /^now(?:(?<sign>[+-])(?<amount>\d+)(?<unit>ms|s|m|h|d|w|M|y))?(?:\/(?<round>s|m|h|d|w|M|y))?$/;

const RELATIVE_UNITS: Record<string, ManipulateType> = {
    ms: 'millisecond',
    s: 'second',
    m: 'minute',
    h: 'hour',
    d: 'day',
    w: 'week',
    M: 'month',
    y: 'year',
};

const ROUND_UNITS: Record<string, OpUnitType> = {
    s: 'second',
    m: 'minute',
    h: 'hour',
    d: 'day',
    w: 'week',
    M: 'month',
    y: 'year',
};

export function serializeGrafanaTimeRange(range: DashboardTimeRange): string {
    return JSON.stringify({from: range.from, to: range.to});
}

export function parseGrafanaTimeRange(value: string): DashboardTimeRange {
    const input = value.trim();
    if (!input) {
        throw new Error('Clipboard is empty');
    }

    const jsonRange = parseGrafanaTimeRangeJson(input);
    if (jsonRange) {
        return jsonRange;
    }

    const paramsRange = parseGrafanaTimeRangeParams(input);
    if (paramsRange) {
        return paramsRange;
    }

    throw new Error('Clipboard does not contain a Grafana time range');
}

export function resolveDashboardTimeRange(
    range: DashboardTimeRange,
    referenceDate: Date = new Date(),
): ResolvedDashboardTimeRange {
    const from = parseTimeExpression(range.from, referenceDate, 'From');
    const to = parseTimeExpression(range.to, referenceDate, 'To');
    const durationMs = to.diff(from);

    if (durationMs <= 0) {
        throw new Error('To must be after From');
    }

    return {
        range: {
            start: from.toISOString(),
            end: to.toISOString(),
            step: stepForDuration(durationMs),
        },
        durationMs,
    };
}

export function timeRangeValidationError(range: DashboardTimeRange): string | null {
    try {
        resolveDashboardTimeRange(range);
        return null;
    } catch (err) {
        return err instanceof Error ? err.message : 'Invalid time range';
    }
}

export function describeDashboardTimeRange(range: DashboardTimeRange): string {
    const quickRange = QUICK_DASHBOARD_TIME_RANGES.find((candidate) => rangesEqual(candidate.range, range));
    if (quickRange) {
        return quickRange.label;
    }

    if (isRelativeExpression(range.from) || isRelativeExpression(range.to)) {
        return `${range.from} to ${range.to}`;
    }

    const from = parseAbsoluteTimeExpression(range.from);
    const to = parseAbsoluteTimeExpression(range.to);
    if (from && to) {
        if (from.isSame(to, 'day')) {
            return `${from.format('MMM D, YYYY HH:mm:ss')} to ${to.format('HH:mm:ss')}`;
        }
        return `${from.format('MMM D, YYYY HH:mm:ss')} to ${to.format('MMM D, YYYY HH:mm:ss')}`;
    }

    return `${range.from} to ${range.to}`;
}

export function formatStepLabel(step: string): string {
    const match = step.match(/^(\d+)([mhd])$/);
    if (!match) {
        return `${step} intervals`;
    }

    const value = Number(match[1]);
    const unit = match[2] === 'm' ? 'minute' : match[2] === 'h' ? 'hour' : 'day';
    return `${value}-${unit} interval${value === 1 ? '' : 's'}`;
}

export function rangesEqual(a: DashboardTimeRange, b: DashboardTimeRange): boolean {
    return a.from === b.from && a.to === b.to;
}

export function browserTimeZoneLabel(): string {
    const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'Browser time';
    return `${timeZone} ${formatUtcOffset(new Date())}`;
}

function parseGrafanaTimeRangeJson(input: string): DashboardTimeRange | null {
    try {
        const parsed = JSON.parse(input) as unknown;
        if (
            typeof parsed === 'object' &&
            parsed !== null &&
            typeof (parsed as {from?: unknown}).from === 'string' &&
            typeof (parsed as {to?: unknown}).to === 'string'
        ) {
            return {
                from: (parsed as {from: string}).from.trim(),
                to: (parsed as {to: string}).to.trim(),
            };
        }
    } catch (_err) {
        return null;
    }
    return null;
}

function parseGrafanaTimeRangeParams(input: string): DashboardTimeRange | null {
    let params: URLSearchParams;
    try {
        params = input.includes('://')
            ? new URL(input).searchParams
            : new URLSearchParams(input.startsWith('?') ? input.slice(1) : input);
    } catch (_err) {
        return null;
    }

    const from = params.get('from');
    const to = params.get('to');
    return from && to ? {from: from.trim(), to: to.trim()} : null;
}

function parseTimeExpression(value: string, referenceDate: Date, label: string): dayjs.Dayjs {
    const expression = value.trim();
    if (!expression) {
        throw new Error(`${label} is required`);
    }

    const relative = parseRelativeTimeExpression(expression, referenceDate);
    if (relative) {
        return relative;
    }

    const absolute = parseAbsoluteTimeExpression(expression);
    if (absolute) {
        return absolute;
    }

    throw new Error(`${label} is not a recognized time expression`);
}

function parseRelativeTimeExpression(value: string, referenceDate: Date): dayjs.Dayjs | null {
    const match = value.match(RELATIVE_TIME_RE);
    if (!match?.groups) {
        return null;
    }

    let parsed = dayjs(referenceDate);
    const {sign, amount, unit, round} = match.groups;
    if (sign && amount && unit) {
        const count = Number(amount);
        parsed = sign === '-'
            ? parsed.subtract(count, RELATIVE_UNITS[unit])
            : parsed.add(count, RELATIVE_UNITS[unit]);
    }

    if (round) {
        parsed = parsed.startOf(ROUND_UNITS[round]);
    }

    return parsed;
}

function parseAbsoluteTimeExpression(value: string): dayjs.Dayjs | null {
    if (/^\d{13}$/.test(value)) {
        const parsed = dayjs(Number(value));
        return parsed.isValid() ? parsed : null;
    }

    if (/^\d{10}$/.test(value)) {
        const parsed = dayjs(Number(value) * 1000);
        return parsed.isValid() ? parsed : null;
    }

    const strictParsed = dayjs(value, ABSOLUTE_TIME_FORMATS, true);
    if (strictParsed.isValid()) {
        return strictParsed;
    }

    const looseParsed = dayjs(value);
    return looseParsed.isValid() ? looseParsed : null;
}

function isRelativeExpression(value: string): boolean {
    return RELATIVE_TIME_RE.test(value.trim());
}

function stepForDuration(durationMs: number): string {
    const minute = 60 * 1000;
    const hour = 60 * minute;
    const day = 24 * hour;

    if (durationMs <= 30 * minute) {
        return '1m';
    }
    if (durationMs <= 3 * hour) {
        return '5m';
    }
    if (durationMs <= day) {
        return '15m';
    }
    if (durationMs <= 2 * day) {
        return '30m';
    }
    if (durationMs <= 7 * day) {
        return '1h';
    }
    if (durationMs <= 30 * day) {
        return '6h';
    }
    return '1d';
}

function formatUtcOffset(date: Date): string {
    const offsetMinutes = -date.getTimezoneOffset();
    const sign = offsetMinutes >= 0 ? '+' : '-';
    const absoluteMinutes = Math.abs(offsetMinutes);
    const hours = String(Math.floor(absoluteMinutes / 60)).padStart(2, '0');
    const minutes = String(absoluteMinutes % 60).padStart(2, '0');
    return `UTC${sign}${hours}:${minutes}`;
}
