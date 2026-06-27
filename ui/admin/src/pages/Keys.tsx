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
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Alert from '@mui/material/Alert';
import AddIcon from '@mui/icons-material/Add';
import {
    listKeys, KeyState, Key, ListResponse,
    ListKeysParams, namespaceAndChildren, createKey, CreateKeyRequest
} from '@authproxy/api';
import dayjs from 'dayjs';
import {useQueryState, parseAsInteger, parseAsStringLiteral, parseAsString} from 'nuqs'
import {useNavigate} from "react-router-dom";
import {useSelector} from "react-redux";
import {selectCurrentNamespacePath} from "../store/namespacesSlice";
import KeyDataForm, {
    buildKeyDataPayload,
    createEmptyKeyDataFormState,
    KeyDataFormState,
    validateKeyDataFormState,
} from '../components/KeyDataForm';
import KeyValueRowsEditor, {
    duplicateKeys,
    KeyValueRow,
    rowsToMap,
} from '../components/KeyValueRowsEditor';

function renderState(state: KeyState) {
    const colors: Record<KeyState, "default" | "success" | "error" | "info" | "warning" | "primary" | "secondary"> = {
        [KeyState.ACTIVE]: 'success',
        [KeyState.DISABLED]: 'default',
    };

    return <Chip label={state} color={colors[state]} size="small" />;
}

