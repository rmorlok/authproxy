import * as React from 'react';
import {useCallback, useEffect, useRef, useState} from 'react';
import {useParams} from 'react-router-dom';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import Tabs from '@mui/material/Tabs';
import Tab from '@mui/material/Tab';
import Chip from '@mui/material/Chip';
import IconButton from '@mui/material/IconButton';
import Tooltip from '@mui/material/Tooltip';
import Button from '@mui/material/Button';
import Stack from '@mui/material/Stack';
import FormControlLabel from '@mui/material/FormControlLabel';
import Switch from '@mui/material/Switch';
import {DataGrid, GridColDef} from '@mui/x-data-grid';
import {BarChart} from '@mui/x-charts/BarChart';
import {useTheme} from '@mui/material/styles';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import ArchiveIcon from '@mui/icons-material/Archive';
import DeleteIcon from '@mui/icons-material/Delete';
import CancelIcon from '@mui/icons-material/Cancel';
import PauseIcon from '@mui/icons-material/Pause';
import PlayCircleOutlineIcon from '@mui/icons-material/PlayCircleOutline';
import {useQueryState, parseAsInteger} from 'nuqs';
import {
    getQueueInfo,
    getQueueHistory,
    listTasksByState,
    runTask,
    archiveTask,
    cancelTask,
    deleteTask,
    pauseQueue,
    unpauseQueue,
    runAllArchivedTasks,
    runAllRetryTasks,
    deleteAllArchivedTasks,
    deleteAllCompletedTasks,
    QueueInfo,
    DailyStats,
    MonitoringTaskInfo,
    MonitoringTaskState,
    ListResponse,
    ListTasksParams,
} from '@authproxy/api';

const TASK_STATES: MonitoringTaskState[] = ['pending', 'active', 'scheduled', 'retry', 'archived', 'completed'];

const stateColors: Record<MonitoringTaskState, 'info' | 'primary' | 'secondary' | 'warning' | 'error' | 'success'> = {
    pending: 'info',
    active: 'primary',
    scheduled: 'secondary',
    retry: 'warning',
    archived: 'error',
    completed: 'success',
};

function SummaryCard({label, value, color}: { label: string; value: number; color: string }) {
    return (
        <Card variant="outlined" sx={{flex: 1, minWidth: 120}}>
            <CardContent sx={{py: 1.5, '&:last-child': {pb: 1.5}}}>
                <Typography variant="caption" color="text.secondary">{label}</Typography>
                <Typography variant="h5" sx={{color, fontWeight: 600}}>{value}</Typography>
            </CardContent>
        </Card>
    );
}

