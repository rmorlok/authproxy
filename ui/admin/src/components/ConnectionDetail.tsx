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
import dayjs from 'dayjs';
import {
  Connection,
  connections,
  ConnectionState,
  ConnectionHealthState,
  Connector,
  ConnectorReleaseState,
  connectors,
  API_VERSION,
  CONNECTION_KIND,
  PollForTaskResult,
  Task,
  tasks,
  TaskState,
  canBeDisconnected,
} from '@authproxy/api';
import WarningAmberIcon from '@mui/icons-material/WarningAmber';
import SwapHorizIcon from '@mui/icons-material/SwapHoriz';
import { Link } from "react-router-dom";
import ResourceIdentifier from './ResourceIdentifier';
import ResourceMetadataMenuItems from './ResourceMetadataMenuItems';
import AnnotationsEditor from "./AnnotationsEditor";
import ResourceNameEditor from './ResourceNameEditor';
import {ResourceLabels, ResourceNamespace} from './ResourceMetadataFields';

const CONNECTION_MIGRATION_TIMEOUT_SECONDS = 600;

type MigrationAction = 'migrate' | 'rollback';

interface MigrationStatus {
  action: MigrationAction;
  state: 'starting' | 'polling' | 'completed' | 'failed';
  targetGeneration: number;
  taskId?: string;
  task?: Task;
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
  const [connector, setConnector] = useState<Connector | null>(null);

  // Actions UI state
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const [confirmDisconnectOpen, setConfirmDisconnectOpen] = useState(false);
  const [forceStateOpen, setForceStateOpen] = useState(false);
  const [selectedState, setSelectedState] = useState<ConnectionState | ''>('');
  const [actionLoading, setActionLoading] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [migrationOpen, setMigrationOpen] = useState(false);
  const [migrationGenerations, setMigrationGenerations] = useState<Connector[]>([]);
  const [migrationGenerationsLoading, setMigrationGenerationsLoading] = useState(false);
  const [migrationGenerationsError, setMigrationGenerationsError] = useState<string | null>(null);
  const [selectedMigrationGeneration, setSelectedMigrationGeneration] = useState<number | ''>('');
  const [migrationStatus, setMigrationStatus] = useState<MigrationStatus | null>(null);

  const stateOptions = useMemo(() => Object.values(ConnectionState), []);
  const eligibleMigrationGenerations = useMemo(() => {
    if (!conn) {
      return [];
    }
    return migrationGenerations.filter((generation) =>
      generation.metadata.generation !== conn.spec.connectorRef.generation &&
      (generation.status.release.state === ConnectorReleaseState.PRIMARY ||
        generation.status.release.state === ConnectorReleaseState.ACTIVE),
    );
  }, [conn, migrationGenerations]);
  const selectedMigrationTarget = useMemo(
    () => eligibleMigrationGenerations.find((generation) => generation.metadata.generation === selectedMigrationGeneration),
    [eligibleMigrationGenerations, selectedMigrationGeneration],
  );
  const migrationInProgress = migrationStatus?.state === 'starting' || migrationStatus?.state === 'polling';

