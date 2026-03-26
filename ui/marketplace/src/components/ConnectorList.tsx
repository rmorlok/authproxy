import React, { useEffect, useCallback } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import {
  Grid,
  Typography,
  Container,
  Alert,
  Box,
  CircularProgress,
  Dialog,
  DialogTitle,
  DialogContent,
} from '@mui/material';
import { isRedirectResponse } from '@authproxy/api';
import {
  selectConnectors,
  selectConnectorsStatus,
  selectConnectorsError,
  fetchConnectorsAsync,
  selectInitiatingConnection,
  selectCurrentFormStep,
  selectSubmittingForm,
  selectFormSubmitError,
  clearFormStep,
  submitConnectionFormAsync,
} from '../store';
import ConnectorCard, { ConnectorCardSkeleton } from './ConnectorCard';
import ConnectionFormStep from './ConnectionFormStep';
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
  const currentFormStep = useSelector(selectCurrentFormStep);
  const isSubmittingForm = useSelector(selectSubmittingForm);
  const formSubmitError = useSelector(selectFormSubmitError);

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
        const response = action.payload as any;
        if (isRedirectResponse(response)) {
          window.location.href = response.redirect_url;
        }
        // If type === 'form', Redux state update triggers the form dialog
      }
    });
  };

  const handleFormSubmit = useCallback((connectionId: string, data: unknown) => {
    dispatch(submitConnectionFormAsync({ connectionId, data })).then((action) => {
      if (action.meta.requestStatus === 'fulfilled') {
        const response = action.payload as any;
        if (isRedirectResponse(response)) {
          window.location.href = response.redirect_url;
        }
        // If type === 'form', Redux state update shows the next form step
        // If type === 'complete', Redux state clears the form
      }
    });
  }, [dispatch]);

  const handleFormCancel = useCallback(() => {
    dispatch(clearFormStep());
  }, [dispatch]);

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

      <Dialog
        open={currentFormStep !== null}
        onClose={handleFormCancel}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Connection Setup</DialogTitle>
        <DialogContent>
          {formSubmitError && (
            <Alert severity="error" sx={{ mb: 2 }}>{formSubmitError}</Alert>
          )}
          {currentFormStep && (
            <ConnectionFormStep
              connectionId={currentFormStep.connectionId}
              jsonSchema={currentFormStep.jsonSchema}
              uiSchema={currentFormStep.uiSchema}
              onSubmit={handleFormSubmit}
              onCancel={handleFormCancel}
              isSubmitting={isSubmittingForm}
            />
          )}
        </DialogContent>
      </Dialog>
    </Container>
  );
};

export default ConnectorList;
