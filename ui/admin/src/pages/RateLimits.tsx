import * as React from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import Grid from '@mui/material/Grid';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import {DataGrid, GridColDef, GridEventListener, GridSortModel} from '@mui/x-data-grid';
import Chip from '@mui/material/Chip';
import Stack from '@mui/material/Stack';
import Switch from '@mui/material/Switch';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Tooltip from '@mui/material/Tooltip';
import Alert from '@mui/material/Alert';
import {
    listRateLimits, RateLimit, RateLimitMode, RateLimitDefinition,
    ListResponse, ListRateLimitsParams, namespaceAndChildren,
    createRateLimit, updateRateLimit, CreateRateLimitRequest,
} from '@authproxy/api';
import RateLimitDefinitionEditor from '../components/RateLimitDefinitionEditor';
import { EMPTY_DEFINITION } from '../components/RateLimitDefinitionForm';
import dayjs from 'dayjs';
import {useQueryState, parseAsInteger, parseAsStringLiteral, parseAsString} from 'nuqs'
import {useNavigate} from "react-router-dom";
import {useSelector} from "react-redux";
import {selectCurrentNamespacePath} from "../store/namespacesSlice";

// Summarise the algorithm variant in one short string. Keeps the column
// scannable instead of showing the full JSON.
function algorithmSummary(def: RateLimitDefinition): string {
    if (def.algorithm.fixed_window) {
        const fw = def.algorithm.fixed_window;
        return `fixed ${fw.limit}/${fw.window}`;
    }
    if (def.algorithm.sliding_window) {
        const sw = def.algorithm.sliding_window;
        return `sliding(${sw.mode}) ${sw.limit}/${sw.window}`;
    }
    if (def.algorithm.token_bucket) {
        const tb = def.algorithm.token_bucket;
        return `token bucket ${tb.capacity} @ ${tb.refill_rate}/s`;
    }
    return '—';
}

function selectorSummary(def: RateLimitDefinition): string {
    const parts: string[] = [];
    if (def.selector.methods && def.selector.methods.length > 0) {
        parts.push(def.selector.methods.join('|'));
    }
    if (def.selector.path_match) {
        parts.push(`${def.selector.path_match.kind}:${def.selector.path_match.value}`);
    }
    if (def.selector.label_selector) {
        parts.push(def.selector.label_selector);
    }
    if (def.selector.request_types && def.selector.request_types.length > 0) {
        parts.push(`types=${def.selector.request_types.join(',')}`);
    }
    return parts.length === 0 ? 'any' : parts.join(' · ');
}

function bucketSummary(def: RateLimitDefinition): string {
    if (!def.bucket.dimensions || def.bucket.dimensions.length === 0) {
        return 'global';
    }
    return def.bucket.dimensions.join(', ');
}

