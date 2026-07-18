import * as React from 'react';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import Chip from '@mui/material/Chip';
import CircularProgress from '@mui/material/CircularProgress';
import Divider from '@mui/material/Divider';
import Grid from '@mui/material/Grid';
import IconButton from '@mui/material/IconButton';
import LinearProgress from '@mui/material/LinearProgress';
import Stack from '@mui/material/Stack';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import RefreshIcon from '@mui/icons-material/Refresh';
import {BarChart} from '@mui/x-charts/BarChart';
import {LineChart} from '@mui/x-charts/LineChart';
import {SparkLineChart} from '@mui/x-charts/SparkLineChart';
import dayjs from 'dayjs';
import type {MetricsQueryRef} from '@authproxy/api';
import HomeTimeRangePicker from '../components/HomeTimeRangePicker';
import {useMetricsQuery} from '../metrics';
import {
    chartTimestamps,
    formatMetricValue,
    latestSeriesValue,
    latestTotalValue,
    latestValuesByLabel,
    metricTrend,
    MetricTrend,
    seriesValues,
    sumTotalValue,
    totalValuesByTimestamp,
    valuesForTimestamps,
} from '../metrics/dashboardMetrics';
import {
    DEFAULT_DASHBOARD_TIME_RANGE,
    describeDashboardTimeRange,
    formatStepLabel,
    resolveDashboardTimeRange,
} from '../metrics/timeRange';
import type {DashboardTimeRange} from '../metrics/timeRange';

const queries: MetricsQueryRef[] = [
    {
        ref_id: 'connections-total',
        metric: 'resources.connections',
        aggregation: 'count',
    },
    {
        ref_id: 'connections-by-state',
        metric: 'resources.connections',
        aggregation: 'count',
        group_by: ['state'],
    },
    {
        ref_id: 'actors-total',
        metric: 'resources.actors',
        aggregation: 'count',
    },
    {
        ref_id: 'request-events',
        metric: 'request_events',
        aggregation: 'count',
    },
    {
        ref_id: 'request-errors',
        metric: 'request_events.errors',
        aggregation: 'count',
    },
    {
        ref_id: 'rate-limits-total',
        metric: 'resources.rate_limits',
        aggregation: 'count',
    },
];

