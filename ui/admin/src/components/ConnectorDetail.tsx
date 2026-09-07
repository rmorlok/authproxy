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
import Menu from '@mui/material/Menu';
import MuiLink from '@mui/material/Link';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import IconButton from '@mui/material/IconButton';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import LinkOffIcon from '@mui/icons-material/LinkOff';
import ArchiveIcon from '@mui/icons-material/Archive';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import dayjs from 'dayjs';
import {
  Connector,
  CONNECTOR_KIND,
  API_VERSION,
  connectors,
  PollForTaskResult,
  Task,
  tasks,
  TaskState,
} from '@authproxy/api';
import {Link, useNavigate} from 'react-router-dom';
import {StateChip} from "./StateChip";
import ConnectorVersionDetail from "./ConnectorVersionDetail";
import ResourceIdentifier from './ResourceIdentifier';
import ResourceMetadataMenuItems from './ResourceMetadataMenuItems';
import AnnotationsEditor from "./AnnotationsEditor";
import ResourceNameEditor from './ResourceNameEditor';

const CONNECTOR_LIFECYCLE_TIMEOUT_SECONDS = 600;

type LifecycleAction = 'disconnect-all' | 'archive';

interface LifecycleStatus {
  action: LifecycleAction;
  state: 'starting' | 'polling' | 'completed' | 'failed';
  taskId?: string;
  task?: Task;
  message?: string;
}

interface AdminConnectorDefinition extends Record<string, unknown> {
  displayName?: string;
  description?: string;
  highlight?: string;
  statusPageUrl?: string;
  logo?: {publicUrl?: string; base64?: string; mimeType?: string};
}

function connectorLogoUrl(definition: AdminConnectorDefinition): string {
  if (definition.logo?.publicUrl) return definition.logo.publicUrl;
  if (!definition.logo?.base64) return '';
  return `data:${definition.logo.mimeType || 'image/png'};base64,${definition.logo.base64}`;
}

