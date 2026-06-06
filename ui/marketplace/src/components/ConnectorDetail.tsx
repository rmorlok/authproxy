import React, { useEffect, useMemo } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useDispatch, useSelector } from 'react-redux';
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Container,
  Divider,
  Paper,
  Skeleton,
  Typography,
} from '@mui/material';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import ConnectionSetupDialog from './ConnectionSetupDialog';
import { AppDispatch, fetchConnectorsAsync } from '../store';
import {
  selectConnectors,
  selectConnectorsError,
  selectConnectorsStatus,
} from '../store';
import { marketplaceTokens } from '../theme';
import { useConnectorConnectionFlow } from './useConnectorConnectionFlow';

interface ConnectorDetailProps {
  connectorId?: string;
}

const markdownComponents = {
  h1: ({ children }: { children?: React.ReactNode }) => <Typography variant="h4" component="h2" sx={{ mt: 4, mb: 2 }}>{children}</Typography>,
  h2: ({ children }: { children?: React.ReactNode }) => <Typography variant="h5" component="h2" sx={{ mt: 4, mb: 2 }}>{children}</Typography>,
  h3: ({ children }: { children?: React.ReactNode }) => <Typography variant="h6" component="h3" sx={{ mt: 3, mb: 1.5 }}>{children}</Typography>,
  p: ({ children }: { children?: React.ReactNode }) => <Typography variant="body1" sx={{ mb: 2, color: 'text.primary' }}>{children}</Typography>,
  a: ({ children, href }: { children?: React.ReactNode; href?: string }) => (
    <Typography component="a" href={href} target="_blank" rel="noreferrer" color="primary" sx={{ fontWeight: 600 }}>
      {children}
    </Typography>
  ),
  table: ({ children }: { children?: React.ReactNode }) => (
    <Box component="table" sx={{
      width: '100%',
      borderCollapse: 'collapse',
      my: 3,
      overflow: 'hidden',
      border: 1,
      borderColor: 'divider',
      borderRadius: marketplaceTokens.radius.card,
    }}>
      {children}
    </Box>
  ),
  th: ({ children }: { children?: React.ReactNode }) => (
    <Box component="th" sx={{ textAlign: 'left', p: 1.5, bgcolor: 'action.hover', borderBottom: 1, borderColor: 'divider' }}>
      <Typography component="span" variant="subtitle2">{children}</Typography>
    </Box>
  ),
  td: ({ children }: { children?: React.ReactNode }) => (
    <Box component="td" sx={{ p: 1.5, borderTop: 1, borderColor: 'divider', verticalAlign: 'top' }}>
      <Typography component="span" variant="body2">{children}</Typography>
    </Box>
  ),
  img: ({ alt, src }: { alt?: string; src?: string }) => (
    <Box
      component="img"
      alt={alt}
      src={src}
      sx={{
        display: 'block',
        width: '100%',
        maxHeight: 320,
        objectFit: 'cover',
        borderRadius: marketplaceTokens.radius.card,
        border: 1,
        borderColor: 'divider',
        my: 3,
      }}
    />
  ),
  code: ({ children }: { children?: React.ReactNode }) => (
    <Typography component="code" sx={{
      bgcolor: 'action.hover',
      borderRadius: marketplaceTokens.radius.control,
      fontFamily: 'monospace',
      fontSize: marketplaceTokens.markdown.codeFontSize,
      px: 0.75,
      py: 0.25,
    }}>
      {children}
    </Typography>
  ),
};

const connectorInitials = (displayName: string): string => {
  const words = displayName.split(/[^a-zA-Z0-9]+/).filter(Boolean);
  if (words.length === 0) {
    return 'AP';
  }
  return words.slice(0, 2).map((word) => word[0].toUpperCase()).join('');
};

