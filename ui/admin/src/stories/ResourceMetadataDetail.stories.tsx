import * as React from 'react';
import type {Meta, StoryObj} from '@storybook/react';
import Box from '@mui/material/Box';
import {configureClient} from '@authproxy/api';
import NamespaceDetail from '../components/NamespaceDetail';
import ConnectorDetail from '../components/ConnectorDetail';
import ConnectionDetail from '../components/ConnectionDetail';
import KeyDetail from '../components/KeyDetail';
import RateLimitDetail from '../components/RateLimitDetail';

const namespace = {
  path: 'root.payments',
  name: 'payments',
  state: 'active',
  labels: {team: 'payments', 'apxy/ns/team': 'platform'},
  annotations: {owner: 'Payments Platform', runbook: 'go/payments'},
  createdAt: '2026-08-05T15:30:00Z',
  updatedAt: '2026-08-05T15:45:00Z',
};

const connector = {
  id: 'cxr_payments_stripe',
  version: 3,
  namespace: 'root.payments',
  name: 'stripe',
  state: 'primary',
  displayName: 'Stripe',
  description: 'Payment processing for the Acme platform.',
  highlight: 'Production connector',
  logo: '',
  hasConfigure: true,
  labels: {provider: 'stripe', 'apxy/ns/team': 'payments'},
  annotations: {owner: 'Payments Platform', runbook: 'go/payments/stripe'},
  createdAt: '2026-08-05T15:30:00Z',
  updatedAt: '2026-08-05T15:45:00Z',
};

const connection = {
  id: 'cxn_payments_production',
  namespace: 'root.payments',
  name: 'stripe-production',
  state: 'configured',
  healthState: 'healthy',
  connector,
  labels: {environment: 'production', 'apxy/ns/team': 'payments'},
  annotations: {owner: 'Payments Platform', runbook: 'go/payments/stripe'},
  createdAt: '2026-08-05T15:30:00Z',
  updatedAt: '2026-08-05T15:45:00Z',
};

const key = {
  id: 'key_payments_primary',
  namespace: 'root.payments',
  name: 'payments-primary',
  state: 'active',
  keyData: {type: 'aes-gcm'},
  labels: {environment: 'production', 'apxy/ns/team': 'payments'},
  annotations: {owner: 'Payments Platform', rotation: 'quarterly'},
  createdAt: '2026-08-05T15:30:00Z',
  updatedAt: '2026-08-05T15:45:00Z',
};

const rateLimit = {
  apiVersion: 'authproxy.net/v1alpha1',
  kind: 'RateLimit',
  metadata: {
    id: 'rl_payments_public_api',
    namespace: 'root.payments',
    name: 'public-api',
    labels: {scope: 'public-api', 'apxy/ns/team': 'payments'},
    annotations: {owner: 'Payments Platform', runbook: 'go/payments/rate-limits'},
    createdAt: '2026-08-05T15:30:00Z',
    updatedAt: '2026-08-05T15:45:00Z',
  },
  spec: {
    mode: 'enforce',
    scope: {
      connectorRef: {
        apiVersion: 'authproxy.net/v1alpha1',
        kind: 'Connector',
        id: 'cxr_payments_stripe',
        generation: 3,
      },
    },
    selector: {methods: ['GET', 'POST'], requestTypes: ['proxy']},
    bucket: {dimensions: ['actor']},
    algorithm: {tokenBucket: {capacity: 100, refillRate: 20}},
  },
  status: {effectiveMode: 'enforce'},
};

const connectorVersion = {
  ...connector,
  definition: {displayName: 'Stripe', setupFlow: {steps: []}},
};

configureClient({
  axiosConfigOverride: {
    adapter: async (config) => ({
      data: responseData(config.url || ''),
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }),
  },
});

const meta = {
  title: 'Admin/Resource Metadata Detail Pages',
  parameters: {layout: 'fullscreen'},
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const Namespace: Story = {
  render: () => <DetailCanvas><NamespaceDetail namespacePath={namespace.path}/></DetailCanvas>,
};

export const Connector: Story = {
  render: () => <DetailCanvas><ConnectorDetail connectorId={connector.id}/></DetailCanvas>,
};

export const Connection: Story = {
  render: () => <DetailCanvas><ConnectionDetail connectionId={connection.id}/></DetailCanvas>,
};

export const Key: Story = {
  render: () => <DetailCanvas><KeyDetail keyId={key.id}/></DetailCanvas>,
};

export const RateLimit: Story = {
  render: () => <DetailCanvas><RateLimitDetail rateLimitId={rateLimit.metadata.id}/></DetailCanvas>,
};

function DetailCanvas({children}: {children: React.ReactNode}) {
  return <Box sx={{maxWidth: 960, minHeight: '100vh', mx: 'auto', bgcolor: 'background.default'}}>{children}</Box>;
}

function responseData(url: string) {
  if (url === `/api/v1/namespaces/${namespace.path}`) return namespace;
  if (url === `/api/v1/connectors/${connector.id}`) return connector;
  if (url === `/api/v1/connectors/${connector.id}/versions`) return {items: [connectorVersion]};
  if (url === `/api/v1/connections/${connection.id}`) return connection;
  if (url === `/api/v1/keys/${key.id}`) return key;
  if (url === `/api/v1/rate-limits/${rateLimit.metadata.id}`) return rateLimit;
  return {};
}
