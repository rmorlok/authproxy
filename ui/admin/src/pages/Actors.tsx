import * as React from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import Grid from '@mui/material/Grid';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import {DataGrid, GridColDef, GridSortModel} from '@mui/x-data-grid';
import {
    listActors, Actor, ListResponse, ListActorsParams
} from '@authproxy/api';
import dayjs from 'dayjs';
import {useQueryState, parseAsInteger, parseAsString} from 'nuqs'

export const columns: GridColDef<Actor>[] = [
    {
        field: 'id',
        headerName: 'ID',
        flex: 0.8,
        minWidth: 110,
        sortable: true,
    },
    {
        field: 'external_id',
        headerName: 'External ID',
        flex: 0.4,
        minWidth: 80,
        sortable: true,
    },
    {
        field: 'email',
        headerName: 'Email',
        flex: 0.3,
        minWidth: 60,
        sortable: true,
    },
    {
        field: 'admin',
        headerName: 'Admin',
        flex: 0.5,
        minWidth: 80,
        sortable: true,
    },
    {
        field: 'super_admin',
        headerName: 'Super Admin',
        flex: 0.5,
        minWidth: 80,
        sortable: true,
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

export default function Actors() {
    const defaultPageSize = 20;

    const [rows, setRows] = useState<Actor[]>([]);
    const [rowCount, setRowCount] = useState<number>(-1);
    const [loading, setLoading] = useState<boolean>(false);
    const [error, setError] = useState<string | null>(null);

    const [page, setPage] = useQueryState<number>('page', parseAsInteger.withDefault(1));
    const [pageSize, setPageSize] = useQueryState<number>('page_size', parseAsInteger.withDefault(defaultPageSize));
    const [sort, setSort] = useQueryState<string>('sort', parseAsString.withDefault(''));

    const [hasNextPage, setHasNextPage] = useState<boolean>(false);

    // Simple cache to allow going back without re-fetching
    const responsesCacheRef = useRef<ListResponse<Actor>[]>([]);
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

                const params: ListActorsParams = prevResp?.cursor ? {cursor: prevResp.cursor} : {
                    order_by: sort || undefined,
                    limit: pageSize,
                };

                const resp = await listActors(params);

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
            setError(e?.message || 'Failed to load actors');
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
    }, [pageSize, sort]);

    useEffect(() => {
        fetchPage(page);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [page, pageSize, sort]); // TODO: only page?

    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            <Typography component="h2" variant="h6" sx={{mb: 2}}>
                Actors
            </Typography>

            <Grid size={{xs: 12, lg: 12}}>
                <DataGrid
                    autoHeight
                    rows={rows}
                    columns={columns}
                    getRowId={(row) => row.id}
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
