import React, {useEffect, useMemo, useState} from 'react';
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
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import FormHelperText from '@mui/material/FormHelperText';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import dayjs from 'dayjs';
import Tooltip from '@mui/material/Tooltip';
import {
  Connection,
  connections,
  ConnectionState,
  ConnectionHealthState,
  ConnectorVersion,
  ConnectorVersionState,
  connectors,
  PollForTaskResult,
  TaskInfoJson,
  tasks,
  TaskState,
  canBeDisconnected,
} from '@authproxy/api';
import WarningAmberIcon from '@mui/icons-material/WarningAmber';
import SwapHorizIcon from '@mui/icons-material/SwapHoriz';
import { Link } from "react-router-dom";
import AnnotationsEditor from "./AnnotationsEditor";
import ResourceNameEditor from './ResourceNameEditor';

const CONNECTION_MIGRATION_TIMEOUT_SECONDS = 600;

type MigrationAction = 'migrate' | 'rollback';

interface MigrationStatus {
  action: MigrationAction;
  state: 'starting' | 'polling' | 'completed' | 'failed';
  targetVersion: number;
  taskId?: string;
  task?: TaskInfoJson;
  message?: string;
}

function StateChip({state}: { state: ConnectionState }) {
  const colors: Record<ConnectionState, "default" | "success" | "error" | "info" | "warning" | "primary" | "secondary"> = {
    [ConnectionState.SETUP]: 'primary',
    [ConnectionState.CONFIGURED]: 'success',
    [ConnectionState.DISABLED]: 'error',
    [ConnectionState.DISCONNECTING]: 'warning',
    [ConnectionState.DISCONNECTED]: 'default',
  };
  return <Chip label={state} color={colors[state]} size="small"/>;
}

// HealthChip surfaces the operational health signal alongside the lifecycle state. A Ready
// connection can be unhealthy when credentials have stopped working — the chip makes that
// visible to operators so they can follow up with the connection owner about reauth.
function HealthChip({health}: { health?: ConnectionHealthState }) {
  if (!health || health === ConnectionHealthState.HEALTHY) {
    return null;
  }
  return (
    <Chip
      icon={<WarningAmberIcon />}
      label="Unhealthy"
      color="warning"
      size="small"
    />
  );
}

