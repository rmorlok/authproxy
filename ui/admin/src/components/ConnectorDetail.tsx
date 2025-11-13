import React, {useEffect, useMemo, useState} from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import Stack from '@mui/material/Stack';
import Avatar from '@mui/material/Avatar';
import Button from '@mui/material/Button';
import Drawer from '@mui/material/Drawer';
import IconButton from '@mui/material/IconButton';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import CloseIcon from '@mui/icons-material/Close';
import dayjs from 'dayjs';
import {Connector, ConnectorVersionState, connectors, ListResponse, ConnectorVersion} from '@authproxy/api';
import YAML from 'yaml';
import {Link, useNavigate} from 'react-router-dom';
import {StateChip} from "./StateChip";
import ConnectorVersionDetail from "./ConnectorVersionDetail";

export default function ConnectorDetail({connectorId, initialVersion}: { connectorId: string, initialVersion?: number }) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [conn, setConn] = useState<Connector | null>(null);

  // versions state
  const [versions, setVersions] = useState<ConnectorVersion[]>([]);
  const [versionsError, setVersionsError] = useState<string | null>(null);
  const [drawerOpen, setDrawerOpen] = useState<boolean>(false);
  const [selectedVersion, setSelectedVersion] = useState<number | undefined>(initialVersion);
  const [viewMode, setViewMode] = useState<'json' | 'yaml'>('json');
  const navigate = useNavigate();

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

  // fetch versions
  useEffect(() => {
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
    </Stack>
  );
}
