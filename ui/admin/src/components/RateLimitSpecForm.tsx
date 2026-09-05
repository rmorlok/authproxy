import React, { useMemo } from 'react';
import Box from '@mui/material/Box';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import TextField from '@mui/material/TextField';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import FormControl from '@mui/material/FormControl';
import FormLabel from '@mui/material/FormLabel';
import FormControlLabel from '@mui/material/FormControlLabel';
import Radio from '@mui/material/Radio';
import RadioGroup from '@mui/material/RadioGroup';
import Switch from '@mui/material/Switch';
import Select from '@mui/material/Select';
import InputLabel from '@mui/material/InputLabel';
import MenuItem from '@mui/material/MenuItem';
import Autocomplete from '@mui/material/Autocomplete';
import Chip from '@mui/material/Chip';
import Divider from '@mui/material/Divider';
import {
    RateLimitSpec, RateLimitMode, PathMatchKind, SlidingWindowMode,
    RateLimitSelector, RateLimitBucket, RateLimitAlgorithm,
    RateLimitFixedWindow, RateLimitSlidingWindow, RateLimitTokenBucket,
    RateLimitScope, RATE_LIMIT_API_VERSION,
} from '@authproxy/api';

// Mirrors the server's validHTTPMethods set in internal/schema/rate_limit/selector.go.
const HTTP_METHODS = ['GET', 'HEAD', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS', 'CONNECT', 'TRACE'];

// Mirrors common.AllRequestTypes() — the rate-limit selector accepts any of
// these though the server defaults to ['proxy', 'probe'] when omitted.
const REQUEST_TYPES = ['proxy', 'probe', 'oauth', 'public', 'global'];

// Reserved dimension names from internal/schema/rate_limit/bucket.go.
const RESERVED_DIMENSIONS = ['actor', 'connection', 'connector', 'connector_version', 'namespace', 'method'];

type AlgorithmVariant = 'token_bucket' | 'fixed_window' | 'sliding_window';
type ScopeKind = 'namespace-default' | 'namespace-matcher' | 'connector' | 'connection';

function detectVariant(algo: RateLimitAlgorithm): AlgorithmVariant {
    if (algo.fixedWindow) return 'fixed_window';
    if (algo.slidingWindow) return 'sliding_window';
    return 'token_bucket';
}

// Defaults for each algorithm variant — picked to be valid out of the box so
// switching variants in the UI doesn't immediately produce a validation error.
const DEFAULT_TOKEN_BUCKET: RateLimitTokenBucket = { capacity: 60, refillRate: 1 };
const DEFAULT_FIXED_WINDOW: RateLimitFixedWindow = { window: '1m', limit: 100 };
const DEFAULT_SLIDING_WINDOW: RateLimitSlidingWindow = { window: '1m', limit: 100, mode: SlidingWindowMode.COUNTER };

export const EMPTY_SPEC: RateLimitSpec = {
    mode: RateLimitMode.ENFORCE,
    selector: {
        methods: ['POST', 'PATCH', 'PUT'],
        requestTypes: ['proxy'],
    },
    bucket: {
        dimensions: ['actor'],
    },
    algorithm: {
        tokenBucket: { ...DEFAULT_TOKEN_BUCKET },
    },
};

interface Props {
    value: RateLimitSpec;
    onChange: (next: RateLimitSpec) => void;
}

export default function RateLimitSpecForm({ value, onChange }: Props) {
    const variant = detectVariant(value.algorithm);

    const update = (patch: Partial<RateLimitSpec>) => onChange({ ...value, ...patch });
    const updateSelector = (patch: Partial<RateLimitSelector>) =>
        update({ selector: { ...value.selector, ...patch } });
    const updateBucket = (patch: Partial<RateLimitBucket>) =>
        update({ bucket: { ...value.bucket, ...patch } });

    const isEnforce = (value.mode || RateLimitMode.ENFORCE) === RateLimitMode.ENFORCE;

    const onChangeVariant = (next: AlgorithmVariant) => {
        if (next === variant) return;
        let nextAlgo: RateLimitAlgorithm;
        switch (next) {
            case 'fixed_window':
                nextAlgo = { fixedWindow: { ...DEFAULT_FIXED_WINDOW } };
                break;
            case 'sliding_window':
                nextAlgo = { slidingWindow: { ...DEFAULT_SLIDING_WINDOW } };
                break;
            default:
                nextAlgo = { tokenBucket: { ...DEFAULT_TOKEN_BUCKET } };
        }
        update({ algorithm: nextAlgo });
    };

    return (
        <Stack spacing={3}>
            <Section title="Mode">
                <Stack direction="row" spacing={1} alignItems="center">
                    <FormControlLabel
                        control={
                            <Switch
                                checked={isEnforce}
                                onChange={(_, checked) => update({ mode: checked ? RateLimitMode.ENFORCE : RateLimitMode.OBSERVE })}
                            />
                        }
                        label={isEnforce ? 'Enforce — return 429 when over limit' : 'Observe — record matches, never reject'}
                    />
                </Stack>
            </Section>

            <Divider />

            <ScopeSection value={value.scope} onChange={(scope) => update({scope})} />

            <Divider />

            <SelectorSection value={value.selector} onChange={updateSelector} />

            <Divider />

            <BucketSection value={value.bucket} onChange={updateBucket} />

            <Divider />

            <AlgorithmSection
                variant={variant}
                algorithm={value.algorithm}
                onChangeVariant={onChangeVariant}
                onChange={(algo) => update({ algorithm: algo })}
            />
        </Stack>
    );
}

function ScopeSection({ value, onChange }: { value?: RateLimitScope; onChange: (scope?: RateLimitScope) => void }) {
    const kind: ScopeKind = value?.namespaceMatcher !== undefined
        ? 'namespace-matcher'
        : value?.connectorRef
            ? 'connector'
            : value?.connectionRef
                ? 'connection'
                : 'namespace-default';

    const changeKind = (next: ScopeKind) => {
        switch (next) {
            case 'namespace-matcher':
                onChange({namespaceMatcher: ''});
                break;
            case 'connector':
                onChange({connectorRef: {apiVersion: RATE_LIMIT_API_VERSION, kind: 'Connector', id: ''}});
                break;
            case 'connection':
                onChange({connectionRef: {apiVersion: RATE_LIMIT_API_VERSION, kind: 'Connection', id: ''}});
                break;
            default:
                onChange(undefined);
        }
    };

    return (
        <Section title="Scope" hint="The resource namespace cascades to descendants by default; optionally narrow it with a namespace matcher, connector, or connection.">
            <Stack spacing={2}>
                <FormControl size="small" sx={{maxWidth: 280}}>
                    <InputLabel id="rate-limit-scope-label">Target</InputLabel>
                    <Select
                        labelId="rate-limit-scope-label"
                        label="Target"
                        value={kind}
                        onChange={(event) => changeKind(event.target.value as ScopeKind)}
                    >
                        <MenuItem value="namespace-default">Resource namespace and descendants</MenuItem>
                        <MenuItem value="namespace-matcher">Namespace matcher</MenuItem>
                        <MenuItem value="connector">Connector</MenuItem>
                        <MenuItem value="connection">Connection</MenuItem>
                    </Select>
                </FormControl>

                {value?.namespaceMatcher !== undefined && (
                    <TextField
                        label="Namespace matcher"
                        size="small"
                        fullWidth
                        value={value.namespaceMatcher}
                        onChange={(event) => onChange({namespaceMatcher: event.target.value})}
                        placeholder="root.acme.payments.**"
                        helperText="Must select only the rate limit namespace or its descendants. Omit .** for an exact namespace."
                    />
                )}

                {value?.connectorRef && (
                    <TextField
                        label="Connector ID"
                        size="small"
                        fullWidth
                        value={value.connectorRef.id || ''}
                        onChange={(event) => onChange({connectorRef: {...value.connectorRef, id: event.target.value}})}
                        placeholder="cxr_..."
                        helperText="Applies to every generation of the connector"
                    />
                )}

                {value?.connectionRef && (
                    <TextField
                        label="Connection ID"
                        size="small"
                        fullWidth
                        value={value.connectionRef.id || ''}
                        onChange={(event) => onChange({connectionRef: {...value.connectionRef, id: event.target.value}})}
                        placeholder="cxn_..."
                    />
                )}
            </Stack>
        </Section>
    );
}

function Section({ title, children, hint }: { title: string; children: React.ReactNode; hint?: string }) {
    return (
        <Box>
            <Typography variant="subtitle1" sx={{ mb: 0.5 }}>{title}</Typography>
            {hint && <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>{hint}</Typography>}
            {children}
        </Box>
    );
}

function SelectorSection({ value, onChange }: { value: RateLimitSelector; onChange: (p: Partial<RateLimitSelector>) => void }) {
    const methods = value.methods || [];
    const requestTypes = value.requestTypes || [];
    const pathMatch = value.pathMatch;

    return (
        <Section title="Selector" hint="Match criteria — clauses are AND-ed. Leave a clause empty to skip it.">
            <Stack spacing={2}>
                <TextField
                    label="Label selector"
                    placeholder="apxy/connector/-/id=salesforce,team=acme"
                    helperText="Kubernetes-style selector evaluated against the per-request label snapshot. Leave blank to skip."
                    fullWidth
                    size="small"
                    value={value.labelSelector || ''}
                    onChange={(e) => onChange({ labelSelector: e.target.value || undefined })}
                />

                <FormControl>
                    <FormLabel sx={{ mb: 0.5 }}>HTTP methods</FormLabel>
                    <ToggleButtonGroup
                        value={methods}
                        onChange={(_, next: string[]) => onChange({ methods: next.length > 0 ? next : undefined })}
                        size="small"
                        color="primary"
                        sx={{ flexWrap: 'wrap', gap: 0.5 }}
                    >
                        {HTTP_METHODS.map(m => (
                            <ToggleButton key={m} value={m} sx={{ px: 1.5 }}>{m}</ToggleButton>
                        ))}
                    </ToggleButtonGroup>
                    <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>
                        Empty = any method.
                    </Typography>
                </FormControl>

                <FormControl>
                    <FormLabel sx={{ mb: 0.5 }}>Request types</FormLabel>
                    <ToggleButtonGroup
                        value={requestTypes}
                        onChange={(_, next: string[]) => onChange({ requestTypes: next.length > 0 ? next : undefined })}
                        size="small"
                        color="primary"
                        sx={{ flexWrap: 'wrap', gap: 0.5 }}
                    >
                        {REQUEST_TYPES.map(t => (
                            <ToggleButton key={t} value={t} sx={{ px: 1.5 }}>{t}</ToggleButton>
                        ))}
                    </ToggleButtonGroup>
                    <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>
                        Empty = default of <code>proxy</code> + <code>probe</code>. An explicit empty list is rejected server-side.
                    </Typography>
                </FormControl>

                <Box>
                    <FormControlLabel
                        control={
                            <Switch
                                checked={!!pathMatch}
                                onChange={(_, checked) => onChange({
                                    pathMatch: checked
                                        ? { kind: PathMatchKind.PREFIX, value: '' }
                                        : undefined,
                                })}
                            />
                        }
                        label="Restrict by request path"
                    />
                    {pathMatch && (
                        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} sx={{ mt: 1 }}>
                            <FormControl size="small" sx={{ minWidth: 140 }}>
                                <InputLabel id="path-kind-label">Kind</InputLabel>
                                <Select
                                    labelId="path-kind-label"
                                    label="Kind"
                                    value={pathMatch.kind}
                                    onChange={(e) => onChange({ pathMatch: { ...pathMatch, kind: e.target.value as PathMatchKind } })}
                                >
                                    <MenuItem value={PathMatchKind.PREFIX}>prefix</MenuItem>
                                    <MenuItem value={PathMatchKind.GLOB}>glob</MenuItem>
                                    <MenuItem value={PathMatchKind.REGEX}>regex</MenuItem>
                                </Select>
                            </FormControl>
                            <TextField
                                label="Value"
                                size="small"
                                fullWidth
                                value={pathMatch.value}
                                onChange={(e) => onChange({ pathMatch: { ...pathMatch, value: e.target.value } })}
                                placeholder={pathMatch.kind === PathMatchKind.REGEX ? '^/v2/users/[^/]+$' : '/services/data/'}
                            />
                        </Stack>
                    )}
                </Box>
            </Stack>
        </Section>
    );
}

