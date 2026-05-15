import React, { useMemo, useState } from 'react';
import { useSelector } from 'react-redux';
import { Link as RouterLink } from 'react-router-dom';
import Box from '@mui/material/Box';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import Paper from '@mui/material/Paper';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Alert from '@mui/material/Alert';
import Link from '@mui/material/Link';
import Accordion from '@mui/material/Accordion';
import AccordionSummary from '@mui/material/AccordionSummary';
import AccordionDetails from '@mui/material/AccordionDetails';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import Divider from '@mui/material/Divider';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
    dryRunRateLimit,
    DryRunRateLimitResponse,
    DryRunRateLimitMatch,
    DryRunRateLimitNotMatched,
} from '@authproxy/api';
import RequestForm, {
    EMPTY_REQUEST_VALUE,
    RequestFormValue,
} from '../components/RequestForm';
import { selectCurrentNamespacePath } from '../store/namespacesSlice';

// Mirrors common.RequestType on the server. The matcher only fires for
// the type the operator picks here, so it's a separate dropdown rather
// than a field on the request itself.
const REQUEST_TYPES = ['proxy', 'probe', 'oauth', 'public', 'global'] as const;
type RequestType = typeof REQUEST_TYPES[number];

export default function RateLimitDryRun() {
    const ns = useSelector(selectCurrentNamespacePath);
    const [formValue, setFormValue] = useState<RequestFormValue>({
        ...EMPTY_REQUEST_VALUE,
        request: { ...EMPTY_REQUEST_VALUE.request, method: 'POST', url: 'https://api.example.com/v1/things' },
        context: { namespace: ns || 'root' },
    });
    const [requestType, setRequestType] = useState<RequestType>('proxy');
    const [running, setRunning] = useState(false);
    const [result, setResult] = useState<DryRunRateLimitResponse | null>(null);
    const [error, setError] = useState<string | null>(null);

    const canRun = useMemo(() => {
        return !!(formValue.request.url && formValue.request.method
            && (formValue.context.connectionId || formValue.context.namespace));
    }, [formValue]);

    const onRun = async () => {
        setRunning(true);
        setError(null);
        setResult(null);
        try {
            const resp = await dryRunRateLimit({
                request: formValue.request,
                request_type: requestType,
                context: {
                    connection_id: formValue.context.connectionId,
                    actor_id: formValue.context.actorId,
                    namespace: formValue.context.namespace,
                },
            });
            setResult(resp.data);
        } catch (e: any) {
            const msg = e?.response?.data?.error || e?.message || 'Dry-run failed';
            setError(msg);
        } finally {
            setRunning(false);
        }
    };

    return (
        <Box sx={{ width: '100%', maxWidth: { sm: '100%', md: '1700px' } }}>
            <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 2 }}>
                <Typography component="h2" variant="h6">Rate Limit Dry Run</Typography>
            </Stack>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                Evaluate which rate-limit rules would apply to a synthesized request — and whether each would limit it — without sending real traffic or consuming any counters.
            </Typography>

            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: '1fr 1fr' }, gap: 4 }}>
                <Box>
                    <Stack direction="row" spacing={2} alignItems="center" sx={{ mb: 3 }}>
                        <FormControl size="small" sx={{ minWidth: 160 }}>
                            <InputLabel id="request-type">Request type</InputLabel>
                            <Select
                                labelId="request-type"
                                label="Request type"
                                value={requestType}
                                onChange={(e) => setRequestType(e.target.value as RequestType)}
                            >
                                {REQUEST_TYPES.map((t) => (
                                    <MenuItem key={t} value={t}>{t}</MenuItem>
                                ))}
                            </Select>
                        </FormControl>
                        <Button
                            variant="contained"
                            onClick={onRun}
                            disabled={!canRun || running}
                        >
                            {running ? 'Running...' : 'Run dry run'}
                        </Button>
                    </Stack>
                    <RequestForm value={formValue} onChange={setFormValue} connectionNamespace={ns} />
                </Box>

                <Box>
                    <Typography variant="subtitle1" sx={{ mb: 2 }}>Results</Typography>
                    {error && (
                        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
                            {error}
                        </Alert>
                    )}
                    {!result && !error && (
                        <Typography variant="body2" color="text.secondary">
                            Configure the request on the left and click <strong>Run dry run</strong>.
                        </Typography>
                    )}
                    {result && <ResultsPanel result={result} />}
                </Box>
            </Box>
        </Box>
    );
}

