import React from 'react';
import {useNavigate, useParams} from 'react-router-dom';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import RequestDetailComponent from '../components/RequestDetail';

export default function RequestDetail() {
    const {id} = useParams();
    const navigate = useNavigate();

    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            {id && (
                <RequestDetailComponent requestId={id}/>
            )}
        </Box>
    );
}
