import React, { useEffect, useCallback } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import {
  Grid,
  Typography,
  Container,
  Alert,
  AlertTitle,
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogTitle,
  DialogContent,
  LinearProgress,
} from '@mui/material';
import { ConnectionState, isCompleteResponse, isRedirectResponse } from '@authproxy/api';
import {
  clearRecentlyCompletedConnection,
  selectConnections,
  selectConnectionsStatus,
  selectConnectionsError,
  fetchConnectionsAsync,
  fetchConnectorsAsync,
  selectConnectors,
  selectConnectorsStatus,
  selectConnectorsError,
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
  cancelSetupConnectionAsync,
  clearVerifyState,
  selectInitiatingConnection,
  initiateConnectionAsync,
  selectRecentlyCompletedConnectionId,
} from '../store';
import ConnectionCard, { ConnectionCardSkeleton } from './ConnectionCard';
import ConnectorCard, { ConnectorCardSkeleton } from './ConnectorCard';
import ConnectionSetupDialog from './ConnectionSetupDialog';
import { AppDispatch } from '../store';
import { Link, useSearchParams } from 'react-router-dom';
import AddIcon from '@mui/icons-material/Add';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';
import HourglassEmptyIcon from '@mui/icons-material/HourglassEmpty';
import { marketplaceTokens } from '../theme';

/**
 * Component to display a list of connections
 */
