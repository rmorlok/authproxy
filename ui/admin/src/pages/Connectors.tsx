import * as React from 'react';
import Grid from '@mui/material/Grid';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import CustomizedDataGrid from '../components/CustomizedDataGrid';

export default function Connectors() {
    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            <Typography component="h2" variant="h6" sx={{mb: 2}}>
                Connectors
            </Typography>
            <Grid size={{xs: 12, lg: 9}}>
                <CustomizedDataGrid/>
            </Grid>
        </Box>
    );
}
