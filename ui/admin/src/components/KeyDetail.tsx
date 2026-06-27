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
import TextField from '@mui/material/TextField';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import dayjs from 'dayjs';
import Tooltip from '@mui/material/Tooltip';
import {Key, keys, KeyState} from '@authproxy/api';
import { useNavigate } from "react-router-dom";
import AnnotationsEditor from "./AnnotationsEditor";

function StateChip({state}: { state: KeyState }) {
  const colors: Record<KeyState, "default" | "success" | "error" | "info" | "warning" | "primary" | "secondary"> = {
    [KeyState.ACTIVE]: 'success',
    [KeyState.DISABLED]: 'default',
  };
  return <Chip label={state} color={colors[state]} size="small"/>;
}

type KeyValueRow = {
  id: string;
  key: string;
  value: string;
};

let keyValueRowSequence = 0;

function nextKeyValueRow(key = '', value = ''): KeyValueRow {
  keyValueRowSequence += 1;
  return {
    id: `kv-${keyValueRowSequence}`,
    key,
    value,
  };
}

function mapToRows(values?: Record<string, string>): KeyValueRow[] {
  return Object.entries(values || {}).map(([key, value]) => nextKeyValueRow(key, value));
}

function rowsToMap(rows: KeyValueRow[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const row of rows) {
    const key = row.key.trim();
    if (key) out[key] = row.value;
  }
  return out;
}

function duplicateKeys(rows: KeyValueRow[]): string[] {
  const seen = new Set<string>();
  const duplicates = new Set<string>();
  for (const row of rows) {
    const key = row.key.trim();
    if (!key) continue;
    if (seen.has(key)) duplicates.add(key);
    seen.add(key);
  }
  return Array.from(duplicates);
}

function KeyValueRowsEditor({
  title,
  rows,
  onChange,
  addLabel,
}: {
  title: string;
  rows: KeyValueRow[];
  onChange: (rows: KeyValueRow[]) => void;
  addLabel: string;
}) {
  const updateRow = (id: string, patch: Partial<KeyValueRow>) => {
    onChange(rows.map(row => row.id === id ? {...row, ...patch} : row));
  };

  const removeRow = (id: string) => {
    onChange(rows.filter(row => row.id !== id));
  };

  return (
    <Box>
      <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{mb: 1}}>
        <Typography variant="subtitle2" color="text.secondary">{title}</Typography>
        <Button
          size="small"
          startIcon={<AddIcon/>}
          onClick={() => onChange([...rows, nextKeyValueRow()])}
        >
          {addLabel}
        </Button>
      </Stack>
      {rows.length === 0 ? (
        <Typography variant="body2" color="text.secondary">None</Typography>
      ) : (
        <Stack spacing={1}>
          {rows.map(row => (
            <Stack key={row.id} direction={{xs: 'column', sm: 'row'}} spacing={1} alignItems={{xs: 'stretch', sm: 'flex-start'}}>
              <TextField
                size="small"
                label="Key"
                value={row.key}
                onChange={(e) => updateRow(row.id, {key: e.target.value})}
                sx={{flex: 0.45}}
              />
              <TextField
                size="small"
                label="Value"
                value={row.value}
                onChange={(e) => updateRow(row.id, {value: e.target.value})}
                multiline
                maxRows={4}
                sx={{flex: 0.55}}
              />
              <Tooltip title="Remove">
                <IconButton size="small" onClick={() => removeRow(row.id)} aria-label={`Remove ${title.toLowerCase()} row`}>
                  <DeleteIcon fontSize="inherit"/>
                </IconButton>
              </Tooltip>
            </Stack>
          ))}
        </Stack>
      )}
    </Box>
  );
}