export default function RateLimits() {
    const defaultPageSize = 20;
    const modeOptions = useMemo(() => [
        { label: 'All', value: '' },
        { label: 'Enforce', value: RateLimitMode.ENFORCE },
        { label: 'Observe', value: RateLimitMode.OBSERVE },
    ], []);
    const navigate = useNavigate();
    const modeVals = useMemo(() => modeOptions.map(opt => opt.value), [modeOptions]);
    const ns = useSelector(selectCurrentNamespacePath);

    const [rows, setRows] = useState<RateLimit[]>([]);
    const [rowCount, setRowCount] = useState<number>(-1);
    const [loading, setLoading] = useState<boolean>(false);
    const [error, setError] = useState<string | null>(null);

    const [page, setPage] = useQueryState<number>('page', parseAsInteger.withDefault(1));
    const [pageSize, setPageSize] = useQueryState<number>('page_size', parseAsInteger.withDefault(defaultPageSize));
    const [modeFilter, setModeFilter] = useQueryState<string>('mode', parseAsStringLiteral(modeVals).withDefault(''));
    const [sort, setSort] = useQueryState<string>('sort', parseAsString.withDefault(''));

    const [hasNextPage, setHasNextPage] = useState<boolean>(false);

    // Per-row in-flight flag for the mode toggle so the Switch shows a
    // pending state and concurrent clicks are coalesced.
    const [pendingToggle, setPendingToggle] = useState<Record<string, boolean>>({});

    // Create dialog state
    const [createOpen, setCreateOpen] = useState(false);
    const [createLoading, setCreateLoading] = useState(false);
    const [createError, setCreateError] = useState<string | null>(null);
    const [createDef, setCreateDef] = useState<RateLimitDefinition>(EMPTY_DEFINITION);

    const responsesCacheRef = useRef<ListResponse<RateLimit>[]>([]);
    const pageRequestCacheRef = useRef<Set<number>>(new Set());

    const handleRowClick: GridEventListener<'rowClick'> = (params, event) => {
        const id = params.id;
        const itemUrl = `/rate-limits/${id}`;
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

                const params: ListRateLimitsParams = prevResp?.cursor ? {cursor: prevResp.cursor} : {
                    namespace: namespaceAndChildren(ns),
                    order_by: sort || undefined,
                    limit: pageSize,
                };

                const resp = await listRateLimits(params);

                if(resp.status !== 200) {
                    setError("Failed to fetch page of results from server");
                    return;
                }

                responsesCacheRef.current[thisPage] = resp.data;
            }

            const data = responsesCacheRef.current[targetPageZeroBased];
            let items = data?.items || [];

            // Mode filter is client-side because the list endpoint doesn't
            // accept it yet — a small set, cheap to filter in the browser.
            if (modeFilter) {
                items = items.filter(rl => (rl.definition.mode || RateLimitMode.ENFORCE) === modeFilter);
            }

            setRows(items);

            const hnp = !!data?.cursor;
            setHasNextPage(hnp);

            if(!hnp) {
                setRowCount(responsesCacheRef.current.map((v) => v.items.length).reduceRight((acc, val)=> acc+val, 0));
            }
        } catch (e: any) {
            setError(e?.message || 'Failed to load rate limits');
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        resetPagination();
        fetchPage(1);
    }, [ns, pageSize, sort, modeFilter]);

    useEffect(() => {
        fetchPage(page);
    }, [page]);

    // Flip a rate limit between enforce and observe. Optimistic update
    // with a revert on failure so the Switch feels responsive.
    const toggleMode = async (rl: RateLimit) => {
        const id = rl.id;
        if (pendingToggle[id]) return;
        setPendingToggle(prev => ({ ...prev, [id]: true }));

        const current = rl.definition.mode || RateLimitMode.ENFORCE;
        const next = current === RateLimitMode.ENFORCE ? RateLimitMode.OBSERVE : RateLimitMode.ENFORCE;
        const nextDef: RateLimitDefinition = { ...rl.definition, mode: next };

        // Optimistic update — apply locally so the Switch flips immediately.
        setRows(prev => prev.map(r => r.id === id ? { ...r, definition: nextDef } : r));

        try {
            const resp = await updateRateLimit(id, { definition: nextDef });
            // Refresh the row from the server's response so any computed
            // fields (updated_at, etc.) reflect reality.
            setRows(prev => prev.map(r => r.id === id ? resp.data : r));
            // Invalidate the cached page so a subsequent navigation away
            // and back doesn't show stale data.
            responsesCacheRef.current = [];
        } catch (e: any) {
            // Revert.
            setRows(prev => prev.map(r => r.id === id ? rl : r));
            setError(e?.response?.data?.error || e?.message || 'Failed to update rate-limit mode');
        } finally {
            setPendingToggle(prev => {
                const next = { ...prev };
                delete next[id];
                return next;
            });
        }
    };

    const onCreateSubmit = async () => {
        setCreateLoading(true);
        setCreateError(null);
        try {
            const request: CreateRateLimitRequest = {
                namespace: ns,
                definition: createDef,
            };
            await createRateLimit(request);
            setCreateOpen(false);
            setCreateDef(EMPTY_DEFINITION);
            resetPagination();
            fetchPage(1);
        } catch (err: any) {
            const msg = err?.response?.data?.error || err.message || 'Failed to create rate limit';
            setCreateError(msg);
        } finally {
            setCreateLoading(false);
        }
    };

    const columns: GridColDef<RateLimit>[] = useMemo(() => [
        {
            field: 'id',
            headerName: 'ID',
            flex: 0.7,
            minWidth: 200,
            sortable: false,
            renderCell: (params) => (
                <Typography variant="body2" component="code" sx={{
                    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                    fontSize: '0.85rem',
                }}>
                    {params.value}
                </Typography>
            ),
        },
        {
            field: 'mode',
            headerName: 'Mode',
            flex: 0.5,
            minWidth: 160,
            sortable: false,
            renderCell: (params) => {
                const rl = params.row;
                const mode = (rl.definition.mode || RateLimitMode.ENFORCE) as RateLimitMode;
                const isEnforce = mode === RateLimitMode.ENFORCE;
                const pending = !!pendingToggle[rl.id];
                return (
                    <Stack direction="row" spacing={1} alignItems="center" sx={{ height: '100%' }}>
                        <Chip
                            label={mode}
                            size="small"
                            color={isEnforce ? 'warning' : 'info'}
                            variant="outlined"
                        />
                        <Tooltip title={isEnforce ? 'Switch to observe (won’t reject)' : 'Switch to enforce (will return 429)'}>
                            <span onClick={(e) => e.stopPropagation()}>
                                <Switch
                                    size="small"
                                    checked={isEnforce}
                                    disabled={pending}
                                    onChange={() => toggleMode(rl)}
                                />
                            </span>
                        </Tooltip>
                    </Stack>
                );
            },
        },
        {
            field: 'algorithm',
            headerName: 'Algorithm',
            flex: 0.7,
            minWidth: 200,
            sortable: false,
            valueGetter: (_value, row) => algorithmSummary((row as RateLimit).definition),
        },
        {
            field: 'selector',
            headerName: 'Selector',
            flex: 1,
            minWidth: 200,
            sortable: false,
            valueGetter: (_value, row) => selectorSummary((row as RateLimit).definition),
        },
        {
            field: 'bucket',
            headerName: 'Bucket',
            flex: 0.5,
            minWidth: 120,
            sortable: false,
            valueGetter: (_value, row) => bucketSummary((row as RateLimit).definition),
        },
        {
            field: 'namespace',
            headerName: 'Namespace',
            flex: 0.5,
            minWidth: 110,
            sortable: false,
        },
        {
            field: 'created_at',
            headerName: 'Created',
            flex: 0.6,
            minWidth: 160,
            sortable: true,
            valueGetter: (value) => dayjs(value).format('MMM DD, YYYY, h:mm A'),
        },
    ], [pendingToggle]);

    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 2 }}>
                <Typography component="h2" variant="h6">Rate Limits</Typography>
                <Button variant="contained" size="small" onClick={() => setCreateOpen(true)}>
                    Create Rate Limit
                </Button>
            </Stack>
            <Stack direction="row" spacing={2} alignItems="center" sx={{ mb: 2 }}>
                <FormControl size="small" sx={{ minWidth: 180 }}>
                    <InputLabel id="mode-filter-label">Mode</InputLabel>
                    <Select
                        labelId="mode-filter-label"
                        value={modeFilter}
                        label="Mode"
                        onChange={(e) => setModeFilter(e.target.value)}
                    >
                        {modeOptions.map(opt => (
                            <MenuItem key={opt.label} value={opt.value}>{opt.label}</MenuItem>
                        ))}
                    </Select>
                </FormControl>
            </Stack>

            {error && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>{error}</Alert>}

            <Grid size={{xs: 12, lg: 12}}>
                <style>{`.clickable-row { cursor: pointer; }`}</style>
                <DataGrid
                    rows={rows}
                    columns={columns}
                    getRowId={(row) => row.id}
                    getRowClassName={() => 'clickable-row'}
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
                    pageSizeOptions={[5, 10, 20, 50, 100]}
                    rowCount={rowCount}
                    onRowClick={handleRowClick}
                    hideFooterSelectedRowCount
                    density="compact"
                />
            </Grid>

            <Dialog open={createOpen} onClose={() => !createLoading && setCreateOpen(false)} fullWidth maxWidth="md">
                <DialogTitle>Create Rate Limit</DialogTitle>
                <DialogContent>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                        Will be created in namespace <Chip size="small" label={ns || 'root'} />.
                    </Typography>
                    <RateLimitDefinitionEditor value={createDef} onChange={setCreateDef} />
                    {createError && <Alert severity="error" sx={{ mt: 2 }}>{createError}</Alert>}
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setCreateOpen(false)} disabled={createLoading}>Cancel</Button>
                    <Button onClick={onCreateSubmit} variant="contained" disabled={createLoading}>
                        Create
                    </Button>
                </DialogActions>
            </Dialog>
        </Box>
    );
}
