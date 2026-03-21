import React, {useState} from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import Stack from '@mui/material/Stack';
import Chip from '@mui/material/Chip';
import IconButton from '@mui/material/IconButton';
import TextField from '@mui/material/TextField';
import Button from '@mui/material/Button';
import Alert from '@mui/material/Alert';
import EditIcon from '@mui/icons-material/Edit';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import SaveIcon from '@mui/icons-material/Save';
import CancelIcon from '@mui/icons-material/Cancel';

interface AnnotationsEditorProps {
  annotations: Record<string, string> | undefined;
  onPut: (key: string, value: string) => Promise<void>;
  onDelete: (key: string) => Promise<void>;
  readOnly?: boolean;
}

export default function AnnotationsEditor({annotations, onPut, onDelete, readOnly}: AnnotationsEditorProps) {
  const [editing, setEditing] = useState(false);
  const [addingNew, setAddingNew] = useState(false);
  const [newKey, setNewKey] = useState('');
  const [newValue, setNewValue] = useState('');
  const [editingKey, setEditingKey] = useState<string | null>(null);
  const [editValue, setEditValue] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const entries = annotations ? Object.entries(annotations) : [];

  const handleSaveNew = async () => {
    if (!newKey.trim()) return;
    setSaving(true);
    setError(null);
    try {
      await onPut(newKey.trim(), newValue);
      setNewKey('');
      setNewValue('');
      setAddingNew(false);
    } catch (err: any) {
      setError(err?.response?.data?.error || err.message || 'Failed to save annotation');
    } finally {
      setSaving(false);
    }
  };

  const handleSaveEdit = async (key: string) => {
    setSaving(true);
    setError(null);
    try {
      await onPut(key, editValue);
      setEditingKey(null);
      setEditValue('');
    } catch (err: any) {
      setError(err?.response?.data?.error || err.message || 'Failed to save annotation');
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (key: string) => {
    setSaving(true);
    setError(null);
    try {
      await onDelete(key);
    } catch (err: any) {
      setError(err?.response?.data?.error || err.message || 'Failed to delete annotation');
    } finally {
      setSaving(false);
    }
  };

  const startEdit = (key: string, value: string) => {
    setEditingKey(key);
    setEditValue(value);
    setEditing(true);
  };

  const cancelEdit = () => {
    setEditingKey(null);
    setEditValue('');
  };

  return (
    <Box>
      <Stack direction="row" spacing={1} alignItems="center">
        <Typography variant="subtitle2" color="text.secondary">Annotations</Typography>
        {!editing && !readOnly && (
          <IconButton size="small" onClick={() => setEditing(true)} aria-label="Edit annotations">
            <EditIcon fontSize="inherit"/>
          </IconButton>
        )}
        {editing && (
          <Button size="small" onClick={() => { setEditing(false); setAddingNew(false); cancelEdit(); }}>
            Done
          </Button>
        )}
      </Stack>

      {error && <Alert severity="error" sx={{mt: 0.5, mb: 0.5}} onClose={() => setError(null)}>{error}</Alert>}

      {entries.length === 0 && !addingNew && (
        <Typography variant="body2" color="text.secondary" sx={{mt: 0.5}}>No annotations</Typography>
      )}

      {entries.length > 0 && !editing && (
        <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{mt: 0.5}}>
          {entries.map(([key, value]) => (
            <Chip
              key={key}
              label={`${key}: ${value.length > 50 ? value.substring(0, 50) + '...' : value}`}
              size="small"
              variant="outlined"
            />
          ))}
        </Stack>
      )}

      {editing && (
        <Stack spacing={1} sx={{mt: 1}}>
          {entries.map(([key, value]) => (
            <Box key={key}>
              {editingKey === key ? (
                <Stack direction="row" spacing={1} alignItems="flex-start">
                  <TextField
                    size="small"
                    label="Key"
                    value={key}
                    disabled
                    sx={{flex: 0.4}}
                  />
                  <TextField
                    size="small"
                    label="Value"
                    value={editValue}
                    onChange={(e) => setEditValue(e.target.value)}
                    multiline
                    maxRows={4}
                    sx={{flex: 0.6}}
                    disabled={saving}
                  />
                  <IconButton size="small" onClick={() => handleSaveEdit(key)} disabled={saving}>
                    <SaveIcon fontSize="inherit"/>
                  </IconButton>
                  <IconButton size="small" onClick={cancelEdit} disabled={saving}>
                    <CancelIcon fontSize="inherit"/>
                  </IconButton>
                </Stack>
              ) : (
                <Stack direction="row" spacing={1} alignItems="center">
                  <Chip
                    label={`${key}: ${value.length > 80 ? value.substring(0, 80) + '...' : value}`}
                    size="small"
                    variant="outlined"
                    onClick={() => startEdit(key, value)}
                  />
                  <IconButton size="small" onClick={() => handleDelete(key)} disabled={saving}>
                    <DeleteIcon fontSize="inherit"/>
                  </IconButton>
                </Stack>
              )}
            </Box>
          ))}

          {addingNew ? (
            <Stack direction="row" spacing={1} alignItems="flex-start">
              <TextField
                size="small"
                label="Key"
                value={newKey}
                onChange={(e) => setNewKey(e.target.value)}
                sx={{flex: 0.4}}
                disabled={saving}
                placeholder="my-annotation"
              />
              <TextField
                size="small"
                label="Value"
                value={newValue}
                onChange={(e) => setNewValue(e.target.value)}
                multiline
                maxRows={4}
                sx={{flex: 0.6}}
                disabled={saving}
                placeholder="Any string value"
              />
              <IconButton size="small" onClick={handleSaveNew} disabled={saving || !newKey.trim()}>
                <SaveIcon fontSize="inherit"/>
              </IconButton>
              <IconButton size="small" onClick={() => { setAddingNew(false); setNewKey(''); setNewValue(''); }} disabled={saving}>
                <CancelIcon fontSize="inherit"/>
              </IconButton>
            </Stack>
          ) : (
            <Button
              size="small"
              startIcon={<AddIcon/>}
              onClick={() => setAddingNew(true)}
              sx={{alignSelf: 'flex-start'}}
            >
              Add Annotation
            </Button>
          )}
        </Stack>
      )}
    </Box>
  );
}
