import React, { useState } from 'react';
import { 
  Card, 
  CardContent, 
  Typography, 
  Chip,
  Box,
  Skeleton,
  Button,
  CardActions,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  IconButton,
  ListItemIcon,
  Menu,
  MenuItem,
} from '@mui/material';
import { alpha } from '@mui/material/styles';
import {tasks, Connection, ConnectionState, ConnectionHealthState, canBeDisconnected, isCompleteResponse, isRedirectResponse, PollForTaskResult, DisconnectResponseJson} from '@authproxy/api';
import { useDispatch } from 'react-redux';
import {
  disconnectConnectionAsync,
  getSetupStepAsync,
  reconfigureConnectionAsync,
  reauthConnectionAsync,
  AppDispatch, addToast, fetchConnectionsAsync,
} from '../store';
import SettingsIcon from '@mui/icons-material/Settings';
import RefreshIcon from '@mui/icons-material/Refresh';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { marketplaceTokens } from '../theme';
import ConnectorLogo from './ConnectorLogo';
interface ConnectionCardProps {
  connection: Connection;
  highlightNew?: boolean;
}

const truncateText = (text: string, maxLength: number = 120): string => {
  if (text.length <= maxLength) return text;
  return text.substring(0, maxLength).trim() + '...';
};

/**
 * Component to display a single connection with its details
 */
