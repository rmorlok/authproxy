import React, {useEffect, useMemo, useState} from 'react';
import {
  Box,
  CircularProgress,
  Divider,
  IconButton,
  Stack,
  Tab,
  Tabs,
  Tooltip,
  Typography,
  Table,
  TableBody,
  TableCell,
  TableRow,
  TableHead,
  Switch,
  FormControlLabel,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import {getRequest, RequestEntry} from '../api';

export interface RequestDetailProps {
  requestId: string;
  onClose?: () => void;
}

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
    return () => { active = false; };
  }, [id]);

  return {data, loading, error};
}

function decodeBase64ToText(b64?: string): string {
  if (!b64) return '';
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
  if (!headers) return false;
  const ct = Object.entries(headers).find(([k]) => k.toLowerCase() === 'content-type')?.[1]?.[0] || '';
  return ct.toLowerCase().includes('application/json') || ct.toLowerCase().includes('+json');
}

function tryFormatJson(text: string): string | null {
  try {
    const obj = JSON.parse(text);
    return JSON.stringify(obj, null, 2);
  } catch {
    return null;
  }
}

function HeadersTable({headers}: {headers: Record<string, string[]>}) {
  const rows = useMemo(() => Object.entries(headers || {}), [headers]);
  if (!rows.length) return <Typography variant="body2" color="text.secondary">No headers</Typography>;
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

function CopyButton({getText}: {getText: () => string}) {
  return (
    <Tooltip title="Copy to clipboard">
      <IconButton size="small" onClick={() => navigator.clipboard.writeText(getText())}>
        <ContentCopyIcon fontSize="inherit" />
      </IconButton>
    </Tooltip>
  );
}

export default function RequestDetail({requestId, onClose}: RequestDetailProps) {
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

  return (
    <Box sx={{width: {xs: '100vw', sm: 520, md: 720}, maxWidth: '100vw'}} role="presentation">
      <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{p: 1.5}}>
        <Typography variant="h6">Request Details</Typography>
        <IconButton onClick={onClose} aria-label="Close details">
          <CloseIcon />
        </IconButton>
      </Stack>
      <Divider />

      {loading && (
        <Stack alignItems="center" justifyContent="center" sx={{p: 3}}>
          <CircularProgress size={24} />
        </Stack>
      )}
      {error && (
        <Typography color="error" sx={{p: 2}}>{error}</Typography>
      )}
      {!loading && !error && data && (
        <>
          <Box sx={{p: 2, pb: 1}}>
            <Stack spacing={1}>
              <Typography variant="subtitle2" color="text.secondary">{data.req.m} {data.req.u}</Typography>
              <Typography variant="body2" color="text.secondary">
                Status: {data.res.sc} • Duration: {data.dur}ms • Req v{data.req.v} • Res v{data.res.v}
              </Typography>
              {data.res.err && (
                <Typography variant="body2" color="error">Error: {data.res.err}</Typography>
              )}
            </Stack>
          </Box>
          <Tabs value={tab} onChange={(_, v) => setTab(v)} variant="scrollable" allowScrollButtonsMobile>
            <Tab label="Overview" />
            <Tab label="Request Headers" />
            <Tab label="Response Headers" />
            <Tab label="Request Body" />
            <Tab label="Response Body" />
          </Tabs>
          <Divider />

          <Box sx={{p: 2}}>
            {tab === 0 && (
              <Stack spacing={1.5}>
                <Typography variant="subtitle2">Overview</Typography>
                <Table size="small">
                  <TableBody>
                    <TableRow><TableCell>ID</TableCell><TableCell>{data.id}</TableCell></TableRow>
                    <TableRow><TableCell>Correlation</TableCell><TableCell>{data.cid}</TableCell></TableRow>
                    <TableRow><TableCell>Timestamp</TableCell><TableCell>{data.ts}</TableCell></TableRow>
                    <TableRow><TableCell>Method</TableCell><TableCell>{data.req.m}</TableCell></TableRow>
                    <TableRow><TableCell>URL</TableCell><TableCell sx={{wordBreak: 'break-all'}}>{data.req.u}</TableCell></TableRow>
                    <TableRow><TableCell>Status</TableCell><TableCell>{data.res.sc}</TableCell></TableRow>
                    {typeof data.req.cl === 'number' && (
                      <TableRow><TableCell>Request Size</TableCell><TableCell>{data.req.cl} bytes</TableCell></TableRow>
                    )}
                    {typeof data.res.cl === 'number' && (
                      <TableRow><TableCell>Response Size</TableCell><TableCell>{data.res.cl} bytes</TableCell></TableRow>
                    )}
                  </TableBody>
                </Table>
              </Stack>
            )}

            {tab === 1 && (
              <Box>
                <Typography variant="subtitle2" sx={{mb: 1}}>Request Headers</Typography>
                <HeadersTable headers={data.req.h} />
              </Box>
            )}
            {tab === 2 && (
              <Box>
                <Typography variant="subtitle2" sx={{mb: 1}}>Response Headers</Typography>
                <HeadersTable headers={data.res.h} />
              </Box>
            )}

            {tab === 3 && (
              <Box>
                <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{mb: 1}}>
                  <FormControlLabel control={<Switch checked={pretty} onChange={(_, v) => setPretty(v)} />} label="Pretty" />
                  <CopyButton getText={() => prettyReq} />
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
                  {prettyReq || <Typography variant="body2" color="text.secondary">No body</Typography>}
                </Box>
              </Box>
            )}

            {tab === 4 && (
              <Box>
                <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{mb: 1}}>
                  <FormControlLabel control={<Switch checked={pretty} onChange={(_, v) => setPretty(v)} />} label="Pretty" />
                  <CopyButton getText={() => prettyRes} />
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
                  {prettyRes || <Typography variant="body2" color="text.secondary">No body</Typography>}
                </Box>
              </Box>
            )}
          </Box>
        </>
      )}
    </Box>
  );
}
