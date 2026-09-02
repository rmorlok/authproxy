import React, { useEffect, useState } from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import Stack from '@mui/material/Stack';
import Chip from '@mui/material/Chip';
import IconButton from '@mui/material/IconButton';
import Menu from '@mui/material/Menu';
import MenuItem from '@mui/material/MenuItem';
import Divider from '@mui/material/Divider';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Button from '@mui/material/Button';
import Switch from '@mui/material/Switch';
import FormControlLabel from '@mui/material/FormControlLabel';
import Tooltip from '@mui/material/Tooltip';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import dayjs from 'dayjs';
import {
    RateLimit, rateLimits, RateLimitMode, RateLimitSpec,
    RATE_LIMIT_API_VERSION, RATE_LIMIT_KIND,
} from '@authproxy/api';
import { useNavigate } from 'react-router-dom';
import RateLimitSpecEditor from './RateLimitSpecEditor';
import { EMPTY_SPEC } from './RateLimitSpecForm';
import ResourceIdentifier from './ResourceIdentifier';
import ResourceMetadataMenuItems from './ResourceMetadataMenuItems';
import AnnotationsEditor from './AnnotationsEditor';
import ResourceNameEditor from './ResourceNameEditor';

function ModeChip({ mode }: { mode: RateLimitMode }) {
    const color = mode === RateLimitMode.ENFORCE ? 'warning' : 'info';
    return <Chip label={mode} color={color} size="small" />;
}

// One-line summary of an algorithm variant, used in the read-only
// algorithm card. Keeps the detail page scannable without rendering the
// full nested JSON.
function algorithmDisplay(def: RateLimitSpec): { label: string; rows: Array<[string, string]> } {
    if (def.algorithm.fixedWindow) {
        const a = def.algorithm.fixedWindow;
        return {
            label: 'Fixed window',
            rows: [['Window', a.window], ['Limit', `${a.limit}`]],
        };
    }
    if (def.algorithm.slidingWindow) {
        const a = def.algorithm.slidingWindow;
        return {
            label: `Sliding window (${a.mode})`,
            rows: [['Window', a.window], ['Limit', `${a.limit}`], ['Mode', a.mode]],
        };
    }
    if (def.algorithm.tokenBucket) {
        const a = def.algorithm.tokenBucket;
        return {
            label: 'Token bucket',
            rows: [['Capacity', `${a.capacity}`], ['Refill rate', `${a.refillRate} tok/s`]],
        };
    }
    return { label: '—', rows: [] };
}

function scopeDisplay(spec: RateLimitSpec): string {
    if (spec.scope?.connectorRef) {
        const ref = spec.scope.connectorRef;
        const target = ref.id || `${ref.namespace}/${ref.name}`;
        return ref.generation ? `Connector ${target}, generation ${ref.generation}` : `Connector ${target}, all generations`;
    }
    if (spec.scope?.connectionRef) {
        const ref = spec.scope.connectionRef;
        return `Connection ${ref.id || `${ref.namespace}/${ref.name}`}`;
    }
    return 'Namespace and descendants';
}

