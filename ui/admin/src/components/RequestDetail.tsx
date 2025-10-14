import React, {useEffect, useMemo, useState} from 'react';
import {
    Box,
    CircularProgress,
    Divider,
    FormControlLabel,
    IconButton,
    Stack,
    Switch,
    Tab,
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableRow,
    Tabs,
    Tooltip,
    Typography,
    Menu,
    MenuItem,
    ListItemIcon,
    ListItemText,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import {Duration, HttpStatusChip} from '../util'
import {getRequest, RequestEntry} from '../api';

function useRequest(id: string | undefined) {
    const [data, setData] = useState<RequestEntry | null>(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        let active = true;

        async function run() {
            if (!id) return;
            setLoading(true);
            setError(null);
            try {
                const resp = await getRequest(id);
                if (!active) return;
                if (resp.status === 200) setData(resp.data);
                else setError('Failed to load request');
            } catch (e: any) {
                if (!active) return;
                setError(e?.message || 'Failed to load request');
            } finally {
                if (active) setLoading(false);
            }
        }

        run();
        return () => {
            active = false;
        };
    }, [id]);

    return {data, loading, error};
}

function decodeBase64ToText(b64?: string): string {
    if (!b64) {
        return '';
    }

    try {
        const bytes = Uint8Array.from(atob(b64), c => c.charCodeAt(0));
        const dec = new TextDecoder('utf-8', {fatal: false});
        return dec.decode(bytes);
    } catch {
        try {
            // fallback
            return atob(b64);
        } catch {
            return '<failed to decode body>';
        }
    }
}

function isJsonContentType(headers: Record<string, string[]> | undefined): boolean {
    if (!headers) {
        return false;
    }

    const ct = Object.entries(headers).find(([k]) => k.toLowerCase() === 'content-type')?.[1]?.[0] || '';
    return ct.toLowerCase().includes('application/json') || ct.toLowerCase().includes('+json');
}

function isTextContentType(headers: Record<string, string[]> | undefined): boolean {
    if (!headers) return false;
    const ct = Object.entries(headers).find(([k]) => k.toLowerCase() === 'content-type')?.[1]?.[0] || '';
    const lct = ct.toLowerCase();
    return lct.startsWith('text/') || lct.includes('application/json') || lct.includes('+json') || lct.includes('application/x-www-form-urlencoded');
}

function tryFormatJson(text: string): string | null {
    try {
        const obj = JSON.parse(text);
        return JSON.stringify(obj, null, 2);
    } catch {
        return null;
    }
}

function headerValues(headers: Record<string, string[]> | undefined, name: string): string[] {
    if (!headers) return [];
    const entry = Object.entries(headers).find(([k]) => k.toLowerCase() === name.toLowerCase());
    return entry?.[1] || [];
}

function headersToHar(headers: Record<string, string[]> | undefined): {name: string, value: string}[] {
    if (!headers) return [];
    const arr: {name: string, value: string}[] = [];
    for (const [k, vals] of Object.entries(headers)) {
        for (const v of vals || []) arr.push({name: k, value: v});
    }
    return arr;
}

