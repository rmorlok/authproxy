import React, {useState} from 'react';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import CircularProgress from '@mui/material/CircularProgress';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import AddIcon from '@mui/icons-material/Add';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import EditIcon from '@mui/icons-material/Edit';
import type {Permission} from '@authproxy/api';

interface ActorPermissionsEditorProps {
  permissions: Permission[] | undefined;
  onSave: (permissions: Permission[]) => Promise<void>;
}

interface PermissionDraft {
  id: number;
  namespace: string;
  resources: string;
  resourceIds: string;
  verbs: string;
}

const joinValues = (values: string[] | undefined) => values?.join(', ') ?? '';

const splitValues = (value: string) => value
  .split(',')
  .map(item => item.trim())
  .filter(Boolean);

const toDrafts = (permissions: Permission[]): PermissionDraft[] => permissions.map((permission, index) => ({
  id: index,
  namespace: permission.namespace,
  resources: joinValues(permission.resources),
  resourceIds: joinValues(permission.resourceIds),
  verbs: joinValues(permission.verbs),
}));

const toPermission = (draft: PermissionDraft): Permission => {
  const resourceIds = splitValues(draft.resourceIds);
  return {
    namespace: draft.namespace.trim(),
    resources: splitValues(draft.resources),
    verbs: splitValues(draft.verbs),
    ...(resourceIds.length > 0 ? {resourceIds} : {}),
  };
};

function ValueChips({values, emptyLabel}: {values: string[] | undefined; emptyLabel: string}) {
  if (!values || values.length === 0) {
    return <Typography variant="body2" color="text.secondary">{emptyLabel}</Typography>;
  }

  return (
    <Stack direction="row" spacing={0.5} useFlexGap flexWrap="wrap">
      {values.map((value, index) => (
        <Chip key={`${value}-${index}`} label={value} size="small" variant="outlined"/>
      ))}
    </Stack>
  );
}

function PermissionValue({label, children}: {label: string; children: React.ReactNode}) {
  return (
    <Box>
      <Typography variant="caption" color="text.secondary">{label}</Typography>
      <Box sx={{mt: 0.25}}>{children}</Box>
    </Box>
  );
}

