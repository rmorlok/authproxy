import React from 'react';
import {useParams} from 'react-router-dom';
import Box from '@mui/material/Box';
import ConnectorVersionDetailComponent from '../components/ConnectorVersionDetail';

export default function ConnectorVersionDetail() {
  const { id, version } = useParams();
  if (!id || !version || isNaN(Number(version))) {
    return null;
  }

  return (
    <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
        Here
      <ConnectorVersionDetailComponent connectorId={id} version={Number(version)} />
    </Box>
  );
}