function ResultsPanel({ result }: { result: DryRunRateLimitResponse }) {
    const hasAny = result.matched.length > 0 || result.not_matched.length > 0;
    return (
        <Stack spacing={3}>
            <Section
                title="Matched"
                count={result.matched.length}
                emptyMessage="No rules matched this request."
            >
                {result.matched.length > 0 && (
                    <Stack spacing={1}>
                        {result.matched.map((m) => <MatchedRow key={m.rate_limit_id} match={m} />)}
                    </Stack>
                )}
            </Section>

            <Section
                title="Not matched"
                count={result.not_matched.length}
                emptyMessage={
                    hasAny
                        ? 'Every in-scope rule matched.'
                        : 'No rules in scope for this namespace.'
                }
            >
                {result.not_matched.length > 0 && (
                    <Stack spacing={1}>
                        {result.not_matched.map((nm) => <NotMatchedRow key={nm.rate_limit_id} nm={nm} />)}
                    </Stack>
                )}
            </Section>

            <LabelSnapshot snapshot={result.request_label_snapshot} />
        </Stack>
    );
}

function Section({
    title,
    count,
    emptyMessage,
    children,
}: {
    title: string;
    count: number;
    emptyMessage: string;
    children?: React.ReactNode;
}) {
    return (
        <Box>
            <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 1 }}>
                <Typography variant="subtitle2">{title}</Typography>
                <Chip label={count} size="small" />
            </Stack>
            {count === 0 ? (
                <Typography variant="body2" color="text.secondary">{emptyMessage}</Typography>
            ) : (
                children
            )}
        </Box>
    );
}

function MatchedRow({ match }: { match: DryRunRateLimitMatch }) {
    const enforce = match.effective_mode === 'enforce';
    return (
        <Paper variant="outlined" sx={{ p: 1.5 }}>
            <Stack spacing={1}>
                <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" rowGap={1}>
                    <Link component={RouterLink} to={`/rate-limits/${match.rate_limit_id}`} variant="body2" sx={{
                        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                    }}>
                        {match.rate_limit_id}
                    </Link>
                    <Chip
                        label={match.effective_mode}
                        size="small"
                        color={enforce ? 'warning' : 'info'}
                        variant="outlined"
                    />
                    <Chip
                        label={match.would_allow ? 'would allow' : 'would 429'}
                        size="small"
                        color={match.would_allow ? 'success' : 'error'}
                    />
                    {match.peek_failed && (
                        <Chip
                            label="peek failed — runtime would fail-open"
                            size="small"
                            color="warning"
                            variant="outlined"
                        />
                    )}
                </Stack>
                <Divider />
                <Stack direction="row" spacing={3} flexWrap="wrap" rowGap={0.5}>
                    <KeyVal label="Algorithm" value={match.algorithm_summary} />
                    {match.would_allow ? (
                        <KeyVal label="Remaining" value={String(match.remaining)} />
                    ) : (
                        <KeyVal label="Retry after" value={`${match.retry_after_ms} ms`} />
                    )}
                    <KeyVal label="Namespace" value={match.namespace} />
                </Stack>
                <KeyVal label="Bucket key" value={match.bucket_key || '(global)'} mono />
            </Stack>
        </Paper>
    );
}

function NotMatchedRow({ nm }: { nm: DryRunRateLimitNotMatched }) {
    return (
        <Paper variant="outlined" sx={{ p: 1.5 }}>
            <Stack direction="row" spacing={2} alignItems="baseline" flexWrap="wrap" rowGap={0.5}>
                <Link component={RouterLink} to={`/rate-limits/${nm.rate_limit_id}`} variant="body2" sx={{
                    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                }}>
                    {nm.rate_limit_id}
                </Link>
                <Typography variant="caption" color="text.secondary">{nm.namespace}</Typography>
                <Typography variant="body2" color="text.secondary" sx={{ flex: 1 }}>{nm.reason}</Typography>
            </Stack>
        </Paper>
    );
}

function KeyVal({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
    return (
        <Box>
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>{label}</Typography>
            <Typography
                variant="body2"
                sx={mono ? {
                    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                    fontSize: '0.85rem',
                    wordBreak: 'break-all',
                } : undefined}
            >
                {value}
            </Typography>
        </Box>
    );
}

function LabelSnapshot({ snapshot }: { snapshot: Record<string, string> }) {
    const entries = Object.entries(snapshot || {});
    return (
        <Accordion variant="outlined" disableGutters>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Stack direction="row" spacing={1} alignItems="center">
                    <Typography variant="subtitle2">Request label snapshot</Typography>
                    <Chip label={entries.length} size="small" />
                </Stack>
            </AccordionSummary>
            <AccordionDetails>
                {entries.length === 0 ? (
                    <Typography variant="body2" color="text.secondary">
                        Empty — no labels resolved for this request.
                    </Typography>
                ) : (
                    <Stack direction="row" spacing={0.5} flexWrap="wrap" rowGap={0.5}>
                        {entries.map(([k, v]) => (
                            <Chip key={k} label={`${k}: ${v}`} size="small" variant="outlined" />
                        ))}
                    </Stack>
                )}
            </AccordionDetails>
        </Accordion>
    );
}