function BucketSection({ value, onChange }: { value: RateLimitBucket; onChange: (p: Partial<RateLimitBucket>) => void }) {
    const dimensions = value.dimensions || [];

    // The full option list = reserved names + any labels/* dimensions the
    // user has already typed. Autocomplete's freeSolo lets them enter new
    // ones, but seeding the list keeps the common case mouse-driven.
    const options = useMemo(() => {
        const seen = new Set(RESERVED_DIMENSIONS);
        const extras = dimensions.filter(d => !seen.has(d));
        return [...RESERVED_DIMENSIONS, ...extras];
    }, [dimensions]);

    return (
        <Section title="Bucket" hint="Projects matched requests into independent counters. Empty list = single global bucket per rule.">
            <Autocomplete
                multiple
                freeSolo
                options={options}
                value={dimensions}
                onChange={(_, next) => onChange({ dimensions: next.length > 0 ? next : undefined })}
                renderTags={(val, getTagProps) =>
                    val.map((dim, index) => {
                        const { key, ...tagProps } = getTagProps({ index });
                        return <Chip key={key} {...tagProps} size="small" label={dim} />;
                    })
                }
                renderInput={(params) => (
                    <TextField
                        {...params}
                        size="small"
                        label="Dimensions"
                        placeholder="Pick reserved name or type labels/<key>"
                        helperText="Reserved: actor, connection, connector, connector_version, namespace, method. Custom: labels/<key>."
                    />
                )}
            />
        </Section>
    );
}

