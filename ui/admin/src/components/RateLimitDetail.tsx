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
import MoreVertIcon from '@mui/icons-material/MoreVert';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import Tooltip from '@mui/material/Tooltip';
import dayjs from 'dayjs';
import {
    RateLimit, rateLimits, RateLimitMode, RateLimitDefinition,
} from '@authproxy/api';
import { useNavigate } from 'react-router-dom';
import AnnotationsEditor from './AnnotationsEditor';
import RateLimitDefinitionEditor from './RateLimitDefinitionEditor';
import { EMPTY_DEFINITION } from './RateLimitDefinitionForm';

function ModeChip({ mode }: { mode: RateLimitMode }) {
    const color = mode === RateLimitMode.ENFORCE ? 'warning' : 'info';
    return <Chip label={mode} color={color} size="small" />;
}

// One-line summary of an algorithm variant, used in the read-only
// algorithm card. Keeps the detail page scannable without rendering the
// full nested JSON.
function algorithmDisplay(def: RateLimitDefinition): { label: string; rows: Array<[string, string]> } {
    if (def.algorithm.fixed_window) {
        const a = def.algorithm.fixed_window;
        return {
            label: 'Fixed window',
            rows: [['Window', a.window], ['Limit', `${a.limit}`]],
        };
    }
    if (def.algorithm.sliding_window) {
        const a = def.algorithm.sliding_window;
        return {
            label: `Sliding window (${a.mode})`,
            rows: [['Window', a.window], ['Limit', `${a.limit}`], ['Mode', a.mode]],
        };
    }
    if (def.algorithm.token_bucket) {
        const a = def.algorithm.token_bucket;
        return {
            label: 'Token bucket',
            rows: [['Capacity', `${a.capacity}`], ['Refill rate', `${a.refill_rate} tok/s`]],
        };
    }
    return { label: '—', rows: [] };
}

