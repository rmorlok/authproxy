import React, { useEffect } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import { 
  Grid, 
  Typography, 
  Container, 
  Alert, 
  Box,
  CircularProgress
} from '@mui/material';
import { 
  selectConnectors, 
  selectConnectorsStatus, 
  selectConnectorsError,
  fetchConnectorsAsync,
  selectInitiatingConnection
} from '../store';
import ConnectorCard, { ConnectorCardSkeleton } from './ConnectorCard';
import { AppDispatch } from '../store';
import { initiateConnectionAsync } from '../store';

/**
 * Component to display a list of available connectors
 */
const ConnectorList: React.FC = () => {
  const dispatch = useDispatch<AppDispatch>();
  const connectors = useSelector(selectConnectors);
  const status = useSelector(selectConnectorsStatus);
  const error = useSelector(selectConnectorsError);
  const isConnecting = useSelector(selectInitiatingConnection);

  useEffect(() => {
    if (status === 'idle') {
      dispatch(fetchConnectorsAsync());
    }
  }, [status, dispatch]);

  const handleConnect = (connectorId: string) => {
    // Get the current URL to use as the return URL
    const returnToUrl = `${window.location.origin}/connections`;
    
    dispatch(initiateConnectionAsync({ 
      connectorId, 
      returnToUrl 
    })).then((action) => {
      if (action.meta.requestStatus === 'fulfilled') {
        // Redirect to the OAuth flow
        const response = action.payload as any;
        if (response.type === 'redirect' && response.redirect_url) {
          window.location.href = response.redirect_url;
        }
      }
    });
  };

  let content;

  if (status === 'loading') {
    content = (
      <Grid container spacing={4}>
        {[1, 2, 3, 4].map((item) => (
          <Grid key={item} size={{ xs: 12, sm: 6, md: 4, lg: 3 }}>
            <ConnectorCardSkeleton />
          </Grid>
        ))}
      </Grid>
    );
  } else if (status === 'failed') {
    content = <Alert severity="error">{error}</Alert>;
  } else if (connectors.length === 0) {
    content = (
      <Box sx={{ textAlign: 'center', py: 4 }}>
        <Typography variant="h6" color="text.secondary">
          No connectors available
        </Typography>
      </Box>
    );
  } else {
    content = (
      <Grid container spacing={4} columnSpacing={20}>
        {connectors.map((connector) => (
          <Grid key={connector.id} size={{ xs: 12, sm: 6, md: 4, lg: 3 }}>
            <ConnectorCard 
              connector={connector} 
              onConnect={handleConnect}
              isConnecting={isConnecting}
            />
          </Grid>
        ))}
      </Grid>
    );
  }

  return (
    <Container sx={{ py: 4 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 4 }}>
        <Typography variant="h4" component="h1">
          Available Connectors
        </Typography>
        {isConnecting && (
          <Box sx={{ display: 'flex', alignItems: 'center' }}>
            <CircularProgress size={24} sx={{ mr: 1 }} />
            <Typography variant="body2" color="text.secondary">
              Connecting...
            </Typography>
          </Box>
        )}
      </Box>
      {content}
    </Container>
  );
};

export default ConnectorList;