function AlgorithmSection({
    variant,
    algorithm,
    onChangeVariant,
    onChange,
}: {
    variant: AlgorithmVariant;
    algorithm: RateLimitAlgorithm;
    onChangeVariant: (v: AlgorithmVariant) => void;
    onChange: (algo: RateLimitAlgorithm) => void;
}) {
    return (
        <Section title="Algorithm" hint="Exactly one variant. Switching variants resets that variant's fields to sensible defaults.">
            <RadioGroup row value={variant} onChange={(_, v) => onChangeVariant(v as AlgorithmVariant)} sx={{ mb: 2 }}>
                <FormControlLabel value="token_bucket" control={<Radio />} label="Token bucket" />
                <FormControlLabel value="fixed_window" control={<Radio />} label="Fixed window" />
                <FormControlLabel value="sliding_window" control={<Radio />} label="Sliding window" />
            </RadioGroup>

            {variant === 'token_bucket' && algorithm.tokenBucket && (
                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
                    <TextField
                        label="Capacity"
                        size="small"
                        type="number"
                        value={algorithm.tokenBucket.capacity}
                        onChange={(e) => onChange({ tokenBucket: { ...algorithm.tokenBucket!, capacity: Number(e.target.value) } })}
                        helperText="Maximum tokens (burst capacity)"
                        slotProps={{ htmlInput: { min: 1, step: 1 } }}
                    />
                    <TextField
                        label="Refill rate"
                        size="small"
                        type="number"
                        value={algorithm.tokenBucket.refillRate}
                        onChange={(e) => onChange({ tokenBucket: { ...algorithm.tokenBucket!, refillRate: Number(e.target.value) } })}
                        helperText="Tokens added per second (fractional OK)"
                        slotProps={{ htmlInput: { min: 0, step: 0.1 } }}
                    />
                </Stack>
            )}

            {variant === 'fixed_window' && algorithm.fixedWindow && (
                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
                    <TextField
                        label="Window"
                        size="small"
                        value={algorithm.fixedWindow.window}
                        onChange={(e) => onChange({ fixedWindow: { ...algorithm.fixedWindow!, window: e.target.value } })}
                        helperText="HumanDuration, e.g. 1m, 5m, 1h"
                    />
                    <TextField
                        label="Limit"
                        size="small"
                        type="number"
                        value={algorithm.fixedWindow.limit}
                        onChange={(e) => onChange({ fixedWindow: { ...algorithm.fixedWindow!, limit: Number(e.target.value) } })}
                        helperText="Maximum requests per window"
                        slotProps={{ htmlInput: { min: 1, step: 1 } }}
                    />
                </Stack>
            )}

            {variant === 'sliding_window' && algorithm.slidingWindow && (
                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
                    <TextField
                        label="Window"
                        size="small"
                        value={algorithm.slidingWindow.window}
                        onChange={(e) => onChange({ slidingWindow: { ...algorithm.slidingWindow!, window: e.target.value } })}
                        helperText="HumanDuration, e.g. 1m, 5m"
                    />
                    <TextField
                        label="Limit"
                        size="small"
                        type="number"
                        value={algorithm.slidingWindow.limit}
                        onChange={(e) => onChange({ slidingWindow: { ...algorithm.slidingWindow!, limit: Number(e.target.value) } })}
                        helperText="Max requests in trailing window"
                        slotProps={{ htmlInput: { min: 1, step: 1 } }}
                    />
                    <FormControl size="small" sx={{ minWidth: 160 }}>
                        <InputLabel id="sw-mode-label">Mode</InputLabel>
                        <Select
                            labelId="sw-mode-label"
                            label="Mode"
                            value={algorithm.slidingWindow.mode}
                            onChange={(e) => onChange({ slidingWindow: { ...algorithm.slidingWindow!, mode: e.target.value as SlidingWindowMode } })}
                        >
                            <MenuItem value={SlidingWindowMode.LOG}>log (exact)</MenuItem>
                            <MenuItem value={SlidingWindowMode.COUNTER}>counter (approximate)</MenuItem>
                        </Select>
                    </FormControl>
                </Stack>
            )}
        </Section>
    );
}

export { detectVariant };
export type { AlgorithmVariant };