export default function RateLimitDetail({ rateLimitId }: { rateLimitId: string }) {
    const navigate = useNavigate();
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [rl, setRl] = useState<RateLimit | null>(null);

    // Action menu / dialog state.
    const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
    const [editDefOpen, setEditDefOpen] = useState(false);
    const [editDef, setEditDef] = useState<RateLimitDefinition>(EMPTY_DEFINITION);
    const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false);
    const [actionLoading, setActionLoading] = useState(false);
    const [actionError, setActionError] = useState<string | null>(null);

    // Pending state for the inline mode toggle.
    const [modePending, setModePending] = useState(false);

    const [copied, setCopied] = useState(false);
    const handleCopyId = async () => {
        try {
            await navigator.clipboard.writeText(rl?.id || '');
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
        } catch (_e) { /* ignore */ }
    };

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

    const mode = (rl.definition.mode || RateLimitMode.ENFORCE) as RateLimitMode;
    const isEnforce = mode === RateLimitMode.ENFORCE;

    // Inline mode toggle for the page header. Same semantics as the list
    // page's row toggle — optimistic, revert on failure.
    const onToggleMode = async () => {
        if (modePending) return;
        setModePending(true);
        const nextMode = isEnforce ? RateLimitMode.OBSERVE : RateLimitMode.ENFORCE;
        const nextDef: RateLimitDefinition = { ...rl.definition, mode: nextMode };
        const prev = rl;
        setRl({ ...rl, definition: nextDef });
        try {
            const resp = await rateLimits.update(rl.id, { definition: nextDef });
            setRl(resp.data);
        } catch (err: any) {
            setRl(prev);
            setActionError(err?.response?.data?.error || err.message || 'Failed to toggle mode');
        } finally {
            setModePending(false);
        }
    };

    const onClickEditDefinition = () => {
        setActionError(null);
        setEditDef(rl.definition);
        closeMenu();
        setEditDefOpen(true);
    };

    const onSubmitEditDefinition = async () => {
        setActionError(null);
        setActionLoading(true);
        try {
            await rateLimits.update(rl.id, { definition: editDef });
            setEditDefOpen(false);
            fetchRl();
        } catch (err: any) {
            const msg = err?.response?.data?.error || err.message || 'Failed to update definition';
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
            await rateLimits.delete(rl.id);
            setConfirmDeleteOpen(false);
            navigate('/rate-limits');
        } catch (err: any) {
            const msg = err?.response?.data?.error || err.message || 'Failed to delete rate limit';
            setActionError(msg);
        } finally {
            setActionLoading(false);
        }
    };

    const algoDisplay = algorithmDisplay(rl.definition);

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
                    <Menu anchorEl={menuAnchorEl} open={Boolean(menuAnchorEl)} onClose={closeMenu}>
                        <MenuItem onClick={onClickEditDefinition}>Edit definition...</MenuItem>
                        <Divider />
                        <MenuItem onClick={onClickDelete} sx={{ color: 'error.main' }}>Delete</MenuItem>
                    </Menu>
                </Stack>
            </Stack>

            {actionError && <Alert severity="error" onClose={() => setActionError(null)}>{actionError}</Alert>}

            <Box>
                <Typography variant="subtitle2" color="text.secondary">ID</Typography>
                <Stack direction="row" spacing={1} alignItems="center" sx={{ mt: 0.5 }}>
                    <Typography
                        variant="body1"
                        component="code"
                        sx={{
                            wordBreak: 'break-all',
                            fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                            bgcolor: 'action.hover',
                            px: 1, py: 0.5, borderRadius: 0.5,
                            fontSize: '0.9rem',
                        }}
                    >
                        {rl.id}
                    </Typography>
                    <Tooltip title={copied ? 'Copied!' : 'Copy'} placement="top">
                        <IconButton size="small" aria-label="Copy rate limit id" onClick={handleCopyId}>
                            <ContentCopyIcon fontSize="inherit" />
                        </IconButton>
                    </Tooltip>
                </Stack>
            </Box>

            <Box>
                <Typography variant="subtitle2" color="text.secondary">Namespace</Typography>
                <Typography variant="body1">{rl.namespace}</Typography>
            </Box>

            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={4}>
                <Box>
                    <Typography variant="subtitle2" color="text.secondary">Created</Typography>
                    <Typography variant="body1">{dayjs(rl.created_at).format('MMM DD, YYYY, h:mm A')}</Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" color="text.secondary">Updated</Typography>
                    <Typography variant="body1">{dayjs(rl.updated_at).format('MMM DD, YYYY, h:mm A')}</Typography>
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
                    {rl.definition.selector.label_selector && (
                        <Chip label={`labels: ${rl.definition.selector.label_selector}`} size="small" variant="outlined" />
                    )}
                    {rl.definition.selector.methods && rl.definition.selector.methods.length > 0 && (
                        <Chip label={`methods: ${rl.definition.selector.methods.join(', ')}`} size="small" variant="outlined" />
                    )}
                    {rl.definition.selector.path_match && (
                        <Chip
                            label={`path ${rl.definition.selector.path_match.kind}: ${rl.definition.selector.path_match.value}`}
                            size="small" variant="outlined"
                        />
                    )}
                    {rl.definition.selector.request_types && rl.definition.selector.request_types.length > 0 && (
                        <Chip
                            label={`types: ${rl.definition.selector.request_types.join(', ')}`}
                            size="small" variant="outlined"
                        />
                    )}
                    {!rl.definition.selector.label_selector
                        && (!rl.definition.selector.methods || rl.definition.selector.methods.length === 0)
                        && !rl.definition.selector.path_match
                        && (!rl.definition.selector.request_types || rl.definition.selector.request_types.length === 0) && (
                        <Typography variant="body2" color="text.secondary">No selector clauses (matches default proxy + probe traffic)</Typography>
                    )}
                </Stack>
            </Box>

            <Box>
                <Typography variant="subtitle2" color="text.secondary">Bucket</Typography>
                {rl.definition.bucket.dimensions && rl.definition.bucket.dimensions.length > 0 ? (
                    <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{ mt: 0.5 }}>
                        {rl.definition.bucket.dimensions.map((d) => (
                            <Chip key={d} label={d} size="small" variant="outlined" />
                        ))}
                    </Stack>
                ) : (
                    <Typography variant="body2" color="text.secondary">Single global bucket per rule</Typography>
                )}
            </Box>

            <Box>
                <Typography variant="subtitle2" color="text.secondary">Labels</Typography>
                {rl.labels && Object.keys(rl.labels).length > 0 ? (
                    <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{ mt: 0.5, rowGap: 0.5 }}>
                        {Object.entries(rl.labels).map(([key, value]) => (
                            <Chip key={key} label={`${key}: ${value}`} size="small" variant="outlined" />
                        ))}
                    </Stack>
                ) : (
                    <Typography variant="body2" color="text.secondary">No labels</Typography>
                )}
            </Box>

            <AnnotationsEditor
                annotations={rl.annotations}
                onPut={async (key, value) => {
                    await rateLimits.putAnnotation(rl.id, key, value);
                    fetchRl();
                }}
                onDelete={async (key) => {
                    await rateLimits.deleteAnnotation(rl.id, key);
                    fetchRl();
                }}
            />

            {/* Edit-definition dialog */}
            <Dialog open={editDefOpen} onClose={() => !actionLoading && setEditDefOpen(false)} fullWidth maxWidth="md">
                <DialogTitle>Edit definition</DialogTitle>
                <DialogContent>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                        The mode toggle in the header is the quick path for the most-common change. Use this dialog for selector / bucket / algorithm edits.
                    </Typography>
                    <RateLimitDefinitionEditor value={editDef} onChange={setEditDef} />
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setEditDefOpen(false)} disabled={actionLoading}>Cancel</Button>
                    <Button onClick={onSubmitEditDefinition} variant="contained" disabled={actionLoading}>Save</Button>
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
