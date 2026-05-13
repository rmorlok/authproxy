import React from 'react';
import { useParams } from 'react-router-dom';
import Box from '@mui/material/Box';
import RateLimitDetailComponent from '../components/RateLimitDetail';

export default function RateLimitDetail() {
    const { id } = useParams();
    return (
        <Box sx={{ width: '100%', maxWidth: { sm: '100%', md: '1700px' } }}>
            {id && <RateLimitDetailComponent rateLimitId={id} />}
        </Box>
    );
}
