import React from 'react';
import { Box, Typography } from '@mui/material';
import { Connector } from '@authproxy/api';
import { marketplaceTokens } from '../theme';

type ConnectorLogoShape = Pick<Connector, 'display_name' | 'logo'>;

interface ConnectorLogoProps {
  connector?: ConnectorLogoShape;
  variant?: 'media' | 'compact';
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

const variantSizing = {
  media: {
    outer: {
      width: '100%',
      height: marketplaceTokens.card.mediaHeight,
      p: 2,
    },
    frame: {
      width: '100%',
      height: '100%',
    },
    initialsVariant: 'h3' as const,
  },
  compact: {
    outer: {
      width: 96,
      height: 48,
      p: 0.75,
      flexShrink: 0,
    },
    frame: {
      width: '100%',
      height: '100%',
    },
    initialsVariant: 'subtitle1' as const,
  },
};

const ConnectorLogo: React.FC<ConnectorLogoProps> = ({ connector, variant = 'compact' }) => {
  const [logoFailed, setLogoFailed] = React.useState(false);
  const displayName = connector?.display_name || 'Unknown Connector';
  const logo = connector?.logo;
  const sizing = variantSizing[variant];

  React.useEffect(() => {
    setLogoFailed(false);
  }, [logo]);

  const sharedOuterSx = {
    ...sizing.outer,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    bgcolor: 'background.default',
    borderRadius: marketplaceTokens.radius.card,
    border: 1,
    borderColor: 'divider',
    overflow: 'hidden',
  };

  if (logo && !logoFailed) {
    return (
      <Box sx={sharedOuterSx}>
        <Box
          component="img"
          src={logo}
          alt={`${displayName} logo`}
          onError={() => setLogoFailed(true)}
          sx={{
            ...sizing.frame,
            display: 'block',
            maxWidth: '100%',
            maxHeight: '100%',
            objectFit: 'contain',
          }}
        />
      </Box>
    );
  }

  return (
    <Box
      role="img"
      aria-label={`${displayName} logo`}
      sx={{
        ...sharedOuterSx,
        bgcolor: 'primary.dark',
        color: 'primary.contrastText',
      }}
    >
      <Typography variant={sizing.initialsVariant} component="span" sx={{ fontWeight: 700 }}>
        {connector ? connectorInitials(displayName) : '?'}
      </Typography>
    </Box>
  );
};

export default ConnectorLogo;
