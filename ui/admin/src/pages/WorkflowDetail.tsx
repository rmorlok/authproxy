import * as React from 'react';
import {useCallback, useEffect, useState} from 'react';
import {useNavigate, useParams} from 'react-router-dom';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Grid from '@mui/material/Grid';
import IconButton from '@mui/material/IconButton';
import Stack from '@mui/material/Stack';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import {DataGrid, GridColDef} from '@mui/x-data-grid';
import CancelIcon from '@mui/icons-material/Cancel';
import DeleteIcon from '@mui/icons-material/Delete';
import RefreshIcon from '@mui/icons-material/Refresh';
import {
    cancelWorkflowInstance,
    getWorkflowInstance,
    getWorkflowTree,
    removeWorkflowInstance,
    WorkflowHistoryEvent,
    WorkflowInstance,
} from '@authproxy/api';

const stateColors: Record<string, 'primary' | 'secondary' | 'success' | 'warning' | 'default'> = {
    active: 'primary',
    continued_as_new: 'warning',
    finished: 'success',
};

function formatTimestamp(value?: string) {
    return value ? new Date(value).toLocaleString() : '-';
}

function stringify(value: unknown) {
    if (value == null) return '';
    if (typeof value === 'string') return value;
    try {
        return JSON.stringify(value);
    } catch {
        return String(value);
    }
}

function TreeNode({node, depth = 0}: { node: WorkflowInstance; depth?: number }) {
    return (
        <Box sx={{pl: depth * 2, py: 0.75}}>
            <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
                <Typography variant="body2" fontWeight={600}>{node.spec.workflowName || node.spec.instanceId || 'Workflow'}</Typography>
                <Chip
                    label={node.status.state || 'unknown'}
                    size="small"
                    color={stateColors[node.status.state] ?? 'default'}
                    variant={node.status.state === 'finished' ? 'outlined' : 'filled'}
                />
                {node.status.error && <Chip label="Error" size="small" color="error"/>}
                {node.spec.queue && <Typography variant="caption" color="text.secondary">{node.spec.queue}</Typography>}
            </Stack>
            <Typography variant="caption" color="text.secondary">
                {node.spec.instanceId || '-'} / {node.metadata.id || '-'}
            </Typography>
            {node.status.children?.map((child) => (
                <TreeNode
                    key={`${child.spec.instanceId}:${child.metadata.id}`}
                    node={child}
                    depth={depth + 1}
                />
            ))}
        </Box>
    );
}

