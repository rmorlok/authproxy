import React, {useEffect, useState} from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import Stack from '@mui/material/Stack';
import Chip from '@mui/material/Chip';
import Avatar from '@mui/material/Avatar';
import dayjs from 'dayjs';
import {Connector, ConnectorVersionState, connectors} from '../api';

function StateChip({state}: { state: ConnectorVersionState }) {
  const colors: Record<ConnectorVersionState, "default" | "success" | "error" | "info" | "warning" | "primary" | "secondary"> = {
    [ConnectorVersionState.DRAFT]: 'secondary',
    [ConnectorVersionState.PRIMARY]: 'primary',
    [ConnectorVersionState.ACTIVE]: 'info',
    [ConnectorVersionState.ARCHIVED]: 'default',
  };
  return <Chip label={state} color={colors[state]} size="small"/>;
}

export default function ConnectorDetail({connectorId}: { connectorId: string }) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [conn, setConn] = useState<Connector | null>(null);

  useEffect(() => {
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

  if (loading) return (<Box sx={{display: 'flex', justifyContent: 'center', p: 4}}><CircularProgress/></Box>);
  if (error) return (<Alert severity="error">{error}</Alert>);
  if (!conn) return null;

  return (
    <Stack spacing={2} sx={{p: 2}}>
      <Stack direction="row" spacing={2} alignItems="center">
        {conn.logo && <Avatar alt={conn.display_name} src={conn.logo} sx={{width: 40, height: 40}} />}
        <Typography variant="h5">{conn.display_name || conn.type}</Typography>
        <StateChip state={conn.state}/>
      </Stack>

      {conn.description && (
        <Typography variant="body1" color="text.secondary">{conn.description}</Typography>
      )}

      {conn.highlight && (
        <Alert severity="info">{conn.highlight}</Alert>
      )}

      <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Connector ID</Typography>
          <Typography variant="body1" sx={{wordBreak: 'break-all'}}>{conn.id}</Typography>
        </Box>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Type</Typography>
          <Typography variant="body1">{conn.type}</Typography>
        </Box>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Version</Typography>
          <Typography variant="body1">{conn.version}</Typography>
        </Box>
      </Stack>

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

      <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Created</Typography>
          <Typography variant="body1">{dayjs((conn as any).created_at).format('MMM DD, YYYY, h:mm A')}</Typography>
        </Box>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Updated</Typography>
          <Typography variant="body1">{dayjs((conn as any).updated_at).format('MMM DD, YYYY, h:mm A')}</Typography>
        </Box>
      </Stack>
    </Stack>
  );
}
