import React from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import RequestDetail from '../components/RequestDetail';

export default function RequestDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();

  return (
    <Box sx={{ width: '100%', maxWidth: { sm: '100%', md: '1700px' } }}>
      <Typography component="h2" variant="h6" sx={{ mb: 2 }}>
        Request
      </Typography>
      {id && (
        <RequestDetail requestId={id} onClose={() => navigate('/requests')} />
      )}
    </Box>
  );
}
