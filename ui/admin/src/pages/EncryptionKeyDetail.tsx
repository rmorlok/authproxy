import React from 'react';
import {useParams} from 'react-router-dom';
import Box from '@mui/material/Box';
import EncryptionKeyDetailComponent from '../components/EncryptionKeyDetail';

export default function EncryptionKeyDetail() {
  const { id } = useParams();
  return (
    <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
      {id && <EncryptionKeyDetailComponent encryptionKeyId={id} />}
    </Box>
  );
}
