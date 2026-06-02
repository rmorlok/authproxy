import React, { useState } from 'react';
import { 
  Card, 
  CardContent, 
  Typography, 
  Chip,
  Box,
  Skeleton,
  CardHeader,
  Avatar,
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
import {tasks, Connection, ConnectionState, ConnectionHealthState, canBeDisconnected, isRedirectResponse, PollForTaskResult, DisconnectResponseJson} from '@authproxy/api';
import { useDispatch } from 'react-redux';
import {
  disconnectConnectionAsync,
  reconfigureConnectionAsync,
  reauthConnectionAsync,
  AppDispatch, addToast, fetchConnectionsAsync,
} from '../store';
import SettingsIcon from '@mui/icons-material/Settings';
import RefreshIcon from '@mui/icons-material/Refresh';
import WarningAmberIcon from '@mui/icons-material/WarningAmber';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { marketplaceTokens } from '../theme';
interface ConnectionCardProps {
  connection: Connection;
}

const truncateText = (text: string, maxLength: number = 120): string => {
  if (text.length <= maxLength) return text;
  return text.substring(0, maxLength).trim() + '...';
};

/**
 * Component to display a single connection with its details
 */
const ConnectionCard: React.FC<ConnectionCardProps> = ({ connection }) => {
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
  const actionsMenuOpen = Boolean(actionsAnchorEl);

  // Format the date
  const createdDate = new Date(connection.created_at).toLocaleDateString();

  // Determine the status color
  let statusColor: 'success' | 'error' | 'warning' | 'default' = marketplaceTokens.status.neutral;
  switch (connection.state) {
    case ConnectionState.CONFIGURED:
      statusColor = marketplaceTokens.status.healthy;
      break;
    case ConnectionState.DISABLED:
      statusColor = marketplaceTokens.status.disabled;
      break;
    case ConnectionState.SETUP:
      statusColor = marketplaceTokens.status.setup;
      break;
    case ConnectionState.DISCONNECTING:
      statusColor = marketplaceTokens.status.setup;
      break;
    default:
      statusColor = marketplaceTokens.status.neutral;
  }

  // Handle reconfigure button click
  const handleReconfigureClick = () => {
    dispatch(reconfigureConnectionAsync(connection.id));
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
      sx={{
        width: '100%',
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        border: 1,
        borderColor: isUnhealthy ? 'warning.main' : marketplaceTokens.card.borderColor,
        borderRadius: marketplaceTokens.radius.card,
        bgcolor: (theme) => (
          isUnhealthy ? alpha(theme.palette.warning.main, 0.08) : theme.palette.background.paper
        ),
        boxShadow: isUnhealthy ? marketplaceTokens.card.attentionShadow : marketplaceTokens.card.shadow,
      }}
    >
      <CardHeader
        sx={{
          alignItems: 'flex-start',
          flexWrap: { xs: 'wrap', sm: 'nowrap' },
          '& .MuiCardHeader-content': {
            minWidth: 0,
          },
          '& .MuiCardHeader-action': {
            ml: 1,
            mt: 0,
          },
        }}
        avatar={
          connector ? (
            <Avatar 
              src={connector.logo} 
              alt={`${connector.display_name} logo`}
              sx={{ width: 40, height: 40 }}
            />
          ) : (
            <Avatar>?</Avatar>
          )
        }
        title={connector ? connector.display_name : 'Unknown Connector'}
        action={(
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <Chip
              label={isUnhealthy ? 'Needs attention' : connection.state}
              color={isUnhealthy ? marketplaceTokens.status.attention : statusColor}
              size="small"
              variant={isUnhealthy ? 'filled' : 'outlined'}
              icon={isUnhealthy ? <WarningAmberIcon /> : undefined}
            />
            {actionMenu}
          </Box>
        )}
        subheader={`Connected on ${createdDate}`}
      />
      <CardContent sx={{ flexGrow: 1 }}>
        {isUnhealthy && (
          <Box
            sx={{
              display: 'flex',
              alignItems: 'flex-start',
              gap: 1,
              mb: 2,
              p: 1.5,
              borderRadius: marketplaceTokens.radius.control,
              bgcolor: (theme) => alpha(theme.palette.warning.main, 0.14),
              color: 'warning.dark',
            }}
          >
            <WarningAmberIcon fontSize="small" sx={{ mt: 0.25 }} />
            <Box>
              <Typography variant="subtitle2" component="p">
                Reauthentication required
              </Typography>
              <Typography variant="body2">
                This connection needs attention. Re-authenticate to restore access.
              </Typography>
            </Box>
          </Box>
        )}
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

      {canBeDisconnected(connection) && (canReconfigure || !isHealthyConfigured) && (
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
          {!isHealthyConfigured && (
            <>
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
      <CardHeader
        avatar={<Skeleton variant="circular" width={40} height={40} />}
        title={<Skeleton variant="text" width="80%" />}
        subheader={<Skeleton variant="text" width="60%" />}
      />
      <CardContent sx={{ flexGrow: 1 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
          <Skeleton variant="text" width="30%" />
          <Skeleton variant="rectangular" width={60} height={24} />
        </Box>
        <Skeleton variant="text" width="100%" />
      </CardContent>
    </Card>
  );
};

export default ConnectionCard;