export default function KeyDetail({keyId}: { keyId: string }) {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [ek, setEk] = useState<Key | null>(null);

  // Actions UI state
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const [editOpen, setEditOpen] = useState(false);
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false);
  const [editState, setEditState] = useState<KeyState>(KeyState.ACTIVE);
  const [editLabelRows, setEditLabelRows] = useState<KeyValueRow[]>([]);
  const [editAnnotationRows, setEditAnnotationRows] = useState<KeyValueRow[]>([]);
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

  const stateOptions = useMemo(() => Object.values(KeyState), []);

  const fetchKey = () => {
    setLoading(true);
    setError(null);
    keys.get(keyId)
      .then(res => {
        setEk(res.data);
      })
      .catch(err => {
        const msg = err?.response?.data?.error || err.message || 'Failed to load key';
        setError(msg);
      })
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    keys.get(keyId)
      .then(res => {
        if (cancelled) return;
        setEk(res.data);
      })
      .catch(err => {
        if (cancelled) return;
        const msg = err?.response?.data?.error || err.message || 'Failed to load key';
        setError(msg);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [keyId]);

  if (loading) return (<Box sx={{display: 'flex', justifyContent: 'center', p: 4}}><CircularProgress/></Box>);
  if (error) return (<Alert severity="error">{error}</Alert>);
  if (!ek) return null;

  const openMenu = (e: React.MouseEvent<HTMLButtonElement>) => setMenuAnchorEl(e.currentTarget);
  const closeMenu = () => setMenuAnchorEl(null);

  const onClickEdit = () => {
    setActionError(null);
    setEditState(ek.state);
    setEditLabelRows(mapToRows(ek.labels));
    setEditAnnotationRows(mapToRows(ek.annotations));
    closeMenu();
    setEditOpen(true);
  };

  const onSubmitEdit = async () => {
    if (!ek) return;
    const duplicateLabels = duplicateKeys(editLabelRows);
    const duplicateAnnotations = duplicateKeys(editAnnotationRows);
    if (duplicateLabels.length > 0 || duplicateAnnotations.length > 0) {
      const parts = [];
      if (duplicateLabels.length > 0) parts.push(`duplicate labels: ${duplicateLabels.join(', ')}`);
      if (duplicateAnnotations.length > 0) parts.push(`duplicate annotations: ${duplicateAnnotations.join(', ')}`);
      setActionError(parts.join('; '));
      return;
    }
    setActionError(null);
    setActionLoading(true);
    try {
      const resp = await keys.update(ek.id, {
        state: editState,
        labels: rowsToMap(editLabelRows),
        annotations: rowsToMap(editAnnotationRows),
      });
      setEk(resp.data);
      setEditOpen(false);
    } catch (err: any) {
      const msg = err?.response?.data?.error || err.message || 'Failed to update key';
      setActionError(msg);
    } finally {
      setActionLoading(false);
    }
  };

  const closeEditDialog = () => {
    if (actionLoading) return;
    setActionError(null);
    setEditOpen(false);
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
      await keys.delete(ek.id);
      setConfirmDeleteOpen(false);
      navigate('/keys');
    } catch (err: any) {
      const msg = err?.response?.data?.error || err.message || 'Failed to delete key';
      setActionError(msg);
    } finally {
      setActionLoading(false);
    }
  };

  return (
    <Stack spacing={2} sx={{p: 2}}>
      <Stack direction="row" justifyContent="space-between" alignItems="center">
        <Typography variant="h5">Key</Typography>
        <Stack direction="row" spacing={1} alignItems="center">
          <StateChip state={ek.state}/>
          <IconButton aria-label="actions" onClick={openMenu} size="small">
            <MoreVertIcon/>
          </IconButton>
          <Menu anchorEl={menuAnchorEl} open={Boolean(menuAnchorEl)} onClose={closeMenu}>
            <MenuItem onClick={onClickEdit}>Edit...</MenuItem>
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
            <IconButton size="small" aria-label="Copy key id" onClick={handleCopyId}>
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

      <AnnotationsEditor
        annotations={ek.annotations}
        onPut={async (key, value) => {
          await keys.putAnnotation(ek.id, key, value);
          fetchKey();
        }}
        onDelete={async (key) => {
          await keys.deleteAnnotation(ek.id, key);
          fetchKey();
        }}
      />

      <Dialog open={editOpen} onClose={closeEditDialog} fullWidth maxWidth="md">
        <DialogTitle>Edit key</DialogTitle>
        <DialogContent>
          {actionError && <Alert severity="error" sx={{mt: 1, mb: 2}} onClose={() => setActionError(null)}>{actionError}</Alert>}
          <FormControl fullWidth sx={{mt: 2}}>
            <InputLabel id="edit-key-state-label">State</InputLabel>
            <Select
              labelId="edit-key-state-label"
              label="State"
              value={editState}
              onChange={(e) => setEditState(e.target.value as KeyState)}
            >
              {stateOptions.map(s => (
                <MenuItem key={s} value={s}>{s}</MenuItem>
              ))}
            </Select>
          </FormControl>

          <Stack spacing={3} sx={{mt: 3}}>
            <KeyValueRowsEditor
              title="Labels"
              rows={editLabelRows}
              onChange={setEditLabelRows}
              addLabel="Add label"
            />
            <KeyValueRowsEditor
              title="Annotations"
              rows={editAnnotationRows}
              onChange={setEditAnnotationRows}
              addLabel="Add annotation"
            />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={closeEditDialog} disabled={actionLoading}>Cancel</Button>
          <Button onClick={onSubmitEdit} variant="contained" disabled={actionLoading}>Save</Button>
        </DialogActions>
      </Dialog>

      {/* Delete confirmation dialog */}
      <Dialog open={confirmDeleteOpen} onClose={() => !actionLoading && setConfirmDeleteOpen(false)}>
        <DialogTitle>Delete key</DialogTitle>
        <DialogContent>
          <Typography variant="body2">
            Are you sure you want to delete this key? This action cannot be undone.
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
