import React, { useEffect, useCallback } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import { Link, useNavigate } from 'react-router-dom';
import {
  Grid,
  Typography,
  Container,
  Alert,
  Box,
  CircularProgress,
  Button,
} from '@mui/material';
import {
  selectConnectors,
  selectConnectorsStatus,
  selectConnectorsError,
  fetchConnectorsAsync,
} from '../store';
import ConnectorCard, { ConnectorCardSkeleton } from './ConnectorCard';
import ConnectionSetupDialog from './ConnectionSetupDialog';
import { AppDispatch } from '../store';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import { marketplaceTokens } from '../theme';
import { useConnectorConnectionFlow } from './useConnectorConnectionFlow';

/**
 * Component to display a list of available connectors
 */
const ConnectorList: React.FC = () => {
  const dispatch = useDispatch<AppDispatch>();
  const navigate = useNavigate();
  const connectors = useSelector(selectConnectors);
  const status = useSelector(selectConnectorsStatus);
  const error = useSelector(selectConnectorsError);
  const {
    cancelForm: handleFormCancel,
    connect: handleConnect,
    currentFormStep,
    formSubmitError,
    isConnecting,
    isSubmittingForm,
    submitForm: handleFormSubmit,
  } = useConnectorConnectionFlow();

  useEffect(() => {
    if (status === 'idle') {
      dispatch(fetchConnectorsAsync());
    }
  }, [status, dispatch]);

  const handleDetails = useCallback((connectorId: string) => {
    navigate(`/connectors/${encodeURIComponent(connectorId)}`);
  }, [navigate]);

  let content;

  if (status === 'loading') {
    content = (
      <Grid container spacing={marketplaceTokens.spacing.gridGap}>
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
      <Box sx={{ textAlign: 'center', py: marketplaceTokens.spacing.pageY }}>
        <Typography variant="h6" color="text.secondary">
          No connectors available
        </Typography>
      </Box>
    );
  } else {
    content = (
      <Grid container spacing={marketplaceTokens.spacing.gridGap}>
        {connectors.map((connector) => (
          <Grid key={connector.id} size={{ xs: 12, sm: 6, md: 4, lg: 3 }}>
            <ConnectorCard
              connector={connector}
              onConnect={handleConnect}
              onDetails={handleDetails}
              isConnecting={isConnecting}
            />
          </Grid>
        ))}
      </Grid>
    );
  }

  return (
    <Container sx={{ py: marketplaceTokens.spacing.pageY }}>
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: { xs: 'flex-start', sm: 'center' },
          flexDirection: { xs: 'column', sm: 'row' },
          gap: marketplaceTokens.spacing.headerGap,
          mb: marketplaceTokens.spacing.sectionGap,
        }}
      >
        <Typography variant="h4" component="h1">
          Available Connectors
        </Typography>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: marketplaceTokens.spacing.headerGap }}>
          {isConnecting && (
            <Box sx={{ display: 'flex', alignItems: 'center' }}>
              <CircularProgress size={24} sx={{ mr: 1 }} />
              <Typography variant="body2" color="text.secondary">
                Connecting...
              </Typography>
            </Box>
          )}
          <Button
            component={Link}
            to="/connections"
            startIcon={<ArrowBackIcon />}
            sx={{ alignSelf: { xs: 'flex-start', sm: 'center' } }}
          >
            Back to Connections
          </Button>
        </Box>
      </Box>
      {content}

      <ConnectionSetupDialog
        currentFormStep={currentFormStep}
        formSubmitError={formSubmitError}
        isSubmittingForm={isSubmittingForm}
        onCancel={handleFormCancel}
        onSubmit={handleFormSubmit}
      />
    </Container>
  );
};

export default ConnectorList;
