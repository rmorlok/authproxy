import React, { useEffect, useCallback } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import {
  Grid,
  Typography,
  Container,
  Alert,
  Box,
  Button,
  Dialog,
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
  clearFormStep,
} from '../store';
import ConnectionCard, { ConnectionCardSkeleton } from './ConnectionCard';
import ConnectionFormStep from './ConnectionFormStep';
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
  const currentFormStep = useSelector(selectCurrentFormStep);
  const isSubmittingForm = useSelector(selectSubmittingForm);
  const formSubmitError = useSelector(selectFormSubmitError);

  useEffect(() => {
    if (status === 'idle') {
      dispatch(fetchConnectionsAsync());
    }

    if (connectorsStatus === 'idle') {
      dispatch(fetchConnectorsAsync());
    }
  }, [status, connectorsStatus, dispatch]);

  const handleFormSubmit = useCallback((connectionId: string, data: unknown) => {
    dispatch(submitConnectionFormAsync({ connectionId, data })).then((action) => {
      if (action.meta.requestStatus === 'fulfilled') {
        const response = action.payload as any;
        if (isRedirectResponse(response)) {
          window.location.href = response.redirect_url;
        }
        // If type === 'complete', Redux state clears the form and we should refresh
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
    </Container>
  );
};

export default ConnectionList;