export default function TaskQueueDetail() {
    const theme = useTheme();
    const {queue} = useParams<{ queue: string }>();

    const [queueInfoState, setQueueInfoState] = useState<QueueInfo | null>(null);
    const [tabIndex, setTabIndex] = useQueryState<number>('tab', parseAsInteger.withDefault(0));
    const [page, setPage] = useQueryState<number>('page', parseAsInteger.withDefault(1));
    const [pageSize, setPageSize] = useQueryState<number>('page_size', parseAsInteger.withDefault(30));
    const [autoRefresh, setAutoRefresh] = useState(true);

    const [history, setHistory] = useState<DailyStats[]>([]);
    const [tasks, setTasks] = useState<MonitoringTaskInfo[]>([]);
    const [tasksLoading, setTasksLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [hasNextPage, setHasNextPage] = useState(false);
    const [rowCount, setRowCount] = useState(-1);

    // Cursor cache for sequential page fetching
    const responsesCacheRef = useRef<ListResponse<MonitoringTaskInfo>[]>([]);
    const pageRequestCacheRef = useRef<Set<number>>(new Set());

    const currentState = TASK_STATES[tabIndex] || 'pending';

    const fetchQueueInfo = useCallback(async () => {
        if (!queue) return;
        try {
            const resp = await getQueueInfo(queue);
            if (resp.status === 200) {
                setQueueInfoState(resp.data);
            }
        } catch {
            setError('Failed to load queue info');
        }
    }, [queue]);

    const fetchHistory = useCallback(async () => {
        if (!queue) return;
        try {
            const resp = await getQueueHistory(queue, {days: 30});
            if (resp.status === 200) {
                setHistory(resp.data);
            }
        } catch {
            // ignore
        }
    }, [queue]);

    const resetPagination = useCallback(() => {
        responsesCacheRef.current = [];
        pageRequestCacheRef.current = new Set();
        setHasNextPage(false);
        setRowCount(-1);
        setPage(1);
    }, [setPage]);

    const fetchPage = useCallback(async (targetPageOneBased: number) => {
        if (!queue) return;

        const targetPageZeroBased = targetPageOneBased - 1;
        setTasksLoading(true);
        setHasNextPage(false);
        setError(null);

        try {
            // If we have it cached, use it
            const cached = responsesCacheRef.current[targetPageZeroBased];
            if (cached) {
                setTasks(cached.items);
                setTasksLoading(false);
                setHasNextPage(!!cached.cursor);
                return;
            }

            // Advance sequentially from the last known cursor
            while (responsesCacheRef.current.length <= targetPageZeroBased && (
                responsesCacheRef.current.length === 0 ||
                !!responsesCacheRef.current[responsesCacheRef.current.length - 1].cursor
            )) {
                const thisPage = responsesCacheRef.current.length;

                // Avoid duplicate requests for the same page
                if (pageRequestCacheRef.current.has(thisPage)) {
                    break;
                }
                pageRequestCacheRef.current.add(thisPage);

                const prevResp = responsesCacheRef.current[responsesCacheRef.current.length - 1];

                const params: ListTasksParams = prevResp?.cursor
                    ? {cursor: prevResp.cursor}
                    : {limit: pageSize};

                const resp = await listTasksByState(queue, currentState, params);

                if (resp.status !== 200) {
                    setError('Failed to fetch tasks from server');
                    return;
                }

                responsesCacheRef.current[thisPage] = resp.data;
            }

            const data = responsesCacheRef.current[targetPageZeroBased];
            setTasks(data?.items || []);

            const hnp = !!data?.cursor;
            setHasNextPage(hnp);

            if (!hnp) {
                setRowCount(
                    responsesCacheRef.current
                        .map((v) => v.items.length)
                        .reduce((acc, val) => acc + val, 0)
                );
            }
        } catch {
            setError('Failed to load tasks');
        } finally {
            setTasksLoading(false);
        }
    }, [queue, currentState, pageSize]);

    // Initial load
    useEffect(() => {
        fetchQueueInfo();
        fetchHistory();
    }, [queue]);

    // Reset cache and fetch first page when state or pageSize changes
    useEffect(() => {
        resetPagination();
        fetchPage(1);
    }, [currentState, pageSize]);

    // Fetch page when page changes
    useEffect(() => {
        fetchPage(page);
    }, [page]);

    // Auto-refresh: invalidate cache and re-fetch current page
    const autoRefreshRef = useRef(autoRefresh);
    autoRefreshRef.current = autoRefresh;

    const refreshTasks = useCallback(() => {
        responsesCacheRef.current = [];
        pageRequestCacheRef.current = new Set();
        setRowCount(-1);
        fetchPage(page);
    }, [fetchPage, page]);

    useEffect(() => {
        if (!autoRefresh) return;

        const interval = setInterval(() => {
            if (document.visibilityState === 'hidden') return;
            if (!autoRefreshRef.current) return;
            fetchQueueInfo();
            refreshTasks();
        }, 5000);

        return () => clearInterval(interval);
    }, [autoRefresh, fetchQueueInfo, refreshTasks]);

    // Action handlers - invalidate cache and re-fetch after mutations
    const handleRunTask = async (q: string, taskId: string) => {
        await runTask(q, taskId);
        refreshTasks();
        fetchQueueInfo();
    };
    const handleArchiveTask = async (q: string, taskId: string) => {
        await archiveTask(q, taskId);
        refreshTasks();
        fetchQueueInfo();
    };
    const handleCancelTask = async (q: string, taskId: string) => {
        await cancelTask(q, taskId);
        refreshTasks();
        fetchQueueInfo();
    };
    const handleDeleteTask = async (q: string, taskId: string) => {
        await deleteTask(q, taskId);
        refreshTasks();
        fetchQueueInfo();
    };
    const handlePauseQueue = async () => {
        if (!queue) return;
        await pauseQueue(queue);
        fetchQueueInfo();
    };
    const handleUnpauseQueue = async () => {
        if (!queue) return;
        await unpauseQueue(queue);
        fetchQueueInfo();
    };
    const handleRunAllArchived = async () => {
        if (!queue) return;
        await runAllArchivedTasks(queue);
        refreshTasks();
        fetchQueueInfo();
    };
    const handleRunAllRetry = async () => {
        if (!queue) return;
        await runAllRetryTasks(queue);
        refreshTasks();
        fetchQueueInfo();
    };
    const handleDeleteAllArchived = async () => {
        if (!queue) return;
        await deleteAllArchivedTasks(queue);
        refreshTasks();
        fetchQueueInfo();
    };
    const handleDeleteAllCompleted = async () => {
        if (!queue) return;
        await deleteAllCompletedTasks(queue);
        refreshTasks();
        fetchQueueInfo();
    };

    // Task table columns
    const taskColumns: GridColDef<MonitoringTaskInfo>[] = [
        {
            field: 'id',
            headerName: 'ID',
            flex: 0.8,
            minWidth: 100,
            renderCell: (params) => (
                <Tooltip title={params.value}>
                    <span>{(params.value as string).substring(0, 12)}...</span>
                </Tooltip>
            ),
        },
        {field: 'type', headerName: 'Type', flex: 1, minWidth: 120},
        {
            field: 'retried',
            headerName: 'Retried',
            flex: 0.5,
            minWidth: 80,
            renderCell: (params) => `${params.row.retried}/${params.row.max_retry}`,
        },
        {
            field: 'last_err',
            headerName: 'Last Error',
            flex: 1.2,
            minWidth: 120,
            renderCell: (params) => params.value ? (
                <Tooltip title={params.value as string}>
                    <span style={{
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap'
                    }}>{params.value as string}</span>
                </Tooltip>
            ) : null,
        },
        {
            field: 'next_process_at',
            headerName: 'Next Process At',
            flex: 0.8,
            minWidth: 140,
            renderCell: (params) => params.value || params.row.completed_at || '-',
        },
        {
            field: 'actions',
            headerName: 'Actions',
            flex: 0.6,
            minWidth: 120,
            sortable: false,
            renderCell: (params) => {
                const row = params.row;
                const state = row.state as MonitoringTaskState;
                return (
                    <Stack direction="row" spacing={0}>
                        {(state === 'scheduled' || state === 'retry' || state === 'archived') && (
                            <Tooltip title="Run now">
                                <IconButton size="small"
                                            onClick={() => handleRunTask(row.queue, row.id)}>
                                    <PlayArrowIcon fontSize="small"/>
                                </IconButton>
                            </Tooltip>
                        )}
                        {(state === 'pending' || state === 'scheduled' || state === 'retry') && (
                            <Tooltip title="Archive">
                                <IconButton size="small"
                                            onClick={() => handleArchiveTask(row.queue, row.id)}>
                                    <ArchiveIcon fontSize="small"/>
                                </IconButton>
                            </Tooltip>
                        )}
                        {state === 'active' && (
                            <Tooltip title="Cancel">
                                <IconButton size="small"
                                            onClick={() => handleCancelTask(row.queue, row.id)}>
                                    <CancelIcon fontSize="small"/>
                                </IconButton>
                            </Tooltip>
                        )}
                        {(state !== 'active') && (
                            <Tooltip title="Delete">
                                <IconButton size="small"
                                            onClick={() => handleDeleteTask(row.queue, row.id)}>
                                    <DeleteIcon fontSize="small"/>
                                </IconButton>
                            </Tooltip>
                        )}
                    </Stack>
                );
            },
        },
    ];

    // Chart data
    const chartDates = history.map(h => h.date);
    const chartProcessed = history.map(h => h.processed);
    const chartFailed = history.map(h => h.failed);

    if (!queue) return null;

    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            {/* Header */}
            <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{mb: 2}}>
                <Stack direction="row" spacing={1} alignItems="center">
                    <Typography component="h2" variant="h6">Queue: {queue}</Typography>
                    {queueInfoState?.paused && (
                        <Chip label="PAUSED" color="warning" size="small"/>
                    )}
                </Stack>
                <Stack direction="row" spacing={2} alignItems="center">
                    {queueInfoState && (
                        queueInfoState.paused ? (
                            <Tooltip title="Unpause queue">
                                <IconButton onClick={handleUnpauseQueue} color="primary">
                                    <PlayCircleOutlineIcon/>
                                </IconButton>
                            </Tooltip>
                        ) : (
                            <Tooltip title="Pause queue">
                                <IconButton onClick={handlePauseQueue} color="warning">
                                    <PauseIcon/>
                                </IconButton>
                            </Tooltip>
                        )
                    )}
                    <FormControlLabel
                        control={
                            <Switch
                                checked={autoRefresh}
                                onChange={(e) => setAutoRefresh(e.target.checked)}
                                size="small"
                            />
                        }
                        label="Auto-refresh"
                    />
                </Stack>
            </Stack>

            {error && (
                <Typography color="error" sx={{mb: 2}}>{error}</Typography>
            )}

            {/* Summary Cards */}
            {queueInfoState && (
                <Stack direction="row" spacing={2} sx={{mb: 3}} flexWrap="wrap">
                    <SummaryCard label="Pending" value={queueInfoState.pending}
                                 color={(theme.vars || theme).palette.info.main}/>
                    <SummaryCard label="Active" value={queueInfoState.active}
                                 color={(theme.vars || theme).palette.primary.main}/>
                    <SummaryCard label="Retry" value={queueInfoState.retry}
                                 color={(theme.vars || theme).palette.warning.main}/>
                    <SummaryCard label="Archived" value={queueInfoState.archived}
                                 color={(theme.vars || theme).palette.error.main}/>
                </Stack>
            )}

            {/* Daily Processing Chart */}
            {history.length > 0 && (
                <Card variant="outlined" sx={{mb: 3}}>
                    <CardContent>
                        <Typography component="h2" variant="subtitle2" gutterBottom>
                            Daily Processing (Last 30 Days)
                        </Typography>
                        <BarChart
                            borderRadius={4}
                            colors={[
                                (theme.vars || theme).palette.success.main,
                                (theme.vars || theme).palette.error.main,
                            ]}
                            xAxis={[
                                {
                                    scaleType: 'band' as const,
                                    data: chartDates,
                                    categoryGapRatio: 0.4,
                                    height: 24,
                                },
                            ]}
                            yAxis={[{width: 50}]}
                            series={[
                                {id: 'processed', label: 'Processed', data: chartProcessed, stack: 'A'},
                                {id: 'failed', label: 'Failed', data: chartFailed, stack: 'A'},
                            ]}
                            height={200}
                            margin={{left: 0, right: 0, top: 20, bottom: 0}}
                            grid={{horizontal: true}}
                        />
                    </CardContent>
                </Card>
            )}

            {/* Task Browser */}
            <Card variant="outlined" sx={{mb: 3}}>
                <CardContent>
                    <Tabs
                        value={tabIndex}
                        onChange={(_, v) => {
                            setTabIndex(v);
                            setPage(1);
                        }}
                        sx={{mb: 2}}
                    >
                        {TASK_STATES.map((state) => {
                            const count = queueInfoState ? (queueInfoState as unknown as Record<string, number>)[state] ?? 0 : 0;
                            return (
                                <Tab
                                    key={state}
                                    label={
                                        <Stack direction="row" spacing={0.5} alignItems="center">
                                            <span>{state.charAt(0).toUpperCase() + state.slice(1)}</span>
                                            {count > 0 && (
                                                <Chip
                                                    label={count}
                                                    size="small"
                                                    color={stateColors[state]}
                                                    sx={{height: 20, fontSize: '0.7rem'}}
                                                />
                                            )}
                                        </Stack>
                                    }
                                />
                            );
                        })}
                    </Tabs>

                    {/* Bulk actions */}
                    <Stack direction="row" spacing={1} sx={{mb: 1}}>
                        {currentState === 'archived' && (
                            <>
                                <Button size="small" variant="outlined" onClick={handleRunAllArchived}>
                                    Run All Archived
                                </Button>
                                <Button size="small" variant="outlined" color="error"
                                        onClick={handleDeleteAllArchived}>
                                    Delete All Archived
                                </Button>
                            </>
                        )}
                        {currentState === 'retry' && (
                            <Button size="small" variant="outlined" onClick={handleRunAllRetry}>
                                Run All Retry
                            </Button>
                        )}
                        {currentState === 'completed' && (
                            <Button size="small" variant="outlined" color="error"
                                    onClick={handleDeleteAllCompleted}>
                                Delete All Completed
                            </Button>
                        )}
                    </Stack>

                    <DataGrid
                        autoHeight
                        rows={tasks}
                        columns={taskColumns}
                        getRowId={(row) => row.id}
                        loading={tasksLoading}
                        paginationMode="server"
                        paginationModel={{page: page - 1, pageSize}}
                        paginationMeta={{hasNextPage}}
                        onPaginationModelChange={(model) => {
                            if (model.pageSize !== pageSize) setPageSize(model.pageSize);
                            if (model.page !== page - 1) setPage(model.page + 1);
                        }}
                        pageSizeOptions={[10, 30, 50, 100]}
                        rowCount={rowCount}
                        hideFooterSelectedRowCount
                        density="compact"
                        disableColumnResize
                    />
                </CardContent>
            </Card>
        </Box>
    );
}
