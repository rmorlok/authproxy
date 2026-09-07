import React, {useEffect, useState} from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import Stack from '@mui/material/Stack';
import IconButton from '@mui/material/IconButton';
import Menu from '@mui/material/Menu';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import dayjs from 'dayjs';
import {API_VERSION, ACTOR_KIND, Actor, actors} from '@authproxy/api';
import AnnotationsEditor from "./AnnotationsEditor";
import ActorPermissionsEditor from './ActorPermissionsEditor';
import ResourceNameEditor from './ResourceNameEditor';
import ResourceIdentifier from './ResourceIdentifier';
import ResourceMetadataMenuItems from './ResourceMetadataMenuItems';
import {ResourceLabels, ResourceNamespace} from './ResourceMetadataFields';

export default function ActorDetail({actorId}: { actorId: string }) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actor, setActor] = useState<Actor | null>(null);
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);

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

  const closeMenu = () => setMenuAnchorEl(null);

  return (
    <Stack spacing={2} sx={{p: 2}}>
      <Stack direction="row" justifyContent="space-between" alignItems="center">
        <Typography variant="h5">Actor</Typography>
        <IconButton aria-label="actions" onClick={(event) => setMenuAnchorEl(event.currentTarget)} size="small">
          <MoreVertIcon/>
        </IconButton>
        <Menu anchorEl={menuAnchorEl} open={Boolean(menuAnchorEl)} onClose={closeMenu} keepMounted>
          <ResourceMetadataMenuItems
            resource="actor"
            name={actor.metadata.name}
            labels={actor.metadata.labels}
            annotations={actor.metadata.annotations}
            onCloseMenu={closeMenu}
            includeRename={false}
            onUpdateLabels={async (labels) => {
              const response = await actors.update(actor.metadata.id, {
                apiVersion: API_VERSION,
                kind: ACTOR_KIND,
                metadata: {labels},
                spec: {},
              });
              setActor(response.data);
            }}
            onUpdateAnnotations={async (annotations) => {
              const response = await actors.update(actor.metadata.id, {
                apiVersion: API_VERSION,
                kind: ACTOR_KIND,
                metadata: {annotations},
                spec: {},
              });
              setActor(response.data);
            }}
          />
        </Menu>
      </Stack>

      <ResourceNameEditor
        name={actor.metadata.name}
        resourceType="Actor"
        onRename={async (name) => {
          const response = await actors.update(actor.metadata.id, {
            apiVersion: API_VERSION,
            kind: ACTOR_KIND,
            metadata: {name},
            spec: {},
          });
          setActor(response.data);
        }}
      />

      <ResourceIdentifier value={actor.metadata.id} copyLabel="Copy actor id"/>

      <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">External ID</Typography>
          <Typography variant="body1">{actor.spec.externalId}</Typography>
        </Box>
        <ResourceNamespace namespace={actor.metadata.namespace}/>
      </Stack>

      <ActorPermissionsEditor
        permissions={actor.spec.permissions}
        onSave={async (permissions) => {
          const response = await actors.update(actor.metadata.id, {
            apiVersion: API_VERSION,
            kind: ACTOR_KIND,
            metadata: {},
            spec: {permissions},
          });
          setActor(response.data);
        }}
      />

      <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Created</Typography>
          <Typography variant="body1">{dayjs(actor.metadata.createdAt).format('MMM DD, YYYY, h:mm A')}</Typography>
        </Box>
        <Box>
          <Typography variant="subtitle2" color="text.secondary">Updated</Typography>
          <Typography variant="body1">{dayjs(actor.metadata.updatedAt).format('MMM DD, YYYY, h:mm A')}</Typography>
        </Box>
      </Stack>

      <ResourceLabels labels={actor.metadata.labels}/>

      <AnnotationsEditor
        annotations={actor.metadata.annotations}
        readOnly
        onPut={async () => {}}
        onDelete={async () => {}}
      />
    </Stack>
  );
}
