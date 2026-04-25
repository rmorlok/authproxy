import React, { useEffect, useCallback } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import {
  Grid,
  Typography,
  Container,
  Alert,
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogTitle,
  DialogContent,
} from '@mui/material';
import { isRedirectResponse } from '@authproxy/api';
import {
  selectConnections,
  selectConnectionsStatus,
  selectConnectionsError,
  fetchConnectionsAsync,
  fetchConnectorsAsync,
  selectConnectorsStatus,
  selectCurrentFormStep,
  selectSubmittingForm,
  selectFormSubmitError,
  submitConnectionFormAsync,
  getSetupStepAsync,
  clearFormStep,
  selectVerifyingConnectionId,
  selectVerifyError,
  selectRetryingConnection,
  retryConnectionAsync,
  abortConnectionAsync,
  clearVerifyState,
} from '../store';
import ConnectionCard, { ConnectionCardSkeleton } from './ConnectionCard';
import ConnectionFormStep from './ConnectionFormStep';
import { AppDispatch } from '../store';
import { Link, useSearchParams } from 'react-router-dom';
import AddIcon from '@mui/icons-material/Add';

/**
 * Component to display a list of connections
 */
const ConnectionList: React.FC = () => {
  const dispatch = useDispatch<AppDispatch>();
  const [searchParams, setSearchParams] = useSearchParams();
  const connections = useSelector(selectConnections);
  const status = useSelector(selectConnectionsStatus);
  const error = useSelector(selectConnectionsError);
  const connectorsStatus = useSelector(selectConnectorsStatus);
  const currentFormStep = useSelector(selectCurrentFormStep);
  const isSubmittingForm = useSelector(selectSubmittingForm);
  const formSubmitError = useSelector(selectFormSubmitError);
  const verifyingConnectionId = useSelector(selectVerifyingConnectionId);
  const verifyError = useSelector(selectVerifyError);
  const isRetrying = useSelector(selectRetryingConnection);

  useEffect(() => {
    if (status === 'idle') {
      dispatch(fetchConnectionsAsync());
    }

    if (connectorsStatus === 'idle') {
      dispatch(fetchConnectorsAsync());
    }
  }, [status, connectorsStatus, dispatch]);

  // After OAuth completes, the callback redirects here with setup=pending&connection_id=...
  // Detect these params and fetch the configure step to show the setup form.
  useEffect(() => {
    const setup = searchParams.get('setup');
    const connectionId = searchParams.get('connection_id');

    if (setup === 'pending' && connectionId) {
      dispatch(getSetupStepAsync(connectionId));
      // Clean up the URL params so a page refresh doesn't re-trigger
      searchParams.delete('setup');
      searchParams.delete('connection_id');
      setSearchParams(searchParams, { replace: true });
    }
  }, [searchParams, setSearchParams, dispatch]);

  // Poll the setup-step endpoint while probes are running so the UI can advance to the
  // next setup step (or surface a failure) as soon as the background task completes.
  useEffect(() => {
    if (!verifyingConnectionId) {
      return;
    }
    const interval = window.setInterval(() => {
      dispatch(getSetupStepAsync(verifyingConnectionId));
    }, 2000);
    return () => window.clearInterval(interval);
  }, [verifyingConnectionId, dispatch]);

  const handleFormSubmit = useCallback((connectionId: string, data: unknown) => {
    const stepId = currentFormStep?.stepId ?? '';
    dispatch(submitConnectionFormAsync({ connectionId, stepId, data })).then((action) => {
      if (action.meta.requestStatus === 'fulfilled') {
        const response = action.payload as any;
        if (isRedirectResponse(response)) {
          window.location.href = response.redirect_url;
        } else {
          // Refresh connections list to reflect updated state
          dispatch(fetchConnectionsAsync());
        }
      }
    });
  }, [dispatch, currentFormStep]);

  const handleFormCancel = useCallback(() => {
    dispatch(clearFormStep());
  }, [dispatch]);

  const handleRetryVerify = useCallback(() => {
    if (!verifyError) return;
    dispatch(retryConnectionAsync({
      connectionId: verifyError.connectionId,
      returnToUrl: window.location.href,
    })).then((action) => {
      if (action.meta.requestStatus === 'fulfilled') {
        const response = action.payload as { type: string; redirect_url?: string };
        if (response.type === 'redirect' && response.redirect_url) {
          window.location.href = response.redirect_url;
        }
      }
    });
  }, [dispatch, verifyError]);

  const handleCancelVerifyError = useCallback(() => {
    if (!verifyError) return;
    dispatch(abortConnectionAsync(verifyError.connectionId)).then(() => {
      dispatch(clearVerifyState());
      dispatch(fetchConnectionsAsync());
    });
  }, [dispatch, verifyError]);

  let content;

  if (status === 'loading') {
    content = (
      <Grid container spacing={4}>
        {[1, 2, 3, 4].map((i) => (
          <Grid key={`connection-skeleton-${i}`} size={{ xs: 12, sm: 6, md: 4, lg: 3 }}>
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
      <Grid container spacing={4} columnSpacing={20}>
        {connections.map((connection) => (
          <Grid key={connection.id} size={{ xs: 12, sm: 6, md: 4, lg: 3 }}>
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
              stepTitle={currentFormStep.stepTitle}
              stepDescription={currentFormStep.stepDescription}
              currentStep={currentFormStep.currentStep}
              totalSteps={currentFormStep.totalSteps}
              jsonSchema={currentFormStep.jsonSchema}
              uiSchema={currentFormStep.uiSchema}
              onSubmit={handleFormSubmit}
              onCancel={handleFormCancel}
              isSubmitting={isSubmittingForm}
            />
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={verifyingConnectionId !== null} maxWidth="xs" fullWidth>
        <DialogTitle>Verifying connection</DialogTitle>
        <DialogContent>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, py: 2 }}>
            <CircularProgress size={24} />
            <Typography variant="body1">
              Checking that your credentials work with the provider…
            </Typography>
          </Box>
        </DialogContent>
      </Dialog>

      <Dialog open={verifyError !== null} onClose={handleCancelVerifyError} maxWidth="sm" fullWidth>
        <DialogTitle>Connection verification failed</DialogTitle>
        <DialogContent>
          <Alert severity="error" sx={{ mb: 2 }}>
            {verifyError?.message ?? 'Verification failed'}
          </Alert>
          <Typography variant="body2" color="text.secondary">
            {verifyError?.canRetry
              ? 'You can retry the setup or cancel to delete this connection.'
              : 'Please cancel and try again from scratch.'}
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCancelVerifyError} disabled={isRetrying}>
            Cancel
          </Button>
          {verifyError?.canRetry && (
            <Button onClick={handleRetryVerify} disabled={isRetrying} variant="contained">
              {isRetrying ? 'Retrying…' : 'Retry'}
            </Button>
          )}
        </DialogActions>
      </Dialog>
    </Container>
  );
};

export default ConnectionList;