function toHar(entry: RequestEntry) {
    const urlObj = new URL(entry.req.u);
    const queryString = Array.from(urlObj.searchParams.entries()).map(([name, value]) => ({name, value}));

    const reqIsText = isTextContentType(entry.req.h);
    const reqBodyText = entry.req.b ? (reqIsText ? decodeBase64ToText(entry.req.b) : entry.req.b) : undefined;
    const reqEncoding = entry.req.b && !reqIsText ? 'base64' : undefined;
    const reqMime = headerValues(entry.req.h, 'content-type')[0];

    const resIsText = isTextContentType(entry.res.h);
    const resBodyText = entry.res.b ? (resIsText ? decodeBase64ToText(entry.res.b) : entry.res.b) : undefined;
    const resEncoding = entry.res.b && !resIsText ? 'base64' : undefined;
    const resMime = headerValues(entry.res.h, 'content-type')[0];

    const har = {
        log: {
            version: '1.2',
            creator: { name: 'AuthProxy Admin', version: 'dev' },
            entries: [
                {
                    startedDateTime: entry.ts,
                    time: entry.dur,
                    request: {
                        method: entry.req.m,
                        url: entry.req.u,
                        httpVersion: entry.req.v,
                        cookies: [],
                        headers: headersToHar(entry.req.h),
                        queryString,
                        headersSize: -1,
                        bodySize: typeof entry.req.cl === 'number' ? entry.req.cl : -1,
                        postData: entry.req.b ? ({
                            mimeType: reqMime || '',
                            text: reqBodyText,
                            ...(reqEncoding ? {encoding: reqEncoding} : {}),
                        }) : undefined,
                    },
                    response: {
                        status: entry.res.sc,
                        statusText: '',
                        httpVersion: entry.res.v,
                        cookies: [],
                        headers: headersToHar(entry.res.h),
                        content: {
                            size: typeof entry.res.cl === 'number' ? entry.res.cl : -1,
                            mimeType: resMime || '',
                            text: resBodyText,
                            ...(resEncoding ? {encoding: resEncoding} : {}),
                        },
                        redirectURL: headerValues(entry.res.h, 'location')[0] || '',
                        headersSize: -1,
                        bodySize: typeof entry.res.cl === 'number' ? entry.res.cl : -1,
                    },
                    cache: {},
                    timings: {
                        blocked: -1,
                        dns: -1,
                        connect: -1,
                        send: -1,
                        wait: entry.dur,
                        receive: -1,
                        ssl: -1,
                    },
                    serverIPAddress: undefined,
                    connection: undefined,
                    pageref: entry.cid || undefined,
                }
            ]
        }
    };
    return har;
}

