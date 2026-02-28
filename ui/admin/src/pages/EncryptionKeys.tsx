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
import TextField from '@mui/material/TextField';
import {
    listEncryptionKeys, EncryptionKeyState, EncryptionKey, ListResponse,
    ListEncryptionKeysParams, namespaceAndChildren, createEncryptionKey, CreateEncryptionKeyRequest
} from '@authproxy/api';
import dayjs from 'dayjs';
import {useQueryState, parseAsInteger, parseAsStringLiteral, parseAsString} from 'nuqs'
import {useNavigate} from "react-router-dom";
import {useSelector} from "react-redux";
import {selectCurrentNamespacePath} from "../store/namespacesSlice";

function renderState(state: EncryptionKeyState) {
    const colors: Record<EncryptionKeyState, "default" | "success" | "error" | "info" | "warning" | "primary" | "secondary"> = {
        [EncryptionKeyState.ACTIVE]: 'success',
        [EncryptionKeyState.DISABLED]: 'default',
    };

    return <Chip label={state} color={colors[state]} size="small" />;
}

export const columns: GridColDef<EncryptionKey>[] = [
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
        renderCell: (params) => renderState(params.value as EncryptionKeyState),
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

const keySourceTypes = [
    { label: 'Random', value: 'random' },
    { label: 'Value', value: 'value' },
    { label: 'Base64', value: 'base64' },
    { label: 'Environment Variable', value: 'env_var' },
    { label: 'AWS Secret', value: 'aws_secret' },
    { label: 'GCP Secret', value: 'gcp_secret' },
    { label: 'HashiCorp Vault', value: 'hashicorp_vault' },
];

export default function EncryptionKeys() {
    const defaultPageSize = 20;
    const stateOptions = useMemo(() => [
        { label: 'All', value: '' },
        { label: 'Active', value: EncryptionKeyState.ACTIVE },
        { label: 'Disabled', value: EncryptionKeyState.DISABLED },
    ], []);
    const navigate = useNavigate();
    const stateVals = useMemo(() => stateOptions.map(opt => opt.value), [stateOptions]);
    const ns = useSelector(selectCurrentNamespacePath);

    const [rows, setRows] = useState<EncryptionKey[]>([]);
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
    const [keySourceType, setKeySourceType] = useState('random');
    const [createFields, setCreateFields] = useState<Record<string, string>>({});

    const responsesCacheRef = useRef<ListResponse<EncryptionKey>[]>([]);
    const pageRequestCacheRef = useRef<Set<number>>(new Set());

    const handleRowClick: GridEventListener<'rowClick'> = (params, event) => {
        const id = params.id;
        const itemUrl = `/encryption-keys/${id}`;
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

                const params: ListEncryptionKeysParams = prevResp?.cursor ? {cursor: prevResp.cursor} : {
                    state: (stateFilter as EncryptionKeyState) || undefined,
                    namespace: namespaceAndChildren(ns),
                    order_by: sort || undefined,
                    limit: pageSize,
                };

                const resp = await listEncryptionKeys(params);

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
            setError(e?.message || 'Failed to load encryption keys');
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

    const buildKeyData = (): Record<string, any> | undefined => {
        switch (keySourceType) {
            case 'random': {
                const numBytes = parseInt(createFields['num_bytes'] || '32', 10);
                return { random: true, num_bytes: numBytes };
            }
            case 'value':
                return createFields['value'] ? { value: createFields['value'] } : undefined;
            case 'base64':
                return createFields['base64'] ? { base64: createFields['base64'] } : undefined;
            case 'env_var':
                return createFields['env_var'] ? { env_var: createFields['env_var'] } : undefined;
            case 'aws_secret': {
                const data: Record<string, string> = {};
                if (createFields['aws_secret_id']) data['aws_secret_id'] = createFields['aws_secret_id'];
                if (createFields['aws_region']) data['aws_region'] = createFields['aws_region'];
                if (createFields['aws_secret_key']) data['aws_secret_key'] = createFields['aws_secret_key'];
                return Object.keys(data).length > 0 ? data : undefined;
            }
            case 'gcp_secret': {
                const data: Record<string, string> = {};
                if (createFields['gcp_secret_name']) data['gcp_secret_name'] = createFields['gcp_secret_name'];
                if (createFields['gcp_project']) data['gcp_project'] = createFields['gcp_project'];
                if (createFields['gcp_secret_version']) data['gcp_secret_version'] = createFields['gcp_secret_version'];
                return Object.keys(data).length > 0 ? data : undefined;
            }
            case 'hashicorp_vault': {
                const data: Record<string, string> = {};
                if (createFields['vault_address']) data['vault_address'] = createFields['vault_address'];
                if (createFields['vault_token']) data['vault_token'] = createFields['vault_token'];
                if (createFields['vault_path']) data['vault_path'] = createFields['vault_path'];
                if (createFields['vault_key']) data['vault_key'] = createFields['vault_key'];
                return Object.keys(data).length > 0 ? data : undefined;
            }
            default:
                return undefined;
        }
    };

    const onCreateSubmit = async () => {
        setCreateLoading(true);
        setCreateError(null);
        try {
            const request: CreateEncryptionKeyRequest = {
                namespace: ns,
                key_data: buildKeyData(),
            };
            await createEncryptionKey(request);
            setCreateOpen(false);
            setKeySourceType('random');
            setCreateFields({});
            resetPagination();
            fetchPage(1);
        } catch (err: any) {
            const msg = err?.response?.data?.error || err.message || 'Failed to create encryption key';
            setCreateError(msg);
        } finally {
            setCreateLoading(false);
        }
    };

    const updateField = (field: string, value: string) => {
        setCreateFields(prev => ({ ...prev, [field]: value }));
    };

    const renderKeySourceFields = () => {
        switch (keySourceType) {
            case 'random':
                return (
                    <TextField
                        label="Number of Bytes"
                        type="number"
                        fullWidth
                        value={createFields['num_bytes'] || '32'}
                        onChange={(e) => updateField('num_bytes', e.target.value)}
                        helperText="Number of random bytes to generate (default: 32)"
                        sx={{ mt: 2 }}
                    />
                );
            case 'value':
                return (
                    <TextField
                        label="Key Value"
                        fullWidth
                        value={createFields['value'] || ''}
                        onChange={(e) => updateField('value', e.target.value)}
                        sx={{ mt: 2 }}
                    />
                );
            case 'base64':
                return (
                    <TextField
                        label="Base64 Encoded Key"
                        fullWidth
                        value={createFields['base64'] || ''}
                        onChange={(e) => updateField('base64', e.target.value)}
                        sx={{ mt: 2 }}
                    />
                );
            case 'env_var':
                return (
                    <TextField
                        label="Environment Variable Name"
                        fullWidth
                        value={createFields['env_var'] || ''}
                        onChange={(e) => updateField('env_var', e.target.value)}
                        sx={{ mt: 2 }}
                    />
                );
            case 'aws_secret':
                return (
                    <Stack spacing={2} sx={{ mt: 2 }}>
                        <TextField
                            label="AWS Secret ID (ARN)"
                            fullWidth
                            required
                            value={createFields['aws_secret_id'] || ''}
                            onChange={(e) => updateField('aws_secret_id', e.target.value)}
                        />
                        <TextField
                            label="AWS Region"
                            fullWidth
                            value={createFields['aws_region'] || ''}
                            onChange={(e) => updateField('aws_region', e.target.value)}
                        />
                        <TextField
                            label="AWS Secret Key (optional)"
                            fullWidth
                            value={createFields['aws_secret_key'] || ''}
                            onChange={(e) => updateField('aws_secret_key', e.target.value)}
                        />
                    </Stack>
                );
            case 'gcp_secret':
                return (
                    <Stack spacing={2} sx={{ mt: 2 }}>
                        <TextField
                            label="GCP Secret Name"
                            fullWidth
                            required
                            value={createFields['gcp_secret_name'] || ''}
                            onChange={(e) => updateField('gcp_secret_name', e.target.value)}
                        />
                        <TextField
                            label="GCP Project (optional)"
                            fullWidth
                            value={createFields['gcp_project'] || ''}
                            onChange={(e) => updateField('gcp_project', e.target.value)}
                        />
                        <TextField
                            label="GCP Secret Version (optional)"
                            fullWidth
                            value={createFields['gcp_secret_version'] || ''}
                            onChange={(e) => updateField('gcp_secret_version', e.target.value)}
                        />
                    </Stack>
                );
            case 'hashicorp_vault':
                return (
                    <Stack spacing={2} sx={{ mt: 2 }}>
                        <TextField
                            label="Vault Address"
                            fullWidth
                            required
                            value={createFields['vault_address'] || ''}
                            onChange={(e) => updateField('vault_address', e.target.value)}
                        />
                        <TextField
                            label="Vault Token"
                            fullWidth
                            required
                            value={createFields['vault_token'] || ''}
                            onChange={(e) => updateField('vault_token', e.target.value)}
                        />
                        <TextField
                            label="Vault Path"
                            fullWidth
                            required
                            value={createFields['vault_path'] || ''}
                            onChange={(e) => updateField('vault_path', e.target.value)}
                        />
                        <TextField
                            label="Vault Key"
                            fullWidth
                            required
                            value={createFields['vault_key'] || ''}
                            onChange={(e) => updateField('vault_key', e.target.value)}
                        />
                    </Stack>
                );
            default:
                return null;
        }
    };

    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{mb: 2}}>
                <Typography component="h2" variant="h6">
                    Encryption Keys
                </Typography>
                <Button variant="contained" size="small" onClick={() => setCreateOpen(true)}>
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

            {/* Create encryption key dialog */}
            <Dialog open={createOpen} onClose={() => !createLoading && setCreateOpen(false)} fullWidth maxWidth="sm">
                <DialogTitle>Create Encryption Key</DialogTitle>
                <DialogContent>
                    {createError && (
                        <Typography color="error" sx={{ mb: 2 }}>{createError}</Typography>
                    )}
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                        Namespace: {ns}
                    </Typography>
                    <FormControl fullWidth sx={{ mt: 1 }}>
                        <InputLabel id="key-source-type-label">Key Source</InputLabel>
                        <Select
                            labelId="key-source-type-label"
                            value={keySourceType}
                            label="Key Source"
                            onChange={(e) => {
                                setKeySourceType(e.target.value);
                                setCreateFields({});
                            }}
                        >
                            {keySourceTypes.map(opt => (
                                <MenuItem key={opt.value} value={opt.value}>{opt.label}</MenuItem>
                            ))}
                        </Select>
                    </FormControl>
                    {renderKeySourceFields()}
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setCreateOpen(false)} disabled={createLoading}>Cancel</Button>
                    <Button onClick={onCreateSubmit} variant="contained" disabled={createLoading}>Create</Button>
                </DialogActions>
            </Dialog>
        </Box>
    );
}