export const columns: GridColDef<Key>[] = [
    {
        field: 'id',
        headerName: 'ID',
        flex: 0.8,
        minWidth: 130,
        sortable: false,
    },
    {
        field: 'namespace',
        headerName: 'Namespace',
        flex: 0.5,
        minWidth: 90,
        sortable: false,
    },
    {
        field: 'state',
        headerName: 'State',
        flex: 0.3,
        minWidth: 80,
        sortable: true,
        renderCell: (params) => renderState(params.value as KeyState),
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

export default function Keys() {
    const defaultPageSize = 20;
    const stateOptions = useMemo(() => [
        { label: 'All', value: '' },
        { label: 'Active', value: KeyState.ACTIVE },
        { label: 'Disabled', value: KeyState.DISABLED },
    ], []);
    const navigate = useNavigate();
    const stateVals = useMemo(() => stateOptions.map(opt => opt.value), [stateOptions]);
    const ns = useSelector(selectCurrentNamespacePath);

    const [rows, setRows] = useState<Key[]>([]);
    const [rowCount, setRowCount] = useState<number>(-1);
    const [loading, setLoading] = useState<boolean>(false);
    const [error, setError] = useState<string | null>(null);

    const [page, setPage] = useQueryState<number>('page', parseAsInteger.withDefault(1));
    const [pageSize, setPageSize] = useQueryState<number>('page_size', parseAsInteger.withDefault(defaultPageSize));
    const [stateFilter, setStateFilter] = useQueryState<string>('state', parseAsStringLiteral(stateVals).withDefault(''));
    const [sort, setSort] = useQueryState<string>('sort', parseAsString.withDefault(''));

    const [hasNextPage, setHasNextPage] = useState<boolean>(false);

    // Create dialog state
    const [createOpen, setCreateOpen] = useState(false);
    const [createLoading, setCreateLoading] = useState(false);
    const [createError, setCreateError] = useState<string | null>(null);
    const [createKeyData, setCreateKeyData] = useState<KeyDataFormState>(createEmptyKeyDataFormState());
    const [createLabelRows, setCreateLabelRows] = useState<KeyValueRow[]>([]);
    const [createAnnotationRows, setCreateAnnotationRows] = useState<KeyValueRow[]>([]);

    const responsesCacheRef = useRef<ListResponse<Key>[]>([]);
    const pageRequestCacheRef = useRef<Set<number>>(new Set());

    const handleRowClick: GridEventListener<'rowClick'> = (params, event) => {
        const id = params.id;
        const itemUrl = `/keys/${id}`;
        if (event.ctrlKey || event.metaKey || event.button === 1) {
            window.open(itemUrl, '_blank');
        } else {
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
        setLoading(true);
        setHasNextPage(false);
        setError(null);
        try {
            const cached = responsesCacheRef.current[targetPageZeroBased];
            if (cached) {
                setRows(cached.items);
                setLoading(false);
                setHasNextPage(!!cached.cursor);
                return;
            }

            while (responsesCacheRef.current.length <= targetPageZeroBased && (
                    responsesCacheRef.current.length === 0 ||
                    !!responsesCacheRef.current[responsesCacheRef.current.length - 1].cursor
                )
            ) {
                if (pageRequestCacheRef.current.has(targetPageZeroBased)) {
                    return;
                }
                pageRequestCacheRef.current.add(targetPageZeroBased);

                const thisPage = responsesCacheRef.current.length;
                const prevResp = responsesCacheRef.current[responsesCacheRef.current.length - 1];

                const params: ListKeysParams = prevResp?.cursor ? {cursor: prevResp.cursor} : {
                    state: (stateFilter as KeyState) || undefined,
                    namespace: namespaceAndChildren(ns),
                    order_by: sort || undefined,
                    limit: pageSize,
                };

                const resp = await listKeys(params);

                if(resp.status !== 200) {
                    setError("Failed to fetch page of results from server");
                    return;
                }

                responsesCacheRef.current[thisPage] = resp.data;
            }

            const data = responsesCacheRef.current[targetPageZeroBased];
            setRows(data?.items || []);

            const hnp = !!data?.cursor;
            setHasNextPage(hnp);

            if(!hnp) {
                setRowCount(responsesCacheRef.current.map((v) => v.items.length).reduceRight((acc, val)=> acc+val, 0));
            }
        } catch (e: any) {
            setError(e?.message || 'Failed to load keys');
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        resetPagination();
        fetchPage(1);
    }, [ns, pageSize, sort, stateFilter]);

    useEffect(() => {
        fetchPage(page);
    }, [page]);

    const onCreateSubmit = async () => {
        const duplicateLabels = duplicateKeys(createLabelRows);
        const duplicateAnnotations = duplicateKeys(createAnnotationRows);
        if (duplicateLabels.length > 0 || duplicateAnnotations.length > 0) {
            const parts = [];
            if (duplicateLabels.length > 0) parts.push(`duplicate labels: ${duplicateLabels.join(', ')}`);
            if (duplicateAnnotations.length > 0) parts.push(`duplicate annotations: ${duplicateAnnotations.join(', ')}`);
            setCreateError(parts.join('; '));
            return;
        }

        const keyDataError = validateKeyDataFormState(createKeyData);
        if (keyDataError) {
            setCreateError(keyDataError);
            return;
        }

        setCreateLoading(true);
        setCreateError(null);
        try {
            const request: CreateKeyRequest = {
                namespace: ns,
                key_data: buildKeyDataPayload(createKeyData),
                labels: rowsToMap(createLabelRows),
                annotations: rowsToMap(createAnnotationRows),
            };
            await createKey(request);
            setCreateOpen(false);
            resetCreateDialog();
            resetPagination();
            fetchPage(1);
        } catch (err: any) {
            const msg = err?.response?.data?.error || err.message || 'Failed to create key';
            setCreateError(msg);
        } finally {
            setCreateLoading(false);
        }
    };

    const resetCreateDialog = () => {
        setCreateError(null);
        setCreateKeyData(createEmptyKeyDataFormState());
        setCreateLabelRows([]);
        setCreateAnnotationRows([]);
    };

    const closeCreateDialog = () => {
        if (createLoading) return;
        setCreateOpen(false);
    };

    const openCreateDialog = () => {
        resetCreateDialog();
        setCreateOpen(true);
    };

    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{mb: 2}}>
                <Typography component="h2" variant="h6">
                    Keys
                </Typography>
                <Button variant="contained" size="small" startIcon={<AddIcon/>} onClick={openCreateDialog}>
                    Create Key
                </Button>
            </Stack>
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

            {/* Create key dialog */}
            <Dialog open={createOpen} onClose={closeCreateDialog} fullWidth maxWidth="md">
                <DialogTitle>Create Key</DialogTitle>
                <DialogContent>
                    {createError && (
                        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setCreateError(null)}>{createError}</Alert>
                    )}
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                        Namespace: {ns}
                    </Typography>
                    <Stack spacing={3}>
                        <KeyDataForm value={createKeyData} onChange={setCreateKeyData} disabled={createLoading}/>
                        <KeyValueRowsEditor
                            title="Labels"
                            rows={createLabelRows}
                            onChange={setCreateLabelRows}
                            addLabel="Add label"
                        />
                        <KeyValueRowsEditor
                            title="Annotations"
                            rows={createAnnotationRows}
                            onChange={setCreateAnnotationRows}
                            addLabel="Add annotation"
                        />
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={closeCreateDialog} disabled={createLoading}>Cancel</Button>
                    <Button onClick={onCreateSubmit} variant="contained" disabled={createLoading}>Create</Button>
                </DialogActions>
            </Dialog>
        </Box>
    );
}
