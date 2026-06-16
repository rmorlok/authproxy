import React from 'react';
import {useParams} from 'react-router-dom';
import Box from '@mui/material/Box';
import KeyDetailComponent from '../components/KeyDetail';

export default function KeyDetail() {
  const { id } = useParams();
  return (
    <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
      {id && <KeyDetailComponent keyId={id} />}
    </Box>
  );
}