const ConnectorDetail: React.FC<ConnectorDetailProps> = ({ connectorId }) => {
  const dispatch = useDispatch<AppDispatch>();
  const params = useParams();
  const selectedConnectorId = connectorId ?? params.connectorId;
  const connectors = useSelector(selectConnectors);
  const status = useSelector(selectConnectorsStatus);
  const error = useSelector(selectConnectorsError);
  const {
    cancelForm,
    connect,
    currentFormStep,
    formSubmitError,
    isConnecting,
    isSubmittingForm,
    submitForm,
  } = useConnectorConnectionFlow();

  useEffect(() => {
    if (status === 'idle') {
      dispatch(fetchConnectorsAsync());
    }
  }, [dispatch, status]);

  const connector = useMemo(() => (
    connectors.find((item) => item.id === selectedConnectorId)
  ), [connectors, selectedConnectorId]);

  const body = connector?.description || connector?.highlight || '';

  let content;
  if (status === 'loading' || status === 'idle') {
    content = (
      <Box>
        <Skeleton variant="text" width="45%" height={56} />
        <Skeleton variant="text" width="70%" height={28} />
        <Skeleton variant="rectangular" height={240} sx={{ mt: 4, borderRadius: marketplaceTokens.radius.card }} />
      </Box>
    );
  } else if (status === 'failed') {
    content = <Alert severity="error">{error}</Alert>;
  } else if (!connector) {
    content = (
      <Alert severity="warning">
        Connector not found.
      </Alert>
    );
  } else {
    content = (
      <>
        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: { xs: '1fr', md: 'minmax(0, 1fr) auto' },
            gap: marketplaceTokens.spacing.sectionGap,
            alignItems: 'center',
            mb: marketplaceTokens.spacing.sectionGap,
          }}
        >
          <Box sx={{ display: 'flex', gap: 2.5, alignItems: 'center', minWidth: 0 }}>
            {connector.logo ? (
              <Box
                component="img"
                src={connector.logo}
                alt={`${connector.display_name} logo`}
                sx={{
                  width: 88,
                  height: 88,
                  objectFit: 'contain',
                  bgcolor: 'background.default',
                  border: 1,
                  borderColor: 'divider',
                  borderRadius: marketplaceTokens.radius.card,
                  p: 1.5,
                  flexShrink: 0,
                }}
              />
            ) : (
              <Box
                role="img"
                aria-label={`${connector.display_name} logo`}
                sx={{
                  width: 88,
                  height: 88,
                  borderRadius: marketplaceTokens.radius.card,
                  bgcolor: 'primary.dark',
                  color: 'primary.contrastText',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  flexShrink: 0,
                }}
              >
                <Typography variant="h4" component="span" sx={{ fontWeight: 700 }}>
                  {connectorInitials(connector.display_name)}
                </Typography>
              </Box>
            )}
            <Box sx={{ minWidth: 0 }}>
              <Typography variant="h3" component="h1" sx={{ mb: 1 }}>
                {connector.display_name}
              </Typography>
              {connector.highlight && (
                <Typography variant="body1" color="text.secondary">
                  {connector.highlight}
                </Typography>
              )}
            </Box>
          </Box>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, justifyContent: { xs: 'flex-start', md: 'flex-end' } }}>
            {isConnecting && <CircularProgress size={22} />}
            <Button
              variant="contained"
              onClick={() => connect(connector.id)}
              disabled={isConnecting}
            >
              Connect
            </Button>
          </Box>
        </Box>
        <Divider sx={{ mb: marketplaceTokens.spacing.sectionGap }} />
        <Paper
          elevation={0}
          sx={{
            p: { xs: 0, sm: 0 },
            bgcolor: 'transparent',
            '& ul, & ol': { pl: 3, mb: 2 },
            '& li': { marginBottom: 0.75 },
            '& pre': {
              bgcolor: 'action.hover',
              p: 2,
              borderRadius: marketplaceTokens.radius.card,
              overflowX: 'auto',
            },
          }}
        >
          <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
            {body}
          </ReactMarkdown>
        </Paper>
      </>
    );
  }

  return (
    <Container sx={{ py: marketplaceTokens.spacing.pageY }}>
      <Box sx={{ mb: marketplaceTokens.spacing.sectionGap }}>
        <Button
          component={Link}
          to="/connectors"
          startIcon={<ArrowBackIcon />}
        >
          Back to connectors
        </Button>
      </Box>
      {content}
      <ConnectionSetupDialog
        currentFormStep={currentFormStep}
        formSubmitError={formSubmitError}
        isSubmittingForm={isSubmittingForm}
        onCancel={cancelForm}
        onSubmit={submitForm}
      />
    </Container>
  );
};

export default ConnectorDetail;
