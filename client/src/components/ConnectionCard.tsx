import React from 'react';
import { 
  Card, 
  CardContent, 
  Typography, 
  Chip,
  Box,
  Skeleton,
  CardHeader,
  Avatar
} from '@mui/material';
import { Connection, ConnectionState } from '../models';
import { useSelector } from 'react-redux';
import { selectConnectors } from '../store';

interface ConnectionCardProps {
  connection: Connection;
}

/**
 * Component to display a single connection with its details
 */
const ConnectionCard: React.FC<ConnectionCardProps> = ({ connection }) => {
  const connectors = useSelector(selectConnectors);
  const connector = connectors.find(c => c.id === connection.connector_id);

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
    default:
      statusColor = 'default';
  }

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