function shellEscapeSingleQuotes(s: string): string {
    // bash-compatible escaping for single-quoted string
    return s.replace(/'/g, `'"'"'`);
}

function toCurl(entry: RequestEntry): string {
    const lines: string[] = [];
    lines.push(`curl -X ${entry.req.m} \\\n  '${entry.req.u}'`);
    // headers
    const excluded = new Set(['host', 'content-length']);
    for (const [k, vals] of Object.entries(entry.req.h || {})) {
        if (excluded.has(k.toLowerCase())) continue;
        for (const v of vals || []) {
            lines.push(`  -H '${shellEscapeSingleQuotes(`${k}: ${v}`)}'`);
        }
    }
    if (entry.req.b) {
        const isText = isTextContentType(entry.req.h);
        if (isText) {
            const body = decodeBase64ToText(entry.req.b);
            lines.push(`  --data-binary '${shellEscapeSingleQuotes(body)}'`);
        } else {
            // Fallback: use base64 decode inline (bash)
            lines.push(`  --data-binary @<(base64 -d <<< '${entry.req.b}')`);
        }
    }
    return lines.join(' \\\n');
}

function CopyMenu({data}: {data: RequestEntry}) {
    const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
    const open = Boolean(anchorEl);
    const handleOpen = (e: React.MouseEvent<HTMLElement>) => setAnchorEl(e.currentTarget);
    const handleClose = () => setAnchorEl(null);

    const copyJson = async () => {
        await navigator.clipboard.writeText(JSON.stringify(data, null, 2));
        handleClose();
    };
    const copyHar = async () => {
        const har = toHar(data);
        await navigator.clipboard.writeText(JSON.stringify(har, null, 2));
        handleClose();
    };
    const copyCurl = async () => {
        await navigator.clipboard.writeText(toCurl(data));
        handleClose();
    };

    return (
        <>
            <Tooltip title="Copy full details">
                <IconButton size="small" onClick={handleOpen} aria-label="Copy full details">
                    <ContentCopyIcon fontSize="small" />
                </IconButton>
            </Tooltip>
            <Menu anchorEl={anchorEl} open={open} onClose={handleClose} anchorOrigin={{vertical: 'bottom', horizontal: 'right'}} transformOrigin={{vertical: 'top', horizontal: 'right'}}>
                <MenuItem onClick={copyJson}>
                    <ListItemIcon><ContentCopyIcon fontSize="small" /></ListItemIcon>
                    <ListItemText>Copy as JSON</ListItemText>
                </MenuItem>
                <MenuItem onClick={copyHar}>
                    <ListItemIcon><ContentCopyIcon fontSize="small" /></ListItemIcon>
                    <ListItemText>Copy as HAR</ListItemText>
                </MenuItem>
                <MenuItem onClick={copyCurl}>
                    <ListItemIcon><ContentCopyIcon fontSize="small" /></ListItemIcon>
                    <ListItemText>Copy as cURL</ListItemText>
                </MenuItem>
                <MenuItem onClick={async () => { await navigator.clipboard.writeText(`${window.location.origin}/requests/${data.id}`); handleClose(); }}>
                    <ListItemIcon><ContentCopyIcon fontSize="small" /></ListItemIcon>
                    <ListItemText>Copy Admin URL</ListItemText>
                </MenuItem>
            </Menu>
        </>
    );
}

function HeadersTable({headers}: { headers: Record<string, string[]> }) {
    const rows = useMemo(() => Object.entries(headers || {}), [headers]);
    if (!rows.length) {
        return <Typography variant="body2" color="text.secondary">No headers</Typography>;
    }

    return (
        <Table size="small" stickyHeader>
            <TableHead>
                <TableRow>
                    <TableCell sx={{width: 220}}>Header</TableCell>
                    <TableCell>Value</TableCell>
                </TableRow>
            </TableHead>
            <TableBody>
                {rows.map(([k, v]) => (
                    <TableRow key={k}>
                        <TableCell sx={{verticalAlign: 'top'}}>{k}</TableCell>
                        <TableCell>
                            <Stack spacing={0.5}>{(v || []).map((vv, i) => (
                                <Typography key={i} variant="body2" sx={{wordBreak: 'break-all'}}>{vv}</Typography>
                            ))}</Stack>
                        </TableCell>
                    </TableRow>
                ))}
            </TableBody>
        </Table>
    );
}

function CopyButton({getText}: { getText: () => string }) {
    return (
        <Tooltip title="Copy to clipboard">
            <IconButton size="small" onClick={() => navigator.clipboard.writeText(getText())}>
                <ContentCopyIcon fontSize="inherit"/>
            </IconButton>
        </Tooltip>
    );
}

export interface RequestDetailProps {
    requestId: string;
    onClose?: () => void;
    showOpenFullPage?: boolean;
}

export default function RequestDetail({requestId, onClose, showOpenFullPage}: RequestDetailProps) {
    const {data, loading, error} = useRequest(requestId);
    const [tab, setTab] = useState(0);
    const [pretty, setPretty] = useState(true);

    const reqText = useMemo(() => decodeBase64ToText(data?.req?.b), [data?.req?.b]);
    const resText = useMemo(() => decodeBase64ToText(data?.res?.b), [data?.res?.b]);

    const prettyReq = useMemo(() => {
        if (!pretty) return reqText;
        if (isJsonContentType(data?.req?.h)) {
            const fm = tryFormatJson(reqText);
            if (fm) return fm;
        }
        return reqText;
    }, [pretty, reqText, data?.req?.h]);

    const prettyRes = useMemo(() => {
        if (!pretty) return resText;
        if (isJsonContentType(data?.res?.h)) {
            const fm = tryFormatJson(resText);
            if (fm) return fm;
        }
        return resText;
    }, [pretty, resText, data?.res?.h]);

    let heading = (<Typography variant="h6">Request Detail</Typography>);
    if (!loading && !error && data) {
        heading = (
            <Box sx={{p: 0}}>
            <Stack spacing={1}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <HttpStatusChip value={data.res.sc} size="medium" sx={{ fontSize: '0.9rem' }}/>
                    <Typography variant="h6">{data.req.m} {data.req.u}</Typography>
                </Box>
                {data.res.err && (
                    <Typography variant="body2" color="error">Error: {data.res.err}</Typography>
                )}
            </Stack>
        </Box>
        )
    }
    return (
        <Box sx={{flex: 1}} role="presentation">
            <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{p: 1.5, pl: 0}}>
                {heading}
                <Stack direction="row" spacing={1} alignItems="center">
                    {/* Copy full menu */}
                    {data && (
                        <>
                            {showOpenFullPage && (
                                <Tooltip title="Open full page">
                                    <IconButton size="small" component="a" href={`/requests/${requestId}`} target="_blank" rel="noopener noreferrer" aria-label="Open full page">
                                        <OpenInNewIcon fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            )}
                            <CopyMenu data={data} />
                        </>
                    )}
                    {onClose && (
                        <IconButton onClick={onClose} aria-label="Close details">
                            <CloseIcon/>
                        </IconButton>
                    )}
                </Stack>
            </Stack>
            <Divider/>

            {loading && (
                <Stack alignItems="center" justifyContent="center" sx={{p: 3}}>
                    <CircularProgress size={24}/>
                </Stack>
            )}
            {error && (
                <Typography color="error" sx={{p: 2}}>{error}</Typography>
            )}
            {!loading && !error && data && (
                <>
                    <Tabs value={tab} onChange={(_, v) => setTab(v)} variant="scrollable" allowScrollButtonsMobile>
                        <Tab label="Overview"/>
                        <Tab label="Headers"/>
                        <Tab label="Request Body"/>
                        <Tab label="Response Body"/>
                    </Tabs>
                    <Divider/>

                    <Box sx={{p: 2}}>
                        {tab === 0 && (
                            <Stack spacing={1.5}>
                                <Typography variant="subtitle2">Overview</Typography>
                                <Table size="small">
                                    <TableBody>
                                        <TableRow><TableCell>ID</TableCell><TableCell>{data.id}</TableCell></TableRow>
                                        {data.cid && (
                                            <TableRow><TableCell>Correlation
                                                ID</TableCell><TableCell>{data.cid}</TableCell></TableRow>
                                        )}
                                        <TableRow><TableCell>Timestamp</TableCell><TableCell>{data.ts}</TableCell></TableRow>
                                        <TableRow><TableCell>Time</TableCell><TableCell><Duration value={data.dur} /></TableCell></TableRow>
                                        <TableRow><TableCell>Method</TableCell><TableCell>{data.req.m}</TableCell></TableRow>
                                        <TableRow><TableCell>URL</TableCell><TableCell
                                            sx={{wordBreak: 'break-all'}}>{data.req.u}</TableCell></TableRow>
                                        <TableRow><TableCell>Status</TableCell><TableCell>{data.res.sc}</TableCell></TableRow>
                                        <TableRow><TableCell>Request
                                            Version</TableCell><TableCell>{data.req.v}</TableCell></TableRow>
                                        {typeof data.req.cl === 'number' && (
                                            <TableRow><TableCell>Request
                                                Size</TableCell><TableCell>{data.req.cl} bytes</TableCell></TableRow>
                                        )}
                                        <TableRow><TableCell>Response
                                            Version</TableCell><TableCell>{data.res.v}</TableCell></TableRow>
                                        {typeof data.res.cl === 'number' && (
                                            <TableRow><TableCell>Response
                                                Size</TableCell><TableCell>{data.res.cl} bytes</TableCell></TableRow>
                                        )}
                                    </TableBody>
                                </Table>
                            </Stack>
                        )}

                        {tab === 1 && (
                            <Box>
                                <Typography variant="subtitle2" sx={{mb: 1}}>Request</Typography>
                                <HeadersTable headers={data.req.h}/>
                                <Typography variant="subtitle2" sx={{mb: 1}} style={{marginTop: "25px"}}>Response</Typography>
                                <HeadersTable headers={data.res.h}/>
                            </Box>
                        )}

                        {tab === 2 && (
                            <Box>
                                <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{mb: 1}}>
                                    <FormControlLabel
                                        control={<Switch checked={pretty} onChange={(_, v) => setPretty(v)}/>}
                                        label="Pretty"/>
                                    <CopyButton getText={() => prettyReq}/>
                                </Stack>
                                <Box sx={{
                                    bgcolor: '#0f172a',
                                    color: '#e2e8f0',
                                    p: 1.5,
                                    borderRadius: 1,
                                    overflow: 'auto',
                                    maxHeight: '50vh',
                                    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
                                    fontSize: 12,
                                    whiteSpace: 'pre',
                                    wordBreak: 'break-word',
                                }}>
                                    {prettyReq ||
                                        <Typography variant="body2" color="text.secondary">No body</Typography>}
                                </Box>
                            </Box>
                        )}

                        {tab === 3 && (
                            <Box>
                                <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{mb: 1}}>
                                    <FormControlLabel
                                        control={<Switch checked={pretty} onChange={(_, v) => setPretty(v)}/>}
                                        label="Pretty"/>
                                    <CopyButton getText={() => prettyRes}/>
                                </Stack>
                                <Box sx={{
                                    bgcolor: '#0f172a',
                                    color: '#e2e8f0',
                                    p: 1.5,
                                    borderRadius: 1,
                                    overflow: 'auto',
                                    maxHeight: '50vh',
                                    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
                                    fontSize: 12,
                                    whiteSpace: 'pre',
                                    wordBreak: 'break-word',
                                }}>
                                    {prettyRes ||
                                        <Typography variant="body2" color="text.secondary">No body</Typography>}
                                </Box>
                            </Box>
                        )}
                    </Box>
                </>
            )}
        </Box>
    );
}
