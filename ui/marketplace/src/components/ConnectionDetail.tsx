import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useDispatch, useSelector } from 'react-redux';
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Container,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Divider,
  Paper,
  Skeleton,
  Typography,
} from '@mui/material';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import RefreshIcon from '@mui/icons-material/Refresh';
import SettingsIcon from '@mui/icons-material/Settings';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import {
  canBeDisconnected,
  ConnectionState,
  DisconnectResponseJson,
  isCompleteResponse,
  isRedirectResponse,
  PollForTaskResult,
  tasks,
} from '@authproxy/api';
import {
  addToast,
  AppDispatch,
  cancelSetupConnectionAsync,
  clearFormStep,
  disconnectConnectionAsync,
  fetchConnectionsAsync,
  getSetupStepAsync,
  reauthConnectionAsync,
  reconfigureConnectionAsync,
  selectConnections,
  selectConnectionsError,
  selectConnectionsStatus,
  selectCurrentFormStep,
  selectFormSubmitError,
  selectSubmittingForm,
  submitConnectionFormAsync,
} from '../store';
import { marketplaceTokens } from '../theme';
import ConnectionSetupDialog from './ConnectionSetupDialog';
import {
  connectorInitials,
  markdownComponents,
  markdownUrlTransform,
} from './ConnectorDetail';
import { getConnectionStatusPresentation } from './connectionPresentation';

interface ConnectionDetailProps {
  connectionId?: string;
}

