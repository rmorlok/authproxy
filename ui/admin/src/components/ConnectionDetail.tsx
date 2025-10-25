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
import {Connection, connections, ConnectionState, canBeDisconnected} from '../api';
import { Link } from "react-router-dom";

function StateChip({state}: { state: ConnectionState }) {
  const colors: Record<ConnectionState, "default" | "success" | "error" | "info" | "warning" | "primary" | "secondary"> = {
    [ConnectionState.CREATED]: 'primary',
    [ConnectionState.CONNECTED]: 'success',
    [ConnectionState.FAILED]: 'error',
    [ConnectionState.DISCONNECTING]: 'warning',
    [ConnectionState.DISCONNECTED]: 'default',
  };
  return <Chip label={state} color={colors[state]} size="small"/>;
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

  const stateOptions = useMemo(() => Object.values(ConnectionState), []);

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
      await connections.force_state(conn.id, selectedState as ConnectionState);
      setForceStateOpen(false);
      fetchConnection();
    } catch (err: any) {
      const msg = err?.response?.data?.error || err.message || 'Failed to force state';
      setActionError(msg);
    } finally {
      setActionLoading(false);
    }
  };

  return (
    <Stack spacing={2} sx={{p: 2}}>
      <Stack direction="row" justifyContent="space-between" alignItems="center">
        <Typography variant="h5">Connection</Typography>
        <Stack direction="row" spacing={1} alignItems="center">
          <StateChip state={conn.state}/>
          <IconButton aria-label="actions" onClick={openMenu} size="small">
            <MoreVertIcon/>
          </IconButton>
          <Menu anchorEl={menuAnchorEl} open={Boolean(menuAnchorEl)} onClose={closeMenu}>
            <MenuItem onClick={onClickDisconnect} disabled={!canBeDisconnected(conn)}>Disconnect</MenuItem>
            <Divider/>
            <MenuItem onClick={onClickForceState}>Force state…</MenuItem>
          </Menu>
        </Stack>
      </Stack>

      {actionError && <Alert severity="error">{actionError}</Alert>}

      <Box>
        <Typography variant="subtitle2" color="text.secondary">Connection ID</Typography>
        <Typography variant="body1" sx={{wordBreak: 'break-all'}}>{conn.id}</Typography>
      </Box>

      <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Created</Typography>
          <Typography variant="body1">{dayjs(conn.created_at).format('MMM DD, YYYY, h:mm A')}</Typography>
        </Box>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Updated</Typography>
          <Typography variant="body1">{dayjs(conn.updated_at).format('MMM DD, YYYY, h:mm A')}</Typography>
        </Box>
      </Stack>

      <Box>
        <Typography variant="h6" sx={{mt: 1}}>Connector</Typography>
        <Stack direction={{xs: 'column', sm: 'row'}} spacing={4} sx={{mt: 1}}>
          <Box>
            <Typography variant="subtitle2" color="text.secondary">ID</Typography>
              <Typography variant="body1" sx={{wordBreak: 'break-all'}}>
                  <Link to={`/connectors/${conn.connector.id}`} style={{color: 'inherit', textDecoration: 'none'}}>
                      {conn.connector.id}
              </Link>
            </Typography>
          </Box>
          <Box>
            <Typography variant="subtitle2" color="text.secondary">Type</Typography>
            <Typography variant="body1">{conn.connector.type}</Typography>
          </Box>
          <Box>
            <Typography variant="subtitle2" color="text.secondary">Version</Typography>
            <Typography variant="body1">{conn.connector.version}</Typography>
          </Box>
        </Stack>
      </Box>

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
    </Stack>
  );
}
