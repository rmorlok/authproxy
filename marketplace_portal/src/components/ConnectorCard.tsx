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
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Connector } from '../models';

interface ConnectorCardProps {
  connector: Connector;
  onConnect: (connectorId: string) => void;
  isConnecting: boolean;
}

/**
 * Truncate text to fit in card design
 */
const truncateText = (text: string, maxLength: number = 120): string => {
  if (text.length <= maxLength) return text;
  return text.substring(0, maxLength).trim() + '...';
};

/**
 * Component to display a single connector with its details
 */
const ConnectorCard: React.FC<ConnectorCardProps> = ({ 
  connector, 
  onConnect,
  isConnecting
}) => {
  // Use highlight field if available, otherwise use truncated description
  const displayText = connector.highlight || truncateText(connector.description);

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