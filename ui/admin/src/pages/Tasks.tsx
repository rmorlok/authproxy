import * as React from 'react';
import {useCallback, useEffect, useRef, useState} from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import Grid from '@mui/material/Grid';
import Tabs from '@mui/material/Tabs';
import Tab from '@mui/material/Tab';
import Chip from '@mui/material/Chip';
import IconButton from '@mui/material/IconButton';
import Tooltip from '@mui/material/Tooltip';
import Button from '@mui/material/Button';
import Stack from '@mui/material/Stack';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
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
import {useQueryState, parseAsInteger, parseAsString} from 'nuqs';
import {
    listQueues,
    getQueueHistory,
    listTasksByState,
    listServers,
    listSchedulerEntries,
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
    ServerInfo,
    SchedulerEntry,
    MonitoringTaskState,
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

export default function Tasks() {
    const theme = useTheme();

    // State
    const [queues, setQueues] = useState<QueueInfo[]>([]);
    const [selectedQueue, setSelectedQueue] = useQueryState<string>('queue', parseAsString.withDefault(''));
    const [tabIndex, setTabIndex] = useQueryState<number>('tab', parseAsInteger.withDefault(0));
    const [page, setPage] = useQueryState<number>('page', parseAsInteger.withDefault(1));
    const [pageSize, setPageSize] = useQueryState<number>('page_size', parseAsInteger.withDefault(30));
    const [autoRefresh, setAutoRefresh] = useState(true);

    const [history, setHistory] = useState<DailyStats[]>([]);
    const [tasks, setTasks] = useState<MonitoringTaskInfo[]>([]);
    const [tasksLoading, setTasksLoading] = useState(false);
    const [servers, setServers] = useState<ServerInfo[]>([]);
    const [schedulerEntries, setSchedulerEntries] = useState<SchedulerEntry[]>([]);
    const [error, setError] = useState<string | null>(null);

    const currentQueue = selectedQueue || (queues.length > 0 ? queues[0].queue : '');
    const currentQueueInfo = queues.find(q => q.queue === currentQueue);
    const currentState = TASK_STATES[tabIndex] || 'pending';

    // Fetch queues
    const fetchQueues = useCallback(async () => {
        try {
            const resp = await listQueues();
            if (resp.status === 200) {
                setQueues(resp.data);
                if (!selectedQueue && resp.data.length > 0) {
                    setSelectedQueue(resp.data[0].queue);
                }
            }
        } catch {
            setError('Failed to load queues');
        }
    }, [selectedQueue, setSelectedQueue]);

    // Fetch history
    const fetchHistory = useCallback(async () => {
        if (!currentQueue) return;
        try {
            const resp = await getQueueHistory(currentQueue, {days: 30});
            if (resp.status === 200) {
                setHistory(resp.data);
            }
        } catch {
            // ignore
        }
    }, [currentQueue]);

    // Fetch tasks
    const fetchTasks = useCallback(async () => {
        if (!currentQueue) return;
        setTasksLoading(true);
        try {
            const resp = await listTasksByState(currentQueue, currentState, {page, page_size: pageSize});
            if (resp.status === 200) {
                setTasks(resp.data);
            }
        } catch {
            setError('Failed to load tasks');
        } finally {
            setTasksLoading(false);
        }
    }, [currentQueue, currentState, page, pageSize]);

    // Fetch servers + scheduler
    const fetchInfra = useCallback(async () => {
        try {
            const [serversResp, schedulerResp] = await Promise.all([
                listServers(),
                listSchedulerEntries(),
            ]);
            if (serversResp.status === 200) setServers(serversResp.data);
            if (schedulerResp.status === 200) setSchedulerEntries(schedulerResp.data);
        } catch {
            // ignore
        }
    }, []);

    // Initial load
    useEffect(() => {
        fetchQueues();
        fetchInfra();
    }, []);

    // Fetch on queue change
    useEffect(() => {
        if (currentQueue) {
            fetchHistory();
            fetchTasks();
        }
    }, [currentQueue, fetchHistory, fetchTasks]);

    // Auto-refresh
    const autoRefreshRef = useRef(autoRefresh);
    autoRefreshRef.current = autoRefresh;

    useEffect(() => {
        if (!autoRefresh) return;

        const interval = setInterval(() => {
            if (document.visibilityState === 'hidden') return;
            if (!autoRefreshRef.current) return;
            fetchQueues();
            fetchTasks();
        }, 5000);

        return () => clearInterval(interval);
    }, [autoRefresh, fetchQueues, fetchTasks]);

    // Action handlers
    const handleRunTask = async (queue: string, taskId: string) => {
        await runTask(queue, taskId);
        fetchTasks();
        fetchQueues();
    };
    const handleArchiveTask = async (queue: string, taskId: string) => {
        await archiveTask(queue, taskId);
        fetchTasks();
        fetchQueues();
    };
    const handleCancelTask = async (queue: string, taskId: string) => {
        await cancelTask(queue, taskId);
        fetchTasks();
        fetchQueues();
    };
    const handleDeleteTask = async (queue: string, taskId: string) => {
        await deleteTask(queue, taskId);
        fetchTasks();
        fetchQueues();
    };
    const handlePauseQueue = async (queue: string) => {
        await pauseQueue(queue);
        fetchQueues();
    };
    const handleUnpauseQueue = async (queue: string) => {
        await unpauseQueue(queue);
        fetchQueues();
    };
    const handleRunAllArchived = async () => {
        if (!currentQueue) return;
        await runAllArchivedTasks(currentQueue);
        fetchTasks();
        fetchQueues();
    };
    const handleRunAllRetry = async () => {
        if (!currentQueue) return;
        await runAllRetryTasks(currentQueue);
        fetchTasks();
        fetchQueues();
    };
    const handleDeleteAllArchived = async () => {
        if (!currentQueue) return;
        await deleteAllArchivedTasks(currentQueue);
        fetchTasks();
        fetchQueues();
    };
    const handleDeleteAllCompleted = async () => {
        if (!currentQueue) return;
        await deleteAllCompletedTasks(currentQueue);
        fetchTasks();
        fetchQueues();
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

    // Server table columns
    const serverColumns: GridColDef<ServerInfo>[] = [
        {field: 'host', headerName: 'Host', flex: 0.8, minWidth: 100},
        {field: 'pid', headerName: 'PID', flex: 0.4, minWidth: 60},
        {field: 'concurrency', headerName: 'Concurrency', flex: 0.4, minWidth: 80},
        {
            field: 'queues',
            headerName: 'Queues',
            flex: 0.8,
            minWidth: 120,
            renderCell: (params) => {
                const queues = params.value as Record<string, number>;
                return Object.entries(queues).map(([q, p]) => `${q}:${p}`).join(', ');
            },
        },
        {field: 'status', headerName: 'Status', flex: 0.4, minWidth: 80},
        {field: 'started', headerName: 'Started', flex: 0.8, minWidth: 140},
        {
            field: 'active_workers',
            headerName: 'Active Workers',
            flex: 0.4,
            minWidth: 100,
            renderCell: (params) => (params.value as unknown[]).length,
        },
    ];

    // Scheduler table columns
    const schedulerColumns: GridColDef<SchedulerEntry>[] = [
        {field: 'id', headerName: 'ID', flex: 0.6, minWidth: 80},
        {field: 'spec', headerName: 'Cron Spec', flex: 0.6, minWidth: 100},
        {field: 'task_type', headerName: 'Task Type', flex: 0.8, minWidth: 120},
        {field: 'next', headerName: 'Next Run', flex: 0.8, minWidth: 140},
        {field: 'prev', headerName: 'Last Run', flex: 0.8, minWidth: 140, renderCell: (params) => params.value || '-'},
    ];

    // Chart data
    const chartDates = history.map(h => h.date);
    const chartProcessed = history.map(h => h.processed);
    const chartFailed = history.map(h => h.failed);

    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            {/* Header */}
            <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{mb: 2}}>
                <Typography component="h2" variant="h6">Task Monitoring</Typography>
                <Stack direction="row" spacing={2} alignItems="center">
                    {queues.length > 1 && (
                        <FormControl size="small" sx={{minWidth: 160}}>
                            <InputLabel>Queue</InputLabel>
                            <Select
                                value={currentQueue}
                                label="Queue"
                                onChange={(e) => setSelectedQueue(e.target.value)}
                            >
                                {queues.map(q => (
                                    <MenuItem key={q.queue} value={q.queue}>{q.queue}</MenuItem>
                                ))}
                            </Select>
                        </FormControl>
                    )}
                    {currentQueueInfo && (
                        currentQueueInfo.paused ? (
                            <Tooltip title="Unpause queue">
                                <IconButton onClick={() => handleUnpauseQueue(currentQueue)} color="primary">
                                    <PlayCircleOutlineIcon/>
                                </IconButton>
                            </Tooltip>
                        ) : (
                            <Tooltip title="Pause queue">
                                <IconButton onClick={() => handlePauseQueue(currentQueue)} color="warning">
                                    <PauseIcon/>
                                </IconButton>
                            </Tooltip>
                        )
                    )}
                    {currentQueueInfo?.paused && (
                        <Chip label="PAUSED" color="warning" size="small"/>
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
            {currentQueueInfo && (
                <Stack direction="row" spacing={2} sx={{mb: 3}} flexWrap="wrap">
                    <SummaryCard label="Pending" value={currentQueueInfo.pending}
                                 color={(theme.vars || theme).palette.info.main}/>
                    <SummaryCard label="Active" value={currentQueueInfo.active}
                                 color={(theme.vars || theme).palette.primary.main}/>
                    <SummaryCard label="Retry" value={currentQueueInfo.retry}
                                 color={(theme.vars || theme).palette.warning.main}/>
                    <SummaryCard label="Archived" value={currentQueueInfo.archived}
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
                        {TASK_STATES.map((state, i) => {
                            const count = currentQueueInfo ? (currentQueueInfo as unknown as Record<string, number>)[state] ?? 0 : 0;
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
                        onPaginationModelChange={(model) => {
                            if (model.pageSize !== pageSize) setPageSize(model.pageSize);
                            if (model.page !== page - 1) setPage(model.page + 1);
                        }}
                        pageSizeOptions={[10, 30, 50, 100]}
                        rowCount={-1}
                        hideFooterSelectedRowCount
                        density="compact"
                        disableColumnResize
                    />
                </CardContent>
            </Card>

            {/* Servers */}
            <Grid container spacing={3}>
                <Grid size={{xs: 12}}>
                    <Card variant="outlined">
                        <CardContent>
                            <Typography component="h2" variant="subtitle2" gutterBottom>
                                Worker Servers
                            </Typography>
                            <DataGrid
                                autoHeight
                                rows={servers}
                                columns={serverColumns}
                                getRowId={(row) => row.id}
                                hideFooterSelectedRowCount
                                density="compact"
                                disableColumnResize
                                pageSizeOptions={[10]}
                            />
                        </CardContent>
                    </Card>
                </Grid>

                {/* Scheduler Entries */}
                <Grid size={{xs: 12}}>
                    <Card variant="outlined">
                        <CardContent>
                            <Typography component="h2" variant="subtitle2" gutterBottom>
                                Scheduler Entries
                            </Typography>
                            <DataGrid
                                autoHeight
                                rows={schedulerEntries}
                                columns={schedulerColumns}
                                getRowId={(row) => row.id}
                                hideFooterSelectedRowCount
                                density="compact"
                                disableColumnResize
                                pageSizeOptions={[10]}
                            />
                        </CardContent>
                    </Card>
                </Grid>
            </Grid>
        </Box>
    );
}
