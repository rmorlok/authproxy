import * as React from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import Grid from '@mui/material/Grid';
import Box from '@mui/material/Box';
import Drawer from '@mui/material/Drawer';
import Typography from '@mui/material/Typography';
import {DataGrid, GridColDef, GridSortModel, GridEventListener} from '@mui/x-data-grid';
import Stack from '@mui/material/Stack';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import {
    ListResponse, RequestEntryRecord,
    RequestType, ListRequestsParams, listRequests
} from '../api';
import dayjs from 'dayjs';
import {HttpStatusChip, Duration} from '../util';
import {useQueryState, parseAsInteger, parseAsStringLiteral, parseAsString} from 'nuqs'
import RequestDetail from "../components/RequestDetail";

export const columns: (GridColDef<RequestEntryRecord> & {hideInitial?: boolean})[] = [
    {
        field: 'timestamp',
        headerName: 'Timestamp',
        minWidth: 220,
        sortable: true,
        valueGetter: (value, _) => {
            return dayjs(value).format('YYYY-MM-DDTHH:mm:ssZ[Z]');
        }
    },
    {
        field: 'method',
        headerName: 'Method',
        align: 'center',
        sortable: true,
    },
    {
        field: 'response_status_code',
        headerName: 'Status',
        sortable: true,
        align: 'center',
        renderCell: (params) => (<HttpStatusChip value={params.value} />),
    },
    {
        field: 'type',
        headerName: 'Type',
        sortable: true,
    },
    {
        field: 'request_id',
        headerName: 'ID',
        sortable: true,
        hideInitial: true,
    },
    {
        field: 'correlation_id',
        headerName: 'Correlation ID',
        sortable: true,
        hideInitial: true,
    },
    {
        field: 'duration',
        headerName: 'Duration',
        sortable: true,
        renderCell: (params) => (<Duration value={params.value} />),

    },
    {
        field: 'connection_id',
        headerName: 'Connection ID',
        minWidth: 290,
        sortable: true,
    },
    {
        field: 'connector_version',
        headerName: 'Connector Version',
        sortable: false,
        hideInitial: true,
    },
    {
        field: 'host',
        headerName: 'Host',
        sortable: true,
        hideInitial: true,
    },
    {
        field: 'scheme',
        headerName: 'Scheme',
        sortable: true,
        hideInitial: true,
    },
    {
        field: 'path',
        headerName: 'Path',
        sortable: true,
        minWidth: 200,
        flex: 1,
    },
    {
        field: 'request_http_version',
        headerName: 'Req. HTTP Version',
        sortable: true,
        hideInitial: true,
    },
    {
        field: 'request_size_bytes',
        headerName: 'Req. Size',
        description: 'request size in bytes',
        sortable: true,
        hideInitial: true,
    },
    {
        field: 'request_mime_type',
        headerName: 'Req. Mime Type',
        description: 'request mime type',
        sortable: true,
        hideInitial: true,
    },
    {
        field: 'response_http_version',
        headerName: 'Resp. HTTP Version',
        description: 'response http version',
        sortable: false,
        hideInitial: true,
    },
    {
        field: 'response_size_bytes',
        headerName: 'Size',
        description: 'response size in bytes',
        sortable: false,
    },
    {
        field: 'response_mime_type',
        headerName: 'Mime Type',
        description: 'response mime type',
        sortable: false,
        minWidth: 250,
    },
    {
        field: 'response_error',
        headerName: 'Error',
        description: 'error message from executing request',
        sortable: false,
    },
    {
        field: 'internal_timeout',
        headerName: 'Timeout',
        description: 'did request recording timeout before completing',
        sortable: false,
        hideInitial: true,
    },
    {
        field: 'request_cancelled',
        headerName: 'Cancelled',
        description: 'was the request cancelled before the full body was consumed',
        sortable: false,
        hideInitial: true,
    },
    {
        field: 'full_request_recorded',
        headerName: 'Full',
        description: 'was the full request/response recorded',
        sortable: false,
        hideInitial: true,
    },
];

const columnVisibilityModel = columns.filter(c => c.hideInitial).reduce((acc, col) => {
        acc[col.field] = false;
        return acc;
    }, {} as Record<string, boolean>);

