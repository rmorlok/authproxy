import React from 'react';
import {useParams} from 'react-router-dom';
import Box from '@mui/material/Box';
import ConnectorDetailComponent from '../components/ConnectorDetail';

export default function ConnectorDetail() {
  const { id, version } = useParams();
  return (
    <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
      {id && <ConnectorDetailComponent connectorId={id} initialVersion={version ? Number(version) : undefined} />}
    </Box>
  );
}
