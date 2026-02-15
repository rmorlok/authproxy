import * as React from 'react';
import {useCallback, useEffect, useRef, useState} from 'react';
import {useNavigate} from 'react-router-dom';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import Grid from '@mui/material/Grid';
import Chip from '@mui/material/Chip';
import Stack from '@mui/material/Stack';
import FormControlLabel from '@mui/material/FormControlLabel';
import Switch from '@mui/material/Switch';
import {DataGrid, GridColDef, GridEventListener} from '@mui/x-data-grid';
import {
    listQueues,
    listServers,
    listSchedulerEntries,
    QueueInfo,
    ServerInfo,
    SchedulerEntry,
} from '@authproxy/api';

export default function Tasks() {
    const navigate = useNavigate();

    const [queues, setQueues] = useState<QueueInfo[]>([]);
    const [autoRefresh, setAutoRefresh] = useState(true);
    const [servers, setServers] = useState<ServerInfo[]>([]);
    const [schedulerEntries, setSchedulerEntries] = useState<SchedulerEntry[]>([]);
    const [error, setError] = useState<string | null>(null);

    const fetchQueues = useCallback(async () => {
        try {
            const resp = await listQueues();
            if (resp.status === 200) {
                setQueues(resp.data);
            }
        } catch {
            setError('Failed to load queues');
        }
    }, []);

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

    useEffect(() => {
        fetchQueues();
        fetchInfra();
    }, []);

    // Auto-refresh
    const autoRefreshRef = useRef(autoRefresh);
    autoRefreshRef.current = autoRefresh;

    useEffect(() => {
        if (!autoRefresh) return;

        const interval = setInterval(() => {
            if (document.visibilityState === 'hidden') return;
            if (!autoRefreshRef.current) return;
            fetchQueues();
        }, 5000);

        return () => clearInterval(interval);
    }, [autoRefresh, fetchQueues]);

    const handleQueueClick: GridEventListener<'rowClick'> = (params) => {
        navigate(`/tasks/queues/${params.row.queue}`);
    };

    const queueColumns: GridColDef<QueueInfo>[] = [
        {field: 'queue', headerName: 'Queue', flex: 1, minWidth: 120},
        {field: 'size', headerName: 'Total', flex: 0.4, minWidth: 70},
        {
            field: 'pending', headerName: 'Pending', flex: 0.4, minWidth: 70,
            renderCell: (params) => params.value ? (
                <Chip label={params.value} size="small" color="info" variant="outlined"/>
            ) : <span>0</span>,
        },
        {
            field: 'active', headerName: 'Active', flex: 0.4, minWidth: 70,
            renderCell: (params) => params.value ? (
                <Chip label={params.value} size="small" color="primary" variant="outlined"/>
            ) : <span>0</span>,
        },
        {field: 'scheduled', headerName: 'Scheduled', flex: 0.4, minWidth: 80},
        {
            field: 'retry', headerName: 'Retry', flex: 0.4, minWidth: 70,
            renderCell: (params) => params.value ? (
                <Chip label={params.value} size="small" color="warning" variant="outlined"/>
            ) : <span>0</span>,
        },
        {
            field: 'archived', headerName: 'Archived', flex: 0.4, minWidth: 70,
            renderCell: (params) => params.value ? (
                <Chip label={params.value} size="small" color="error" variant="outlined"/>
            ) : <span>0</span>,
        },
        {field: 'completed', headerName: 'Completed', flex: 0.4, minWidth: 80},
        {
            field: 'processed_total', headerName: 'Processed', flex: 0.5, minWidth: 80,
        },
        {
            field: 'failed_total', headerName: 'Failed', flex: 0.4, minWidth: 70,
            renderCell: (params) => params.value ? (
                <Typography color="error" variant="body2">{params.value}</Typography>
            ) : <span>0</span>,
        },
        {
            field: 'paused', headerName: 'Status', flex: 0.4, minWidth: 80,
            renderCell: (params) => params.value ?
                <Chip label="Paused" size="small" color="warning"/> :
                <Chip label="Active" size="small" color="success" variant="outlined"/>,
        },
        {
            field: 'latency_seconds', headerName: 'Latency', flex: 0.4, minWidth: 80,
            renderCell: (params) => {
                const secs = params.value as number;
                if (secs < 1) return '<1s';
                if (secs < 60) return `${Math.round(secs)}s`;
                return `${Math.round(secs / 60)}m`;
            },
        },
    ];

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

    const schedulerColumns: GridColDef<SchedulerEntry>[] = [
        {field: 'id', headerName: 'ID', flex: 0.6, minWidth: 80},
        {field: 'spec', headerName: 'Cron Spec', flex: 0.6, minWidth: 100},
        {field: 'task_type', headerName: 'Task Type', flex: 0.8, minWidth: 120},
        {field: 'next', headerName: 'Next Run', flex: 0.8, minWidth: 140},
        {field: 'prev', headerName: 'Last Run', flex: 0.8, minWidth: 140, renderCell: (params) => params.value || '-'},
    ];

    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{mb: 2}}>
                <Typography component="h2" variant="h6">Task Monitoring</Typography>
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

            {error && (
                <Typography color="error" sx={{mb: 2}}>{error}</Typography>
            )}

            {/* Queues Table */}
            <Card variant="outlined" sx={{mb: 3}}>
                <CardContent>
                    <Typography component="h2" variant="subtitle2" gutterBottom>
                        Queues
                    </Typography>
                    <style>{`.clickable-row { cursor: pointer; }`}</style>
                    <DataGrid
                        autoHeight
                        rows={queues}
                        columns={queueColumns}
                        getRowId={(row) => row.queue}
                        getRowClassName={() => 'clickable-row'}
                        onRowClick={handleQueueClick}
                        hideFooterSelectedRowCount
                        density="compact"
                        disableColumnResize
                        pageSizeOptions={[10, 25]}
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
