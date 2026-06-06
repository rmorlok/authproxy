import React from 'react';
import {
  Card,
  CardContent,
  Typography,
  Button,
  CardActions,
  Box,
  Skeleton,
  CardActionArea,
} from '@mui/material';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Connector } from '@authproxy/api';
import { marketplaceTokens } from '../theme';
import ConnectorLogo from './ConnectorLogo';

interface ConnectorCardProps {
  connector: Connector;
  onConnect: (connectorId: string) => void;
  onDetails?: (connectorId: string) => void;
  isConnecting: boolean;
}

/**
 * Component to display a single connector with its details
 */
const ConnectorCard: React.FC<ConnectorCardProps> = ({
  connector,
  onConnect,
  onDetails,
  isConnecting
}) => {
  const displayText = connector.highlight;
  const cardBody = (
    <>
      <ConnectorLogo connector={connector} variant="media" />
      <CardContent sx={{ flexGrow: 1, width: '100%' }}>
        <Typography gutterBottom variant="h5" component="div">
          {connector.display_name}
        </Typography>
        {displayText && (
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
                p: ({ children }) => <Typography variant="body2" color="text.secondary">{children}</Typography>,
                strong: ({ children }) => <Typography component="span" sx={{ fontWeight: 'bold', color: 'text.primary' }}>{children}</Typography>,
                em: ({ children }) => <Typography component="span" sx={{ fontStyle: 'italic', color: 'text.secondary' }}>{children}</Typography>,
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
        )}
      </CardContent>
    </>
  );

  return (
    <Card
      sx={{
        width: '100%',
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        borderRadius: marketplaceTokens.radius.card,
        boxShadow: marketplaceTokens.card.shadow,
      }}
    >
      {onDetails ? (
        <CardActionArea
          onClick={() => onDetails(connector.id)}
          sx={{ flexGrow: 1, alignItems: 'stretch', display: 'flex', flexDirection: 'column' }}
          aria-label={`View ${connector.display_name} details`}
        >
          {cardBody}
        </CardActionArea>
      ) : (
        <Box sx={{ flexGrow: 1, display: 'flex', flexDirection: 'column' }}>
          {cardBody}
        </Box>
      )}
      <CardActions sx={{ justifyContent: 'space-between', px: 2, pb: 2 }}>
        {onDetails && (
          <Button
            size="small"
            color="inherit"
            onClick={() => onDetails(connector.id)}
            sx={{
              color: 'text.secondary',
              '&:hover': {
                bgcolor: 'action.hover',
                color: 'text.primary',
              },
            }}
          >
            Details
          </Button>
        )}
        <Button 
          size="small" 
          color="primary" 
          onClick={() => onConnect(connector.id)}
          disabled={isConnecting}
          sx={{ ml: 'auto' }}
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
    <Card
      sx={{
        maxWidth: 345,
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        borderRadius: marketplaceTokens.radius.card,
      }}
    >
      <Skeleton variant="rectangular" height={marketplaceTokens.card.mediaHeight} />
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
