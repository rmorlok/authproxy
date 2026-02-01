import * as React from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import Grid from '@mui/material/Grid';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import {DataGrid, GridColDef, GridEventListener, GridSortModel} from '@mui/x-data-grid';
import Chip from '@mui/material/Chip';
import Stack from '@mui/material/Stack';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import {
    listConnectors, ConnectorVersionState, Connector, ListResponse, ListConnectorsParams, namespaceAndChildren
} from '@authproxy/api';
import dayjs from 'dayjs';
import {useQueryState, parseAsInteger, parseAsStringLiteral, parseAsString} from 'nuqs'
import {useNavigate} from "react-router-dom";
import {useSelector} from "react-redux";
import {selectCurrentNamespacePath} from "../store/namespacesSlice";

function renderState(state: ConnectorVersionState) {
    const colors: Record<ConnectorVersionState, "default" | "success" | "error" | "info" | "warning" | "primary" | "secondary"> = {
        [ConnectorVersionState.DRAFT]: 'secondary',
        [ConnectorVersionState.PRIMARY]: 'primary',
        [ConnectorVersionState.ACTIVE]: 'info',
        [ConnectorVersionState.ARCHIVED]: 'default',
    };

    return <Chip label={state} color={colors[state]} size="small" />;
}

export const columns: GridColDef<Connector>[] = [
    {
        field: 'id',
        headerName: 'ID',
        flex: 0.8,
        minWidth: 110,
        sortable: true,
    },
    {
        field: 'version',
        headerName: 'Version',
        flex: 0.4,
        minWidth: 70,
        sortable: true,
    },
    {
        field: 'namespace',
        headerName: 'Namespace',
        flex: 0.4,
        minWidth: 90,
        sortable: true,
    },
    {
        field: 'state',
        headerName: 'State',
        flex: 0.3,
        minWidth: 60,
        sortable: true,
        renderCell: (params) => renderState(params.value as ConnectorVersionState),
    },
    {
        field: 'labels',
        headerName: 'Labels',
        flex: 0.7,
        minWidth: 120,
        sortable: false,
        renderCell: (params) => {
            const labels = params.value as Record<string, string> | undefined;
            if (!labels || Object.keys(labels).length === 0) return null;
            return (
                <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{ py: 0.5 }}>
                    {Object.entries(labels).map(([key, value]) => (
                        <Chip key={key} label={`${key}: ${value}`} size="small" variant="outlined" />
                    ))}
                </Stack>
            );
        },
    },
    {
        field: 'display_name',
        headerName: 'Display Name',
        flex: 0.5,
        minWidth: 80,
        sortable: false,
    },
    {
        field: 'description',
        headerName: 'Description',
        flex: 0.8,
        minWidth: 80,
        sortable: false,
    },
    {
        field: 'versions',
        headerName: 'Num Versions',
        flex: 0.4,
        minWidth: 80,
        sortable: false,
    },
    {
        field: 'states',
        headerName: 'States',
        flex: 0.4,
        minWidth: 80,
        sortable: false,
        renderCell: (params) => {
            return (<div>
                {(params.value as ConnectorVersionState[]).map((v) => renderState(v))}
            </div>);
        },
    },
    {
        field: 'created_at',
        headerName: 'Created At',
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
        flex: 1,
        minWidth: 100,
        sortable: true,
        valueGetter: (value) => {
            return dayjs(value).format('MMM DD, YYYY, h:mm A');
        }

    },
];

export default function Connectors() {
    const defaultPageSize = 20;
    const stateOptions = useMemo(() => [
        { label: 'All', value: '' },
        { label: 'Draft', value: ConnectorVersionState.DRAFT },
        { label: 'Primary', value: ConnectorVersionState.PRIMARY },
        { label: 'Active', value: ConnectorVersionState.ACTIVE },
        { label: 'Archived', value: ConnectorVersionState.ARCHIVED },
    ], []);
    const navigate = useNavigate();
    const stateVals = useMemo(() => stateOptions.map(opt => opt.value), [stateOptions]);
    const ns = useSelector(selectCurrentNamespacePath);

    const [rows, setRows] = useState<Connector[]>([]);
    const [rowCount, setRowCount] = useState<number>(-1);
    const [loading, setLoading] = useState<boolean>(false);
    const [error, setError] = useState<string | null>(null);

    const [page, setPage] = useQueryState<number>('page', parseAsInteger.withDefault(1));
    const [pageSize, setPageSize] = useQueryState<number>('page_size', parseAsInteger.withDefault(defaultPageSize));
    const [stateFilter, setStateFilter] = useQueryState<string>('state', parseAsStringLiteral(stateVals).withDefault('')); // empty = all
    const [sort, setSort] = useQueryState<string>('sort', parseAsString.withDefault(''));

    const [hasNextPage, setHasNextPage] = useState<boolean>(false);

    // Simple cache to allow going back without re-fetching
    const responsesCacheRef = useRef<ListResponse<Connector>[]>([]);
    const pageRequestCacheRef = useRef<Set<number>>(new Set());

    // Handle row click with meta/ctrl key checking
    const handleRowClick: GridEventListener<'rowClick'> = (params, event) => {
        // Get the ID of the clicked row
        const id = params.id;

        // Determine the URL for this item
        const itemUrl = `/connectors/${id}`;

        // Handle ctrl/cmd+click or middle click (open in new tab)
        if (event.ctrlKey || event.metaKey || event.button === 1) {
            window.open(itemUrl, '_blank');
        } else {
            // Regular click - navigate in current tab
            navigate(itemUrl);
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

                const params: ListConnectorsParams = prevResp?.cursor ? {cursor: prevResp.cursor} : {
                    state: (stateFilter as ConnectorVersionState) || undefined,
                    namespace: namespaceAndChildren(ns),
                    order_by: sort || undefined,
                    limit: pageSize,
                };

                const resp = await listConnectors(params);

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
            setError(e?.message || 'Failed to load connectors');
        } finally {
            setLoading(false);
        }
    };

    // Initial load and when filter/pageSize changes
    useEffect(() => {
        // Reset cursors/cache and immediately fetch first page to ensure initial load
        resetPagination();
        fetchPage(1);
    }, [ns, pageSize, sort, stateFilter]);

    useEffect(() => {
        fetchPage(page);
    }, [page]);

    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            <Typography component="h2" variant="h6" sx={{mb: 2}}>
                Connectors
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

            <style>
                {`
                  .clickable-row {
                    cursor: pointer;
                  }
                `}
            </style>
            <Grid size={{xs: 12, lg: 12}}>
                <DataGrid
                    autoHeight
                    rows={rows}
                    columns={columns}
                    getRowId={(row) => row.id}
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