export default function WorkflowDetail() {
    const navigate = useNavigate();
    const {instanceId, executionId} = useParams<{ instanceId: string; executionId: string }>();
    const decodedInstanceId = instanceId ? decodeURIComponent(instanceId) : '';
    const decodedExecutionId = executionId ? decodeURIComponent(executionId) : '';

    const [info, setInfo] = useState<WorkflowInstance | null>(null);
    const [tree, setTree] = useState<WorkflowInstance | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [actionError, setActionError] = useState<string | null>(null);
    const [actionLoading, setActionLoading] = useState(false);
    const [confirmCancelOpen, setConfirmCancelOpen] = useState(false);
    const [confirmRemoveOpen, setConfirmRemoveOpen] = useState(false);

    const fetchWorkflow = useCallback(async () => {
        if (!decodedInstanceId || !decodedExecutionId) return;
        setLoading(true);
        setError(null);
        try {
            const [infoResp, treeResp] = await Promise.all([
                getWorkflowInstance(decodedInstanceId, decodedExecutionId),
                getWorkflowTree(decodedInstanceId, decodedExecutionId),
            ]);
            if (infoResp.status === 200) setInfo(infoResp.data);
            if (treeResp.status === 200) setTree(treeResp.data);
        } catch {
            setError('Failed to load workflow instance');
        } finally {
            setLoading(false);
        }
    }, [decodedInstanceId, decodedExecutionId]);

    useEffect(() => {
        fetchWorkflow();
    }, [fetchWorkflow]);

    const handleCancel = async () => {
        setActionLoading(true);
        setActionError(null);
        try {
            await cancelWorkflowInstance(decodedInstanceId, decodedExecutionId);
            setConfirmCancelOpen(false);
            fetchWorkflow();
        } catch {
            setActionError('Failed to cancel workflow instance');
        } finally {
            setActionLoading(false);
        }
    };

    const handleRemove = async () => {
        setActionLoading(true);
        setActionError(null);
        try {
            await removeWorkflowInstance(decodedInstanceId, decodedExecutionId);
            setConfirmRemoveOpen(false);
            navigate('/internal-workflows');
        } catch {
            setActionError('Failed to remove workflow instance');
        } finally {
            setActionLoading(false);
        }
    };

    const historyColumns: GridColDef<WorkflowHistoryEvent>[] = [
        {field: 'sequenceId', headerName: 'Seq', flex: 0.35, minWidth: 70, valueGetter: (_, row) => row.spec.sequenceId},
        {field: 'type', headerName: 'Type', flex: 0.9, minWidth: 180, valueGetter: (_, row) => row.spec.type},
        {
            field: 'timestamp',
            headerName: 'Timestamp',
            flex: 0.8,
            minWidth: 170,
            valueGetter: (_, row) => row.metadata.createdAt,
            renderCell: (params) => formatTimestamp(params.value as string | undefined),
        },
        {field: 'scheduleEventId', headerName: 'Schedule Event', flex: 0.5, minWidth: 120, valueGetter: (_, row) => row.spec.scheduleEventId},
        {
            field: 'attributes',
            headerName: 'Attributes',
            flex: 1.4,
            minWidth: 240,
            valueGetter: (_, row) => row.spec.attributes,
            renderCell: (params) => {
                const text = stringify(params.value);
                if (!text) return null;
                return (
                    <Tooltip title={text}>
                        <span style={{overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap'}}>{text}</span>
                    </Tooltip>
                );
            },
        },
    ];

    const history = info?.status.history || [];
    const canCancel = info?.status.state === 'active';

    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{mb: 2}}>
                <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
                    <Typography component="h2" variant="h6">Workflow Instance</Typography>
                    {info?.status.state && (
                        <Chip
                            label={info.status.state}
                            color={stateColors[info.status.state] ?? 'default'}
                            size="small"
                            variant={info.status.state === 'finished' ? 'outlined' : 'filled'}
                        />
                    )}
                </Stack>
                <Stack direction="row" spacing={1}>
                    <Tooltip title="Refresh">
                        <IconButton onClick={fetchWorkflow} disabled={loading}>
                            <RefreshIcon/>
                        </IconButton>
                    </Tooltip>
                    <Tooltip title={canCancel ? 'Cancel workflow' : 'Only active workflows can be cancelled'}>
                        <span>
                            <IconButton
                                aria-label="Cancel workflow"
                                color="warning"
                                onClick={() => setConfirmCancelOpen(true)}
                                disabled={!canCancel || actionLoading}
                            >
                                <CancelIcon/>
                            </IconButton>
                        </span>
                    </Tooltip>
                    <Tooltip title="Remove workflow">
                        <IconButton
                            aria-label="Remove workflow"
                            color="error"
                            onClick={() => setConfirmRemoveOpen(true)}
                            disabled={actionLoading}
                        >
                            <DeleteIcon/>
                        </IconButton>
                    </Tooltip>
                </Stack>
            </Stack>

            {error && <Alert severity="error" sx={{mb: 2}}>{error}</Alert>}
            {actionError && <Alert severity="error" sx={{mb: 2}} onClose={() => setActionError(null)}>{actionError}</Alert>}

            <Grid container spacing={3}>
                <Grid size={{xs: 12, lg: 5}}>
                    <Card variant="outlined" sx={{mb: 3}}>
                        <CardContent>
                            <Typography component="h2" variant="subtitle2" gutterBottom>
                                Details
                            </Typography>
                            <Stack spacing={1}>
                                <Typography variant="body2"><strong>Instance ID:</strong> {decodedInstanceId}</Typography>
                                <Typography variant="body2"><strong>Execution ID:</strong> {decodedExecutionId}</Typography>
                                <Typography variant="body2"><strong>Queue:</strong> {info?.spec.queue || '-'}</Typography>
                                <Typography variant="body2"><strong>Created:</strong> {formatTimestamp(info?.metadata.createdAt)}</Typography>
                                <Typography variant="body2"><strong>Completed:</strong> {formatTimestamp(info?.status.completedAt)}</Typography>
                                {info?.spec.parentRef && (
                                    <Typography variant="body2">
                                        <strong>Parent:</strong> {info.spec.parentRef.instanceId} / {info.spec.parentRef.target.id || '-'}
                                    </Typography>
                                )}
                            </Stack>
                        </CardContent>
                    </Card>

                    <Card variant="outlined">
                        <CardContent>
                            <Typography component="h2" variant="subtitle2" gutterBottom>
                                Tree
                            </Typography>
                            {tree ? <TreeNode node={tree}/> : (
                                <Typography variant="body2" color="text.secondary">
                                    {loading ? 'Loading...' : 'No workflow tree found.'}
                                </Typography>
                            )}
                        </CardContent>
                    </Card>
                </Grid>

                <Grid size={{xs: 12, lg: 7}}>
                    <Card variant="outlined">
                        <CardContent>
                            <Typography component="h2" variant="subtitle2" gutterBottom>
                                History
                            </Typography>
                            <DataGrid
                                autoHeight
                                rows={history}
                                columns={historyColumns}
                                getRowId={(row) => `${row.spec.sequenceId ?? ''}:${row.metadata.id ?? ''}:${row.spec.type ?? ''}`}
                                loading={loading}
                                hideFooterSelectedRowCount
                                density="compact"
                                disableColumnResize
                                pageSizeOptions={[10, 25, 50]}
                                initialState={{
                                    pagination: {
                                        paginationModel: {pageSize: 25},
                                    },
                                }}
                            />
                        </CardContent>
                    </Card>
                </Grid>
            </Grid>

            <Dialog open={confirmCancelOpen} onClose={() => !actionLoading && setConfirmCancelOpen(false)}>
                <DialogTitle>Cancel workflow</DialogTitle>
                <DialogContent>
                    <Typography variant="body2">
                        Cancel this workflow instance and let workers process the cancellation event.
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setConfirmCancelOpen(false)} disabled={actionLoading}>Close</Button>
                    <Button onClick={handleCancel} color="warning" disabled={actionLoading}>Cancel workflow</Button>
                </DialogActions>
            </Dialog>

            <Dialog open={confirmRemoveOpen} onClose={() => !actionLoading && setConfirmRemoveOpen(false)}>
                <DialogTitle>Remove workflow</DialogTitle>
                <DialogContent>
                    <Typography variant="body2">
                        Remove this workflow instance from workflow storage.
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setConfirmRemoveOpen(false)} disabled={actionLoading}>Close</Button>
                    <Button onClick={handleRemove} color="error" disabled={actionLoading}>Remove</Button>
                </DialogActions>
            </Dialog>
        </Box>
    );
}