export default function ConnectorDetail({connectorId, initialVersion}: { connectorId: string, initialVersion?: number }) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [conn, setConn] = useState<Connector | null>(null);
  const [confirmDisconnectAllOpen, setConfirmDisconnectAllOpen] = useState(false);
  const [confirmArchiveOpen, setConfirmArchiveOpen] = useState(false);
  const [lifecycleStatus, setLifecycleStatus] = useState<LifecycleStatus | null>(null);

  // versions state
  const [versions, setVersions] = useState<Connector[]>([]);
  const [versionsError, setVersionsError] = useState<string | null>(null);
  const [drawerOpen, setDrawerOpen] = useState<boolean>(false);
  const [selectedVersion, setSelectedVersion] = useState<number | undefined>(initialVersion);
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const [metadataNotice, setMetadataNotice] = useState<string | null>(null);
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
    connectors.listGenerations(connectorId, { limit: 100, orderBy: 'generation desc' })
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

  const onRowClick = (v: Connector) => {
    setSelectedVersion(v.metadata.generation);
    setDrawerOpen(true);
  };

  const closeDrawer = () => {
    setDrawerOpen(false);
    setSelectedVersion(undefined);
    navigate(`/connectors/${connectorId}`);
  };

  const selected = useMemo<Connector | undefined>(
    () => versions.find(v => v.metadata.generation === selectedVersion),
    [versions, selectedVersion],
  );
  const availableStates = useMemo(
    () => Array.from(new Set(versions.map(v => v.status.release.state))),
    [versions],
  );

  const refreshConnectorData = useCallback(() => {
    fetchConnector();
    fetchVersions();
  }, [fetchConnector, fetchVersions]);

  const runLifecycleAction = async (action: LifecycleAction) => {
    if (!conn) return;

    setLifecycleStatus({action, state: 'starting'});
    try {
      const request = {timeoutSeconds: CONNECTOR_LIFECYCLE_TIMEOUT_SECONDS};
      const response = action === 'archive'
        ? await connectors.archive(conn.metadata.id, request)
        : await connectors.disconnectAll(conn.metadata.id, request);

      setLifecycleStatus({
        action,
        state: 'polling',
        taskId: response.data.status.taskId,
      });

      const result = await tasks.pollForTaskFinalized(response.data.status.taskId, {
        initialDelay: 1000,
        maxDelay: 5000,
        maxAttempts: 140,
        backoffFactor: 1.4,
      });

      if (result.result !== PollForTaskResult.FINALIZED || result.task?.status.state !== TaskState.COMPLETED) {
        setLifecycleStatus({
          action,
          state: 'failed',
          taskId: response.data.status.taskId,
          task: result.task,
          message: result.task?.status.state === TaskState.FAILED
            ? 'Workflow failed before completing.'
            : 'Task polling ended before the operation completed.',
        });
        refreshConnectorData();
        return;
      }

      setLifecycleStatus({
        action,
        state: 'completed',
        taskId: response.data.status.taskId,
        task: result.task,
      });
      refreshConnectorData();
    } catch (err: any) {
      setLifecycleStatus({
        action,
        state: 'failed',
        message: err?.response?.data?.error || err.message || 'Connector lifecycle operation failed.',
      });
    }
  };

  const lifecycleActionLabel = lifecycleStatus?.action === 'archive' ? 'Archive' : 'Disconnect all';

  const openMenu = (event: React.MouseEvent<HTMLButtonElement>) => setMenuAnchorEl(event.currentTarget);
  const closeMenu = () => setMenuAnchorEl(null);

  if (loading) return (<Box sx={{display: 'flex', justifyContent: 'center', p: 4}}><CircularProgress/></Box>);
  if (error) return (<Alert severity="error">{error}</Alert>);
  if (!conn) return null;
  const definition = conn.spec.definition as AdminConnectorDefinition;

  return (
    <Stack spacing={2} sx={{p: 2}}>
      <Stack direction="row" spacing={2} alignItems="center">
        {definition.logo && <Avatar alt={definition.displayName} src={connectorLogoUrl(definition)} sx={{width: 40, height: 40}} />}
        <ResourceNameEditor
          name={conn.metadata.name}
          resourceType="Connector"
          onRename={async (name) => {
            await connectors.update(conn.metadata.id, {
              apiVersion: API_VERSION,
              kind: CONNECTOR_KIND,
              metadata: {name},
              spec: {},
            });
            refreshConnectorData();
          }}
        />
        <StateChip state={conn.status.release.state}/>
        <Box sx={{flexGrow: 1}}/>
        <IconButton aria-label="actions" onClick={openMenu} size="small">
          <MoreVertIcon/>
        </IconButton>
        <Menu anchorEl={menuAnchorEl} open={Boolean(menuAnchorEl)} onClose={closeMenu} keepMounted>
          <ResourceMetadataMenuItems
            resource="connector"
            name={conn.metadata.name}
            labels={conn.metadata.labels}
            annotations={conn.metadata.annotations}
            onCloseMenu={closeMenu}
            includeRename={false}
            onUpdateLabels={async (labels) => {
              await connectors.update(conn.metadata.id, {
                apiVersion: API_VERSION,
                kind: CONNECTOR_KIND,
                metadata: {labels},
                spec: {},
              });
              setMetadataNotice('Label changes were saved to a draft connector version.');
              refreshConnectorData();
            }}
            onUpdateAnnotations={async (annotations) => {
              await connectors.update(conn.metadata.id, {
                apiVersion: API_VERSION,
                kind: CONNECTOR_KIND,
                metadata: {annotations},
                spec: {},
              });
              setMetadataNotice('Annotation changes were saved to a draft connector version.');
              refreshConnectorData();
            }}
            disabled={actionInProgress}
          />
        </Menu>
      </Stack>

      {definition.displayName && (
        <Typography variant="body2" color="text.secondary">Definition display name: {definition.displayName}</Typography>
      )}

      {definition.description && (
        <Typography variant="body1" color="text.secondary">{definition.description}</Typography>
      )}

      {definition.highlight && (
        <Alert severity="info">{definition.highlight}</Alert>
      )}

      {metadataNotice && <Alert severity="info" onClose={() => setMetadataNotice(null)}>{metadataNotice}</Alert>}

      {definition.statusPageUrl && (
        <MuiLink href={definition.statusPageUrl} target="_blank" rel="noopener noreferrer" underline="hover" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}>
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
                {lifecycleStatus.task?.status.state ? ` (${lifecycleStatus.task.status.state})` : ''}
              </Typography>
            )}
          </Alert>
        )}
      </Box>

      <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
        <ResourceIdentifier value={conn.metadata.id} copyLabel="Copy connector id"/>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Labels</Typography>
          {conn.metadata.labels && Object.keys(conn.metadata.labels).length > 0 ? (
            <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{ mt: 0.5 }}>
              {Object.entries(conn.metadata.labels).map(([key, value]) => (
                <Chip key={key} label={`${key}: ${value}`} size="small" variant="outlined" />
              ))}
            </Stack>
          ) : (
            <Typography variant="body2" color="text.secondary">No labels</Typography>
          )}
        </Box>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Version</Typography>
          <Typography variant="body1">{conn.metadata.generation}</Typography>
        </Box>
      </Stack>

      <AnnotationsEditor annotations={conn.metadata.annotations} readOnly onPut={async () => {}} onDelete={async () => {}}/>

      <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Available States</Typography>
          <Stack direction="row" spacing={1} sx={{mt: 0.5}}>
            {availableStates.map(s => <StateChip key={s} state={s} />)}
          </Stack>
        </Box>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Versions</Typography>
          <Typography variant="body1">{versions.length}</Typography>
        </Box>
      </Stack>

      <Box>
        <Typography variant="h6" sx={{mt:2, mb:1}}>All Versions</Typography>
        {versionsError && <Alert severity="error">{versionsError}</Alert>}
        <Stack spacing={1}>
          {versions.map(v => (
            <Box key={`${v.metadata.id}:${v.metadata.generation}`} sx={{border: '1px solid', borderColor: 'divider', borderRadius: 1, p: 1.5}}>
              <Stack direction={{xs: 'column', sm: 'row'}} spacing={1} alignItems={{sm: 'center'}} justifyContent="space-between">
                <Stack direction="row" spacing={1} alignItems="center">
                  <Typography variant="body1">v{v.metadata.generation}</Typography>
                  <StateChip state={v.status.release.state} />
                  <Typography variant="body2" color="text.secondary">{dayjs(v.metadata.createdAt).format('MMM DD, YYYY')}</Typography>
                </Stack>
                <Stack direction="row" spacing={1}>
                  <Button size="small" onClick={() => onRowClick(v)}>View Definition</Button>
                  <Button component={Link} size="small" to={`/connectors/${connectorId}/generations/${v.metadata.generation}`}>Open Page</Button>
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