export default function RateLimitDetail({ rateLimitId }: { rateLimitId: string }) {
    const navigate = useNavigate();
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [rl, setRl] = useState<RateLimit | null>(null);

    // Action menu / dialog state.
    const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
    const [editSpecOpen, setEditSpecOpen] = useState(false);
    const [editSpec, setEditSpec] = useState<RateLimitSpec>(EMPTY_SPEC);
    const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false);
    const [actionLoading, setActionLoading] = useState(false);
    const [actionError, setActionError] = useState<string | null>(null);

    // Pending state for the inline mode toggle.
    const [modePending, setModePending] = useState(false);

    const fetchRl = () => {
        setLoading(true);
        setError(null);
        rateLimits.get(rateLimitId)
            .then(res => setRl(res.data))
            .catch(err => {
                const msg = err?.response?.data?.error || err.message || 'Failed to load rate limit';
                setError(msg);
            })
            .finally(() => setLoading(false));
    };

    useEffect(() => {
        let cancelled = false;
        setLoading(true);
        setError(null);
        rateLimits.get(rateLimitId)
            .then(res => { if (!cancelled) setRl(res.data); })
            .catch(err => {
                if (cancelled) return;
                const msg = err?.response?.data?.error || err.message || 'Failed to load rate limit';
                setError(msg);
            })
            .finally(() => { if (!cancelled) setLoading(false); });
        return () => { cancelled = true; };
    }, [rateLimitId]);

    if (loading) return (<Box sx={{display: 'flex', justifyContent: 'center', p: 4}}><CircularProgress/></Box>);
    if (error) return (<Alert severity="error">{error}</Alert>);
    if (!rl) return null;

    const openMenu = (e: React.MouseEvent<HTMLButtonElement>) => setMenuAnchorEl(e.currentTarget);
    const closeMenu = () => setMenuAnchorEl(null);

    const mode = (rl.spec.mode || RateLimitMode.ENFORCE) as RateLimitMode;
    const isEnforce = mode === RateLimitMode.ENFORCE;

    // Inline mode toggle for the page header. Same semantics as the list
    // page's row toggle — optimistic, revert on failure.
    const onToggleMode = async () => {
        if (modePending) return;
        setModePending(true);
        const nextMode = isEnforce ? RateLimitMode.OBSERVE : RateLimitMode.ENFORCE;
        const nextSpec: RateLimitSpec = { ...rl.spec, mode: nextMode };
        const prev = rl;
        setRl({ ...rl, spec: nextSpec });
        try {
            const resp = await rateLimits.update(rl.metadata.id, {
                apiVersion: RATE_LIMIT_API_VERSION,
                kind: RATE_LIMIT_KIND,
                metadata: {},
                spec: nextSpec,
            });
            setRl(resp.data);
        } catch (err: any) {
            setRl(prev);
            setActionError(err?.response?.data?.error || err.message || 'Failed to toggle mode');
        } finally {
            setModePending(false);
        }
    };

    const onClickEditSpec = () => {
        setActionError(null);
        setEditSpec(rl.spec);
        closeMenu();
        setEditSpecOpen(true);
    };

    const onSubmitEditSpec = async () => {
        setActionError(null);
        setActionLoading(true);
        try {
            await rateLimits.update(rl.metadata.id, {
                apiVersion: RATE_LIMIT_API_VERSION,
                kind: RATE_LIMIT_KIND,
                metadata: {},
                // PATCH distinguishes an omitted scope (leave unchanged)
                // from null (restore namespace-and-descendants scope).
                spec: {...editSpec, scope: editSpec.scope ?? null},
            });
            setEditSpecOpen(false);
            fetchRl();
        } catch (err: any) {
            const msg = err?.response?.data?.error || err.message || 'Failed to update spec';
            setActionError(msg);
        } finally {
            setActionLoading(false);
        }
    };

    const onClickDelete = () => {
        setActionError(null);
        closeMenu();
        setConfirmDeleteOpen(true);
    };

    const onConfirmDelete = async () => {
        setActionError(null);
        setActionLoading(true);
        try {
            await rateLimits.delete(rl.metadata.id);
            setConfirmDeleteOpen(false);
            navigate('/rate-limits');
        } catch (err: any) {
            const msg = err?.response?.data?.error || err.message || 'Failed to delete rate limit';
            setActionError(msg);
        } finally {
            setActionLoading(false);
        }
    };

    const algoDisplay = algorithmDisplay(rl.spec);

    return (
        <Stack spacing={2} sx={{ p: 2 }}>
            <Stack direction="row" justifyContent="space-between" alignItems="center">
                <Typography variant="h5">Rate Limit</Typography>
                <Stack direction="row" spacing={1} alignItems="center">
                    <ModeChip mode={mode} />
                    <Tooltip title={isEnforce ? 'Switch to observe (won’t reject)' : 'Switch to enforce (will return 429)'}>
                        <span>
                            <FormControlLabel
                                control={
                                    <Switch
                                        size="small"
                                        checked={isEnforce}
                                        disabled={modePending}
                                        onChange={onToggleMode}
                                    />
                                }
                                label={isEnforce ? 'Enforce' : 'Observe'}
                                labelPlacement="start"
                                sx={{ mr: 0 }}
                            />
                        </span>
                    </Tooltip>
                    <IconButton aria-label="actions" onClick={openMenu} size="small">
                        <MoreVertIcon />
                    </IconButton>
                    <Menu anchorEl={menuAnchorEl} open={Boolean(menuAnchorEl)} onClose={closeMenu} keepMounted>
                        <ResourceMetadataMenuItems
                            resource="rate limit"
                            name={rl.metadata.name}
                            labels={rl.metadata.labels}
                            annotations={rl.metadata.annotations}
                            onCloseMenu={closeMenu}
                            includeRename={false}
                            onUpdateLabels={async (labels) => {
                                const response = await rateLimits.update(rl.metadata.id, {
                                    apiVersion: RATE_LIMIT_API_VERSION,
                                    kind: RATE_LIMIT_KIND,
                                    metadata: {labels},
                                    spec: {},
                                });
                                setRl(response.data);
                            }}
                            onUpdateAnnotations={async (annotations) => {
                                const response = await rateLimits.update(rl.metadata.id, {
                                    apiVersion: RATE_LIMIT_API_VERSION,
                                    kind: RATE_LIMIT_KIND,
                                    metadata: {annotations},
                                    spec: {},
                                });
                                setRl(response.data);
                            }}
                            disabled={actionLoading || modePending}
                        />
                        <Divider />
                        <MenuItem onClick={onClickEditSpec}>Edit spec...</MenuItem>
                        <Divider />
                        <MenuItem onClick={onClickDelete} sx={{ color: 'error.main' }}>Delete</MenuItem>
                    </Menu>
                </Stack>
            </Stack>

            {actionError && <Alert severity="error" onClose={() => setActionError(null)}>{actionError}</Alert>}

            <ResourceNameEditor
                name={rl.metadata.name}
                resourceType="Rate Limit"
                onRename={async (name) => {
                    const response = await rateLimits.update(rl.metadata.id, {
                        apiVersion: RATE_LIMIT_API_VERSION,
                        kind: RATE_LIMIT_KIND,
                        metadata: {name},
                        spec: {},
                    });
                    setRl(response.data);
                }}
            />

            <ResourceIdentifier value={rl.metadata.id} copyLabel="Copy rate limit id" />

            <Box>
                <Typography variant="subtitle2" color="text.secondary">Namespace</Typography>
                <Typography variant="body1">{rl.metadata.namespace}</Typography>
            </Box>

            <Box>
                <Typography variant="subtitle2" color="text.secondary">Scope</Typography>
                <Typography variant="body1">{scopeDisplay(rl.spec)}</Typography>
            </Box>

            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={4}>
                <Box>
                    <Typography variant="subtitle2" color="text.secondary">Created</Typography>
                    <Typography variant="body1">{dayjs(rl.metadata.createdAt).format('MMM DD, YYYY, h:mm A')}</Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" color="text.secondary">Updated</Typography>
                    <Typography variant="body1">{dayjs(rl.metadata.updatedAt).format('MMM DD, YYYY, h:mm A')}</Typography>
                </Box>
            </Stack>

            <Box>
                <Typography variant="subtitle2" color="text.secondary">Algorithm</Typography>
                <Stack direction="row" spacing={2} alignItems="center" sx={{ mt: 0.5 }}>
                    <Chip label={algoDisplay.label} variant="outlined" />
                    {algoDisplay.rows.map(([k, v]) => (
                        <Typography key={k} variant="body2" color="text.secondary">
                            <strong>{k}:</strong> {v}
                        </Typography>
                    ))}
                </Stack>
            </Box>

            <Box>
                <Typography variant="subtitle2" color="text.secondary">Selector</Typography>
                <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{ mt: 0.5, rowGap: 0.5 }}>
                    {rl.spec.selector.labelSelector && (
                        <Chip label={`labels: ${rl.spec.selector.labelSelector}`} size="small" variant="outlined" />
                    )}
                    {rl.spec.selector.methods && rl.spec.selector.methods.length > 0 && (
                        <Chip label={`methods: ${rl.spec.selector.methods.join(', ')}`} size="small" variant="outlined" />
                    )}
                    {rl.spec.selector.pathMatch && (
                        <Chip
                            label={`path ${rl.spec.selector.pathMatch.kind}: ${rl.spec.selector.pathMatch.value}`}
                            size="small" variant="outlined"
                        />
                    )}
                    {rl.spec.selector.requestTypes && rl.spec.selector.requestTypes.length > 0 && (
                        <Chip
                            label={`types: ${rl.spec.selector.requestTypes.join(', ')}`}
                            size="small" variant="outlined"
                        />
                    )}
                    {!rl.spec.selector.labelSelector
                        && (!rl.spec.selector.methods || rl.spec.selector.methods.length === 0)
                        && !rl.spec.selector.pathMatch
                        && (!rl.spec.selector.requestTypes || rl.spec.selector.requestTypes.length === 0) && (
                        <Typography variant="body2" color="text.secondary">No selector clauses (matches default proxy + probe traffic)</Typography>
                    )}
                </Stack>
            </Box>

            <Box>
                <Typography variant="subtitle2" color="text.secondary">Bucket</Typography>
                {rl.spec.bucket.dimensions && rl.spec.bucket.dimensions.length > 0 ? (
                    <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{ mt: 0.5 }}>
                        {rl.spec.bucket.dimensions.map((d) => (
                            <Chip key={d} label={d} size="small" variant="outlined" />
                        ))}
                    </Stack>
                ) : (
                    <Typography variant="body2" color="text.secondary">Single global bucket per rule</Typography>
                )}
            </Box>

            <Box>
                <Typography variant="subtitle2" color="text.secondary">Labels</Typography>
                {rl.metadata.labels && Object.keys(rl.metadata.labels).length > 0 ? (
                    <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{ mt: 0.5, rowGap: 0.5 }}>
                        {Object.entries(rl.metadata.labels).map(([key, value]) => (
                            <Chip key={key} label={`${key}: ${value}`} size="small" variant="outlined" />
                        ))}
                    </Stack>
                ) : (
                    <Typography variant="body2" color="text.secondary">No labels</Typography>
                )}
            </Box>

            <AnnotationsEditor annotations={rl.metadata.annotations} readOnly onPut={async () => {}} onDelete={async () => {}} />

            {/* Edit-spec dialog */}
            <Dialog open={editSpecOpen} onClose={() => !actionLoading && setEditSpecOpen(false)} fullWidth maxWidth="md">
                <DialogTitle>Edit spec</DialogTitle>
                <DialogContent>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                        The mode toggle in the header is the quick path for the most-common change. Use this dialog for selector / bucket / algorithm edits.
                    </Typography>
                    <RateLimitSpecEditor value={editSpec} onChange={setEditSpec} />
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setEditSpecOpen(false)} disabled={actionLoading}>Cancel</Button>
                    <Button onClick={onSubmitEditSpec} variant="contained" disabled={actionLoading}>Save</Button>
                </DialogActions>
            </Dialog>

            {/* Delete confirmation dialog */}
            <Dialog open={confirmDeleteOpen} onClose={() => !actionLoading && setConfirmDeleteOpen(false)}>
                <DialogTitle>Delete rate limit</DialogTitle>
                <DialogContent>
                    <Typography variant="body2">
                        Are you sure you want to delete this rate limit? This action cannot be undone.
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setConfirmDeleteOpen(false)} disabled={actionLoading}>Cancel</Button>
                    <Button onClick={onConfirmDelete} color="error" variant="contained" disabled={actionLoading}>Delete</Button>
                </DialogActions>
            </Dialog>
        </Stack>
    );
}
