import * as React from 'react';
import {useEffect, useMemo, useRef, useState} from 'react';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import FormControl from '@mui/material/FormControl';
import Grid from '@mui/material/Grid';
import InputLabel from '@mui/material/InputLabel';
import MenuItem from '@mui/material/MenuItem';
import Select from '@mui/material/Select';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import {DataGrid, GridColDef, GridEventListener, GridSortModel} from '@mui/x-data-grid';
import {
    ListNamespaceParams,
    ListResponse,
    Namespace,
    NamespaceState,
    namespaces,
} from '@authproxy/api';
import dayjs from 'dayjs';
import {parseAsInteger, parseAsString, parseAsStringLiteral, useQueryState} from 'nuqs';
import {useNavigate} from 'react-router-dom';
import {useSelector} from 'react-redux';
import {selectCurrentNamespaceMatcher} from '../store/namespacesSlice';
import {namespaceDetailPath, toSnakeCase} from '../util';
import {ResourceLabelChips} from '../components/ResourceMetadataFields';

function renderState(state: NamespaceState) {
    const colors: Record<string, 'default' | 'success' | 'warning'> = {
        [NamespaceState.ACTIVE]: 'success',
        [NamespaceState.DESTROYING]: 'warning',
        [NamespaceState.DESTROYED]: 'default',
    };

    return <Chip label={state} color={colors[state] || 'default'} size="small"/>;
}

export const columns: GridColDef<Namespace>[] = [
    {
        field: 'name',
        headerName: 'Name',
        flex: 0.7,
        minWidth: 120,
        sortable: false,
    },
    {
        field: 'path',
        headerName: 'Path',
        flex: 1,
        minWidth: 180,
        sortable: true,
    },
    {
        field: 'state',
        headerName: 'State',
        flex: 0.4,
        minWidth: 100,
        sortable: true,
        renderCell: (params) => renderState(params.value as NamespaceState),
    },
    {
        field: 'keyId',
        headerName: 'Key',
        flex: 0.8,
        minWidth: 140,
        sortable: false,
        valueGetter: (value) => value || 'Inherited',
    },
    {
        field: 'labels',
        headerName: 'Labels',
        flex: 0.9,
        minWidth: 160,
        sortable: false,
        renderCell: (params) => <ResourceLabelChips labels={params.value as Record<string, string> | undefined}/>,
    },
    {
        field: 'createdAt',
        headerName: 'Created At',
        flex: 0.9,
        minWidth: 170,
        sortable: true,
        valueGetter: (value) => dayjs(value).format('MMM DD, YYYY, h:mm A'),
    },
    {
        field: 'updatedAt',
        headerName: 'Updated At',
        flex: 0.9,
        minWidth: 170,
        sortable: true,
        valueGetter: (value) => dayjs(value).format('MMM DD, YYYY, h:mm A'),
    },
];

