import React from 'react';
import { 
  Card, 
  CardContent, 
  CardMedia, 
  Typography, 
  Button, 
  CardActions,
  Box,
  Skeleton
} from '@mui/material';
import { Connector } from '../models';

interface ConnectorCardProps {
  connector: Connector;
  onConnect: (connectorId: string) => void;
  isConnecting: boolean;
}

/**
 * Component to display a single connector with its details
 */
const ConnectorCard: React.FC<ConnectorCardProps> = ({ 
  connector, 
  onConnect,
  isConnecting
}) => {
  return (
    <Card sx={{ maxWidth: 345, height: '100%', display: 'flex', flexDirection: 'column' }}>
      <CardMedia
        component="img"
        height="140"
        image={connector.logo}
        alt={`${connector.display_name} logo`}
      />
      <CardContent sx={{ flexGrow: 1 }}>
        <Typography gutterBottom variant="h5" component="div">
          {connector.display_name}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {connector.description}
        </Typography>
      </CardContent>
      <CardActions>
        <Button 
          size="small" 
          color="primary" 
          onClick={() => onConnect(connector.id)}
          disabled={isConnecting}
        >
          Connect
        </Button>
      </CardActions>
    </Card>
  );
};

/**
 * Skeleton loader for the ConnectorCard
 */
export const ConnectorCardSkeleton: React.FC = () => {
  return (
    <Card sx={{ maxWidth: 345, height: '100%', display: 'flex', flexDirection: 'column' }}>
      <Skeleton variant="rectangular" height={140} />
      <CardContent sx={{ flexGrow: 1 }}>
        <Skeleton variant="text" height={32} width="80%" />
        <Box sx={{ mt: 1 }}>
          <Skeleton variant="text" height={20} />
          <Skeleton variant="text" height={20} />
          <Skeleton variant="text" height={20} width="60%" />
        </Box>
      </CardContent>
      <CardActions>
        <Skeleton variant="rectangular" width={80} height={30} />
      </CardActions>
    </Card>
  );
};

export default ConnectorCard;