import React from 'react';
import {useParams} from 'react-router-dom';
import Box from '@mui/material/Box';
import RequestDetailComponent from '../components/RequestDetail';

export default function RequestDetail() {
    const {id} = useParams();

    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            {id && (
                <RequestDetailComponent requestId={id}/>
            )}
        </Box>
    );
}
