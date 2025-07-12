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
  Snackbar,
  Alert
} from '@mui/material';
import { Connection, ConnectionState, TaskState } from '../models';
import { useSelector, useDispatch } from 'react-redux';
import { 
  selectConnectors, 
  disconnectConnectionAsync, 
  pollTaskStatusAsync,
  selectCurrentTaskId
} from '../store';

interface ConnectionCardProps {
  connection: Connection;
}

/**
 * Component to display a single connection with its details
 */
const ConnectionCard: React.FC<ConnectionCardProps> = ({ connection }) => {
  const dispatch = useDispatch();
  const connectors = useSelector(selectConnectors);
  const currentTaskId = useSelector(selectCurrentTaskId);
  const connector = connectors.find(c => c.id === connection.connector_id);

  // State for confirmation dialog
  const [openDialog, setOpenDialog] = useState(false);
  // State for toast notification
  const [openToast, setOpenToast] = useState(false);
  const [toastMessage, setToastMessage] = useState('');
  const [toastSeverity, setToastSeverity] = useState<'success' | 'error'>('success');

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

  // Check if the connection can be disconnected
  const canDisconnect = connection.state !== ConnectionState.DISCONNECTING && 
                        connection.state !== ConnectionState.DISCONNECTED;

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
      await dispatch(disconnectConnectionAsync(connection.id) as any);

      // Set up polling for task status
      if (currentTaskId) {
        const pollInterval = setInterval(async () => {
          const result = await dispatch(pollTaskStatusAsync(currentTaskId) as any);
          if (result.payload.completed) {
            clearInterval(pollInterval);
            setToastMessage('Connection successfully disconnected');
            setToastSeverity('success');
            setOpenToast(true);
          }
        }, 2000); // Poll every 2 seconds
      }
    } catch (error) {
      setToastMessage('Failed to disconnect connection');
      setToastSeverity('error');
      setOpenToast(true);
    }
  };

  // Handle toast close
  const handleToastClose = () => {
    setOpenToast(false);
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
        subheader={`Connected on ${createdDate}`}
      />
      <CardContent sx={{ flexGrow: 1 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
          <Typography variant="body2" color="text.secondary">
            Status:
          </Typography>
          <Chip 
            label={connection.state} 
            color={statusColor} 
            size="small" 
            variant="outlined"
          />
        </Box>
        <Typography variant="body2" color="text.secondary">
          ID: {connection.id}
        </Typography>
      </CardContent>

      {canDisconnect && (
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

      {/* Toast Notification */}
      <Snackbar
        open={openToast}
        autoHideDuration={6000}
        onClose={handleToastClose}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert onClose={handleToastClose} severity={toastSeverity} sx={{ width: '100%' }}>
          {toastMessage}
        </Alert>
      </Snackbar>
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
