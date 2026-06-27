import Box from '@mui/material/Box';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import Button from '@mui/material/Button';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import IconButton from '@mui/material/IconButton';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';

export type KeyValueRow = {
  id: string;
  key: string;
  value: string;
  readonly?: boolean;
};

let keyValueRowSequence = 0;

export const SYSTEM_LABEL_PREFIX = 'apxy/';

export function nextKeyValueRow(key = '', value = '', readonly = false): KeyValueRow {
  keyValueRowSequence += 1;
  return {
    id: `kv-${keyValueRowSequence}`,
    key,
    value,
    readonly,
  };
}

export function mapToRows(values?: Record<string, string>, options?: { readonlyKeyPrefix?: string }): KeyValueRow[] {
  return Object.entries(values || {}).map(([key, value]) => {
    return nextKeyValueRow(key, value, !!options?.readonlyKeyPrefix && key.startsWith(options.readonlyKeyPrefix));
  });
}

export function rowsToMap(rows: KeyValueRow[], options?: { includeReadonly?: boolean }): Record<string, string> {
  const includeReadonly = options?.includeReadonly ?? true;
  const out: Record<string, string> = {};
  for (const row of rows) {
    if (row.readonly && !includeReadonly) continue;
    const key = row.key.trim();
    if (key) out[key] = row.value;
  }
  return out;
}

export function duplicateKeys(rows: KeyValueRow[]): string[] {
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

export function editableReservedKeys(rows: KeyValueRow[], prefix: string): string[] {
  const reserved = new Set<string>();
  for (const row of rows) {
    const key = row.key.trim();
    if (!row.readonly && key.startsWith(prefix)) reserved.add(key);
  }
  return Array.from(reserved);
}

export default function KeyValueRowsEditor({
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
                disabled={row.readonly}
                sx={{flex: 0.45}}
              />
              <TextField
                size="small"
                label="Value"
                value={row.value}
                onChange={(e) => updateRow(row.id, {value: e.target.value})}
                disabled={row.readonly}
                multiline
                maxRows={4}
                sx={{flex: 0.55}}
              />
              <Tooltip title={row.readonly ? 'System-managed' : 'Remove'}>
                <span>
                  <IconButton
                    size="small"
                    onClick={() => removeRow(row.id)}
                    aria-label={`Remove ${title.toLowerCase()} row`}
                    disabled={row.readonly}
                  >
                    <DeleteIcon fontSize="inherit"/>
                  </IconButton>
                </span>
              </Tooltip>
            </Stack>
          ))}
        </Stack>
      )}
    </Box>
  );
}