export default function Namespaces() {
    const defaultPageSize = 20;
    const navigate = useNavigate();
    const namespaceMatcher = useSelector(selectCurrentNamespaceMatcher);
    const stateOptions = useMemo(() => [
        {label: 'All', value: ''},
        {label: 'Active', value: NamespaceState.ACTIVE},
        {label: 'Destroying', value: NamespaceState.DESTROYING},
        {label: 'Destroyed', value: NamespaceState.DESTROYED},
    ], []);
    const stateValues = useMemo(() => stateOptions.map((option) => option.value), [stateOptions]);

    const [rows, setRows] = useState<Namespace[]>([]);
    const [rowCount, setRowCount] = useState(-1);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [hasNextPage, setHasNextPage] = useState(false);

    const [page, setPage] = useQueryState<number>('page', parseAsInteger.withDefault(1));
    const [pageSize, setPageSize] = useQueryState<number>('pageSize', parseAsInteger.withDefault(defaultPageSize));
    const [stateFilter, setStateFilter] = useQueryState<string>(
        'state',
        parseAsStringLiteral(stateValues).withDefault(''),
    );
    const [sort, setSort] = useQueryState<string>('sort', parseAsString.withDefault(''));

    const responsesCacheRef = useRef<ListResponse<Namespace>[]>([]);
    const pageRequestCacheRef = useRef<Set<number>>(new Set());

    const resetPagination = () => {
        responsesCacheRef.current = [];
        pageRequestCacheRef.current = new Set();
        setHasNextPage(false);
        setPage(1);
        setRowCount(-1);
    };

    const fetchPage = async (targetPageOneBased: number) => {
        const targetPageZeroBased = targetPageOneBased - 1;
        setLoading(true);
        setHasNextPage(false);
        setError(null);

        try {
            const cached = responsesCacheRef.current[targetPageZeroBased];
            if (cached) {
                setRows(cached.items);
                setHasNextPage(Boolean(cached.cursor));
                return;
            }

            while (
                responsesCacheRef.current.length <= targetPageZeroBased &&
                (responsesCacheRef.current.length === 0 || Boolean(responsesCacheRef.current.at(-1)?.cursor))
            ) {
                const thisPage = responsesCacheRef.current.length;
                if (pageRequestCacheRef.current.has(thisPage)) {
                    return;
                }
                pageRequestCacheRef.current.add(thisPage);

                const previousResponse = responsesCacheRef.current.at(-1);
                const params: ListNamespaceParams = previousResponse?.cursor
                    ? {cursor: previousResponse.cursor}
                    : {
                        state: (stateFilter as NamespaceState) || undefined,
                        namespace: namespaceMatcher,
                        orderBy: sort || undefined,
                        limit: pageSize,
                    };
                const response = await namespaces.list(params);

                if (response.status !== 200) {
                    setError('Failed to fetch page of results from server');
                    return;
                }

                responsesCacheRef.current[thisPage] = response.data;
            }

            const data = responsesCacheRef.current[targetPageZeroBased];
            setRows(data?.items || []);
            const pageHasNext = Boolean(data?.cursor);
            setHasNextPage(pageHasNext);

            if (!pageHasNext) {
                setRowCount(responsesCacheRef.current.reduce((count, response) => count + response.items.length, 0));
            }
        } catch (err: any) {
            setError(err?.message || 'Failed to load namespaces');
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        resetPagination();
        void fetchPage(1);
    }, [namespaceMatcher, pageSize, sort, stateFilter]);

    useEffect(() => {
        void fetchPage(page);
    }, [page]);

    const handleRowClick: GridEventListener<'rowClick'> = (params, event) => {
        const itemUrl = namespaceDetailPath(params.row.path);
        if (event.ctrlKey || event.metaKey || event.button === 1) {
            window.open(itemUrl, '_blank');
        } else {
            navigate(itemUrl);
        }
    };

    const handleSortModelChange = React.useCallback((sortModel: GridSortModel) => {
        if (sortModel.length === 0) {
            setSort('');
            return;
        }

        const sortField = toSnakeCase(sortModel[0].field);
        const sortDirection = sortModel[0].sort === 'desc' ? 'desc' : 'asc';
        setSort(`${sortField} ${sortDirection}`);
    }, [setSort]);

    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            <Typography component="h2" variant="h6" sx={{mb: 2}}>
                Namespaces
            </Typography>
            <Stack direction="row" spacing={2} alignItems="center" sx={{mb: 2}}>
                <FormControl size="small" sx={{minWidth: 220}}>
                    <InputLabel id="namespace-state-filter-label">State</InputLabel>
                    <Select
                        labelId="namespace-state-filter-label"
                        value={stateFilter}
                        label="State"
                        onChange={(event) => setStateFilter(event.target.value)}
                    >
                        {stateOptions.map((option) => (
                            <MenuItem key={option.label} value={option.value}>{option.label}</MenuItem>
                        ))}
                    </Select>
                </FormControl>
            </Stack>

            <style>{`.clickable-row { cursor: pointer; }`}</style>
            <Grid size={{xs: 12, lg: 12}}>
                <DataGrid
                    autoHeight
                    rows={rows}
                    columns={columns}
                    getRowId={(row) => row.path}
                    getRowClassName={(params) =>
                        params.indexRelativeToCurrentPage % 2 === 0
                            ? 'clickable-row even'
                            : 'clickable-row odd'
                    }
                    loading={loading}
                    sortingMode="server"
                    onSortModelChange={handleSortModelChange}
                    paginationMode="server"
                    paginationModel={{page: page - 1, pageSize}}
                    paginationMeta={{hasNextPage}}
                    onPaginationModelChange={(model) => {
                        if (model.pageSize !== pageSize) setPageSize(model.pageSize);
                        if (model.page !== page - 1) setPage(model.page + 1);
                    }}
                    pageSizeOptions={[5, 10, 20, 50, 100]}
                    rowCount={rowCount}
                    onRowClick={handleRowClick}
                    hideFooterSelectedRowCount
                    disableColumnResize
                    density="compact"
                />
                {error && <Typography color="error" sx={{mt: 1}}>{error}</Typography>}
            </Grid>
        </Box>
    );
}