const ConnectionCard: React.FC<ConnectionCardProps> = ({ connection, highlightNew = false }) => {
  const dispatch = useDispatch<AppDispatch>();
  const connector = connection.connector;

  // Use highlight field if available, otherwise use truncated description.
  // Be defensive in case the connector is missing.
  const displayText = connector?.highlight ?? (
    connector?.description ? truncateText(connector.description) : ''
  );

  // State for confirmation dialog
  const [openDialog, setOpenDialog] = useState(false);
  const [actionsAnchorEl, setActionsAnchorEl] = useState<null | HTMLElement>(null);
  const [isResumingSetup, setIsResumingSetup] = useState(false);
  const actionsMenuOpen = Boolean(actionsAnchorEl);

  // Format the date
  const createdDate = new Date(connection.created_at).toLocaleDateString();

  // Handle reconfigure button click
  const handleReconfigureClick = () => {
    dispatch(reconfigureConnectionAsync(connection.id));
  };

  const handleResumeSetupClick = () => {
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
  };

  // Handle re-authenticate button click. Reauth returns a setup-flow response
  // (form or redirect) — the store's setup-flow handling renders the form
  // dialog; OAuth2 redirects are followed in-page.
  const handleReauthClick = () => {
    handleActionsMenuClose();
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
  };

  // Reauth is meaningful only on Configured connections (any state earlier is still
  // in initial setup; later states are tearing down). Visibility itself is the
  // signal — when health is unhealthy the button is emphasized.
  const canReauth = connection.state === ConnectionState.CONFIGURED;
  const isUnhealthy =
    connection.state === ConnectionState.CONFIGURED &&
    connection.health_state === ConnectionHealthState.UNHEALTHY;
  const isHealthyConfigured =
    connection.state === ConnectionState.CONFIGURED &&
    !isUnhealthy;
  const canReconfigure = connection.state === ConnectionState.CONFIGURED && connector?.has_configure;
  const requiresSetup = connection.state === ConnectionState.SETUP;
  const requiresReconnection = isUnhealthy || connection.state === ConnectionState.DISABLED;
  const statusBadgeLabel = requiresSetup
    ? 'Requires setup'
    : requiresReconnection
      ? 'Requires reconnection'
      : null;
  const statusBadgeColor: 'warning' | 'error' = requiresReconnection ? 'error' : 'warning';
  const statusText = requiresSetup
    ? 'Setup required'
    : requiresReconnection
      ? 'Reconnection required'
      : connection.state === ConnectionState.DISCONNECTING
        ? 'Disconnecting'
        : connection.state === ConnectionState.DISCONNECTED
          ? 'Disconnected'
          : `Connected on ${createdDate}`;
  const statusDotColor = requiresReconnection
    ? 'error.main'
    : requiresSetup || connection.state === ConnectionState.DISCONNECTING
      ? 'warning.main'
      : isHealthyConfigured
        ? 'success.main'
        : 'text.disabled';

  const handleActionsMenuOpen = (event: React.MouseEvent<HTMLElement>) => {
    setActionsAnchorEl(event.currentTarget);
  };

  const handleActionsMenuClose = () => {
    setActionsAnchorEl(null);
  };

  // Handle disconnect button click
  const handleDisconnectClick = () => {
    handleActionsMenuClose();
    setOpenDialog(true);
  };

  // Handle dialog close
  const handleDialogClose = () => {
    setOpenDialog(false);
  };

  // Handle disconnect confirmation
  const handleDisconnectConfirm = async () => {
    setOpenDialog(false);
    try {
      // Dispatch the disconnect action
      const disconnectResult = await dispatch(disconnectConnectionAsync(connection.id));
      const responsePayload = disconnectResult.payload;

      if (responsePayload &&
          typeof responsePayload === 'object' &&
          'task_id' in responsePayload) {

        const taskPollResult =
            await tasks.pollForTaskFinalized((responsePayload as DisconnectResponseJson).task_id);
        if (taskPollResult.result !== PollForTaskResult.FINALIZED) {
            addToast({
              message: 'Error while checking for status of disconnect',
              type: 'warning',
              durationMs: 4000,
            });
        } else {
          addToast({
            message: 'Successfully disconnected connection',
            type: 'success',
            durationMs: 2000,
          });
        }
      }

    } catch (_error) {
      addToast({
        message: 'Failed to disconnect connection',
        type: 'error',
        durationMs: 6000,
      });
    }

    // Refresh the connections list
    await dispatch(fetchConnectionsAsync());
  };

  const actionMenu = isHealthyConfigured ? (
    <>
      <IconButton
        aria-label="Connection actions"
        aria-controls={actionsMenuOpen ? `connection-actions-${connection.id}` : undefined}
        aria-haspopup="menu"
        aria-expanded={actionsMenuOpen ? 'true' : undefined}
        size="small"
        onClick={handleActionsMenuOpen}
      >
        <MoreVertIcon />
      </IconButton>
      <Menu
        id={`connection-actions-${connection.id}`}
        anchorEl={actionsAnchorEl}
        open={actionsMenuOpen}
        onClose={handleActionsMenuClose}
      >
        {canReauth && (
          <MenuItem onClick={handleReauthClick}>
            <ListItemIcon>
              <RefreshIcon fontSize="small" />
            </ListItemIcon>
            Re-authenticate
          </MenuItem>
        )}
        <MenuItem
          onClick={handleDisconnectClick}
          disabled={connection.state === ConnectionState.DISCONNECTING}
          sx={{ color: 'error.main' }}
        >
          <ListItemIcon sx={{ color: 'error.main' }}>
            <DeleteOutlineIcon fontSize="small" />
          </ListItemIcon>
          {connection.state === ConnectionState.DISCONNECTING ? 'Disconnecting...' : 'Disconnect'}
        </MenuItem>
      </Menu>
    </>
  ) : null;

  return (
    <Card
      data-testid={`connection-card-${connection.id}`}
      data-highlight-new={highlightNew ? 'true' : undefined}
      sx={{
        width: '100%',
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        border: 1,
        borderColor: isUnhealthy ? 'warning.main' : highlightNew ? 'primary.main' : marketplaceTokens.card.borderColor,
        borderRadius: marketplaceTokens.radius.card,
        bgcolor: (theme) => (
          isUnhealthy
            ? alpha(theme.palette.warning.main, 0.08)
            : highlightNew
              ? alpha(theme.palette.primary.main, 0.06)
              : theme.palette.background.paper
        ),
        boxShadow: isUnhealthy || highlightNew ? marketplaceTokens.card.attentionShadow : marketplaceTokens.card.shadow,
        transition: 'background-color 500ms ease, border-color 500ms ease, box-shadow 500ms ease',
      }}
    >
      <Box sx={{ position: 'relative' }}>
        <ConnectorLogo connector={connector} variant="media" />
        {statusBadgeLabel && (
          <Box sx={{ position: 'absolute', top: 12, right: 12 }}>
            <Chip
              label={statusBadgeLabel}
              color={statusBadgeColor}
              size="small"
              variant="filled"
              sx={{
                boxShadow: 2,
                bgcolor: (theme) => alpha(theme.palette[statusBadgeColor].main, 0.92),
              }}
            />
          </Box>
        )}
      </Box>
      <CardContent sx={{ flexGrow: 1, width: '100%', boxSizing: 'border-box' }}>
        <Typography gutterBottom variant="h5" component="div">
          {connector ? connector.display_name : 'Unknown Connector'}
        </Typography>
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 0.75,
            mb: displayText ? 2 : 0,
            color: requiresReconnection ? 'error.main' : 'text.secondary',
          }}
        >
          <Box
            aria-hidden="true"
            sx={{
              width: 8,
              height: 8,
              borderRadius: '50%',
              bgcolor: statusDotColor,
              flexShrink: 0,
            }}
          />
          <Typography variant="body2" color="inherit">
            {statusText}
          </Typography>
        </Box>
        <Box sx={{
          '& p': { margin: 0, fontSize: marketplaceTokens.markdown.bodyFontSize, color: 'text.secondary' },
          '& strong': { color: 'text.primary' },
          '& em': { color: 'text.secondary' },
          '& code': {
            backgroundColor: 'action.hover',
            padding: marketplaceTokens.markdown.codePadding,
            borderRadius: marketplaceTokens.radius.control,
            fontSize: marketplaceTokens.markdown.codeFontSize
          }
        }}>
          <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              components={{
                // Override paragraph to remove default margins
                p: ({ children }) => <Typography variant="body2" color="text.secondary">{children}</Typography>,
                // Override strong to use primary color
                strong: ({ children }) => <Typography component="span" sx={{ fontWeight: 'bold', color: 'text.primary' }}>{children}</Typography>,
                // Override em to use secondary color
                em: ({ children }) => <Typography component="span" sx={{ fontStyle: 'italic', color: 'text.secondary' }}>{children}</Typography>,
                // Override code to use custom styling
                code: ({ children }) => <Typography component="code" sx={{
                  backgroundColor: 'action.hover',
                  padding: marketplaceTokens.markdown.codePadding,
                  borderRadius: marketplaceTokens.radius.control,
                  fontSize: marketplaceTokens.markdown.codeFontSize,
                  fontFamily: 'monospace'
                }}>{children}</Typography>
              }}
          >
            {displayText}
          </ReactMarkdown>
        </Box>
      </CardContent>

      {canBeDisconnected(connection) && (canReconfigure || actionMenu || !isHealthyConfigured) && (
        <CardActions
          sx={{
            alignItems: 'flex-start',
            flexDirection: isHealthyConfigured ? 'row' : 'column',
            flexWrap: 'wrap',
            justifyContent: isHealthyConfigured ? 'space-between' : 'flex-start',
            gap: marketplaceTokens.spacing.cardActionGap,
            '& .MuiButton-root': {
              ml: '0 !important',
            },
          }}
        >
          {canReconfigure && (
            <Button
              size="small"
              startIcon={<SettingsIcon />}
              onClick={handleReconfigureClick}
            >
              Reconfigure
            </Button>
          )}
          {actionMenu && (
            <Box sx={{ ml: 'auto' }}>
              {actionMenu}
            </Box>
          )}
          {!isHealthyConfigured && (
            <>
              {requiresSetup && (
                <Button
                  size="medium"
                  startIcon={<PlayArrowIcon />}
                  onClick={handleResumeSetupClick}
                  color="warning"
                  variant="contained"
                  fullWidth
                  disabled={isResumingSetup}
                  sx={{ justifyContent: 'flex-start' }}
                >
                  {isResumingSetup ? 'Resuming setup...' : 'Resume setup'}
                </Button>
              )}
              {canReauth && (
                <Button
                  size={isUnhealthy ? 'medium' : 'small'}
                  startIcon={<RefreshIcon />}
                  onClick={handleReauthClick}
                  color={isUnhealthy ? marketplaceTokens.status.attention : 'primary'}
                  variant={isUnhealthy ? 'contained' : 'text'}
                  fullWidth={isUnhealthy}
                  sx={{ justifyContent: isUnhealthy ? 'flex-start' : 'center' }}
                >
                  Re-authenticate
                </Button>
              )}
              <Button
                size="small"
                color="error"
                onClick={handleDisconnectClick}
                disabled={connection.state === ConnectionState.DISCONNECTING}
              >
                {connection.state === ConnectionState.DISCONNECTING ? 'Disconnecting...' : 'Disconnect'}
              </Button>
            </>
          )}
        </CardActions>
      )}

      {/* Confirmation Dialog */}
      <Dialog
        open={openDialog}
        onClose={handleDialogClose}
      >
        <DialogTitle>Disconnect Confirmation</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Are you sure you want to disconnect from {connector?.display_name || 'this connector'}?
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleDialogClose}>Cancel</Button>
          <Button onClick={handleDisconnectConfirm} color="error" autoFocus>
            Disconnect
          </Button>
        </DialogActions>
      </Dialog>
    </Card>
  );
};

/**
 * Skeleton loader for the ConnectionCard
 */
export const ConnectionCardSkeleton: React.FC = () => {
  return (
    <Card sx={{ maxWidth: 345, height: '100%', display: 'flex', flexDirection: 'column' }}>
      <Skeleton variant="rectangular" height={marketplaceTokens.card.mediaHeight} />
      <CardContent sx={{ flexGrow: 1 }}>
        <Skeleton variant="text" height={32} width="80%" />
        <Skeleton variant="text" width="60%" />
        <Skeleton variant="text" width="100%" />
      </CardContent>
    </Card>
  );
};

export default ConnectionCard;
