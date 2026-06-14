import React, {useCallback, useEffect, useMemo, useState} from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import Stack from '@mui/material/Stack';
import Avatar from '@mui/material/Avatar';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Drawer from '@mui/material/Drawer';
import MuiLink from '@mui/material/Link';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import LinkOffIcon from '@mui/icons-material/LinkOff';
import ArchiveIcon from '@mui/icons-material/Archive';
import dayjs from 'dayjs';
import {
  Connector,
  connectors,
  ConnectorVersion,
  PollForTaskResult,
  TaskInfoJson,
  tasks,
  TaskState,
} from '@authproxy/api';
import {Link, useNavigate} from 'react-router-dom';
import {StateChip} from "./StateChip";
import ConnectorVersionDetail from "./ConnectorVersionDetail";
import AnnotationsEditor from "./AnnotationsEditor";

const CONNECTOR_LIFECYCLE_TIMEOUT_SECONDS = 600;

type LifecycleAction = 'disconnect-all' | 'archive';

interface LifecycleStatus {
  action: LifecycleAction;
  state: 'starting' | 'polling' | 'completed' | 'failed';
  taskId?: string;
  task?: TaskInfoJson;
  message?: string;
}

export default function ConnectorDetail({connectorId, initialVersion}: { connectorId: string, initialVersion?: number }) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [conn, setConn] = useState<Connector | null>(null);
  const [confirmDisconnectAllOpen, setConfirmDisconnectAllOpen] = useState(false);
  const [confirmArchiveOpen, setConfirmArchiveOpen] = useState(false);
  const [lifecycleStatus, setLifecycleStatus] = useState<LifecycleStatus | null>(null);

  // versions state
  const [versions, setVersions] = useState<ConnectorVersion[]>([]);
  const [versionsError, setVersionsError] = useState<string | null>(null);
  const [drawerOpen, setDrawerOpen] = useState<boolean>(false);
  const [selectedVersion, setSelectedVersion] = useState<number | undefined>(initialVersion);
  const navigate = useNavigate();
  const actionInProgress = lifecycleStatus?.state === 'starting' || lifecycleStatus?.state === 'polling';

  const fetchConnector = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    connectors.get(connectorId)
      .then(res => {
        if (cancelled) return;
        setConn(res.data);
      })
      .catch(err => {
        if (cancelled) return;
        const msg = err?.response?.data?.error || err.message || 'Failed to load connector';
        setError(msg);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [connectorId]);

  const fetchVersions = useCallback(() => {
    let cancelled = false;
    setVersionsError(null);
    setVersions([]);
    connectors.listVersions(connectorId, { limit: 100, order_by: 'version desc' })
      .then(resp => {
        if (cancelled) return;
        setVersions(resp.data.items || []);
      })
      .catch(err => {
        if (cancelled) return;
        setVersionsError(err?.response?.data?.error || err.message || 'Failed to load versions');
      });
    return () => { cancelled = true; };
  }, [connectorId]);

  useEffect(() => fetchConnector(), [fetchConnector]);

  // fetch versions
  useEffect(() => fetchVersions(), [fetchVersions]);

  useEffect(() => {
    setConfirmDisconnectAllOpen(false);
    setConfirmArchiveOpen(false);
    setLifecycleStatus(null);
  }, [connectorId]);

  // open drawer if initialVersion provided
  useEffect(() => {
    if (initialVersion) {
      setSelectedVersion(initialVersion);
      setDrawerOpen(true);
    }
  }, [initialVersion]);

  const onRowClick = (v: ConnectorVersion) => {
    setSelectedVersion(v.version);
    setDrawerOpen(true);
  };

  const closeDrawer = () => {
    setDrawerOpen(false);
    setSelectedVersion(undefined);
    navigate(`/connectors/${connectorId}`);
  };

  const selected = useMemo<ConnectorVersion | undefined>(() => versions.find(v => v.version === selectedVersion), [versions, selectedVersion]);

  const refreshConnectorData = useCallback(() => {
    fetchConnector();
    fetchVersions();
  }, [fetchConnector, fetchVersions]);

  const runLifecycleAction = async (action: LifecycleAction) => {
    if (!conn) return;

    setLifecycleStatus({action, state: 'starting'});
    try {
      const request = {timeout_seconds: CONNECTOR_LIFECYCLE_TIMEOUT_SECONDS};
      const response = action === 'archive'
        ? await connectors.archive(conn.id, request)
        : await connectors.disconnectAll(conn.id, request);

      setLifecycleStatus({
        action,
        state: 'polling',
        taskId: response.data.task_id,
      });

      const result = await tasks.pollForTaskFinalized(response.data.task_id, {
        initialDelay: 1000,
        maxDelay: 5000,
        maxAttempts: 140,
        backoffFactor: 1.4,
      });

      if (result.result !== PollForTaskResult.FINALIZED || result.taskInfo?.state !== TaskState.COMPLETED) {
        setLifecycleStatus({
          action,
          state: 'failed',
          taskId: response.data.task_id,
          task: result.taskInfo,
          message: result.taskInfo?.state === TaskState.FAILED
            ? 'Workflow failed before completing.'
            : 'Task polling ended before the operation completed.',
        });
        refreshConnectorData();
        return;
      }

      setLifecycleStatus({
        action,
        state: 'completed',
        taskId: response.data.task_id,
        task: result.taskInfo,
      });
      refreshConnectorData();
    } catch (err: any) { // eslint-disable-line @typescript-eslint/no-explicit-any
      setLifecycleStatus({
        action,
        state: 'failed',
        message: err?.response?.data?.error || err.message || 'Connector lifecycle operation failed.',
      });
    }
  };

  const lifecycleActionLabel = lifecycleStatus?.action === 'archive' ? 'Archive' : 'Disconnect all';

  if (loading) return (<Box sx={{display: 'flex', justifyContent: 'center', p: 4}}><CircularProgress/></Box>);
  if (error) return (<Alert severity="error">{error}</Alert>);
  if (!conn) return null;

  return (
    <Stack spacing={2} sx={{p: 2}}>
      <Stack direction="row" spacing={2} alignItems="center">
        {conn.logo && <Avatar alt={conn.display_name} src={conn.logo} sx={{width: 40, height: 40}} />}
        <Typography variant="h5">{conn.display_name || conn.labels?.type || 'Unnamed Connector'}</Typography>
        <StateChip state={conn.state}/>
      </Stack>

      {conn.description && (
        <Typography variant="body1" color="text.secondary">{conn.description}</Typography>
      )}

      {conn.highlight && (
        <Alert severity="info">{conn.highlight}</Alert>
      )}

      {conn.status_page_url && (
        <MuiLink href={conn.status_page_url} target="_blank" rel="noopener noreferrer" underline="hover" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}>
          Status Page <OpenInNewIcon fontSize="inherit" />
        </MuiLink>
      )}

      <Box sx={{border: '1px solid', borderColor: 'divider', borderRadius: 1, p: 2}}>
        <Stack direction={{xs: 'column', sm: 'row'}} spacing={2} justifyContent="space-between" alignItems={{xs: 'stretch', sm: 'center'}}>
          <Box>
            <Typography variant="h6">Lifecycle</Typography>
            <Typography variant="body2" color="text.secondary">
              Connector-wide operations run in the background and can take several minutes.
            </Typography>
          </Box>
          <Stack direction={{xs: 'column', sm: 'row'}} spacing={1}>
            <Button
              variant="outlined"
              color="warning"
              startIcon={<LinkOffIcon />}
              disabled={actionInProgress}
              onClick={() => setConfirmDisconnectAllOpen(true)}
            >
              Disconnect all
            </Button>
            <Button
              variant="contained"
              color="error"
              startIcon={<ArchiveIcon />}
              disabled={actionInProgress}
              onClick={() => setConfirmArchiveOpen(true)}
            >
              Archive
            </Button>
          </Stack>
        </Stack>

        {lifecycleStatus && (
          <Alert
            severity={
              lifecycleStatus.state === 'completed'
                ? 'success'
                : lifecycleStatus.state === 'failed'
                  ? 'error'
                  : 'info'
            }
            sx={{mt: 2}}
          >
            {lifecycleStatus.state === 'starting' && `${lifecycleActionLabel} is starting...`}
            {lifecycleStatus.state === 'polling' && `${lifecycleActionLabel} is running. Task state will update when the workflow completes.`}
            {lifecycleStatus.state === 'completed' && `${lifecycleActionLabel} completed.`}
            {lifecycleStatus.state === 'failed' && (lifecycleStatus.message || `${lifecycleActionLabel} failed.`)}
            {lifecycleStatus.taskId && (
              <Typography component="div" variant="caption" sx={{mt: 0.5, wordBreak: 'break-all'}}>
                Task: {lifecycleStatus.taskId}
                {lifecycleStatus.task?.state ? ` (${lifecycleStatus.task.state})` : ''}
              </Typography>
            )}
          </Alert>
        )}
      </Box>

      <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Connector ID</Typography>
          <Typography variant="body1" sx={{wordBreak: 'break-all'}}>{conn.id}</Typography>
        </Box>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Labels</Typography>
          {conn.labels && Object.keys(conn.labels).length > 0 ? (
            <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{ mt: 0.5 }}>
              {Object.entries(conn.labels).map(([key, value]) => (
                <Chip key={key} label={`${key}: ${value}`} size="small" variant="outlined" />
              ))}
            </Stack>
          ) : (
            <Typography variant="body2" color="text.secondary">No labels</Typography>
          )}
        </Box>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Version</Typography>
          <Typography variant="body1">{conn.version}</Typography>
        </Box>
      </Stack>

      <AnnotationsEditor
        annotations={conn.annotations}
        readOnly
        onPut={async () => {}}
        onDelete={async () => {}}
      />

      <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Available States</Typography>
          <Stack direction="row" spacing={1} sx={{mt: 0.5}}>
            {conn.states?.map(s => <StateChip key={s} state={s} />)}
          </Stack>
        </Box>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Versions</Typography>
          <Typography variant="body1">{conn.versions}</Typography>
        </Box>
      </Stack>

      <Box>
        <Typography variant="h6" sx={{mt:2, mb:1}}>All Versions</Typography>
        {versionsError && <Alert severity="error">{versionsError}</Alert>}
        <Stack spacing={1}>
          {versions.map(v => (
            <Box key={v.id} sx={{border: '1px solid', borderColor: 'divider', borderRadius: 1, p: 1.5}}>
              <Stack direction={{xs: 'column', sm: 'row'}} spacing={1} alignItems={{sm: 'center'}} justifyContent="space-between">
                <Stack direction="row" spacing={1} alignItems="center">
                  <Typography variant="body1">v{v.version}</Typography>
                  <StateChip state={v.state} />
                  <Typography variant="body2" color="text.secondary">{dayjs(v.created_at).format('MMM DD, YYYY')}</Typography>
                </Stack>
                <Stack direction="row" spacing={1}>
                  <Button size="small" onClick={() => onRowClick(v)}>View Definition</Button>
                  <Button component={Link} size="small" to={`/connectors/${connectorId}/versions/${v.version}`}>Open Page</Button>
                </Stack>
              </Stack>
            </Box>
          ))}
          {versions.length === 0 && (
            <Typography variant="body2" color="text.secondary">No versions found.</Typography>
          )}
        </Stack>
      </Box>

      <Drawer anchor="right" open={drawerOpen} onClose={closeDrawer} sx={{'& .MuiDrawer-paper': { width: { xs: '100%', sm: 800 }}}}>
          {(selected && <ConnectorVersionDetail connectorVersion={selected} />)}
      </Drawer>

      <Dialog open={confirmDisconnectAllOpen} onClose={() => !actionInProgress && setConfirmDisconnectAllOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>Disconnect all connections</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary">
            This starts a workflow that disconnects every connection for this connector. Connections may need to be reconnected before they can be used again.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmDisconnectAllOpen(false)} disabled={actionInProgress}>Cancel</Button>
          <Button
            color="warning"
            variant="contained"
            disabled={actionInProgress}
            startIcon={actionInProgress ? <CircularProgress size={16} /> : <LinkOffIcon />}
            onClick={() => {
              setConfirmDisconnectAllOpen(false);
              void runLifecycleAction('disconnect-all');
            }}
          >
            Disconnect all
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={confirmArchiveOpen} onClose={() => !actionInProgress && setConfirmArchiveOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>Archive connector</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary">
            This archives draft versions, prevents new connections, disconnects existing connections, and archives active versions when the workflow finishes.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmArchiveOpen(false)} disabled={actionInProgress}>Cancel</Button>
          <Button
            color="error"
            variant="contained"
            disabled={actionInProgress}
            startIcon={actionInProgress ? <CircularProgress size={16} /> : <ArchiveIcon />}
            onClick={() => {
              setConfirmArchiveOpen(false);
              void runLifecycleAction('archive');
            }}
          >
            Archive
          </Button>
        </DialogActions>
      </Dialog>
    </Stack>
  );
}
