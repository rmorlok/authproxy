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
} from '@mui/material';
import {tasks, Connection, ConnectionState, canBeDisconnected, PollForTaskResult, DisconnectResponseJson} from '../api';
import { useDispatch } from 'react-redux';
import {
  disconnectConnectionAsync,
  AppDispatch, addToast, fetchConnectionsAsync,
} from '../store';
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
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

  // Use highlight field if available, otherwise use truncated description
  const displayText = connector.highlight || truncateText(connector.description);

  // State for confirmation dialog
  const [openDialog, setOpenDialog] = useState(false);

  // Format the date
  const createdDate = new Date(connection.created_at).toLocaleDateString();

  // Determine the status color
  let statusColor: 'success' | 'error' | 'warning' | 'default' = 'default';
  switch (connection.state) {
    case ConnectionState.CONNECTED:
      statusColor = 'success';
      break;
    case ConnectionState.FAILED:
      statusColor = 'error';
      break;
    case ConnectionState.CREATED:
      statusColor = 'warning';
      break;
    case ConnectionState.DISCONNECTING:
      statusColor = 'warning';
      break;
    default:
      statusColor = 'default';
  }

  // Handle disconnect button click
  const handleDisconnectClick = () => {
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

    } catch (error) {
      addToast({
        message: 'Failed to disconnect connection',
        type: 'error',
        durationMs: 6000,
      });
    }

    // Refresh the connections list
    await dispatch(fetchConnectionsAsync());
  };

  return (
    <Card sx={{ maxWidth: 345, height: '100%', display: 'flex', flexDirection: 'column' }}>
      <CardHeader
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
        action={(<Chip
            label={connection.state}
            color={statusColor}
            size="small"
            variant="outlined"
        />)}
        subheader={`Connected on ${createdDate}`}
      />
      <CardContent sx={{ flexGrow: 1 }}>
        <Box sx={{
          '& p': { margin: 0, fontSize: '0.875rem', color: 'text.secondary' },
          '& strong': { color: 'text.primary' },
          '& em': { color: 'text.secondary' },
          '& code': {
            backgroundColor: 'action.hover',
            padding: '2px 4px',
            borderRadius: '4px',
            fontSize: '0.8rem'
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
                  padding: '2px 4px',
                  borderRadius: '4px',
                  fontSize: '0.8rem',
                  fontFamily: 'monospace'
                }}>{children}</Typography>
              }}
          >
            {displayText}
          </ReactMarkdown>
        </Box>
      </CardContent>

      {canBeDisconnected(connection) && (
        <CardActions>
          <Button 
            size="small" 
            color="error" 
            onClick={handleDisconnectClick}
            disabled={connection.state === ConnectionState.DISCONNECTING}
          >
            {connection.state === ConnectionState.DISCONNECTING ? 'Disconnecting...' : 'Disconnect'}
          </Button>
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