export default function Home() {
    const [timeRange, setTimeRange] = React.useState<DashboardTimeRange>(DEFAULT_DASHBOARD_TIME_RANGE);
    const [rangeResolvedAt, setRangeResolvedAt] = React.useState(() => new Date());
    const resolvedTimeRange = React.useMemo(
        () => resolveDashboardTimeRange(timeRange, rangeResolvedAt),
        [timeRange, rangeResolvedAt],
    );
    const range = resolvedTimeRange.range;

    const {
        loading,
        error,
        empty,
        seriesByRef,
        reload,
    } = useMetricsQuery({range, queries});

    const connections = seriesByRef['connections-total'] || [];
    const connectionsByState = seriesByRef['connections-by-state'] || [];
    const actors = seriesByRef['actors-total'] || [];
    const requestEvents = seriesByRef['request-events'] || [];
    const requestErrors = seriesByRef['request-errors'] || [];
    const rateLimits = seriesByRef['rate-limits-total'] || [];

    const requestValues = totalValuesByTimestamp(requestEvents);
    const errorValues = totalValuesByTimestamp(requestErrors);
    const connectionValues = seriesValues(connections[0]);
    const actorValues = seriesValues(actors[0]);
    const rateLimitValues = seriesValues(rateLimits[0]);

    const stateTimestamps = chartTimestamps(connectionsByState);
    const stateRows = latestValuesByLabel(connectionsByState, 'state');
    const hasStateSeries = stateTimestamps.length > 0 && connectionsByState.some((item) => latestSeriesValue(item) > 0);

    const requestTimestamps = chartTimestamps(requestEvents);
    const hasRequestSeries = requestTimestamps.length > 0 && sumTotalValue(requestEvents) > 0;

    const refresh = () => {
        setRangeResolvedAt(new Date());
        reload();
    };

    const applyTimeRange = (nextRange: DashboardTimeRange) => {
        setTimeRange(nextRange);
        setRangeResolvedAt(new Date());
    };

    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            <Stack
                direction={{xs: 'column', sm: 'row'}}
                spacing={1.5}
                sx={{alignItems: {xs: 'flex-start', sm: 'center'}, justifyContent: 'space-between', mb: 2}}
            >
                <Box>
                    <Typography component="h2" variant="h6">
                        Overview
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        {describeDashboardTimeRange(timeRange)} · {formatStepLabel(range.step)}
                    </Typography>
                </Box>
                <Stack direction="row" spacing={1} sx={{alignItems: 'center', width: {xs: '100%', sm: 'auto'}}}>
                    <HomeTimeRangePicker value={timeRange} onApply={applyTimeRange} />
                    <Tooltip title="Refresh metrics">
                        <span>
                            <IconButton aria-label="Refresh metrics" onClick={refresh} disabled={loading}>
                                {loading ? <CircularProgress size={20} /> : <RefreshIcon />}
                            </IconButton>
                        </span>
                    </Tooltip>
                </Stack>
            </Stack>

            {error && (
                <Alert
                    severity="error"
                    action={(
                        <Button color="inherit" size="small" onClick={refresh}>
                            Retry
                        </Button>
                    )}
                    sx={{mb: 2}}
                >
                    {error}
                </Alert>
            )}

            {loading && <LinearProgress sx={{mb: 2}} />}

            {!error && empty && !loading && (
                <Alert severity="info" sx={{mb: 2}}>
                    No metrics have been recorded for this namespace in the selected window.
                </Alert>
            )}

            <Grid container spacing={2} columns={12} sx={{mb: 2}}>
                <Grid size={{xs: 12, sm: 6, lg: 3}}>
                    <MetricStatCard
                        title="Connections"
                        value={formatMetricValue(latestTotalValue(connections))}
                        interval="Current snapshot"
                        trend={metricTrend(connectionValues)}
                        data={connectionValues}
                    />
                </Grid>
                <Grid size={{xs: 12, sm: 6, lg: 3}}>
                    <MetricStatCard
                        title="Actors"
                        value={formatMetricValue(latestTotalValue(actors))}
                        interval="Current snapshot"
                        trend={metricTrend(actorValues)}
                        data={actorValues}
                    />
                </Grid>
                <Grid size={{xs: 12, sm: 6, lg: 3}}>
                    <MetricStatCard
                        title="Requests"
                        value={formatMetricValue(sumTotalValue(requestEvents))}
                        interval="Total in range"
                        trend={metricTrend(requestValues)}
                        data={requestValues}
                    />
                </Grid>
                <Grid size={{xs: 12, sm: 6, lg: 3}}>
                    <MetricStatCard
                        title="Errors"
                        value={formatMetricValue(sumTotalValue(requestErrors))}
                        interval="Total in range"
                        trend={metricTrend(errorValues)}
                        data={errorValues}
                    />
                </Grid>

                <Grid size={{xs: 12, lg: 8}}>
                    <Card variant="outlined" sx={{height: '100%'}}>
                        <CardContent>
                            <SectionHeader title="Connections by state" value={`${formatMetricValue(latestTotalValue(connections))} total`} />
                            {hasStateSeries ? (
                                <LineChart
                                    height={300}
                                    margin={{left: 48, right: 24, top: 24, bottom: 48}}
                                    xAxis={[{
                                        scaleType: 'point',
                                        data: stateTimestamps.map(formatTimestamp),
                                    }]}
                                    yAxis={[{min: 0}]}
                                    series={connectionsByState.map((item) => ({
                                        data: valuesForTimestamps(item, stateTimestamps),
                                        label: item.labels?.state || 'unknown',
                                        curve: 'linear' as const,
                                    }))}
                                />
                            ) : (
                                <EmptyPanel label="No connection snapshots in this range" />
                            )}
                        </CardContent>
                    </Card>
                </Grid>

                <Grid size={{xs: 12, lg: 4}}>
                    <Card variant="outlined" sx={{height: '100%'}}>
                        <CardContent>
                            <SectionHeader title="State totals" value={`${stateRows.length} states`} />
                            <Stack spacing={1.5} divider={<Divider flexItem />}>
                                {stateRows.length > 0 ? stateRows.map((item) => (
                                    <Stack
                                        key={item.label}
                                        direction="row"
                                        sx={{alignItems: 'center', justifyContent: 'space-between'}}
                                    >
                                        <Typography variant="body2" sx={{textTransform: 'capitalize'}}>
                                            {item.label}
                                        </Typography>
                                        <Typography variant="subtitle2">
                                            {formatMetricValue(item.value)}
                                        </Typography>
                                    </Stack>
                                )) : (
                                    <EmptyPanel label="No state totals" compact />
                                )}
                            </Stack>
                        </CardContent>
                    </Card>
                </Grid>

                <Grid size={{xs: 12, lg: 8}}>
                    <Card variant="outlined" sx={{height: '100%'}}>
                        <CardContent>
                            <SectionHeader title="Request volume" value={`${formatMetricValue(sumTotalValue(requestEvents))} requests`} />
                            {hasRequestSeries ? (
                                <BarChart
                                    height={300}
                                    margin={{left: 56, right: 24, top: 24, bottom: 48}}
                                    xAxis={[{
                                        scaleType: 'band',
                                        data: requestTimestamps.map(formatTimestamp),
                                    }]}
                                    yAxis={[{min: 0}]}
                                    series={[{
                                        data: totalValuesByTimestamp(requestEvents),
                                        label: 'Requests',
                                    }]}
                                />
                            ) : (
                                <EmptyPanel label="No request events in this range" />
                            )}
                        </CardContent>
                    </Card>
                </Grid>

                <Grid size={{xs: 12, lg: 4}}>
                    <Card variant="outlined" sx={{height: '100%'}}>
                        <CardContent>
                            <SectionHeader title="Controls" value={`${formatMetricValue(latestTotalValue(rateLimits))} rate limits`} />
                            <Stack spacing={2}>
                                <MetricStatCard
                                    title="Rate limits"
                                    value={formatMetricValue(latestTotalValue(rateLimits))}
                                    interval="Current snapshot"
                                    trend={metricTrend(rateLimitValues)}
                                    data={rateLimitValues}
                                    flat
                                />
                                <Stack direction="row" sx={{alignItems: 'center', justifyContent: 'space-between'}}>
                                    <Typography variant="body2" color="text.secondary">
                                        Error events
                                    </Typography>
                                    <Typography variant="subtitle2">
                                        {formatMetricValue(sumTotalValue(requestErrors))}
                                    </Typography>
                                </Stack>
                            </Stack>
                        </CardContent>
                    </Card>
                </Grid>
            </Grid>
        </Box>
    );
}

