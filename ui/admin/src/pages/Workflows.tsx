import * as React from 'react';
import {useCallback, useEffect, useRef, useState} from 'react';
import {useNavigate} from 'react-router-dom';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import Chip from '@mui/material/Chip';
import FormControlLabel from '@mui/material/FormControlLabel';
import Stack from '@mui/material/Stack';
import Switch from '@mui/material/Switch';
import Typography from '@mui/material/Typography';
import {DataGrid, GridColDef, GridEventListener} from '@mui/x-data-grid';
import {useQueryState, parseAsInteger} from 'nuqs';
import {
    listWorkflowInstances,
    WorkflowInstanceRef,
    ListWorkflowInstancesResponse,
} from '@authproxy/api';

const stateColors: Record<string, 'primary' | 'secondary' | 'success' | 'warning' | 'default'> = {
    active: 'primary',
    continued_as_new: 'warning',
    finished: 'success',
};

function workflowId(row: WorkflowInstanceRef) {
    return `${row.instance?.instance_id ?? ''}:${row.instance?.execution_id ?? ''}`;
}

function formatTimestamp(value?: string) {
    return value ? new Date(value).toLocaleString() : '-';
}

export default function Workflows() {
    const navigate = useNavigate();
    const [page, setPage] = useQueryState<number>('page', parseAsInteger.withDefault(1));
    const [pageSize, setPageSize] = useQueryState<number>('page_size', parseAsInteger.withDefault(30));
    const [autoRefresh, setAutoRefresh] = useState(true);
    const [instances, setInstances] = useState<WorkflowInstanceRef[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [hasNextPage, setHasNextPage] = useState(false);
    const [rowCount, setRowCount] = useState(-1);

    const responsesCacheRef = useRef<ListWorkflowInstancesResponse[]>([]);
    const pageRequestCacheRef = useRef<Set<number>>(new Set());

    const fetchPage = useCallback(async (targetPageOneBased: number) => {
        const targetPageZeroBased = targetPageOneBased - 1;
        setLoading(true);
        setHasNextPage(false);
        setError(null);

        try {
            const cached = responsesCacheRef.current[targetPageZeroBased];
            if (cached) {
                setInstances(cached.items);
                setLoading(false);
                setHasNextPage(!!cached.cursor);
                return;
            }

            while (responsesCacheRef.current.length <= targetPageZeroBased && (
                responsesCacheRef.current.length === 0 ||
                !!responsesCacheRef.current[responsesCacheRef.current.length - 1].cursor
            )) {
                const thisPage = responsesCacheRef.current.length;
                if (pageRequestCacheRef.current.has(thisPage)) break;
                pageRequestCacheRef.current.add(thisPage);

                const prevResp = responsesCacheRef.current[responsesCacheRef.current.length - 1];
                const resp = await listWorkflowInstances(prevResp?.cursor ? {cursor: prevResp.cursor} : {limit: pageSize});

                if (resp.status !== 200) {
                    setError('Failed to fetch workflow instances from server');
                    return;
                }

                responsesCacheRef.current[thisPage] = resp.data;
            }

            const data = responsesCacheRef.current[targetPageZeroBased];
            setInstances(data?.items || []);
            setHasNextPage(!!data?.cursor);

            if (!data?.cursor) {
                setRowCount(
                    responsesCacheRef.current
                        .map((v) => v.items.length)
                        .reduce((acc, val) => acc + val, 0)
                );
            }
        } catch {
            setError('Failed to load workflow instances');
        } finally {
            setLoading(false);
        }
    }, [pageSize]);

    const refreshInstances = useCallback(() => {
        responsesCacheRef.current = [];
        pageRequestCacheRef.current = new Set();
        setRowCount(-1);
        fetchPage(page);
    }, [fetchPage, page]);

    useEffect(() => {
        responsesCacheRef.current = [];
        pageRequestCacheRef.current = new Set();
        setRowCount(-1);
        setPage(1);
        fetchPage(1);
    }, [pageSize]);

    useEffect(() => {
        fetchPage(page);
    }, [page]);

    const autoRefreshRef = useRef(autoRefresh);
    autoRefreshRef.current = autoRefresh;

    useEffect(() => {
        if (!autoRefresh) return;

        const interval = setInterval(() => {
            if (document.visibilityState === 'hidden') return;
            if (!autoRefreshRef.current) return;
            refreshInstances();
        }, 5000);

        return () => clearInterval(interval);
    }, [autoRefresh, refreshInstances]);

    const handleWorkflowClick: GridEventListener<'rowClick'> = (params) => {
        const instance = params.row.instance;
        if (!instance) return;
        navigate(`/workflows/${encodeURIComponent(instance.instance_id)}/${encodeURIComponent(instance.execution_id)}`);
    };

    const columns: GridColDef<WorkflowInstanceRef>[] = [
        {
            field: 'instance_id',
            headerName: 'Instance ID',
            flex: 1.2,
            minWidth: 180,
            valueGetter: (_value, row) => row.instance?.instance_id ?? '',
        },
        {
            field: 'execution_id',
            headerName: 'Execution ID',
            flex: 1.2,
            minWidth: 180,
            valueGetter: (_value, row) => row.instance?.execution_id ?? '',
        },
        {field: 'queue', headerName: 'Queue', flex: 0.6, minWidth: 110},
        {
            field: 'state',
            headerName: 'State',
            flex: 0.5,
            minWidth: 120,
            renderCell: (params) => (
                <Chip
                    label={params.value || 'unknown'}
                    size="small"
                    color={stateColors[String(params.value)] ?? 'default'}
                    variant={params.value === 'finished' ? 'outlined' : 'filled'}
                />
            ),
        },
        {
            field: 'created_at',
            headerName: 'Created',
            flex: 0.8,
            minWidth: 160,
            renderCell: (params) => formatTimestamp(params.value as string | undefined),
        },
        {
            field: 'completed_at',
            headerName: 'Completed',
            flex: 0.8,
            minWidth: 160,
            renderCell: (params) => formatTimestamp(params.value as string | undefined),
        },
    ];

    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{mb: 2}}>
                <Typography component="h2" variant="h6">Workflow Monitoring</Typography>
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

            {error && <Alert severity="error" sx={{mb: 2}} onClose={() => setError(null)}>{error}</Alert>}

            <Card variant="outlined" sx={{mb: 3}}>
                <CardContent>
                    <Typography component="h2" variant="subtitle2" gutterBottom>
                        Instances
                    </Typography>
                    <style>{`.clickable-row { cursor: pointer; }`}</style>
                    <DataGrid
                        autoHeight
                        rows={instances}
                        columns={columns}
                        getRowId={workflowId}
                        getRowClassName={() => 'clickable-row'}
                        onRowClick={handleWorkflowClick}
                        loading={loading}
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
