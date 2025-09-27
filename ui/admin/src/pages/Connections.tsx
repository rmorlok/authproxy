import * as React from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import Grid from '@mui/material/Grid';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import {DataGrid, GridColDef, GridSortModel} from '@mui/x-data-grid';
import Chip from '@mui/material/Chip';
import Stack from '@mui/material/Stack';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import {Connection, ConnectionState, listConnections, ListConnectionsParams, ListConnectionsResponse} from '../api';
import dayjs from 'dayjs';
import {useQueryState, parseAsInteger, parseAsStringLiteral, parseAsString} from 'nuqs'

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
        minWidth: 110,
        sortable: true,
    },
    {
        field: 'state',
        headerName: 'State',
        flex: 0.4,
        minWidth: 80,
        sortable: true,
        renderCell: (params) => renderState(params.value as ConnectionState),
    },
    {
        field: 'connector.type',
        headerName: 'Connector Type',
        flex: 0.5,
        minWidth: 80,
        sortable: false,
        valueGetter: (_, row) => row.connector.type,
    },
    {
        field: 'connector.id',
        headerName: 'Connector ID',
        flex: 0.8,
        minWidth: 80,
        sortable: false,
        valueGetter: (_, row) => row.connector.id,
    },
    {
        field: 'connector.version',
        headerName: 'Connector Version',
        flex: 0.4,
        minWidth: 80,
        sortable: false,
        valueGetter: (_, row) => row.connector.version,
    },
    {
        field: 'created_at',
        headerName: 'Created At',
        headerAlign: 'right',
        align: 'right',
        flex: 1,
        minWidth: 80,
        sortable: true,
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
        sortable: true,
        valueGetter: (value) => {
            return dayjs(value).format('MMM DD, YYYY, h:mm A');
        }

    },
];

export default function Connections() {
    const defaultPageSize = 20;
    const stateOptions = useMemo(() => [
        { label: 'All', value: '' },
        { label: 'Created', value: ConnectionState.CREATED },
        { label: 'Connected', value: ConnectionState.CONNECTED },
        { label: 'Failed', value: ConnectionState.FAILED },
        { label: 'Disconnecting', value: ConnectionState.DISCONNECTING },
        { label: 'Disconnected', value: ConnectionState.DISCONNECTED },
    ], []);
    const stateVals = useMemo(() => stateOptions.map(opt => opt.value), [stateOptions]);

    const [rows, setRows] = useState<Connection[]>([]);
    const [rowCount, setRowCount] = useState<number>(-1);
    const [loading, setLoading] = useState<boolean>(false);
    const [error, setError] = useState<string | null>(null);

    const [page, setPage] = useQueryState<number>('page', parseAsInteger.withDefault(1));
    const [pageSize, setPageSize] = useQueryState<number>('page_size', parseAsInteger.withDefault(defaultPageSize));
    const [stateFilter, setStateFilter] = useQueryState<string>('state', parseAsStringLiteral(stateVals).withDefault('')); // empty = all
    const [sort, setSort] = useQueryState<string>('sort', parseAsString.withDefault(''));

    const [hasNextPage, setHasNextPage] = useState<boolean>(false);

    // Simple cache to allow going back without re-fetching
    const responsesCacheRef = useRef<ListConnectionsResponse[]>([]);
    const pageRequestCacheRef = useRef<Set<number>>(new Set());

    const handleSortModelChange = React.useCallback((sortModel: GridSortModel) => {
        if(sortModel.length === 0) {
            setSort('');
        } else {
            const sortField = sortModel[0].field;
            const sortDir = sortModel[0].sort === 'desc' ? 'desc' : 'asc';
            setSort(`${sortField} ${sortDir}`);
        }
    }, []);

    const resetPagination = () => {
        responsesCacheRef.current = [];
        pageRequestCacheRef.current = new Set();
        setHasNextPage(false);
        setPage(1);
        setRowCount(-1);
    };

    const fetchPage = async (targetPageOneBased: number) => {
        const targetPageZeroBased = targetPageOneBased - 1;
        // Require stepping forward: if asking to jump ahead more than one page and cursor missing, fetch sequentially
        setLoading(true);
        setHasNextPage(false);
        setError(null);
        try {
            // If we have it cached, use it
            const cached = responsesCacheRef.current[targetPageZeroBased];
            if (cached) {
                setRows(cached.items);
                setLoading(false);
                setHasNextPage(!!cached.cursor);
                return;
            }

            // If we don't know the cursor for this page yet, advance sequentially from the last known
            while (responsesCacheRef.current.length <= targetPageZeroBased && (
                responsesCacheRef.current.length === 0 ||
                !!responsesCacheRef.current[responsesCacheRef.current.length - 1].cursor
                )
            ) {
                // Avoid multiple calls for the same page
                if (pageRequestCacheRef.current.has(targetPageZeroBased)) {
                    return;
                }
                pageRequestCacheRef.current.add(targetPageZeroBased);

                const thisPage = responsesCacheRef.current.length;
                const prevResp = responsesCacheRef.current[responsesCacheRef.current.length - 1];

                const params: ListConnectionsParams = prevResp?.cursor ? {cursor: prevResp.cursor} : {
                    state: (stateFilter as ConnectionState) || undefined,
                    order_by: sort || undefined,
                    limit: pageSize,
                };

                const resp = await listConnections(params);

                if(resp.status !== 200) {
                    setError("Failed to fetch page of results from server");
                    return;
                }

                responsesCacheRef.current[thisPage] = resp.data; // This handles cases where the same page is requested multiple times
            }

            const data = responsesCacheRef.current[targetPageZeroBased];
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

    // Initial load and when filter/pageSize changes
    useEffect(() => {
        // Reset cursors/cache and immediately fetch first page to ensure initial load
        resetPagination();
        fetchPage(1);
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [pageSize, sort, stateFilter]);

    useEffect(() => {
        fetchPage(page);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [page, pageSize, sort, stateFilter]); // TODO: only page?

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
                    onSortModelChange={handleSortModelChange}
                    paginationMode="server"
                    paginationModel={{ page: page-1, pageSize }}
                    paginationMeta={{hasNextPage}}
                    onPaginationModelChange={(model) => {
                        console.log(model);
                        // DataGrid uses 0-based page index
                        if (model.pageSize !== pageSize) setPageSize(model.pageSize);
                        if (model.page !== page-1) setPage(model.page+1);
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
                {error && (
                    <Typography color="error" sx={{ mt: 1 }}>{error}</Typography>
                )}
            </Grid>
        </Box>
    );
}