  const fetchConnection = () => {
    setLoading(true);
    setError(null);
    connections.get(connectionId)
      .then(async res => {
        const connection = res.data;
        setConn(connection);
        const connectorId = connection.spec.connectorRef.id;
        if (!connectorId) {
          setConnector(null);
          return;
        }
        try {
          const connectorResponse = await connectors.getGeneration(
            connectorId,
            connection.spec.connectorRef.generation,
          );
          setConnector(connectorResponse.data);
        } catch {
          setConnector(null);
        }
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
      .then(async res => {
        if (cancelled) return;
        const connection = res.data;
        setConn(connection);
        const connectorId = connection.spec.connectorRef.id;
        if (!connectorId) {
          setConnector(null);
          return;
        }
        try {
          const connectorResponse = await connectors.getGeneration(
            connectorId,
            connection.spec.connectorRef.generation,
          );
          if (!cancelled) setConnector(connectorResponse.data);
        } catch {
          if (!cancelled) setConnector(null);
        }
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
      await connections.disconnect(conn.metadata.id);
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
    setSelectedState(conn.status.lifecycle.state);
    closeMenu();
    setForceStateOpen(true);
  };

  const onSubmitForceState = async () => {
    if (!conn || !selectedState) return;
    setActionError(null);
    setActionLoading(true);
    try {
      await connections.forceState(conn.metadata.id, selectedState as ConnectionState);
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
    setMigrationGenerations([]);
    setMigrationGenerationsError(null);
    setSelectedMigrationGeneration('');
    closeMenu();
    setMigrationOpen(true);
    setMigrationGenerationsLoading(true);

    try {
      const connectorId = conn.spec.connectorRef.id;
      if (!connectorId) {
        throw new Error('Connection connector reference does not contain an id');
      }
      const response = await connectors.listGenerations(connectorId, {
        limit: 100,
        orderBy: 'generation desc',
      });
      const eligible = response.data.items.filter((generation) =>
        generation.metadata.generation !== conn.spec.connectorRef.generation &&
        (generation.status.release.state === ConnectorReleaseState.PRIMARY ||
          generation.status.release.state === ConnectorReleaseState.ACTIVE),
      );
      setMigrationGenerations(response.data.items);
      setSelectedMigrationGeneration(
        eligible.find((generation) => generation.status.release.state === ConnectorReleaseState.PRIMARY)?.metadata.generation ??
        eligible[0]?.metadata.generation ??
        '',
      );
    } catch (err: any) {
      setMigrationGenerationsError(err?.response?.data?.error || err.message || 'Failed to load connector generations');
    } finally {
      setMigrationGenerationsLoading(false);
    }
  };

  const onConfirmMigration = async () => {
    if (!conn || !selectedMigrationTarget) return;

    const targetGeneration = selectedMigrationTarget.metadata.generation;
    const action: MigrationAction = targetGeneration < conn.spec.connectorRef.generation ? 'rollback' : 'migrate';
    setActionError(null);
    setActionLoading(true);
    setMigrationOpen(false);
    setMigrationStatus({
      action,
      state: 'starting',
      targetGeneration: targetGeneration,
    });

    try {
      const response = await connections.migrateGeneration(conn.metadata.id, {
        connectorRef: {
          ...conn.spec.connectorRef,
          generation: targetGeneration,
        },
        timeoutSeconds: CONNECTION_MIGRATION_TIMEOUT_SECONDS,
      });
      setMigrationStatus({
        action,
        state: 'polling',
        targetGeneration: targetGeneration,
        taskId: response.data.status.taskId,
      });

      const result = await tasks.pollForTaskFinalized(response.data.status.taskId, {
        initialDelay: 1000,
        maxDelay: 5000,
        maxAttempts: 140,
        backoffFactor: 1.4,
      });
      if (result.result !== PollForTaskResult.FINALIZED || result.task?.status.state !== TaskState.COMPLETED) {
        setMigrationStatus({
          action,
          state: 'failed',
          targetGeneration: targetGeneration,
          taskId: response.data.status.taskId,
          task: result.task,
          message: result.task?.status.state === TaskState.FAILED
            ? 'Migration workflow failed before completing.'
            : 'Task polling ended before the migration completed.',
        });
        fetchConnection();
        return;
      }

      setMigrationStatus({
        action,
        state: 'completed',
        targetGeneration: targetGeneration,
        taskId: response.data.status.taskId,
        task: result.task,
      });
      fetchConnection();
    } catch (err: any) {
      setMigrationStatus({
        action,
        state: 'failed',
        targetGeneration: targetGeneration,
        message: err?.response?.data?.error || err.message || 'Failed to start connection migration.',
      });
    } finally {
      setActionLoading(false);
    }
  };

  const migrationActionLabel = migrationStatus?.action === 'rollback' ? 'Rollback' : 'Migration';
  const selectedMigrationActionLabel = selectedMigrationTarget &&
    selectedMigrationTarget.metadata.generation < conn.spec.connectorRef.generation
    ? 'Rollback'
    : 'Migrate';

  return (
    <Stack spacing={2} sx={{p: 2}}>
      <Stack direction="row" justifyContent="space-between" alignItems="center">
        <Typography variant="h5">Connection</Typography>
        <Stack direction="row" spacing={1} alignItems="center">
          <HealthChip health={conn.status.health.state}/>
          <StateChip state={conn.status.lifecycle.state}/>
          <IconButton aria-label="actions" onClick={openMenu} size="small">
            <MoreVertIcon/>
          </IconButton>
          <Menu anchorEl={menuAnchorEl} open={Boolean(menuAnchorEl)} onClose={closeMenu} keepMounted>
            <ResourceMetadataMenuItems
              resource="connection"
              name={conn.metadata.name}
              labels={conn.metadata.labels}
              annotations={conn.metadata.annotations}
              onCloseMenu={closeMenu}
              includeRename={false}
              onUpdateLabels={async (labels) => {
                const response = await connections.update(conn.metadata.id, {
                  apiVersion: API_VERSION,
                  kind: CONNECTION_KIND,
                  metadata: {labels},
                  spec: {},
                });
                setConn(response.data);
              }}
              onUpdateAnnotations={async (annotations) => {
                const response = await connections.update(conn.metadata.id, {
                  apiVersion: API_VERSION,
                  kind: CONNECTION_KIND,
                  metadata: {annotations},
                  spec: {},
                });
                setConn(response.data);
              }}
              disabled={actionLoading || migrationInProgress}
            />
            <Divider/>
            <MenuItem onClick={onClickDisconnect} disabled={!canBeDisconnected(conn) || actionLoading || migrationInProgress}>Disconnect</MenuItem>
            <MenuItem onClick={onClickMigration} disabled={actionLoading || migrationInProgress}>Change generation…</MenuItem>
            <Divider/>
            <MenuItem onClick={onClickForceState} disabled={actionLoading || migrationInProgress}>Force state…</MenuItem>
          </Menu>
        </Stack>
      </Stack>

      <ResourceNameEditor
        name={conn.metadata.name}
        resourceType="Connection"
        onRename={async (name) => {
          const response = await connections.update(conn.metadata.id, {
            apiVersion: API_VERSION,
            kind: CONNECTION_KIND,
            metadata: {name},
            spec: {},
          });
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
          {migrationStatus.state === 'starting' && `${migrationActionLabel} to v${migrationStatus.targetGeneration} is starting...`}
          {migrationStatus.state === 'polling' && `${migrationActionLabel} to v${migrationStatus.targetGeneration} is running.`}
          {migrationStatus.state === 'completed' && `${migrationActionLabel} to v${migrationStatus.targetGeneration} completed.`}
          {migrationStatus.state === 'failed' && (migrationStatus.message || `${migrationActionLabel} failed.`)}
          {migrationStatus.taskId && (
            <Typography component="div" variant="caption" sx={{mt: 0.5, wordBreak: 'break-all'}}>
              Task: {migrationStatus.taskId}
              {migrationStatus.task?.status.state ? ` (${migrationStatus.task.status.state})` : ''}
            </Typography>
          )}
        </Alert>
      )}

      {conn.status.health.state === ConnectionHealthState.UNHEALTHY && (
        <Alert severity="warning">
          This connection requires re-authentication before it can be used.
        </Alert>
      )}
      {conn.status.health.state !== ConnectionHealthState.UNHEALTHY && conn.status.setup?.stepId && (
        <Alert severity="warning">
          This connection requires setup at {conn.status.setup.stepId} before it can be used.
        </Alert>
      )}

      <ResourceIdentifier value={conn.metadata.id} copyLabel="Copy connection id"/>

      <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
        <ResourceNamespace namespace={conn.metadata.namespace}/>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Created</Typography>
          <Typography variant="body1">{dayjs(conn.metadata.createdAt).format('MMM DD, YYYY, h:mm A')}</Typography>
        </Box>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Updated</Typography>
          <Typography variant="body1">{dayjs(conn.metadata.updatedAt).format('MMM DD, YYYY, h:mm A')}</Typography>
        </Box>
      </Stack>

      <Box>
        <Typography variant="h6" sx={{mt: 1}}>Connector</Typography>
        <Stack direction={{xs: 'column', sm: 'row'}} spacing={4} sx={{mt: 1}}>
          <Box>
            <Typography variant="subtitle2" color="text.secondary">Name</Typography>
            <Typography variant="body1">
              <Link to={`/connectors/${conn.spec.connectorRef.id}/generations/${conn.spec.connectorRef.generation}`} style={{color: 'inherit', textDecoration: 'none'}}>
                {connector?.metadata.name || conn.spec.connectorRef.name || conn.spec.connectorRef.id}
              </Link>
            </Typography>
          </Box>
          <Box>
            <Typography variant="subtitle2" color="text.secondary">ID</Typography>
              <Typography variant="body1" sx={{wordBreak: 'break-all'}}>
                  <Link to={`/connectors/${conn.spec.connectorRef.id}/generations/${conn.spec.connectorRef.generation}`} style={{color: 'inherit', textDecoration: 'none'}}>
                      {conn.spec.connectorRef.id || '—'}
              </Link>
            </Typography>
          </Box>
          <ResourceLabels labels={connector?.metadata.labels}/>
          <Box>
            <Typography variant="subtitle2" color="text.secondary">Generation</Typography>
            <Typography variant="body1">
                <Link to={`/connectors/${conn.spec.connectorRef.id}/generations/${conn.spec.connectorRef.generation}`} style={{color: 'inherit', textDecoration: 'none'}}>
                    {conn.spec.connectorRef.generation}
                </Link>
            </Typography>
          </Box>
        </Stack>
      </Box>

      <ResourceLabels labels={conn.metadata.labels}/>

      <AnnotationsEditor annotations={conn.metadata.annotations} readOnly onPut={async () => {}} onDelete={async () => {}}/>

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
        <DialogTitle>Change connection generation</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary">
            Move this connection from v{conn.spec.connectorRef.generation} to an eligible generation of the same connector.
          </Typography>
          {migrationGenerationsError && <Alert severity="error" sx={{mt: 2}}>{migrationGenerationsError}</Alert>}
          {migrationGenerationsLoading && (
            <Box sx={{display: 'flex', justifyContent: 'center', py: 3}}>
              <CircularProgress size={24}/>
            </Box>
          )}
          {!migrationGenerationsLoading && !migrationGenerationsError && (
            eligibleMigrationGenerations.length === 0 ? (
              <Alert severity="info" sx={{mt: 2}}>
                No other active or primary generations are available.
              </Alert>
            ) : (
              <FormControl fullWidth sx={{mt: 2}}>
                <InputLabel id="target-generation-label" htmlFor="target-generation">Target generation</InputLabel>
                <Select
                  native
                  inputProps={{
                    id: 'target-generation',
                    'aria-labelledby': 'target-generation-label',
                  }}
                  labelId="target-generation-label"
                  label="Target generation"
                  value={selectedMigrationGeneration}
                  onChange={(event) => {
                    const value = String(event.target.value);
                    setSelectedMigrationGeneration(value === '' ? '' : Number(value));
                  }}
                >
                  <option aria-label="None" value="" />
                  {eligibleMigrationGenerations.map((generation) => (
                    <option key={generation.metadata.generation} value={generation.metadata.generation}>
                      v{generation.metadata.generation} ({generation.status.release.state})
                    </option>
                  ))}
                </Select>
                <FormHelperText>
                  {selectedMigrationActionLabel === 'Rollback'
                    ? 'An earlier target generation rolls this connection back.'
                    : 'The migration can require setup or re-authentication after it completes.'}
                </FormHelperText>
              </FormControl>
            )
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setMigrationOpen(false)} disabled={actionLoading}>
            {eligibleMigrationGenerations.length === 0 && !migrationGenerationsLoading && !migrationGenerationsError ? 'Close' : 'Cancel'}
          </Button>
          {eligibleMigrationGenerations.length > 0 && (
            <Button
              variant="contained"
              startIcon={actionLoading ? <CircularProgress size={16}/> : <SwapHorizIcon/>}
              disabled={!selectedMigrationTarget || migrationGenerationsLoading || actionLoading}
              onClick={() => void onConfirmMigration()}
            >
              {selectedMigrationTarget ? `${selectedMigrationActionLabel} to v${selectedMigrationTarget.metadata.generation}` : 'Change generation'}
            </Button>
          )}
        </DialogActions>
      </Dialog>
    </Stack>
  );
}
