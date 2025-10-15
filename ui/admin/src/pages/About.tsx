import * as React from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import Link from '@mui/material/Link';

export default function About() {
    return (
        <Box sx={{ width: '100%', maxWidth: { sm: '100%', md: '1700px' } }}>
            <Typography component="h2" variant="h6" sx={{ mb: 2 }}>
                About
            </Typography>
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                <Typography variant="body1">
                    AuthProxy is developed and maintained by <strong>Ryan Morlok</strong>.
                </Typography>
                <Typography variant="body1">
                    Contact: <Link href="mailto:ryan.morlok@morlok.com">ryan.morlok@morlok.com</Link>
                </Typography>
                <Typography variant="body1">
                    Project GitHub: <Link href="https://github.com/rmorlok/authproxy" target="_blank" rel="noopener noreferrer">github.com/rmorlok/authproxy</Link>
                </Typography>
            </Box>
        </Box>
    );
}
