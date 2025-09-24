import * as React from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import Grid from '@mui/material/Grid';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import {DataGrid, GridColDef, GridPaginationMeta} from '@mui/x-data-grid';
import Chip from '@mui/material/Chip';
import Stack from '@mui/material/Stack';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import {Connection, ConnectionState, listConnections, ListConnectionsResponse} from '../api';

function renderState(state: ConnectionState) {
    const colors: Record<ConnectionState, "default" | "success" | "error" | "info" | "warning" | "primary" | "secondary"> = {
        [ConnectionState.CREATED]: 'primary',
        [ConnectionState.CONNECTED]: 'success',
        [ConnectionState.FAILED]: 'error',
        [ConnectionState.DISCONNECTING]: 'warning',
        [ConnectionState.DISCONNECTED]: 'default'
    };

    return <Chip label={state} color={colors[state]} size="small" />;
}

export const columns: GridColDef[] = [
    { field: 'id',
        headerName: 'ID',
        flex: 1.5,
        minWidth: 200
    },
    {
        field: 'state',
        headerName: 'State',
        flex: 0.5,
        minWidth: 80,
        renderCell: (params) => renderState(params.value as ConnectionState),
    },
    {
        field: 'created_at',
        headerName: 'Created At',
        headerAlign: 'right',
        align: 'right',
        flex: 1,
        minWidth: 80,
    },
    {
        field: 'updated_at',
        headerName: 'Updated At',
        headerAlign: 'right',
        align: 'right',
        flex: 1,
        minWidth: 100,
    },
];

export default function Connections() {
    const [rows, setRows] = useState<Connection[]>([]);
    const [loading, setLoading] = useState<boolean>(false);
    const [error, setError] = useState<string | null>(null);

    const [page, setPage] = useState<number>(0);
    const [pageSize, setPageSize] = useState<number>(20);
    const [hasNextPage, setHasNextPage] = useState<boolean>(false);

    const [stateFilter, setStateFilter] = useState<string>(''); // empty = all

    // Simple cache to allow going back without re-fetching
    const responsesCacheRef = useRef<ListConnectionsResponse[]>([]);
    const pageRequestCacheRef = useRef<Set<number>>(new Set());

    const resetPagination = () => {
        responsesCacheRef.current = [];
        pageRequestCacheRef.current = new Set();
        setHasNextPage(false);
        setPage(0);
    };

    const fetchPage = async (targetPage: number) => {
        // Require stepping forward: if asking to jump ahead more than one page and cursor missing, fetch sequentially
        setLoading(true);
        setHasNextPage(false);
        setError(null);
        try {
            // If we have it cached, use it
            const cached = responsesCacheRef.current[targetPage];
            if (cached) {
                setRows(cached.items);
                setLoading(false);
                setHasNextPage(!!cached.cursor);
                return;
            }

            // If we don't know the cursor for this page yet, advance sequentially from the last known
            while (responsesCacheRef.current.length <= targetPage && (
                responsesCacheRef.current.length === 0 ||
                !!responsesCacheRef.current[responsesCacheRef.current.length - 1].cursor
                )
            ) {
                // Avoid multiple calls for the same page
                if (pageRequestCacheRef.current.has(targetPage)) {
                    return;
                }
                pageRequestCacheRef.current.add(targetPage);

                const thisPage = responsesCacheRef.current.length;
                const prevResp = responsesCacheRef.current[responsesCacheRef.current.length - 1];
                const resp = await listConnections(stateFilter || undefined, prevResp?.cursor, pageSize);

                if(resp.status !== 200) {
                    setError("Failed to fetch page of results from server");
                    return;
                }

                responsesCacheRef.current[thisPage] = resp.data; // This handles cases where the same page is requested multiple times
            }

            const data = responsesCacheRef.current[targetPage];
            setRows(data?.items || []);
            setHasNextPage(!!data?.cursor);
        } catch (e: any) {
            setError(e?.message || 'Failed to load connections');
        } finally {
            setLoading(false);
        }
    };

    // Initial load and when filter/pageSize changes
    useEffect(() => {
        // Reset cursors/cache and immediately fetch first page to ensure initial load
        resetPagination();
        fetchPage(0);
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [stateFilter, pageSize]);

    useEffect(() => {
        fetchPage(page);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [page, stateFilter, pageSize]);


    const stateOptions = useMemo(() => [
        { label: 'All', value: '' },
        { label: 'Created', value: ConnectionState.CREATED },
        { label: 'Connected', value: ConnectionState.CONNECTED },
        { label: 'Failed', value: ConnectionState.FAILED },
        { label: 'Disconnecting', value: ConnectionState.DISCONNECTING },
        { label: 'Disconnected', value: ConnectionState.DISCONNECTED },
    ], []);

    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            <Typography component="h2" variant="h6" sx={{mb: 2}}>
                Connections
            </Typography>
            <Stack direction="row" spacing={2} alignItems="center" sx={{ mb: 2 }}>
                <FormControl size="small" sx={{ minWidth: 220 }}>
                    <InputLabel id="state-filter-label">State</InputLabel>
                    <Select
                        labelId="state-filter-label"
                        value={stateFilter}
                        label="State"
                        onChange={(e) => setStateFilter(e.target.value)}
                    >
                        {stateOptions.map(opt => (
                            <MenuItem key={opt.label} value={opt.value}>{opt.label}</MenuItem>
                        ))}
                    </Select>
                </FormControl>
            </Stack>

            <Grid size={{xs: 12, lg: 12}}>
                <DataGrid
                    autoHeight
                    rows={rows}
                    columns={columns}
                    getRowId={(row) => (row as Connection).id}
                    getRowClassName={(params) =>
                        params.indexRelativeToCurrentPage % 2 === 0 ? 'even' : 'odd'
                    }
                    loading={loading}
                    sortingMode="server"
                    paginationMode="server"
                    paginationModel={{ page, pageSize }}
                    paginationMeta={{hasNextPage}}
                    onPaginationModelChange={(model) => {
                        console.log(model);
                        // DataGrid uses 0-based page index
                        if (model.pageSize !== pageSize) setPageSize(model.pageSize);
                        if (model.page !== page) setPage(model.page);
                    }}
                    pageSizeOptions={[2, 5, 10, 20, 50, 100]}
                    rowCount={hasNextPage
                        ? -1
                        : responsesCacheRef.current.map((v) => v.items.length).reduceRight((acc, val)=> acc+val, 0) /* this is a weird bug that requires this */}
                    hideFooterSelectedRowCount
                    disableColumnResize
                    density="compact"
                    slots={{}}
                    slotProps={{
                        filterPanel: {
                            filterFormProps: {
                                logicOperatorInputProps: {
                                    variant: 'outlined',
                                    size: 'small',
                                },
                                columnInputProps: {
                                    variant: 'outlined',
                                    size: 'small',
                                    sx: { mt: 'auto' },
                                },
                                operatorInputProps: {
                                    variant: 'outlined',
                                    size: 'small',
                                    sx: { mt: 'auto' },
                                },
                                valueInputProps: {
                                    InputComponentProps: {
                                        variant: 'outlined',
                                        size: 'small',
                                    },
                                },
                            },
                        },
                    }}
                />
                <Typography>
                    Page: {page}; HasNextPage: {String(hasNextPage)}; Row Count: {hasNextPage
                    ? -1
                    : responsesCacheRef.current.map((v) => v.items.length).reduceRight((acc, val)=> acc+val, 0) /* this is a weird bug that requires this */}
                </Typography>
                {error && (
                    <Typography color="error" sx={{ mt: 1 }}>{error}</Typography>
                )}
            </Grid>
        </Box>
    );
}