function MetricStatCard({
    title,
    value,
    interval,
    trend,
    data,
    flat = false,
}: {
    title: string;
    value: string;
    interval: string;
    trend: MetricTrend;
    data: number[];
    flat?: boolean;
}) {
    const chip = trendChip(trend);
    const content = (
        <CardContent sx={flat ? {p: 0, '&:last-child': {pb: 0}} : undefined}>
            <Stack spacing={1.5}>
                <Stack direction="row" sx={{alignItems: 'flex-start', justifyContent: 'space-between', gap: 1}}>
                    <Box>
                        <Typography component="h3" variant="subtitle2">
                            {title}
                        </Typography>
                        <Typography variant="h4" component="p" sx={{mt: 0.5}}>
                            {value}
                        </Typography>
                    </Box>
                    <Chip size="small" color={chip.color} label={chip.label} />
                </Stack>
                <Typography variant="caption" color="text.secondary">
                    {interval}
                </Typography>
                <Box sx={{height: 54, width: '100%'}}>
                    <SparkLineChart
                        data={data}
                        curve="linear"
                        showHighlight
                        showTooltip
                        xAxis={{scaleType: 'point', data: data.map((_, index) => String(index + 1))}}
                    />
                </Box>
            </Stack>
        </CardContent>
    );

    if (flat) {
        return content;
    }
    return (
        <Card variant="outlined" sx={{height: '100%'}}>
            {content}
        </Card>
    );
}

function SectionHeader({title, value}: { title: string; value: string }) {
    return (
        <Stack
            direction="row"
            sx={{alignItems: 'center', justifyContent: 'space-between', gap: 2, mb: 1}}
        >
            <Typography component="h3" variant="subtitle1">
                {title}
            </Typography>
            <Typography variant="body2" color="text.secondary">
                {value}
            </Typography>
        </Stack>
    );
}

function EmptyPanel({label, compact = false}: { label: string; compact?: boolean }) {
    return (
        <Box
            sx={{
                minHeight: compact ? 72 : 260,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                border: (theme) => `1px dashed ${theme.palette.divider}`,
                borderRadius: 1,
                color: 'text.secondary',
            }}
        >
            <Typography variant="body2">{label}</Typography>
        </Box>
    );
}

function trendChip(trend: MetricTrend): { label: string; color: 'success' | 'error' | 'default' } {
    switch (trend) {
        case 'up':
            return {label: 'Rising', color: 'success'};
        case 'down':
            return {label: 'Falling', color: 'error'};
        default:
            return {label: 'Steady', color: 'default'};
    }
}

function formatTimestamp(timestamp: string): string {
    return dayjs(timestamp).format('HH:mm');
}
