import * as React from 'react';
import type {Meta, StoryObj} from '@storybook/react';
import Box from '@mui/material/Box';
import {configureClient, ConnectorVersionState} from '@authproxy/api';
import NamespaceDetail from '../components/NamespaceDetail';
import ConnectorDetail from '../components/ConnectorDetail';
import ConnectorVersionDetail from '../components/ConnectorVersionDetail';
import ConnectionDetail from '../components/ConnectionDetail';
import ActorDetail from '../components/ActorDetail';
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

const actor = {
  id: 'act_payments_service',
  namespace: 'root.payments',
  name: 'payments-service',
  externalId: 'service:payments-api',
  permissions: [
    {
      namespace: 'root.payments.**',
      resources: ['connections', 'connectors'],
      verbs: ['get', 'list', 'proxy'],
    },
  ],
  labels: {team: 'payments', 'apxy/ns/team': 'platform'},
  annotations: {owner: 'Payments Platform', runbook: 'go/payments/actors'},
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
  id: 'rl_payments_public_api',
  namespace: 'root.payments',
  name: 'public-api',
  definition: {
    mode: 'enforce',
    selector: {methods: ['GET', 'POST'], requestTypes: ['proxy']},
    bucket: {dimensions: ['actor']},
    algorithm: {tokenBucket: {capacity: 100, refillRate: 20}},
  },
  labels: {scope: 'public-api', 'apxy/ns/team': 'payments'},
  annotations: {owner: 'Payments Platform', runbook: 'go/payments/rate-limits'},
  createdAt: '2026-08-05T15:30:00Z',
  updatedAt: '2026-08-05T15:45:00Z',
};

const connectorVersion = {
  ...connector,
  definition: {displayName: 'Stripe', setupFlow: {steps: []}},
};

const draftConnectorVersion = {
  ...connectorVersion,
  version: 4,
  state: ConnectorVersionState.DRAFT,
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

export const ConnectorVersion: Story = {
  render: () => <DetailCanvas><ConnectorVersionDetail connectorVersion={draftConnectorVersion}/></DetailCanvas>,
};

export const Connection: Story = {
  render: () => <DetailCanvas><ConnectionDetail connectionId={connection.id}/></DetailCanvas>,
};

export const Actor: Story = {
  render: () => <DetailCanvas><ActorDetail actorId={actor.id}/></DetailCanvas>,
};

export const Key: Story = {
  render: () => <DetailCanvas><KeyDetail keyId={key.id}/></DetailCanvas>,
};

export const RateLimit: Story = {
  render: () => <DetailCanvas><RateLimitDetail rateLimitId={rateLimit.id}/></DetailCanvas>,
};

function DetailCanvas({children}: {children: React.ReactNode}) {
  return <Box sx={{maxWidth: 960, minHeight: '100vh', mx: 'auto', bgcolor: 'background.default'}}>{children}</Box>;
}

function responseData(url: string) {
  if (url === `/api/v1/namespaces/${namespace.path}`) return namespace;
  if (url === `/api/v1/connectors/${connector.id}`) return connector;
  if (url === `/api/v1/connectors/${connector.id}/versions`) return {items: [connectorVersion]};
  if (url === `/api/v1/connections/${connection.id}`) return connection;
  if (url === `/api/v1/actors/${actor.id}`) return actor;
  if (url === `/api/v1/keys/${key.id}`) return key;
  if (url === `/api/v1/rate-limits/${rateLimit.id}`) return rateLimit;
  return {};
}
