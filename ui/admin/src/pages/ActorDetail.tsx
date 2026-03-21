import React from 'react';
import {useParams} from 'react-router-dom';
import Box from '@mui/material/Box';
import ActorDetailComponent from '../components/ActorDetail';

export default function ActorDetail() {
  const { id } = useParams();
  return (
    <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
      {id && <ActorDetailComponent actorId={id} />}
    </Box>
  );
}