export default function ConnectionDetail({connectionId}: { connectionId: string }) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [conn, setConn] = useState<Connection | null>(null);

  // Actions UI state
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const [confirmDisconnectOpen, setConfirmDisconnectOpen] = useState(false);
  const [forceStateOpen, setForceStateOpen] = useState(false);
  const [selectedState, setSelectedState] = useState<ConnectionState | ''>('');
  const [actionLoading, setActionLoading] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [migrationOpen, setMigrationOpen] = useState(false);
  const [migrationVersions, setMigrationVersions] = useState<ConnectorVersion[]>([]);
  const [migrationVersionsLoading, setMigrationVersionsLoading] = useState(false);
  const [migrationVersionsError, setMigrationVersionsError] = useState<string | null>(null);
  const [selectedMigrationVersion, setSelectedMigrationVersion] = useState<number | ''>('');
  const [migrationStatus, setMigrationStatus] = useState<MigrationStatus | null>(null);

  // Copy-to-clipboard UI state for connection ID
  const [copied, setCopied] = useState(false);
  const handleCopyId = async () => {
    try {
      await navigator.clipboard.writeText(conn?.id || '');
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch (_e: any) {
      // ignore
    }
  };

  const stateOptions = useMemo(() => Object.values(ConnectionState), []);
  const eligibleMigrationVersions = useMemo(() => {
    if (!conn) {
      return [];
    }
    return migrationVersions.filter((version) =>
      version.version !== conn.connector.version &&
      (version.state === ConnectorVersionState.PRIMARY || version.state === ConnectorVersionState.ACTIVE),
    );
  }, [conn, migrationVersions]);
  const selectedMigrationTarget = useMemo(
    () => eligibleMigrationVersions.find((version) => version.version === selectedMigrationVersion),
    [eligibleMigrationVersions, selectedMigrationVersion],
  );
  const migrationInProgress = migrationStatus?.state === 'starting' || migrationStatus?.state === 'polling';

  const fetchConnection = () => {
    setLoading(true);
    setError(null);
    connections.get(connectionId)
      .then(res => {
        setConn(res.data);
      })
      .catch(err => {
        const msg = err?.response?.data?.error || err.message || 'Failed to load connection';
        setError(msg);
      })
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    connections.get(connectionId)
      .then(res => {
        if (cancelled) return;
        setConn(res.data);
      })
      .catch(err => {
        if (cancelled) return;
        const msg = err?.response?.data?.error || err.message || 'Failed to load connection';
        setError(msg);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [connectionId]);

  if (loading) return (<Box sx={{display: 'flex', justifyContent: 'center', p: 4}}><CircularProgress/></Box>);
  if (error) return (<Alert severity="error">{error}</Alert>);
  if (!conn) return null;

  const openMenu = (e: React.MouseEvent<HTMLButtonElement>) => setMenuAnchorEl(e.currentTarget);
  const closeMenu = () => setMenuAnchorEl(null);

  const onClickDisconnect = () => {
    setActionError(null);
    closeMenu();
    setConfirmDisconnectOpen(true);
  };

  const onConfirmDisconnect = async () => {
    if (!conn) return;
    setActionError(null);
    setActionLoading(true);
    try {
      await connections.disconnect(conn.id);
      setConfirmDisconnectOpen(false);
      fetchConnection();
    } catch (err: any) {
      const msg = err?.response?.data?.error || err.message || 'Failed to disconnect';
      setActionError(msg);
    } finally {
      setActionLoading(false);
    }
  };

  const onClickForceState = () => {
    setActionError(null);
    setSelectedState(conn.state);
    closeMenu();
    setForceStateOpen(true);
  };

  const onSubmitForceState = async () => {
    if (!conn || !selectedState) return;
    setActionError(null);
    setActionLoading(true);
    try {
      await connections.forceState(conn.id, selectedState as ConnectionState);
      setForceStateOpen(false);
      fetchConnection();
    } catch (err: any) {
      const msg = err?.response?.data?.error || err.message || 'Failed to force state';
      setActionError(msg);
    } finally {
      setActionLoading(false);
    }
  };

  const onClickMigration = async () => {
    if (!conn) return;

    setActionError(null);
    setMigrationStatus(null);
    setMigrationVersions([]);
    setMigrationVersionsError(null);
    setSelectedMigrationVersion('');
    closeMenu();
    setMigrationOpen(true);
    setMigrationVersionsLoading(true);

    try {
      const response = await connectors.listVersions(conn.connector.id, {
        limit: 100,
        orderBy: 'version desc',
      });
      const eligible = response.data.items.filter((version) =>
        version.version !== conn.connector.version &&
        (version.state === ConnectorVersionState.PRIMARY || version.state === ConnectorVersionState.ACTIVE),
      );
      setMigrationVersions(response.data.items);
      setSelectedMigrationVersion(
        eligible.find((version) => version.state === ConnectorVersionState.PRIMARY)?.version ??
        eligible[0]?.version ??
        '',
      );
    } catch (err: any) {
      setMigrationVersionsError(err?.response?.data?.error || err.message || 'Failed to load connector versions');
    } finally {
      setMigrationVersionsLoading(false);
    }
  };

  const onConfirmMigration = async () => {
    if (!conn || !selectedMigrationTarget) return;

    const action: MigrationAction = selectedMigrationTarget.version < conn.connector.version ? 'rollback' : 'migrate';
    setActionError(null);
    setActionLoading(true);
    setMigrationOpen(false);
    setMigrationStatus({
      action,
      state: 'starting',
      targetVersion: selectedMigrationTarget.version,
    });

    try {
      const response = await connections.migrateVersion(conn.id, {
        targetVersion: selectedMigrationTarget.version,
        timeoutSeconds: CONNECTION_MIGRATION_TIMEOUT_SECONDS,
      });
      setMigrationStatus({
        action,
        state: 'polling',
        targetVersion: selectedMigrationTarget.version,
        taskId: response.data.taskId,
      });

      const result = await tasks.pollForTaskFinalized(response.data.taskId, {
        initialDelay: 1000,
        maxDelay: 5000,
        maxAttempts: 140,
        backoffFactor: 1.4,
      });
      if (result.result !== PollForTaskResult.FINALIZED || result.taskInfo?.state !== TaskState.COMPLETED) {
        setMigrationStatus({
          action,
          state: 'failed',
          targetVersion: selectedMigrationTarget.version,
          taskId: response.data.taskId,
          task: result.taskInfo,
          message: result.taskInfo?.state === TaskState.FAILED
            ? 'Migration workflow failed before completing.'
            : 'Task polling ended before the migration completed.',
        });
        fetchConnection();
        return;
      }

      setMigrationStatus({
        action,
        state: 'completed',
        targetVersion: selectedMigrationTarget.version,
        taskId: response.data.taskId,
        task: result.taskInfo,
      });
      fetchConnection();
    } catch (err: any) {
      setMigrationStatus({
        action,
        state: 'failed',
        targetVersion: selectedMigrationTarget.version,
        message: err?.response?.data?.error || err.message || 'Failed to start connection migration.',
      });
    } finally {
      setActionLoading(false);
    }
  };

  const migrationActionLabel = migrationStatus?.action === 'rollback' ? 'Rollback' : 'Migration';
  const selectedMigrationActionLabel = selectedMigrationTarget && selectedMigrationTarget.version < conn.connector.version
    ? 'Rollback'
    : 'Migrate';

  return (
    <Stack spacing={2} sx={{p: 2}}>
      <Stack direction="row" justifyContent="space-between" alignItems="center">
        <Typography variant="h5">Connection</Typography>
        <Stack direction="row" spacing={1} alignItems="center">
          <HealthChip health={conn.healthState}/>
          <StateChip state={conn.state}/>
          <IconButton aria-label="actions" onClick={openMenu} size="small">
            <MoreVertIcon/>
          </IconButton>
          <Menu anchorEl={menuAnchorEl} open={Boolean(menuAnchorEl)} onClose={closeMenu}>
            <MenuItem onClick={onClickDisconnect} disabled={!canBeDisconnected(conn) || actionLoading || migrationInProgress}>Disconnect</MenuItem>
            <MenuItem onClick={onClickMigration} disabled={actionLoading || migrationInProgress}>Change version…</MenuItem>
            <Divider/>
            <MenuItem onClick={onClickForceState} disabled={actionLoading || migrationInProgress}>Force state…</MenuItem>
          </Menu>
        </Stack>
      </Stack>

      <ResourceNameEditor
        name={conn.name}
        resourceType="Connection"
        onRename={async (name) => {
          const response = await connections.update(conn.id, {name});
          setConn(response.data);
        }}
      />

      {actionError && <Alert severity="error">{actionError}</Alert>}

      {migrationStatus && (
        <Alert
          severity={
            migrationStatus.state === 'completed'
              ? 'success'
              : migrationStatus.state === 'failed'
                ? 'error'
                : 'info'
          }
        >
          {migrationStatus.state === 'starting' && `${migrationActionLabel} to v${migrationStatus.targetVersion} is starting...`}
          {migrationStatus.state === 'polling' && `${migrationActionLabel} to v${migrationStatus.targetVersion} is running.`}
          {migrationStatus.state === 'completed' && `${migrationActionLabel} to v${migrationStatus.targetVersion} completed.`}
          {migrationStatus.state === 'failed' && (migrationStatus.message || `${migrationActionLabel} failed.`)}
          {migrationStatus.taskId && (
            <Typography component="div" variant="caption" sx={{mt: 0.5, wordBreak: 'break-all'}}>
              Task: {migrationStatus.taskId}
              {migrationStatus.task?.state ? ` (${migrationStatus.task.state})` : ''}
            </Typography>
          )}
        </Alert>
      )}

      {conn.healthState === ConnectionHealthState.UNHEALTHY && (
        <Alert severity="warning">
          This connection requires re-authentication before it can be used.
        </Alert>
      )}
      {conn.healthState !== ConnectionHealthState.UNHEALTHY && conn.setupStepId && (
        <Alert severity="warning">
          This connection requires setup at {conn.setupStepId} before it can be used.
        </Alert>
      )}

      <Box>
        <Typography variant="subtitle2" color="text.secondary">Connection ID</Typography>
        <Stack direction="row" spacing={1} alignItems="center" sx={{mt: 0.5}}>
          <Typography
            variant="body1"
            component="code"
            sx={{
              wordBreak: 'break-all',
              fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Roboto Mono", monospace',
              bgcolor: 'action.hover',
              px: 1,
              py: 0.5,
              borderRadius: 0.5,
              fontSize: '0.9rem',
              letterSpacing: '0.02em',
            }}
          >
            {conn.id}
          </Typography>
          <Tooltip title={copied ? 'Copied!' : 'Copy'} placement="top">
            <IconButton size="small" aria-label="Copy connection id" onClick={handleCopyId}>
              <ContentCopyIcon fontSize="inherit" />
            </IconButton>
          </Tooltip>
        </Stack>
      </Box>

      <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Created</Typography>
          <Typography variant="body1">{dayjs(conn.createdAt).format('MMM DD, YYYY, h:mm A')}</Typography>
        </Box>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Updated</Typography>
          <Typography variant="body1">{dayjs(conn.updatedAt).format('MMM DD, YYYY, h:mm A')}</Typography>
        </Box>
      </Stack>

      <Box>
        <Typography variant="h6" sx={{mt: 1}}>Connector</Typography>
        <Stack direction={{xs: 'column', sm: 'row'}} spacing={4} sx={{mt: 1}}>
          <Box>
            <Typography variant="subtitle2" color="text.secondary">Name</Typography>
            <Typography variant="body1">
              <Link to={`/connectors/${conn.connector.id}/versions/${conn.connector.version}`} style={{color: 'inherit', textDecoration: 'none'}}>
                {conn.connector.name}
              </Link>
            </Typography>
          </Box>
          <Box>
            <Typography variant="subtitle2" color="text.secondary">ID</Typography>
              <Typography variant="body1" sx={{wordBreak: 'break-all'}}>
                  <Link to={`/connectors/${conn.connector.id}/versions/${conn.connector.version}`} style={{color: 'inherit', textDecoration: 'none'}}>
                      {conn.connector.id}
              </Link>
            </Typography>
          </Box>
          <Box>
            <Typography variant="subtitle2" color="text.secondary">Labels</Typography>
            {conn.connector.labels && Object.keys(conn.connector.labels).length > 0 ? (
              <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{ mt: 0.5 }}>
                {Object.entries(conn.connector.labels).map(([key, value]) => (
                  <Chip key={key} label={`${key}: ${value}`} size="small" variant="outlined" />
                ))}
              </Stack>
            ) : (
              <Typography variant="body2" color="text.secondary">No labels</Typography>
            )}
          </Box>
          <Box>
            <Typography variant="subtitle2" color="text.secondary">Version</Typography>
            <Typography variant="body1">
                <Link to={`/connectors/${conn.connector.id}/versions/${conn.connector.version}`} style={{color: 'inherit', textDecoration: 'none'}}>
                    {conn.connector.version}
                </Link>
            </Typography>
          </Box>
        </Stack>
      </Box>

      <Box>
        <Typography variant="subtitle2" color="text.secondary">Labels</Typography>
        {conn.labels && Object.keys(conn.labels).length > 0 ? (
          <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{mt: 0.5}}>
            {Object.entries(conn.labels).map(([key, value]) => (
              <Chip key={key} label={`${key}: ${value}`} size="small" variant="outlined"/>
            ))}
          </Stack>
        ) : (
          <Typography variant="body2" color="text.secondary">No labels</Typography>
        )}
      </Box>

      <AnnotationsEditor
        annotations={conn.annotations}
        onPut={async (key, value) => {
          await connections.putAnnotation(conn.id, key, value);
          fetchConnection();
        }}
        onDelete={async (key) => {
          await connections.deleteAnnotation(conn.id, key);
          fetchConnection();
        }}
      />

      {/* Disconnect confirmation dialog */}
      <Dialog open={confirmDisconnectOpen} onClose={() => !actionLoading && setConfirmDisconnectOpen(false)}>
        <DialogTitle>Disconnect connection</DialogTitle>
        <DialogContent>
          <Typography variant="body2">
            Are you sure you want to disconnect this connection? You may need to reconnect to use it again.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmDisconnectOpen(false)} disabled={actionLoading}>Cancel</Button>
          <Button onClick={onConfirmDisconnect} color="error" variant="contained" disabled={actionLoading}>Disconnect</Button>
        </DialogActions>
      </Dialog>

      {/* Force state dialog */}
      <Dialog open={forceStateOpen} onClose={() => !actionLoading && setForceStateOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>Force connection state</DialogTitle>
        <DialogContent>
          <FormControl fullWidth sx={{mt: 2}}>
            <InputLabel id="force-state-label">State</InputLabel>
            <Select
              native
              labelId="force-state-label"
              label="State"
              value={selectedState || ''}
              onChange={(e) => setSelectedState((e.target as HTMLSelectElement).value as ConnectionState)}
            >
              <option aria-label="None" value="" />
              {stateOptions.map(s => (
                <option key={s} value={s}>{s}</option>
              ))}
            </Select>
            <FormHelperText>Select the state to force for this connection.</FormHelperText>
          </FormControl>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setForceStateOpen(false)} disabled={actionLoading}>Cancel</Button>
          <Button onClick={onSubmitForceState} variant="contained" disabled={!selectedState || actionLoading}>Apply</Button>
        </DialogActions>
      </Dialog>

      <Dialog open={migrationOpen} onClose={() => !actionLoading && setMigrationOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>Change connection version</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary">
            Move this connection from v{conn.connector.version} to an eligible version of the same connector.
          </Typography>
          {migrationVersionsError && <Alert severity="error" sx={{mt: 2}}>{migrationVersionsError}</Alert>}
          {migrationVersionsLoading && (
            <Box sx={{display: 'flex', justifyContent: 'center', py: 3}}>
              <CircularProgress size={24}/>
            </Box>
          )}
          {!migrationVersionsLoading && !migrationVersionsError && (
            eligibleMigrationVersions.length === 0 ? (
              <Alert severity="info" sx={{mt: 2}}>
                No other active or primary versions are available.
              </Alert>
            ) : (
              <FormControl fullWidth sx={{mt: 2}}>
                <InputLabel id="target-version-label" htmlFor="target-version">Target version</InputLabel>
                <Select
                  native
                  inputProps={{
                    id: 'target-version',
                    'aria-labelledby': 'target-version-label',
                  }}
                  labelId="target-version-label"
                  label="Target version"
                  value={selectedMigrationVersion}
                  onChange={(event) => {
                    const value = String(event.target.value);
                    setSelectedMigrationVersion(value === '' ? '' : Number(value));
                  }}
                >
                  <option aria-label="None" value="" />
                  {eligibleMigrationVersions.map((version) => (
                    <option key={version.version} value={version.version}>
                      v{version.version} ({version.state})
                    </option>
                  ))}
                </Select>
                <FormHelperText>
                  {selectedMigrationActionLabel === 'Rollback'
                    ? 'An earlier target version rolls this connection back.'
                    : 'The migration can require setup or re-authentication after it completes.'}
                </FormHelperText>
              </FormControl>
            )
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setMigrationOpen(false)} disabled={actionLoading}>
            {eligibleMigrationVersions.length === 0 && !migrationVersionsLoading && !migrationVersionsError ? 'Close' : 'Cancel'}
          </Button>
          {eligibleMigrationVersions.length > 0 && (
            <Button
              variant="contained"
              startIcon={actionLoading ? <CircularProgress size={16}/> : <SwapHorizIcon/>}
              disabled={!selectedMigrationTarget || migrationVersionsLoading || actionLoading}
              onClick={() => void onConfirmMigration()}
            >
              {selectedMigrationTarget ? `${selectedMigrationActionLabel} to v${selectedMigrationTarget.version}` : 'Change version'}
            </Button>
          )}
        </DialogActions>
      </Dialog>
    </Stack>
  );
}