export default function ActorPermissionsEditor({permissions, onSave}: ActorPermissionsEditorProps) {
  const currentPermissions = permissions ?? [];
  const [open, setOpen] = useState(false);
  const [drafts, setDrafts] = useState<PermissionDraft[]>([]);
  const [nextId, setNextId] = useState(0);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const openEditor = () => {
    setDrafts(toDrafts(currentPermissions));
    setNextId(currentPermissions.length);
    setError(null);
    setOpen(true);
  };

  const closeEditor = () => {
    if (saving) return;
    setOpen(false);
    setError(null);
  };

  const updateDraft = (id: number, field: keyof Omit<PermissionDraft, 'id'>, value: string) => {
    setDrafts(current => current.map(draft => draft.id === id ? {...draft, [field]: value} : draft));
  };

  const addPermission = () => {
    setDrafts(current => [
      ...current,
      {id: nextId, namespace: '', resources: '', resourceIds: '', verbs: ''},
    ]);
    setNextId(current => current + 1);
  };

  const save = async () => {
    const nextPermissions = drafts.map(toPermission);
    const invalidIndex = nextPermissions.findIndex(permission => (
      permission.namespace.length === 0 || permission.resources.length === 0 || permission.verbs.length === 0
    ));
    if (invalidIndex >= 0) {
      setError(`Permission ${invalidIndex + 1} requires a namespace, at least one resource, and at least one verb.`);
      return;
    }

    setSaving(true);
    setError(null);
    try {
      await onSave(nextPermissions);
      setOpen(false);
    } catch (err: any) {
      setError(err?.response?.data?.error || err.message || 'Failed to save permissions');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Box>
      <Stack direction="row" spacing={1} alignItems="center" sx={{mb: currentPermissions.length > 0 ? 1 : 0.5}}>
        <Typography variant="subtitle2" color="text.secondary">Permissions</Typography>
        <IconButton size="small" onClick={openEditor} aria-label="Edit actor permissions">
          <EditIcon fontSize="inherit"/>
        </IconButton>
      </Stack>

      {currentPermissions.length === 0 ? (
        <Typography variant="body2" color="text.secondary">No permissions</Typography>
      ) : (
        <Stack spacing={1}>
          {currentPermissions.map((permission, index) => (
            <Paper key={`${permission.namespace}-${index}`} variant="outlined" sx={{p: 1.5}}>
              <Stack spacing={1.25}>
                <PermissionValue label="Namespace">
                  <Typography
                    component="code"
                    variant="body2"
                    sx={{fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace'}}
                  >
                    {permission.namespace}
                  </Typography>
                </PermissionValue>
                <Stack direction={{xs: 'column', sm: 'row'}} spacing={3}>
                  <Box sx={{flex: 1}}>
                    <PermissionValue label="Resources">
                      <ValueChips values={permission.resources} emptyLabel="No resources"/>
                    </PermissionValue>
                  </Box>
                  <Box sx={{flex: 1}}>
                    <PermissionValue label="Verbs">
                      <ValueChips values={permission.verbs} emptyLabel="No verbs"/>
                    </PermissionValue>
                  </Box>
                  <Box sx={{flex: 1}}>
                    <PermissionValue label="Resource IDs">
                      <ValueChips values={permission.resourceIds} emptyLabel="All resource IDs"/>
                    </PermissionValue>
                  </Box>
                </Stack>
              </Stack>
            </Paper>
          ))}
        </Stack>
      )}

      <Dialog open={open} onClose={closeEditor} fullWidth maxWidth="md">
        <DialogTitle>Edit actor permissions</DialogTitle>
        <DialogContent>
          <Alert severity="warning" sx={{mt: 1, mb: 2}}>
            Permission changes take effect immediately and may change which AuthProxy resources this actor can access.
          </Alert>

          {error && <Alert severity="error" sx={{mb: 2}} onClose={() => setError(null)}>{error}</Alert>}

          <Stack spacing={2}>
            {drafts.map((draft, index) => (
              <Paper key={draft.id} variant="outlined" sx={{p: 2}}>
                <Stack spacing={2}>
                  <Stack direction="row" justifyContent="space-between" alignItems="center">
                    <Typography variant="subtitle2">Permission {index + 1}</Typography>
                    <IconButton
                      size="small"
                      color="error"
                      aria-label={`Remove permission ${index + 1}`}
                      onClick={() => setDrafts(current => current.filter(item => item.id !== draft.id))}
                      disabled={saving}
                    >
                      <DeleteOutlineIcon fontSize="small"/>
                    </IconButton>
                  </Stack>
                  <TextField
                    label="Namespace"
                    value={draft.namespace}
                    onChange={event => updateDraft(draft.id, 'namespace', event.target.value)}
                    placeholder="root.tenant.**"
                    helperText="Exact namespace or subtree matcher"
                    required
                    fullWidth
                    disabled={saving}
                  />
                  <Stack direction={{xs: 'column', sm: 'row'}} spacing={2}>
                    <TextField
                      label="Resources"
                      value={draft.resources}
                      onChange={event => updateDraft(draft.id, 'resources', event.target.value)}
                      placeholder="connections, connectors"
                      helperText="Comma-separated resource types; * matches all"
                      required
                      fullWidth
                      disabled={saving}
                    />
                    <TextField
                      label="Verbs"
                      value={draft.verbs}
                      onChange={event => updateDraft(draft.id, 'verbs', event.target.value)}
                      placeholder="get, list, proxy"
                      helperText="Comma-separated actions; * matches all"
                      required
                      fullWidth
                      disabled={saving}
                    />
                  </Stack>
                  <TextField
                    label="Resource IDs"
                    value={draft.resourceIds}
                    onChange={event => updateDraft(draft.id, 'resourceIds', event.target.value)}
                    placeholder="cxn_example, cxn_other"
                    helperText="Optional comma-separated resource IDs; leave empty for all"
                    fullWidth
                    disabled={saving}
                  />
                </Stack>
              </Paper>
            ))}

            {drafts.length === 0 && (
              <Box sx={{py: 2, textAlign: 'center'}}>
                <Typography variant="body2" color="text.secondary">
                  This actor has no permissions.
                </Typography>
              </Box>
            )}

            <Button startIcon={<AddIcon/>} onClick={addPermission} sx={{alignSelf: 'flex-start'}} disabled={saving}>
              Add permission
            </Button>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={closeEditor} disabled={saving}>Cancel</Button>
          <Button onClick={save} variant="contained" disabled={saving}>
            {saving ? <CircularProgress size={20} aria-label="Saving permissions"/> : 'Save permissions'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
