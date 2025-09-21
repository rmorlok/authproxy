import * as React from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import Grid from '@mui/material/Grid';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import { DataGrid, GridColDef } from '@mui/x-data-grid';
import Chip from '@mui/material/Chip';
import Stack from '@mui/material/Stack';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import { Connection, ConnectionState, listConnections } from '../api';

function renderState(state: ConnectionState) {
    const colors: Record<ConnectionState, "default" | "success" | "error" | "info" | "warning" | "primary" | "secondary"> = {
        [ConnectionState.CREATED]: 'default',
        [ConnectionState.CONNECTED]: 'success',
        [ConnectionState.FAILED]: 'error',
        [ConnectionState.DISCONNECTING]: 'info',
        [ConnectionState.DISCONNECTED]: 'warning'
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

    const [stateFilter, setStateFilter] = useState<string>(''); // empty = all

    // Cursor management: cursors[pageIndex] gives the cursor to fetch that page
    const cursorsRef = useRef<(string | undefined)[]>([undefined]);
    // Simple cache to allow going back without re-fetching
    const pageCacheRef = useRef<Map<number, Connection[]>>(new Map());

    const hasNextRef = useRef<boolean>(true); // if last fetch returned no cursor => at end

    const resetPagination = () => {
        cursorsRef.current = [undefined];
        pageCacheRef.current = new Map();
        hasNextRef.current = true;
        setPage(0);
    };

    const fetchPage = async (targetPage: number) => {
        // Require stepping forward: if asking to jump ahead more than one page and cursor missing, fetch sequentially
        setLoading(true);
        setError(null);
        try {
            // If we have it cached, use it
            const cached = pageCacheRef.current.get(targetPage);
            if (cached) {
                setRows(cached);
                setLoading(false);
                return;
            }

            // If we don't know the cursor for this page yet, advance sequentially from the last known
            while (cursorsRef.current.length <= targetPage) {
                const cursorForNext = cursorsRef.current[cursorsRef.current.length - 1];
                const resp = await listConnections(stateFilter || undefined, cursorForNext, pageSize);
                const items = resp.data.items;
                const nextCursor = resp.data.cursor;
                const newPageIndex = cursorsRef.current.length - 1; // the page we just fetched
                pageCacheRef.current.set(newPageIndex, items);
                // Store cursor for the next page
                cursorsRef.current.push(nextCursor);
                if (!nextCursor) {
                    hasNextRef.current = false;
                    break;
                }
            }

            const data = pageCacheRef.current.get(targetPage) || [];
            setRows(data);
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
                    paginationMode="server"
                    paginationModel={{ page, pageSize }}
                    onPaginationModelChange={(model) => {
                        // DataGrid uses 0-based page index
                        if (model.pageSize !== pageSize) setPageSize(model.pageSize);
                        if (model.page !== page) setPage(model.page);
                    }}
                    rowCount={hasNextRef.current ? (page + 2) * pageSize : page * pageSize + rows.length}
                    pageSizeOptions={[10, 20, 50, 100]}
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
