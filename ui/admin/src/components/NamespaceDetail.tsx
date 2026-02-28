import React, {useEffect, useState} from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import Stack from '@mui/material/Stack';
import Chip from '@mui/material/Chip';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import ListSubheader from '@mui/material/ListSubheader';
import MenuItem from '@mui/material/MenuItem';
import {Link as RouterLink} from 'react-router-dom';
import Link from '@mui/material/Link';
import dayjs from 'dayjs';
import {
  Namespace,
  NamespaceState,
  namespaces,
  EncryptionKey,
  EncryptionKeyState,
  encryptionKeys,
  NAMESPACE_PATH_SEPARATOR,
} from '@authproxy/api';

function StateChip({state}: { state: NamespaceState }) {
  const colors: Record<string, "default" | "success" | "error" | "info" | "warning" | "primary" | "secondary"> = {
    [NamespaceState.ACTIVE]: 'success',
    [NamespaceState.DISCONNECTING]: 'warning',
    [NamespaceState.DISCONNECTED]: 'default',
  };
  return <Chip label={state} color={colors[state] || 'default'} size="small"/>;
}

function getStrictAncestorPaths(path: string): string[] {
  const parts = path.split(NAMESPACE_PATH_SEPARATOR);
  const prefixes: string[] = [];
  for (let i = 1; i < parts.length; i++) {
    prefixes.push(parts.slice(0, i).join(NAMESPACE_PATH_SEPARATOR));
  }
  return prefixes;
}

