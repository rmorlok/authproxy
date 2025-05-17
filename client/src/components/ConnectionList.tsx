import React, { useEffect } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import { 
  Grid, 
  Typography, 
  Container, 
  Alert, 
  Box,
  Button
} from '@mui/material';
import { 
  selectConnections, 
  selectConnectionsStatus, 
  selectConnectionsError,
  fetchConnectionsAsync,
  fetchConnectorsAsync,
  selectConnectorsStatus
} from '../store';
import ConnectionCard, { ConnectionCardSkeleton } from './ConnectionCard';
import { AppDispatch } from '../store';
import { Link } from 'react-router-dom';
import AddIcon from '@mui/icons-material/Add';

/**
 * Component to display a list of connections
 */
const ConnectionList: React.FC = () => {
  const dispatch = useDispatch<AppDispatch>();
  const connections = useSelector(selectConnections);
  const status = useSelector(selectConnectionsStatus);
  const error = useSelector(selectConnectionsError);
  const connectorsStatus = useSelector(selectConnectorsStatus);

  useEffect(() => {
    if (status === 'idle') {
      dispatch(fetchConnectionsAsync());
    }
    
    if (connectorsStatus === 'idle') {
      dispatch(fetchConnectorsAsync());
    }
  }, [status, connectorsStatus, dispatch]);

  let content;

  if (status === 'loading') {
    content = (
      <Grid container spacing={4}>
        {[1, 2, 3, 4].map((item) => (
          <Grid item key={item} xs={12} sm={6} md={4} lg={3}>
            <ConnectionCardSkeleton />
          </Grid>
        ))}
      </Grid>
    );
  } else if (status === 'failed') {
    content = <Alert severity="error">{error}</Alert>;
  } else if (connections.length === 0) {
    content = (
      <Box sx={{ textAlign: 'center', py: 4 }}>
        <Typography variant="h6" color="text.secondary" gutterBottom>
          No connections yet
        </Typography>
        <Button 
          variant="contained" 
          color="primary" 
          startIcon={<AddIcon />}
          component={Link}
          to="/connectors"
          sx={{ mt: 2 }}
        >
          Connect an Application
        </Button>
      </Box>
    );
  } else {
    content = (
      <Grid container spacing={4}>
        {connections.map((connection) => (
          <Grid item key={connection.id} xs={12} sm={6} md={4} lg={3}>
            <ConnectionCard connection={connection} />
          </Grid>
        ))}
      </Grid>
    );
  }

  return (
    <Container sx={{ py: 4 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 4 }}>
        <Typography variant="h4" component="h1">
          Your Connections
        </Typography>
        {connections.length > 0 && (
          <Button 
            variant="contained" 
            color="primary" 
            startIcon={<AddIcon />}
            component={Link}
            to="/connectors"
          >
            Connect More
          </Button>
        )}
      </Box>
      {content}
    </Container>
  );
};

export default ConnectionList;