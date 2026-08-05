import React, {useState} from 'react';
import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import MenuItem from '@mui/material/MenuItem';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import KeyValueRowsEditor, {
  duplicateKeys,
  editableReservedKeys,
  KeyValueRow,
  mapToRows,
  rowsToMap,
  SYSTEM_LABEL_PREFIX,
} from './KeyValueRowsEditor';

type MetadataDialog = 'rename' | 'labels' | 'annotations' | null;

interface ResourceMetadataMenuItemsProps {
  resource: string;
  name?: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  onCloseMenu: () => void;
  onRename?: (name: string) => Promise<void>;
  onUpdateLabels: (labels: Record<string, string>) => Promise<void>;
  onUpdateAnnotations: (annotations: Record<string, string>) => Promise<void>;
  renameDisabledReason?: string;
  disabled?: boolean;
}

export default function ResourceMetadataMenuItems({
  resource,
  name,
  labels,
  annotations,
  onCloseMenu,
  onRename,
  onUpdateLabels,
  onUpdateAnnotations,
  renameDisabledReason,
  disabled,
}: ResourceMetadataMenuItemsProps) {
  const [dialog, setDialog] = useState<MetadataDialog>(null);
  const [nextName, setNextName] = useState(name || '');
  const [labelRows, setLabelRows] = useState<KeyValueRow[]>([]);
  const [annotationRows, setAnnotationRows] = useState<KeyValueRow[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const openDialog = (nextDialog: Exclude<MetadataDialog, null>) => {
    setError(null);
    if (nextDialog === 'rename') {
      setNextName(name || '');
    } else if (nextDialog === 'labels') {
      setLabelRows(mapToRows(labels, {readonlyKeyPrefix: SYSTEM_LABEL_PREFIX}));
    } else {
      setAnnotationRows(mapToRows(annotations));
    }
    onCloseMenu();
    setDialog(nextDialog);
  };

  const closeDialog = () => {
    if (saving) return;
    setError(null);
    setDialog(null);
  };

  const save = async (action: () => Promise<void>) => {
    setSaving(true);
    setError(null);
    try {
      await action();
      setDialog(null);
    } catch (err: any) {
      setError(err?.response?.data?.error || err.message || `Failed to update ${resource}`);
    } finally {
      setSaving(false);
    }
  };

  const saveRename = () => {
    const trimmedName = nextName.trim();
    if (!trimmedName) {
      setError('Name is required');
      return;
    }
    if (onRename) {
      void save(() => onRename(trimmedName));
    }
  };

  const saveLabels = () => {
    const duplicates = duplicateKeys(labelRows);
    if (duplicates.length > 0) {
      setError(`duplicate labels: ${duplicates.join(', ')}`);
      return;
    }

    const reserved = editableReservedKeys(labelRows, SYSTEM_LABEL_PREFIX);
    if (reserved.length > 0) {
      setError(`system-managed labels cannot be edited: ${reserved.join(', ')}`);
      return;
    }

    void save(() => onUpdateLabels(rowsToMap(labelRows, {includeReadonly: false})));
  };

  const saveAnnotations = () => {
    const duplicates = duplicateKeys(annotationRows);
    if (duplicates.length > 0) {
      setError(`duplicate annotations: ${duplicates.join(', ')}`);
      return;
    }

    void save(() => onUpdateAnnotations(rowsToMap(annotationRows)));
  };

  const dialogTitle = dialog === 'rename'
    ? `Rename ${resource}`
    : dialog === 'labels'
      ? `Edit ${resource} labels`
      : `Edit ${resource} annotations`;

  return (
    <>
      {onRename ? (
        <MenuItem onClick={() => openDialog('rename')} disabled={disabled}>Rename…</MenuItem>
      ) : (
        <Tooltip title={renameDisabledReason || `This ${resource} cannot be renamed`} placement="left">
          <span><MenuItem disabled>Rename…</MenuItem></span>
        </Tooltip>
      )}
      <MenuItem onClick={() => openDialog('labels')} disabled={disabled}>Edit labels…</MenuItem>
      <MenuItem onClick={() => openDialog('annotations')} disabled={disabled}>Edit annotations…</MenuItem>

      {dialog !== null && (
        <Dialog open onClose={closeDialog} fullWidth maxWidth="sm">
          <DialogTitle>{dialogTitle}</DialogTitle>
          <DialogContent>
            {error && <Alert severity="error" sx={{mt: 1, mb: 2}} onClose={() => setError(null)}>{error}</Alert>}
            {dialog === 'rename' && (
              <TextField
                autoFocus
                fullWidth
                label="Name"
                value={nextName}
                onChange={(event) => setNextName(event.target.value)}
                disabled={saving}
                helperText="Names must be unique within their namespace."
                sx={{mt: 2}}
              />
            )}
            {dialog === 'labels' && (
              <>
                <Alert severity="info" sx={{mt: 2, mb: 2}}>
                  System-managed and propagated labels are read-only and are not included when saving.
                </Alert>
                <KeyValueRowsEditor
                  title="Labels"
                  rows={labelRows}
                  onChange={setLabelRows}
                  addLabel="Add label"
                />
              </>
            )}
            {dialog === 'annotations' && (
              <BoxedEditor
                rows={annotationRows}
                onChange={setAnnotationRows}
              />
            )}
          </DialogContent>
          <DialogActions>
            <Button onClick={closeDialog} disabled={saving}>Cancel</Button>
            <Button
              onClick={dialog === 'rename' ? saveRename : dialog === 'labels' ? saveLabels : saveAnnotations}
              variant="contained"
              disabled={saving}
            >
              Save
            </Button>
          </DialogActions>
        </Dialog>
      )}
    </>
  );
}

function BoxedEditor({rows, onChange}: { rows: KeyValueRow[]; onChange: (rows: KeyValueRow[]) => void }) {
  return (
    <div style={{marginTop: 16}}>
      <KeyValueRowsEditor
        title="Annotations"
        rows={rows}
        onChange={onChange}
        addLabel="Add annotation"
      />
    </div>
  );
}
