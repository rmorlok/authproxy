import React from 'react';
import {useParams} from 'react-router-dom';
import Box from '@mui/material/Box';
import ConnectionDetailComponent from '../components/ConnectionDetail';

export default function ConnectionDetail() {
  const { id } = useParams();
  return (
    <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
      {id && <ConnectionDetailComponent connectionId={id} />}
    </Box>
  );
}