const ConnectionDetail: React.FC<ConnectionDetailProps> = ({ connectionId: providedConnectionId }) => {
  const dispatch = useDispatch<AppDispatch>();
  const params = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const connectionId = providedConnectionId ?? params.connectionId;
  const connections = useSelector(selectConnections);
  const status = useSelector(selectConnectionsStatus);
  const error = useSelector(selectConnectionsError);
  const currentFormStep = useSelector(selectCurrentFormStep);
  const isSubmittingForm = useSelector(selectSubmittingForm);
  const formSubmitError = useSelector(selectFormSubmitError);
  const [openDisconnectDialog, setOpenDisconnectDialog] = useState(false);
  const [isResumingSetup, setIsResumingSetup] = useState(false);
  const handledActionRef = useRef<string | null>(null);

  useEffect(() => {
    if (status === 'idle') {
      dispatch(fetchConnectionsAsync());
    }
  }, [dispatch, status]);

  const connection = useMemo(() => (
    connections.find((item) => item.id === connectionId)
  ), [connections, connectionId]);

  const connector = connection?.connector;
  const presentation = connection ? getConnectionStatusPresentation(connection) : null;
  const canReauth =
    connection?.state === ConnectionState.CONFIGURED &&
    (presentation?.requiresReconnection || !presentation?.requiresSetup);
  const canReconfigure = connection?.state === ConnectionState.CONFIGURED && connector?.has_configure && !presentation?.requiresSetup;
  const canDisconnect = connection ? canBeDisconnected(connection) : false;
  const body = connector?.description || connector?.highlight || '';

  const handleReconfigureClick = useCallback(() => {
    if (!connection) return;
    dispatch(reconfigureConnectionAsync(connection.id));
  }, [connection, dispatch]);

  const handleResumeSetupClick = useCallback(() => {
    if (!connection) return;
    setIsResumingSetup(true);
    dispatch(getSetupStepAsync({
      connectionId: connection.id,
      returnToUrl: window.location.href,
    })).then((action) => {
      if (action.meta.requestStatus === 'fulfilled') {
        const response = action.payload as any;
        if (isRedirectResponse(response) && response.redirect_url) {
          window.location.href = response.redirect_url;
          return;
        }
        if (isCompleteResponse(response)) {
          dispatch(fetchConnectionsAsync());
        }
      }
      setIsResumingSetup(false);
    });
  }, [connection, dispatch]);

  const handleReauthClick = useCallback(() => {
    if (!connection) return;
    dispatch(reauthConnectionAsync({
      connectionId: connection.id,
      returnToUrl: window.location.href,
    })).then((action) => {
      if (action.meta.requestStatus === 'fulfilled') {
        const response = action.payload as any;
        if (isRedirectResponse(response)) {
          window.location.href = response.redirect_url;
        }
      }
    });
  }, [connection, dispatch]);

  useEffect(() => {
    if (!connection) {
      return;
    }

    const action = searchParams.get('action');
    if (action !== 'reauth' && action !== 'configure') {
      return;
    }

    const actionKey = `${connection.id}:${action}`;
    if (handledActionRef.current === actionKey) {
      return;
    }
    handledActionRef.current = actionKey;

    const nextSearchParams = new URLSearchParams(searchParams);
    nextSearchParams.delete('action');
    setSearchParams(nextSearchParams, { replace: true });

    if (action === 'reauth') {
      if (canReauth) {
        handleReauthClick();
      }
      return;
    }

    if (presentation?.requiresSetup) {
      handleResumeSetupClick();
    } else if (canReconfigure) {
      handleReconfigureClick();
    }
  }, [
    canReauth,
    canReconfigure,
    connection,
    handleReauthClick,
    handleReconfigureClick,
    handleResumeSetupClick,
    presentation?.requiresSetup,
    searchParams,
    setSearchParams,
  ]);

  const handleDisconnectConfirm = async () => {
    if (!connection) return;
    setOpenDisconnectDialog(false);
    try {
      const disconnectResult = await dispatch(disconnectConnectionAsync(connection.id));
      const responsePayload = disconnectResult.payload;

      if (
        responsePayload &&
        typeof responsePayload === 'object' &&
        'task_id' in responsePayload
      ) {
        const taskPollResult =
          await tasks.pollForTaskFinalized((responsePayload as DisconnectResponseJson).task_id);
        if (taskPollResult.result !== PollForTaskResult.FINALIZED) {
          dispatch(addToast({
            message: 'Error while checking for status of disconnect',
            type: 'warning',
            durationMs: 4000,
          }));
        } else {
          dispatch(addToast({
            message: 'Successfully disconnected connection',
            type: 'success',
            durationMs: 2000,
          }));
        }
      }
    } catch (_error) {
      dispatch(addToast({
        message: 'Failed to disconnect connection',
        type: 'error',
        durationMs: 6000,
      }));
    }

    await dispatch(fetchConnectionsAsync());
  };

  const handleFormSubmit = useCallback((formConnectionId: string, data: unknown) => {
    const stepId = currentFormStep?.stepId ?? '';
    dispatch(submitConnectionFormAsync({
      connectionId: formConnectionId,
      stepId,
      data,
      returnToUrl: window.location.href,
    })).then((action) => {
      if (action.meta.requestStatus === 'fulfilled') {
        const response = action.payload as any;
        if (isRedirectResponse(response)) {
          window.location.href = response.redirect_url;
        } else {
          dispatch(fetchConnectionsAsync());
        }
      }
    });
  }, [dispatch, currentFormStep]);

  const handleFormCancel = useCallback(() => {
    if (connection && connection.state === ConnectionState.CONFIGURED) {
      dispatch(cancelSetupConnectionAsync(connection.id));
    }
    dispatch(clearFormStep());
  }, [dispatch, connection]);

  let content;
  if (status === 'loading' || status === 'idle') {
    content = (
      <Box>
        <Skeleton variant="text" width="45%" height={56} />
        <Skeleton variant="text" width="70%" height={28} />
        <Skeleton variant="rectangular" height={240} sx={{ mt: 4, borderRadius: marketplaceTokens.radius.card }} />
      </Box>
    );
  } else if (status === 'failed') {
    content = <Alert severity="error">{error}</Alert>;
  } else if (!connection || !connector || !presentation) {
    content = <Alert severity="warning">Connection not found.</Alert>;
  } else {
    content = (
      <>
        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: { xs: '1fr', md: 'minmax(0, 1fr) auto' },
            gap: marketplaceTokens.spacing.sectionGap,
            alignItems: 'center',
            mb: marketplaceTokens.spacing.sectionGap,
          }}
        >
          <Box sx={{ display: 'flex', gap: 2.5, alignItems: 'center', minWidth: 0 }}>
            {connector.logo ? (
              <Box
                component="img"
                src={connector.logo}
                alt={`${connector.display_name} logo`}
                sx={{
                  width: 88,
                  height: 88,
                  objectFit: 'contain',
                  bgcolor: 'background.default',
                  border: 1,
                  borderColor: 'divider',
                  borderRadius: marketplaceTokens.radius.card,
                  p: 1.5,
                  flexShrink: 0,
                }}
              />
            ) : (
              <Box
                role="img"
                aria-label={`${connector.display_name} logo`}
                sx={{
                  width: 88,
                  height: 88,
                  borderRadius: marketplaceTokens.radius.card,
                  bgcolor: 'primary.dark',
                  color: 'primary.contrastText',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  flexShrink: 0,
                }}
              >
                <Typography variant="h4" component="span" sx={{ fontWeight: 700 }}>
                  {connectorInitials(connector.display_name)}
                </Typography>
              </Box>
            )}
            <Box sx={{ minWidth: 0 }}>
              <Typography variant="h3" component="h1" sx={{ mb: 1 }}>
                {connector.display_name}
              </Typography>
              <Box
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 0.75,
                  color: presentation.requiresReconnection ? 'error.main' : 'text.secondary',
                }}
              >
                <Box
                  aria-hidden="true"
                  sx={{
                    width: 8,
                    height: 8,
                    borderRadius: '50%',
                    bgcolor: presentation.statusDotColor,
                    flexShrink: 0,
                  }}
                />
                <Typography variant="body1" color="inherit">
                  {presentation.statusText}
                </Typography>
              </Box>
            </Box>
          </Box>
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              flexWrap: 'wrap',
              gap: 1.5,
              justifyContent: { xs: 'flex-start', md: 'flex-end' },
            }}
          >
            {presentation.requiresSetup && (
              <Button
                startIcon={isResumingSetup ? <CircularProgress color="inherit" size={18} /> : <PlayArrowIcon />}
                onClick={handleResumeSetupClick}
                color="warning"
                variant="contained"
                disabled={isResumingSetup}
              >
                {isResumingSetup ? 'Resuming setup...' : 'Resume setup'}
              </Button>
            )}
            {canReconfigure && (
              <Button
                startIcon={<SettingsIcon />}
                onClick={handleReconfigureClick}
              >
                Reconfigure
              </Button>
            )}
            {canReauth && (
              <Button
                startIcon={<RefreshIcon />}
                onClick={handleReauthClick}
                color={presentation.isUnhealthy ? marketplaceTokens.status.attention : 'primary'}
                variant={presentation.isUnhealthy ? 'contained' : 'outlined'}
              >
                Re-authenticate
              </Button>
            )}
            {canDisconnect && (
              <Button
                startIcon={<DeleteOutlineIcon />}
                color="error"
                onClick={() => setOpenDisconnectDialog(true)}
                disabled={connection.state === ConnectionState.DISCONNECTING}
              >
                {connection.state === ConnectionState.DISCONNECTING ? 'Disconnecting...' : 'Disconnect'}
              </Button>
            )}
          </Box>
        </Box>
        <Divider sx={{ mb: marketplaceTokens.spacing.sectionGap }} />
        <Paper
          elevation={0}
          sx={{
            p: { xs: 0, sm: 0 },
            bgcolor: 'transparent',
            '& ul, & ol': { pl: 3, mb: 2 },
            '& li': { marginBottom: 0.75 },
            '& pre': {
              bgcolor: 'action.hover',
              p: 2,
              borderRadius: marketplaceTokens.radius.card,
              overflowX: 'auto',
            },
          }}
        >
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            components={markdownComponents}
            urlTransform={markdownUrlTransform}
          >
            {body}
          </ReactMarkdown>
        </Paper>
      </>
    );
  }

  return (
    <Container sx={{ py: marketplaceTokens.spacing.pageY }}>
      <Box sx={{ mb: marketplaceTokens.spacing.sectionGap }}>
        <Button
          component={Link}
          to="/connections"
          startIcon={<ArrowBackIcon />}
        >
          Back to connections
        </Button>
      </Box>
      {content}
      <ConnectionSetupDialog
        currentFormStep={currentFormStep}
        formSubmitError={formSubmitError}
        isSubmittingForm={isSubmittingForm}
        onCancel={handleFormCancel}
        onSubmit={handleFormSubmit}
      />
      <Dialog
        open={openDisconnectDialog}
        onClose={() => setOpenDisconnectDialog(false)}
      >
        <DialogTitle>Disconnect Confirmation</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Are you sure you want to disconnect from {connector?.display_name || 'this connector'}?
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpenDisconnectDialog(false)}>Cancel</Button>
          <Button onClick={handleDisconnectConfirm} color="error" autoFocus>
            Disconnect
          </Button>
        </DialogActions>
      </Dialog>
    </Container>
  );
};

export default ConnectionDetail;
