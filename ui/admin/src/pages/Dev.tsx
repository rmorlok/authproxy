import * as React from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';

export default function Dev() {
    return (
        <Box sx={{ p: 4 }}>
            <Typography component="h1" variant="h5" sx={{ mb: 2 }}>
                Dev
            </Typography>
            <Typography variant="body1">
                This page is available only in development mode and bypasses the auth flow.
                Use the browser dev tools to inspect cookies, storage, and network state.
            </Typography>
        </Box>
    );
}
