import React, {useEffect, useState} from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import Stack from '@mui/material/Stack';
import Chip from '@mui/material/Chip';
import dayjs from 'dayjs';
import {Connection, connections, ConnectionState} from '../api';

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

  return (
    <Stack spacing={2} sx={{p: 2}}>
      <Stack direction="row" justifyContent="space-between" alignItems="center">
        <Typography variant="h5">Connection</Typography>
        <StateChip state={conn.state}/>
      </Stack>

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
            <Typography variant="body1" sx={{wordBreak: 'break-all'}}>{conn.connector.id}</Typography>
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
    </Stack>
  );
}
