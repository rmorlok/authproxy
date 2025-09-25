import * as React from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import Grid from '@mui/material/Grid';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import {DataGrid, GridColDef} from '@mui/x-data-grid';
import Chip from '@mui/material/Chip';
import Stack from '@mui/material/Stack';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import {Connection, ConnectionState, listConnections, ListConnectionsResponse} from '../api';
import dayjs from 'dayjs';

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

export const columns: GridColDef<Connection>[] = [
    { field: 'id',
        headerName: 'ID',
        flex: 0.8,
        minWidth: 110
    },
    {
        field: 'state',
        headerName: 'State',
        flex: 0.4,
        minWidth: 80,
        renderCell: (params) => renderState(params.value as ConnectionState),
    },
    {
        field: 'connector.type',
        headerName: 'Connector Type',
        flex: 0.5,
        minWidth: 80,
        valueGetter: (_, row) => row.connector.type,
    },
    {
        field: 'connector.id',
        headerName: 'Connector ID',
        flex: 0.8,
        minWidth: 80,
        valueGetter: (_, row) => row.connector.id,
    },
    {
        field: 'connector.version',
        headerName: 'Connector Version',
        flex: 0.4,
        minWidth: 80,
        valueGetter: (_, row) => row.connector.version,
    },
    {
        field: 'created_at',
        headerName: 'Created At',
        headerAlign: 'right',
        align: 'right',
        flex: 1,
        minWidth: 80,
        valueGetter: (value, _) => {
            return dayjs(value).format('MMM DD, YYYY, h:mm A');
        }

    },
    {
        field: 'updated_at',
        headerName: 'Updated At',
        headerAlign: 'right',
        align: 'right',
        flex: 1,
        minWidth: 100,
        valueGetter: (value) => {
            return dayjs(value).format('MMM DD, YYYY, h:mm A');
        }

    },
];

export default function Connections() {
    const [searchParams, setSearchParams] = useSearchParams();
    const [rows, setRows] = useState<Connection[]>([]);
    const [rowCount, setRowCount] = useState<number>(-1);
    const [loading, setLoading] = useState<boolean>(false);
    const [error, setError] = useState<string | null>(null);

    // Initialize from URL params
    const qpPage = Number(searchParams.get('page'));
    const initialPage = Number.isFinite(qpPage) && qpPage >= 0 ? qpPage : 0;
    const initialStateFilter = searchParams.get('state') ?? '';
    const qpPageSize = Number(searchParams.get('pageSize'));
    const defaultPageSize = 20;
    const initialPageSize = Number.isFinite(qpPageSize) && qpPageSize > 0 ? qpPageSize : defaultPageSize;

    const [page, setPage] = useState<number>(initialPage);
    const [pageSize, setPageSize] = useState<number>(initialPageSize);
    const [hasNextPage, setHasNextPage] = useState<boolean>(false);

    const [stateFilter, setStateFilter] = useState<string>(initialStateFilter); // empty = all

    // Simple cache to allow going back without re-fetching
    const responsesCacheRef = useRef<ListConnectionsResponse[]>([]);
    const pageRequestCacheRef = useRef<Set<number>>(new Set());

    const resetPagination = () => {
        responsesCacheRef.current = [];
        pageRequestCacheRef.current = new Set();
        setHasNextPage(false);
        setPage(0);
        setRowCount(-1);
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

            const hnp = !!data?.cursor;
            setHasNextPage(hnp);

            if(!hnp) {
                setRowCount(responsesCacheRef.current.map((v) => v.items.length).reduceRight((acc, val)=> acc+val, 0));
            }
        } catch (e: any) {
            setError(e?.message || 'Failed to load connections');
        } finally {
            setLoading(false);
        }
    };

    // Sync state to URL when page, stateFilter or pageSize changes
    useEffect(() => {
        const params = new URLSearchParams(searchParams);
        // Only write when different to avoid loops
        const curPage = params.get('page');
        const curState = params.get('state') ?? '';
        const curPageSize = params.get('pageSize');
        const desiredPage = String(page);
        const desiredState = stateFilter || '';
        const desiredPageSize = String(pageSize);
        let changed = false;
        if (curPage !== desiredPage) { params.set('page', desiredPage); changed = true; }
        if (curState !== desiredState) {
            if (desiredState) params.set('state', desiredState); else params.delete('state');
            changed = true;
        }
        // Only include pageSize in URL if it's not the default
        if (desiredPageSize !== String(defaultPageSize)) {
            if (curPageSize !== desiredPageSize) { params.set('pageSize', desiredPageSize); changed = true; }
        } else if (curPageSize) {
            params.delete('pageSize');
            changed = true;
        }
        if (changed) {
            setSearchParams(params, { replace: true });
        }
    }, [page, stateFilter, pageSize, setSearchParams, searchParams]);

    // React to URL changes (back/forward or external navigation)
    useEffect(() => {
        const urlPageRaw = searchParams.get('page');
        const urlPage = urlPageRaw ? Number(urlPageRaw) : 0;
        const safePage = Number.isFinite(urlPage) && urlPage >= 0 ? urlPage : 0;
        const urlState = searchParams.get('state') ?? '';
        const urlPageSizeRaw = searchParams.get('pageSize');
        const urlPageSize = urlPageSizeRaw ? Number(urlPageSizeRaw) : defaultPageSize;
        const safePageSize = Number.isFinite(urlPageSize) && urlPageSize > 0 ? urlPageSize : defaultPageSize;

        // If URL differs from component state, update component state
        if (safePage !== page) {
            setPage(safePage);
        }
        if (urlState !== stateFilter) {
            setStateFilter(urlState);
        }
        if (safePageSize !== pageSize) {
            setPageSize(safePageSize);
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [searchParams]);

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
                    rowCount={rowCount}
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
                    Page: {page}; HasNextPage: {String(hasNextPage)}; Row Count: {rowCount}
                </Typography>
                {error && (
                    <Typography color="error" sx={{ mt: 1 }}>{error}</Typography>
                )}
            </Grid>
        </Box>
    );
}