export default function NamespaceDetail({namespacePath}: { namespacePath: string }) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [ns, setNs] = useState<Namespace | null>(null);

  const [selectorOpen, setSelectorOpen] = useState(false);
  const [ancestorKeys, setAncestorKeys] = useState<EncryptionKey[]>([]);
  const [keysLoading, setKeysLoading] = useState(false);
  const [selectedKeyId, setSelectedKeyId] = useState<string>('');
  const [actionLoading, setActionLoading] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  const ancestorPaths = getStrictAncestorPaths(namespacePath);
  const isRoot = ancestorPaths.length === 0;

  const fetchNamespace = () => {
    setLoading(true);
    setError(null);
    namespaces.getByPath(namespacePath)
      .then(res => setNs(res.data))
      .catch(err => {
        const msg = err?.response?.data?.error || err.message || 'Failed to load namespace';
        setError(msg);
      })
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    namespaces.getByPath(namespacePath)
      .then(res => { if (!cancelled) setNs(res.data); })
      .catch(err => {
        if (cancelled) return;
        const msg = err?.response?.data?.error || err.message || 'Failed to load namespace';
        setError(msg);
      })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [namespacePath]);

  const openSelector = async () => {
    setActionError(null);
    setSelectorOpen(true);
    setKeysLoading(true);
    setSelectedKeyId('');

    try {
      const results = await Promise.all(
        ancestorPaths.map(p =>
          encryptionKeys.list({namespace: p, state: EncryptionKeyState.ACTIVE, limit: 100})
        )
      );
      const allKeys: EncryptionKey[] = [];
      for (const res of results) {
        if (res.data.items) {
          allKeys.push(...res.data.items);
        }
      }
      setAncestorKeys(allKeys);
    } catch (err: any) {
      const msg = err?.response?.data?.error || err.message || 'Failed to load encryption keys';
      setActionError(msg);
    } finally {
      setKeysLoading(false);
    }
  };

  const onSetKey = async () => {
    if (!selectedKeyId) return;
    setActionError(null);
    setActionLoading(true);
    try {
      await namespaces.setEncryptionKey(namespacePath, selectedKeyId);
      setSelectorOpen(false);
      fetchNamespace();
    } catch (err: any) {
      const msg = err?.response?.data?.error || err.message || 'Failed to set encryption key';
      setActionError(msg);
    } finally {
      setActionLoading(false);
    }
  };

  const onClearKey = async () => {
    setActionError(null);
    setActionLoading(true);
    try {
      await namespaces.clearEncryptionKey(namespacePath);
      fetchNamespace();
    } catch (err: any) {
      const msg = err?.response?.data?.error || err.message || 'Failed to clear encryption key';
      setActionError(msg);
    } finally {
      setActionLoading(false);
    }
  };

  if (loading) return (<Box sx={{display: 'flex', justifyContent: 'center', p: 4}}><CircularProgress/></Box>);
  if (error) return (<Alert severity="error">{error}</Alert>);
  if (!ns) return null;

  // Group keys by namespace for the selector
  const keysByNamespace: Record<string, EncryptionKey[]> = {};
  for (const ek of ancestorKeys) {
    if (!keysByNamespace[ek.namespace]) keysByNamespace[ek.namespace] = [];
    keysByNamespace[ek.namespace].push(ek);
  }

  return (
    <Stack spacing={2} sx={{p: 2}}>
      <Typography variant="h5">Namespace</Typography>

      {actionError && <Alert severity="error">{actionError}</Alert>}

      <Box>
        <Typography variant="subtitle2" color="text.secondary">Path</Typography>
        <Typography variant="body1" component="code" sx={{
          fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
          bgcolor: 'action.hover', px: 1, py: 0.5, borderRadius: 0.5, fontSize: '0.9rem',
        }}>
          {ns.path}
        </Typography>
      </Box>

      <Box>
        <Typography variant="subtitle2" color="text.secondary">State</Typography>
        <Box sx={{mt: 0.5}}><StateChip state={ns.state}/></Box>
      </Box>

      <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Created</Typography>
          <Typography variant="body1">{dayjs(ns.created_at).format('MMM DD, YYYY, h:mm A')}</Typography>
        </Box>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Updated</Typography>
          <Typography variant="body1">{dayjs(ns.updated_at).format('MMM DD, YYYY, h:mm A')}</Typography>
        </Box>
      </Stack>

      <Box>
        <Typography variant="subtitle2" color="text.secondary">Labels</Typography>
        {ns.labels && Object.keys(ns.labels).length > 0 ? (
          <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{mt: 0.5}}>
            {Object.entries(ns.labels).map(([key, value]) => (
              <Chip key={key} label={`${key}: ${value}`} size="small" variant="outlined"/>
            ))}
          </Stack>
        ) : (
          <Typography variant="body2" color="text.secondary">No labels</Typography>
        )}
      </Box>

      <Box>
        <Typography variant="subtitle2" color="text.secondary">Encryption Key</Typography>
        {ns.encryption_key_id ? (
          <Stack direction="row" spacing={1} alignItems="center" sx={{mt: 0.5}}>
            <Link component={RouterLink} to={`/encryption-keys/${ns.encryption_key_id}`}>
              <Typography variant="body1" component="code" sx={{
                fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                fontSize: '0.9rem',
              }}>
                {ns.encryption_key_id}
              </Typography>
            </Link>
            <Button size="small" onClick={openSelector} disabled={actionLoading || isRoot}>Change Key</Button>
            <Button size="small" color="warning" onClick={onClearKey} disabled={actionLoading}>Clear Key</Button>
          </Stack>
        ) : (
          <Stack direction="row" spacing={1} alignItems="center" sx={{mt: 0.5}}>
            <Typography variant="body2" color="text.secondary">None</Typography>
            {isRoot ? (
              <Typography variant="body2" color="text.secondary">(root namespace cannot have an encryption key other than the global key)</Typography>
            ) : (
              <Button size="small" onClick={openSelector} disabled={actionLoading}>Assign Key</Button>
            )}
          </Stack>
        )}
      </Box>

      {/* Encryption key selector dialog */}
      <Dialog open={selectorOpen} onClose={() => !actionLoading && setSelectorOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>Select Encryption Key</DialogTitle>
        <DialogContent>
          {keysLoading ? (
            <Box sx={{display: 'flex', justifyContent: 'center', p: 2}}><CircularProgress/></Box>
          ) : ancestorKeys.length === 0 ? (
            <Alert severity="info">No active encryption keys found in ancestor namespaces.</Alert>
          ) : (
            <FormControl fullWidth sx={{mt: 2}}>
              <InputLabel id="select-ek-label">Encryption Key</InputLabel>
              <Select
                labelId="select-ek-label"
                label="Encryption Key"
                value={selectedKeyId}
                onChange={(e) => setSelectedKeyId(e.target.value as string)}
              >
                {ancestorPaths.map(p => {
                  const keys = keysByNamespace[p];
                  if (!keys || keys.length === 0) return null;
                  return [
                    <ListSubheader key={`header-${p}`}>{p}</ListSubheader>,
                    ...keys.map(ek => (
                      <MenuItem key={ek.id} value={ek.id}>
                        {ek.id}
                      </MenuItem>
                    )),
                  ];
                })}
              </Select>
            </FormControl>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setSelectorOpen(false)} disabled={actionLoading}>Cancel</Button>
          <Button onClick={onSetKey} variant="contained" disabled={!selectedKeyId || actionLoading}>Assign</Button>
        </DialogActions>
      </Dialog>
    </Stack>
  );
}