const ConnectionList: React.FC = () => {
  const dispatch = useDispatch<AppDispatch>();
  const [searchParams, setSearchParams] = useSearchParams();
  const connections = useSelector(selectConnections);
  const status = useSelector(selectConnectionsStatus);
  const error = useSelector(selectConnectionsError);
  const connectors = useSelector(selectConnectors);
  const connectorsStatus = useSelector(selectConnectorsStatus);
  const connectorsError = useSelector(selectConnectorsError);
  const isConnecting = useSelector(selectInitiatingConnection);
  const currentFormStep = useSelector(selectCurrentFormStep);
  const isSubmittingForm = useSelector(selectSubmittingForm);
  const formSubmitError = useSelector(selectFormSubmitError);
  const verifyingConnectionId = useSelector(selectVerifyingConnectionId);
  const verifyError = useSelector(selectVerifyError);
  const isRetrying = useSelector(selectRetryingConnection);
  const recentlyCompletedConnectionId = useSelector(selectRecentlyCompletedConnectionId);

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

  useEffect(() => {
    if (!recentlyCompletedConnectionId) {
      return;
    }

    dispatch(fetchConnectionsAsync());
    const timeout = window.setTimeout(() => {
      dispatch(clearRecentlyCompletedConnection());
    }, 3500);

    return () => window.clearTimeout(timeout);
  }, [dispatch, recentlyCompletedConnectionId]);

  const handleFormSubmit = useCallback((connectionId: string, data: unknown) => {
    const stepId = currentFormStep?.stepId ?? '';
    dispatch(submitConnectionFormAsync({
      connectionId,
      stepId,
      data,
      returnToUrl: window.location.href,
    })).then((action) => {
      if (action.meta.requestStatus === 'fulfilled') {
        const response = action.payload as any;
        if (isRedirectResponse(response)) {
          window.location.href = response.redirect_url;
        } else if (isCompleteResponse(response)) {
          dispatch(fetchConnectionsAsync());
        } else {
          // Refresh connections list to reflect updated state
          dispatch(fetchConnectionsAsync());
        }
      }
    });
  }, [dispatch, currentFormStep]);

  const handleFormCancel = useCallback(() => {
    const connectionId = currentFormStep?.connectionId;
    const conn = connectionId
      ? connections.find((c) => c.id === connectionId)
      : undefined;
    // If the connection is already ready, the form is from a reconfigure flow.
    // Clearing the form step alone leaves setup_step_id set on the server,
    // so the dialog reappears on next load — call cancel_setup to clear it server-side.
    if (conn && conn.state === ConnectionState.CONFIGURED) {
      dispatch(cancelSetupConnectionAsync(conn.id));
    }
    dispatch(clearFormStep());
  }, [dispatch, currentFormStep, connections]);

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

  const handleConnect = useCallback((connectorId: string) => {
    dispatch(initiateConnectionAsync({
      connectorId,
      returnToUrl: `${window.location.origin}/connections`,
    })).then((action) => {
      if (action.meta.requestStatus === 'fulfilled') {
        const response = action.payload as any;
        if (isRedirectResponse(response)) {
          window.location.href = response.redirect_url;
        } else if (isCompleteResponse(response)) {
          dispatch(fetchConnectionsAsync());
        }
      }
    });
  }, [dispatch]);

  const renderConnectorDiscovery = () => {
    if (connectorsStatus === 'loading' || connectorsStatus === 'idle') {
      return (
        <Grid container spacing={marketplaceTokens.spacing.gridGap}>
          {[1, 2, 3, 4].map((i) => (
            <Grid key={`connector-skeleton-${i}`} size={{ xs: 12, sm: 6, md: 4, lg: 3 }}>
              <ConnectorCardSkeleton />
            </Grid>
          ))}
        </Grid>
      );
    }

    if (connectorsStatus === 'failed') {
      return <Alert severity="error">{connectorsError}</Alert>;
    }

    if (connectors.length === 0) {
      return (
        <Box sx={{ py: 3 }}>
          <Typography color="text.secondary">
            No connectors are available right now.
          </Typography>
        </Box>
      );
    }

    return (
      <Grid container spacing={marketplaceTokens.spacing.gridGap}>
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
  };

  let content;

  if (status === 'loading') {
    content = (
      <Grid container spacing={marketplaceTokens.spacing.gridGap}>
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
      <>
        <Box
          sx={{
            border: 1,
            borderColor: marketplaceTokens.card.borderColor,
            borderRadius: marketplaceTokens.radius.panel,
            bgcolor: marketplaceTokens.card.surface,
            mb: marketplaceTokens.spacing.sectionGap,
            p: marketplaceTokens.spacing.panelPadding,
          }}
        >
          <Typography variant="h5" component="h2" gutterBottom>
            Connect your first application
          </Typography>
          <Typography color="text.secondary" sx={{ maxWidth: 680 }}>
            Choose a connector below to create a connection. Once connected, it will appear
            here for ongoing setup, health, and management.
          </Typography>
          {isConnecting && (
            <Box sx={{ display: 'flex', alignItems: 'center', mt: 3 }}>
              <CircularProgress size={24} sx={{ mr: 1 }} />
              <Typography variant="body2" color="text.secondary">
                Starting connection...
              </Typography>
            </Box>
          )}
        </Box>
        <Box>
          <Typography variant="h6" component="h2" sx={{ mb: 2 }}>
            Available connectors
          </Typography>
          {renderConnectorDiscovery()}
        </Box>
      </>
    );
  } else {
    content = (
      <Grid container spacing={marketplaceTokens.spacing.gridGap}>
        {connections.map((connection) => (
          <Grid key={connection.id} size={{ xs: 12, sm: 6, md: 4, lg: 3 }}>
            <ConnectionCard
              connection={connection}
              highlightNew={connection.id === recentlyCompletedConnectionId}
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

      <ConnectionSetupDialog
        currentFormStep={currentFormStep}
        formSubmitError={formSubmitError}
        isSubmittingForm={isSubmittingForm}
        onCancel={handleFormCancel}
        onSubmit={handleFormSubmit}
      />

      <Dialog open={verifyingConnectionId !== null} maxWidth="xs" fullWidth>
        <DialogTitle sx={{ pb: 1 }}>Verifying connection</DialogTitle>
        <DialogContent dividers>
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: marketplaceTokens.spacing.headerGap, py: 3 }}>
            <HourglassEmptyIcon color="primary" sx={{ fontSize: 40 }} />
            <Box sx={{ textAlign: 'center' }}>
              <Typography variant="subtitle1" component="p">
                Checking credentials
              </Typography>
              <Typography variant="body2" color="text.secondary">
                AuthProxy is confirming that this connection can reach the provider.
              </Typography>
            </Box>
            <LinearProgress sx={{ width: '100%' }} />
          </Box>
        </DialogContent>
      </Dialog>

      <Dialog open={verifyError !== null} onClose={handleCancelVerifyError} maxWidth="sm" fullWidth>
        <DialogTitle sx={{ pb: 1 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <ErrorOutlineIcon color="error" />
            <Typography variant="h6" component="span">
              Connection verification failed
            </Typography>
          </Box>
        </DialogTitle>
        <DialogContent dividers>
          <Alert severity="error" sx={{ mb: 2 }}>
            <AlertTitle>Provider check failed</AlertTitle>
            {verifyError?.message ?? 'Verification failed'}
          </Alert>
          <Typography variant="body2" color="text.secondary">
            {verifyError?.canRetry
              ? 'Retry setup to run verification again. Cancel setup deletes this unfinished connection.'
              : 'Cancel setup to delete this unfinished connection, then start again from the connector.'}
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCancelVerifyError} disabled={isRetrying}>
            Cancel setup
          </Button>
          {verifyError?.canRetry && (
            <Button
              onClick={handleRetryVerify}
              disabled={isRetrying}
              variant="contained"
              startIcon={isRetrying ? <CircularProgress size={16} /> : undefined}
            >
              {isRetrying ? 'Retrying setup...' : 'Retry setup'}
            </Button>
          )}
        </DialogActions>
      </Dialog>
    </Container>
  );
};

export default ConnectionList;
