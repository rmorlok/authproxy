import React, { useEffect, useMemo, useState } from 'react';
import Box from '@mui/material/Box';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import TextField from '@mui/material/TextField';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import Switch from '@mui/material/Switch';
import FormControlLabel from '@mui/material/FormControlLabel';
import Button from '@mui/material/Button';
import IconButton from '@mui/material/IconButton';
import Autocomplete from '@mui/material/Autocomplete';
import Collapse from '@mui/material/Collapse';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import AddIcon from '@mui/icons-material/AddOutlined';
import DeleteIcon from '@mui/icons-material/DeleteOutline';
import { useTheme } from '@mui/material/styles';
import CodeMirror from '@uiw/react-codemirror';
import { json as jsonMode } from '@codemirror/lang-json';
import { oneDark } from '@codemirror/theme-one-dark';
import {
    Connection, listConnections, PROXY_METHODS, ProxyMethod, ProxyRequest,
} from '@authproxy/api';

// RequestFormValue mirrors what the rate-limit dry-run endpoint takes
// today and what a future "send this for real" page will hand to
// /connections/{id}/_proxy. The form is intentionally HTTP-shape-only
// (no rate-limit knowledge); higher-level pages combine this value with
// their own extras (e.g. request_type for dry-run).
export interface RequestFormValue {
    request: ProxyRequest;
    context: RequestFormContext;
}

export interface RequestFormContext {
    connectionId?: string;
    actorId?: string;
    namespace?: string;
}

interface Props {
    value: RequestFormValue;
    onChange: (next: RequestFormValue) => void;
    /**
     * When set, the connection autocomplete is scoped to this namespace
     * (and descendants). Most callers pass the admin UI's current
     * namespace from the redux selector.
     */
    connectionNamespace?: string;
}

export const EMPTY_REQUEST_VALUE: RequestFormValue = {
    request: { url: '', method: 'GET', headers: {}, labels: {} },
    context: {},
};

type BodyKind = 'none' | 'json' | 'raw';

export default function RequestForm({ value, onChange, connectionNamespace }: Props) {
    const theme = useTheme();
    const [advancedOpen, setAdvancedOpen] = useState(
        !!(value.context.actorId || value.context.namespace),
    );
    const [bodyOpen, setBodyOpen] = useState(
        !!(value.request.body_raw || value.request.body_json !== undefined),
    );

    const update = (patch: Partial<RequestFormValue>) => onChange({ ...value, ...patch });
    const updateRequest = (patch: Partial<ProxyRequest>) =>
        update({ request: { ...value.request, ...patch } });
    const updateContext = (patch: Partial<RequestFormContext>) =>
        update({ context: { ...value.context, ...patch } });

    return (
        <Stack spacing={3}>
            <ConnectionSection
                connectionId={value.context.connectionId}
                namespace={connectionNamespace}
                onChange={(connectionId) => updateContext({ connectionId })}
            />

            <AdvancedSection
                open={advancedOpen}
                onToggle={() => setAdvancedOpen(!advancedOpen)}
                context={value.context}
                onChange={updateContext}
            />

            <Box>
                <Typography variant="subtitle1" sx={{ mb: 1.5 }}>HTTP request</Typography>
                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} sx={{ mb: 2 }}>
                    <FormControl size="small" sx={{ minWidth: 140 }}>
                        <InputLabel id="request-form-method">Method</InputLabel>
                        <Select
                            labelId="request-form-method"
                            label="Method"
                            value={value.request.method || 'GET'}
                            onChange={(e) => updateRequest({ method: e.target.value as ProxyMethod })}
                        >
                            {PROXY_METHODS.map((m) => (
                                <MenuItem key={m} value={m}>{m}</MenuItem>
                            ))}
                        </Select>
                    </FormControl>
                    <TextField
                        label="URL"
                        size="small"
                        fullWidth
                        value={value.request.url || ''}
                        onChange={(e) => updateRequest({ url: e.target.value })}
                        placeholder="https://api.example.com/v1/things"
                    />
                </Stack>

                <KeyValueEditor
                    label="Headers"
                    helperText="Sent as HTTP headers on the request."
                    addButtonLabel="Add header"
                    value={value.request.headers || {}}
                    onChange={(headers) => updateRequest({ headers })}
                    keyPlaceholder="X-Custom-Header"
                    valuePlaceholder="value"
                />

                <Box sx={{ mt: 2 }}>
                    <KeyValueEditor
                        label="Labels"
                        helperText="Per-request labels merged into the label snapshot. Override connection labels with the same key. apxy/* keys are reserved and dropped server-side."
                        addButtonLabel="Add label"
                        value={value.request.labels || {}}
                        onChange={(labels) => updateRequest({ labels })}
                        keyPlaceholder="team"
                        valuePlaceholder="acme"
                    />
                </Box>

                <Box sx={{ mt: 2 }}>
                    <FormControlLabel
                        control={
                            <Switch
                                checked={bodyOpen}
                                onChange={(_, checked) => {
                                    setBodyOpen(checked);
                                    if (!checked) {
                                        // Collapsing clears the body so the
                                        // outgoing request doesn't keep
                                        // hidden state.
                                        updateRequest({ body_raw: undefined, body_json: undefined });
                                    }
                                }}
                            />
                        }
                        label="Request body"
                    />
                    <Collapse in={bodyOpen}>
                        <BodyEditor
                            value={value.request}
                            onChange={updateRequest}
                            dark={theme.palette.mode === 'dark'}
                        />
                    </Collapse>
                </Box>
            </Box>
        </Stack>
    );
}

