import React, {useEffect, useState} from 'react';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import EditOutlinedIcon from '@mui/icons-material/EditOutlined';

const RESOURCE_NAME_PATTERN = /^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,61}[A-Za-z0-9])?$/;

interface ResourceNameEditorProps {
    name: string;
    resourceType: string;
    onRename?: (name: string) => Promise<void>;
}

export default function ResourceNameEditor({name, resourceType, onRename}: ResourceNameEditorProps) {
    const [open, setOpen] = useState(false);
    const [value, setValue] = useState(name);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => setValue(name), [name]);

    const close = () => {
        if (saving) return;
        setOpen(false);
        setError(null);
    };

    const startRename = () => {
        setValue(name);
        setError(null);
        setOpen(true);
    };

    const save = async () => {
        const nextName = value.trim();
        if (!RESOURCE_NAME_PATTERN.test(nextName)) {
            setError('Use 1–63 characters; start and end with a letter or number and use only letters, numbers, dot, underscore, or hyphen.');
            return;
        }
        if (!onRename || nextName === name) {
            setOpen(false);
            return;
        }

        setSaving(true);
        setError(null);
        try {
            await onRename(nextName);
            setOpen(false);
        } catch (err: any) {
            setError(err?.response?.data?.error || err?.message || `Failed to rename ${resourceType.toLowerCase()}`);
        } finally {
            setSaving(false);
        }
    };

    return (
        <Box>
            <Typography variant="subtitle2" color="text.secondary">Name</Typography>
            <Stack direction="row" spacing={1} alignItems="center" sx={{mt: 0.25}}>
                <Typography variant="h6" sx={{wordBreak: 'break-word'}}>{name}</Typography>
                {onRename && (
                    <Button
                        size="small"
                        startIcon={<EditOutlinedIcon/>}
                        onClick={startRename}
                        aria-label={`Rename ${resourceType.toLowerCase()}`}
                    >
                        Rename
                    </Button>
                )}
            </Stack>

            <Dialog open={open} onClose={close} fullWidth maxWidth="xs">
                <DialogTitle>Rename {resourceType}</DialogTitle>
                <DialogContent>
                    {error && <Alert severity="error" sx={{mb: 2}}>{error}</Alert>}
                    <TextField
                        autoFocus
                        fullWidth
                        label="Name"
                        value={value}
                        onChange={(event) => setValue(event.target.value)}
                        onKeyDown={(event) => {
                            if (event.key === 'Enter') {
                                event.preventDefault();
                                void save();
                            }
                        }}
                        helperText="The immutable ID and resource URL do not change."
                        disabled={saving}
                        inputProps={{maxLength: 63}}
                        sx={{mt: 1}}
                    />
                </DialogContent>
                <DialogActions>
                    <Button onClick={close} disabled={saving}>Cancel</Button>
                    <Button onClick={() => void save()} variant="contained" disabled={saving || value.trim() === name}>
                        Save
                    </Button>
                </DialogActions>
            </Dialog>
        </Box>
    );
}
