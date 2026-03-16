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
import {EncryptionKey, encryptionKeys, EncryptionKeyState} from '@authproxy/api';
import { useNavigate } from "react-router-dom";

function StateChip({state}: { state: EncryptionKeyState }) {
  const colors: Record<EncryptionKeyState, "default" | "success" | "error" | "info" | "warning" | "primary" | "secondary"> = {
    [EncryptionKeyState.ACTIVE]: 'success',
    [EncryptionKeyState.DISABLED]: 'default',
  };
  return <Chip label={state} color={colors[state]} size="small"/>;
}

export default function EncryptionKeyDetail({encryptionKeyId}: { encryptionKeyId: string }) {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [ek, setEk] = useState<EncryptionKey | null>(null);

  // Actions UI state
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const [changeStateOpen, setChangeStateOpen] = useState(false);
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false);
  const [selectedState, setSelectedState] = useState<EncryptionKeyState | ''>('');
  const [actionLoading, setActionLoading] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  // Copy-to-clipboard
  const [copied, setCopied] = useState(false);
  const handleCopyId = async () => {
    try {
      await navigator.clipboard.writeText(ek?.id || '');
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch (_e: any) {
      // ignore
    }
  };

  const stateOptions = useMemo(() => Object.values(EncryptionKeyState), []);

  const fetchKey = () => {
    setLoading(true);
    setError(null);
    encryptionKeys.get(encryptionKeyId)
      .then(res => {
        setEk(res.data);
      })
      .catch(err => {
        const msg = err?.response?.data?.error || err.message || 'Failed to load encryption key';
        setError(msg);
      })
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    encryptionKeys.get(encryptionKeyId)
      .then(res => {
        if (cancelled) return;
        setEk(res.data);
      })
      .catch(err => {
        if (cancelled) return;
        const msg = err?.response?.data?.error || err.message || 'Failed to load encryption key';
        setError(msg);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [encryptionKeyId]);

  if (loading) return (<Box sx={{display: 'flex', justifyContent: 'center', p: 4}}><CircularProgress/></Box>);
  if (error) return (<Alert severity="error">{error}</Alert>);
  if (!ek) return null;

  const openMenu = (e: React.MouseEvent<HTMLButtonElement>) => setMenuAnchorEl(e.currentTarget);
  const closeMenu = () => setMenuAnchorEl(null);

  const onClickChangeState = () => {
    setActionError(null);
    setSelectedState(ek.state);
    closeMenu();
    setChangeStateOpen(true);
  };

  const onSubmitChangeState = async () => {
    if (!ek || !selectedState) return;
    setActionError(null);
    setActionLoading(true);
    try {
      await encryptionKeys.update(ek.id, { state: selectedState as EncryptionKeyState });
      setChangeStateOpen(false);
      fetchKey();
    } catch (err: any) {
      const msg = err?.response?.data?.error || err.message || 'Failed to change state';
      setActionError(msg);
    } finally {
      setActionLoading(false);
    }
  };

  const onClickDelete = () => {
    setActionError(null);
    closeMenu();
    setConfirmDeleteOpen(true);
  };

  const onConfirmDelete = async () => {
    if (!ek) return;
    setActionError(null);
    setActionLoading(true);
    try {
      await encryptionKeys.delete(ek.id);
      setConfirmDeleteOpen(false);
      navigate('/encryption-keys');
    } catch (err: any) {
      const msg = err?.response?.data?.error || err.message || 'Failed to delete encryption key';
      setActionError(msg);
    } finally {
      setActionLoading(false);
    }
  };

  return (
    <Stack spacing={2} sx={{p: 2}}>
      <Stack direction="row" justifyContent="space-between" alignItems="center">
        <Typography variant="h5">Encryption Key</Typography>
        <Stack direction="row" spacing={1} alignItems="center">
          <StateChip state={ek.state}/>
          <IconButton aria-label="actions" onClick={openMenu} size="small">
            <MoreVertIcon/>
          </IconButton>
          <Menu anchorEl={menuAnchorEl} open={Boolean(menuAnchorEl)} onClose={closeMenu}>
            <MenuItem onClick={onClickChangeState}>Change state...</MenuItem>
            <Divider/>
            <MenuItem onClick={onClickDelete} sx={{color: 'error.main'}}>Delete</MenuItem>
          </Menu>
        </Stack>
      </Stack>

      {actionError && <Alert severity="error">{actionError}</Alert>}

      <Box>
        <Typography variant="subtitle2" color="text.secondary">ID</Typography>
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
            {ek.id}
          </Typography>
          <Tooltip title={copied ? 'Copied!' : 'Copy'} placement="top">
            <IconButton size="small" aria-label="Copy encryption key id" onClick={handleCopyId}>
              <ContentCopyIcon fontSize="inherit" />
            </IconButton>
          </Tooltip>
        </Stack>
      </Box>

      <Box>
        <Typography variant="subtitle2" color="text.secondary">Namespace</Typography>
        <Typography variant="body1">{ek.namespace}</Typography>
      </Box>

      <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Created</Typography>
          <Typography variant="body1">{dayjs(ek.created_at).format('MMM DD, YYYY, h:mm A')}</Typography>
        </Box>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Updated</Typography>
          <Typography variant="body1">{dayjs(ek.updated_at).format('MMM DD, YYYY, h:mm A')}</Typography>
        </Box>
      </Stack>

      <Box>
        <Typography variant="subtitle2" color="text.secondary">Labels</Typography>
        {ek.labels && Object.keys(ek.labels).length > 0 ? (
          <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{ mt: 0.5 }}>
            {Object.entries(ek.labels).map(([key, value]) => (
              <Chip key={key} label={`${key}: ${value}`} size="small" variant="outlined" />
            ))}
          </Stack>
        ) : (
          <Typography variant="body2" color="text.secondary">No labels</Typography>
        )}
      </Box>

      {/* Change state dialog */}
      <Dialog open={changeStateOpen} onClose={() => !actionLoading && setChangeStateOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>Change encryption key state</DialogTitle>
        <DialogContent>
          <FormControl fullWidth sx={{mt: 2}}>
            <InputLabel id="change-state-label">State</InputLabel>
            <Select
              native
              labelId="change-state-label"
              label="State"
              value={selectedState || ''}
              onChange={(e) => setSelectedState((e.target as HTMLSelectElement).value as EncryptionKeyState)}
            >
              <option aria-label="None" value="" />
              {stateOptions.map(s => (
                <option key={s} value={s}>{s}</option>
              ))}
            </Select>
            <FormHelperText>Select the new state for this encryption key.</FormHelperText>
          </FormControl>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setChangeStateOpen(false)} disabled={actionLoading}>Cancel</Button>
          <Button onClick={onSubmitChangeState} variant="contained" disabled={!selectedState || actionLoading}>Apply</Button>
        </DialogActions>
      </Dialog>

      {/* Delete confirmation dialog */}
      <Dialog open={confirmDeleteOpen} onClose={() => !actionLoading && setConfirmDeleteOpen(false)}>
        <DialogTitle>Delete encryption key</DialogTitle>
        <DialogContent>
          <Typography variant="body2">
            Are you sure you want to delete this encryption key? This action cannot be undone.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmDeleteOpen(false)} disabled={actionLoading}>Cancel</Button>
          <Button onClick={onConfirmDelete} color="error" variant="contained" disabled={actionLoading}>Delete</Button>
        </DialogActions>
      </Dialog>
    </Stack>
  );
}