// ----- Connection picker -----

function ConnectionSection({
    connectionId,
    namespace,
    onChange,
}: {
    connectionId?: string;
    namespace?: string;
    onChange: (id: string | undefined) => void;
}) {
    const [connections, setConnections] = useState<Connection[]>([]);
    const [loading, setLoading] = useState(false);

    useEffect(() => {
        let cancelled = false;
        setLoading(true);
        // Best-effort load — failures fall back to "no connections" rather
        // than blocking the form. Callers can still type a raw connection
        // id via the Advanced section.
        listConnections({ namespace, limit: 100 })
            .then((res) => {
                if (!cancelled) setConnections(res.data.items || []);
            })
            .catch(() => {
                if (!cancelled) setConnections([]);
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });
        return () => { cancelled = true; };
    }, [namespace]);

    const options = useMemo(() => connections.map((c) => c.id), [connections]);
    const selected = options.find((id) => id === connectionId) || null;

    return (
        <Box>
            <Typography variant="subtitle1" sx={{ mb: 1.5 }}>Connection</Typography>
            <Autocomplete
                size="small"
                options={options}
                value={selected}
                loading={loading}
                onChange={(_, id) => onChange(id || undefined)}
                getOptionLabel={(id) => {
                    const c = connections.find((c) => c.id === id);
                    return c ? `${c.id} — ${c.connector?.id || ''} (${c.namespace})` : id;
                }}
                renderInput={(params) => (
                    <TextField
                        {...params}
                        label="Connection"
                        helperText="Picking a connection hydrates namespace, connector, and label snapshot the way the runtime does."
                    />
                )}
            />
        </Box>
    );
}

// ----- Advanced overrides -----

function AdvancedSection({
    open,
    onToggle,
    context,
    onChange,
}: {
    open: boolean;
    onToggle: () => void;
    context: RequestFormContext;
    onChange: (patch: Partial<RequestFormContext>) => void;
}) {
    return (
        <Box>
            <FormControlLabel
                control={<Switch checked={open} onChange={onToggle} />}
                label="Advanced: override actor / namespace"
            />
            <Collapse in={open}>
                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} sx={{ mt: 1 }}>
                    <TextField
                        label="Actor ID"
                        size="small"
                        value={context.actorId || ''}
                        onChange={(e) => onChange({ actorId: e.target.value || undefined })}
                        placeholder="act_abc"
                        helperText="Overrides the actor used for bucket-key resolution."
                        fullWidth
                    />
                    <TextField
                        label="Namespace"
                        size="small"
                        value={context.namespace || ''}
                        onChange={(e) => onChange({ namespace: e.target.value || undefined })}
                        placeholder="root.acme"
                        helperText="Required when no connection is picked."
                        fullWidth
                    />
                </Stack>
            </Collapse>
        </Box>
    );
}

// ----- Body editor -----

