import React, {useEffect, useState} from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import Stack from '@mui/material/Stack';
import Chip from '@mui/material/Chip';
import IconButton from '@mui/material/IconButton';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import dayjs from 'dayjs';
import Tooltip from '@mui/material/Tooltip';
import {Actor, actors} from '@authproxy/api';
import AnnotationsEditor from "./AnnotationsEditor";

export default function ActorDetail({actorId}: { actorId: string }) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actor, setActor] = useState<Actor | null>(null);

  const [copied, setCopied] = useState(false);
  const handleCopyId = async () => {
    try {
      await navigator.clipboard.writeText(actor?.id || '');
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch (_e: any) {
      // ignore
    }
  };

  const fetchActor = () => {
    setLoading(true);
    setError(null);
    actors.getById(actorId)
      .then(res => setActor(res.data))
      .catch(err => {
        const msg = err?.response?.data?.error || err.message || 'Failed to load actor';
        setError(msg);
      })
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    actors.getById(actorId)
      .then(res => {
        if (cancelled) return;
        setActor(res.data);
      })
      .catch(err => {
        if (cancelled) return;
        const msg = err?.response?.data?.error || err.message || 'Failed to load actor';
        setError(msg);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [actorId]);

  if (loading) return (<Box sx={{display: 'flex', justifyContent: 'center', p: 4}}><CircularProgress/></Box>);
  if (error) return (<Alert severity="error">{error}</Alert>);
  if (!actor) return null;

  return (
    <Stack spacing={2} sx={{p: 2}}>
      <Typography variant="h5">Actor</Typography>

      <Box>
        <Typography variant="subtitle2" color="text.secondary">ID</Typography>
        <Stack direction="row" spacing={1} alignItems="center" sx={{mt: 0.5}}>
          <Typography
            variant="body1"
            component="code"
            sx={{
              wordBreak: 'break-all',
              fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Roboto Mono", monospace',
              bgcolor: 'action.hover',
              px: 1,
              py: 0.5,
              borderRadius: 0.5,
              fontSize: '0.9rem',
              letterSpacing: '0.02em',
            }}
          >
            {actor.id}
          </Typography>
          <Tooltip title={copied ? 'Copied!' : 'Copy'} placement="top">
            <IconButton size="small" aria-label="Copy actor id" onClick={handleCopyId}>
              <ContentCopyIcon fontSize="inherit" />
            </IconButton>
          </Tooltip>
        </Stack>
      </Box>

      <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">External ID</Typography>
          <Typography variant="body1">{actor.external_id}</Typography>
        </Box>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Namespace</Typography>
          <Typography variant="body1">{actor.namespace}</Typography>
        </Box>
      </Stack>

      <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Created</Typography>
          <Typography variant="body1">{dayjs(actor.created_at).format('MMM DD, YYYY, h:mm A')}</Typography>
        </Box>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Updated</Typography>
          <Typography variant="body1">{dayjs(actor.updated_at).format('MMM DD, YYYY, h:mm A')}</Typography>
        </Box>
      </Stack>

      <Box>
        <Typography variant="subtitle2" color="text.secondary">Labels</Typography>
        {actor.labels && Object.keys(actor.labels).length > 0 ? (
          <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{mt: 0.5}}>
            {Object.entries(actor.labels).map(([key, value]) => (
              <Chip key={key} label={`${key}: ${value}`} size="small" variant="outlined"/>
            ))}
          </Stack>
        ) : (
          <Typography variant="body2" color="text.secondary">No labels</Typography>
        )}
      </Box>

      <AnnotationsEditor
        annotations={actor.annotations}
        onPut={async (key, value) => {
          await actors.putAnnotation(actor.id, key, value);
          fetchActor();
        }}
        onDelete={async (key) => {
          await actors.deleteAnnotation(actor.id, key);
          fetchActor();
        }}
      />
    </Stack>
  );
}
