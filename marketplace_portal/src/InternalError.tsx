import React from 'react';
import { 
  Typography, 
  Box, 
  Button, 
  Container, 
  Paper,
  Alert
} from "@mui/material";
import { useNavigate } from "react-router-dom";

export function InternalError() {
  const navigate = useNavigate();

  const handleRetry = () => {
    // Reload the page to retry the authentication flow
    window.location.reload();
  };

  const handleGoHome = () => {
    navigate('/');
  };

  return (
    <Container maxWidth="sm" sx={{ mt: 8 }}>
      <Paper sx={{ p: 4, textAlign: 'center' }}>
        <Typography variant="h4" component="h1" gutterBottom color="error">
          Internal Error
        </Typography>
        
        <Alert severity="error" sx={{ mb: 3 }}>
          <Typography variant="body1" gutterBottom>
            An unexpected error occurred while trying to authenticate or load the application.
          </Typography>
          <Typography variant="body2">
            This could be due to a server issue, network problem, or authentication failure.
          </Typography>
        </Alert>

        <Box sx={{ mt: 3, display: 'flex', gap: 2, justifyContent: 'center' }}>
          <Button 
            variant="contained" 
            onClick={handleRetry}
            color="primary"
          >
            Retry
          </Button>
          <Button 
            variant="outlined" 
            onClick={handleGoHome}
          >
            Go Home
          </Button>
        </Box>

        <Typography variant="body2" color="text.secondary" sx={{ mt: 3 }}>
          If the problem persists, please contact support or try again later.
        </Typography>
      </Paper>
    </Container>
  );
}