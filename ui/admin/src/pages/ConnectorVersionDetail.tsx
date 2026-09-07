import React from 'react';
import {useParams} from 'react-router-dom';
import Box from '@mui/material/Box';
import ConnectorVersionDetailComponent from '../components/ConnectorVersionDetail';

export default function ConnectorVersionDetail() {
  const { id, generation } = useParams();
  if (!id || !generation || isNaN(Number(generation))) {
    return null;
  }

  return (
    <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
        Here
      <ConnectorVersionDetailComponent connectorId={id} version={Number(generation)} />
    </Box>
  );
}