function BodyEditor({
    value,
    onChange,
    dark,
}: {
    value: ProxyRequest;
    onChange: (patch: Partial<ProxyRequest>) => void;
    dark: boolean;
}) {
    const initialKind: BodyKind = value.body_json !== undefined
        ? 'json'
        : value.body_raw
            ? 'raw'
            : 'json';
    const [kind, setKind] = useState<BodyKind>(initialKind);

    const jsonText = useMemo(() => {
        if (value.body_json === undefined) return '';
        try {
            return JSON.stringify(value.body_json, null, 2);
        } catch {
            return '';
        }
    }, [value.body_json]);

    const rawText = value.body_raw || '';

    return (
        <Box sx={{ mt: 1 }}>
            <ToggleButtonGroup
                value={kind}
                exclusive
                size="small"
                onChange={(_, next: BodyKind | null) => {
                    if (!next) return;
                    setKind(next);
                    // Switching kind clears the other channel so we never
                    // accidentally send both — the server rejects that.
                    if (next === 'json') onChange({ body_raw: undefined });
                    if (next === 'raw') onChange({ body_json: undefined });
                }}
                sx={{ mb: 1 }}
            >
                <ToggleButton value="json">JSON</ToggleButton>
                <ToggleButton value="raw">Raw text</ToggleButton>
            </ToggleButtonGroup>
            <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 1, overflow: 'hidden' }}>
                {kind === 'json' ? (
                    <CodeMirror
                        value={jsonText}
                        theme={dark ? oneDark : undefined}
                        extensions={[jsonMode()]}
                        minHeight="120px"
                        onChange={(text) => {
                            if (text.trim() === '') {
                                onChange({ body_json: undefined });
                                return;
                            }
                            try {
                                const parsed = JSON.parse(text);
                                onChange({ body_json: parsed });
                            } catch {
                                // Keep typing; only commit when valid.
                            }
                        }}
                    />
                ) : (
                    <CodeMirror
                        value={rawText}
                        theme={dark ? oneDark : undefined}
                        minHeight="120px"
                        onChange={(text) => onChange({ body_raw: text })}
                    />
                )}
            </Box>
        </Box>
    );
}

// ----- Shared key/value editor (used by Headers + Labels) -----

interface Row {
    key: string;
    value: string;
}

function entriesAsRows(entries: Record<string, string>): Row[] {
    return Object.entries(entries).map(([key, value]) => ({ key, value }));
}

function rowsAsEntries(rows: Row[]): Record<string, string> {
    const out: Record<string, string> = {};
    for (const row of rows) {
        if (row.key) out[row.key] = row.value;
    }
    return out;
}

function KeyValueEditor({
    label,
    helperText,
    addButtonLabel,
    value,
    onChange,
    keyPlaceholder,
    valuePlaceholder,
}: {
    label: string;
    helperText?: string;
    addButtonLabel: string;
    value: Record<string, string>;
    onChange: (next: Record<string, string>) => void;
    keyPlaceholder?: string;
    valuePlaceholder?: string;
}) {
    // Local row state lets a user keep typing a blank key without us
    // collapsing the row out from under them. Sync from prop only on
    // mount + when the prop reference changes from a non-editing source.
    const [rows, setRows] = useState<Row[]>(() => entriesAsRows(value));

    useEffect(() => {
        // Only resync when the incoming map differs in keyset from what
        // we'd produce — avoids fighting per-keystroke updates.
        const computed = rowsAsEntries(rows);
        const keysSame = JSON.stringify(Object.keys(computed).sort())
            === JSON.stringify(Object.keys(value).sort());
        if (!keysSame) setRows(entriesAsRows(value));
    }, [value, rows]);

    const commit = (next: Row[]) => {
        setRows(next);
        onChange(rowsAsEntries(next));
    };

    return (
        <Box>
            <Typography variant="subtitle2" sx={{ mb: 0.5 }}>{label}</Typography>
            {helperText && (
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
                    {helperText}
                </Typography>
            )}
            <Stack spacing={1}>
                {rows.map((row, i) => (
                    <Stack key={i} direction="row" spacing={1} alignItems="center">
                        <TextField
                            size="small"
                            value={row.key}
                            onChange={(e) => {
                                const next = [...rows];
                                next[i] = { ...row, key: e.target.value };
                                commit(next);
                            }}
                            placeholder={keyPlaceholder}
                            sx={{ flex: 0.4 }}
                        />
                        <TextField
                            size="small"
                            value={row.value}
                            onChange={(e) => {
                                const next = [...rows];
                                next[i] = { ...row, value: e.target.value };
                                commit(next);
                            }}
                            placeholder={valuePlaceholder}
                            sx={{ flex: 0.6 }}
                        />
                        <IconButton
                            size="small"
                            aria-label={`Remove ${label} row ${i + 1}`}
                            onClick={() => commit(rows.filter((_, j) => j !== i))}
                        >
                            <DeleteIcon fontSize="small" />
                        </IconButton>
                    </Stack>
                ))}
                <Button
                    size="small"
                    startIcon={<AddIcon />}
                    onClick={() => commit([...rows, { key: '', value: '' }])}
                    sx={{ alignSelf: 'flex-start' }}
                >
                    {addButtonLabel}
                </Button>
            </Stack>
        </Box>
    );
}
