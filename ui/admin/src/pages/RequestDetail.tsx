import React from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import RequestDetailComponent from '../components/RequestDetail';

export default function RequestDetail() {
  const { id } = useParams();
  const navigate = useNavigate();

  return (
    <Box sx={{ width: '100%', maxWidth: '100%' }}>
      <Typography component="h2" variant="h6" sx={{ mb: 2 }}>
        Request
      </Typography>
      {id && (
        <RequestDetailComponent requestId={id} fullPage onClose={() => navigate('/requests')} />
      )}
    </Box>
  );
}