export default function Requests() {
    const defaultPageSize = 20;
    const stateOptions = useMemo(() => [
        { label: 'All', value: '' },
        { label: 'Global', value: RequestType.GLOBAL },
        { label: 'Proxy', value: RequestType.PROXY },
        { label: 'OAuth', value: RequestType.OAUTH },
        { label: 'Public', value: RequestType.PUBLIC },
    ], []);
    const stateVals = useMemo(() => stateOptions.map(opt => opt.value), [stateOptions]);

    const [rows, setRows] = useState<RequestEntryRecord[]>([]);
    const [rowCount, setRowCount] = useState<number>(-1);
    const [loading, setLoading] = useState<boolean>(false);
    const [error, setError] = useState<string | null>(null);
    const [drawerOpen, setDrawerOpen] = useState<boolean>(false);

    const [page, setPage] = useQueryState<number>('page', parseAsInteger.withDefault(1));
    const [pageSize, setPageSize] = useQueryState<number>('page_size', parseAsInteger.withDefault(defaultPageSize));
    const [typeFilter, setTypeFilter] = useQueryState<string>('type', parseAsStringLiteral(stateVals).withDefault('')); // empty = all
    const [sort, setSort] = useQueryState<string>('sort', parseAsString.withDefault(''));
    const [requestId, setRequestId] = useQueryState<string>('id', parseAsString.withDefault(''));

    const [hasNextPage, setHasNextPage] = useState<boolean>(false);

    // Simple cache to allow going back without re-fetching
    const responsesCacheRef = useRef<ListResponse<RequestEntryRecord>[]>([]);
    const pageRequestCacheRef = useRef<Set<number>>(new Set());

    // Handle row click with meta/ctrl key checking
    const handleRowClick: GridEventListener<'rowClick'> = (params, event) => {
        // Get the ID of the clicked row
        const id = params.id;

        // Determine the URL for this item
        const itemUrl = `/requests/${id}`;

        // Handle ctrl/cmd+click or middle click (open in new tab)
        if (event.ctrlKey || event.metaKey || event.button === 1) {
            window.open(itemUrl, '_blank');
        } else {
            // Regular click - open drawer
            setRequestId(params.id.toString());
        }
    };

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

                const params: ListRequestsParams = prevResp?.cursor ? {cursor: prevResp.cursor} : {
                    type: (typeFilter as RequestType) || undefined,
                    order_by: sort || undefined,
                    limit: pageSize,
                };

                const resp = await listRequests(params);

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
            setError(e?.message || 'Failed to load requests');
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
    }, [pageSize, sort, typeFilter]);

    useEffect(() => {
        fetchPage(page);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [page, pageSize, sort, typeFilter]); // TODO: only page?

    useEffect(() => {
        setDrawerOpen(requestId !== '');
    }, [requestId]);

    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            <Typography component="h2" variant="h6" sx={{mb: 2}}>
                Requests
            </Typography>
            <Stack direction="row" spacing={2} alignItems="center" sx={{ mb: 2 }}>
                <FormControl size="small" sx={{ minWidth: 220 }}>
                    <InputLabel id="type-filter-label">Request Type</InputLabel>
                    <Select
                        labelId="type-filter-label"
                        value={typeFilter}
                        label="Request Type"
                        onChange={(e) => setTypeFilter(e.target.value)}
                    >
                        {stateOptions.map(opt => (
                            <MenuItem key={opt.label} value={opt.value}>{opt.label}</MenuItem>
                        ))}
                    </Select>
                </FormControl>
            </Stack>

            <Grid size={{xs: 12, lg: 12}}>
                <style>
                    {`
                  .clickable-row {
                    cursor: pointer;
                  }
                `}
                </style>
                <DataGrid
                    rows={rows}
                    columns={columns}
                    getRowId={(row) => row.request_id}
                    getRowClassName={(params) =>
                        params.indexRelativeToCurrentPage % 2 === 0 ? 'clickable-row even' : 'clickable-row odd'
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
                    onRowClick={handleRowClick}
                    hideFooterSelectedRowCount
                    density="compact"
                    autosizeOnMount={true}
                    initialState={{
                        columns: {
                            columnVisibilityModel,
                        }
                    }}
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
            <Drawer
                anchor="right"
                open={drawerOpen}
                onClose={() => setRequestId('')}
            >
                <Box sx={{width: {xs: '100vw', sm: 520, md: 720}, pl: 2}}>
                    {(() => {
                        const rec = rows.find(r => r.request_id === requestId);
                        if (!requestId) return null;
                        if (rec && rec.full_request_recorded === false) {
                            return (
                                <Box sx={{p: 2}}>
                                    <Typography variant="h6" sx={{mb: 1}}>Request Details</Typography>
                                    <Typography variant="body2" color="text.secondary">
                                        Full request/response was not recorded for this entry.
                                    </Typography>
                                </Box>
                            );
                        }
                        return (
                            <RequestDetail
                                requestId={requestId}
                                onClose={() => setRequestId('')}
                                showOpenFullPage={true} />
                        );
                    })()}
                </Box>
            </Drawer>
        </Box>
    );
}
