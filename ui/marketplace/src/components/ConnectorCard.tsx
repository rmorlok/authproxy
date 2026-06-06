import React from 'react';
import {
  Card,
  CardContent,
  CardMedia,
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

interface ConnectorCardProps {
  connector: Connector;
  onConnect: (connectorId: string) => void;
  onDetails?: (connectorId: string) => void;
  isConnecting: boolean;
}

const connectorInitials = (displayName: string): string => {
  const words = displayName
    .split(/[^a-zA-Z0-9]+/)
    .filter(Boolean);

  if (words.length === 0) {
    return 'AP';
  }

  return words.slice(0, 2).map((word) => word[0].toUpperCase()).join('');
};

const ConnectorLogoMedia: React.FC<{connector: Connector}> = ({connector}) => {
  const [logoFailed, setLogoFailed] = React.useState(false);

  React.useEffect(() => {
    setLogoFailed(false);
  }, [connector.logo]);

  if (connector.logo && !logoFailed) {
    return (
      <CardMedia
        component="img"
        image={connector.logo}
        alt={`${connector.display_name} logo`}
        onError={() => setLogoFailed(true)}
        sx={{
          height: marketplaceTokens.card.mediaHeight,
          objectFit: 'contain',
          bgcolor: 'background.default',
          p: 2,
        }}
      />
    );
  }

  return (
    <Box
      role="img"
      aria-label={`${connector.display_name} logo`}
      sx={{
        height: marketplaceTokens.card.mediaHeight,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        bgcolor: 'primary.dark',
        color: 'primary.contrastText',
      }}
    >
      <Typography variant="h3" component="span" sx={{ fontWeight: 700 }}>
        {connectorInitials(connector.display_name)}
      </Typography>
    </Box>
  );
};

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
      <ConnectorLogoMedia connector={connector} />
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
      <CardActions>
        {onDetails && (
          <Button
            size="small"
            color="primary"
            onClick={() => onDetails(connector.id)}
          >
            Details
          </Button>
        )}
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
