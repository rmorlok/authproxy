import * as React from 'react';
import type {Meta, StoryObj} from '@storybook/react';
import Box from '@mui/material/Box';
import {API_VERSION, configureClient} from '@authproxy/api';
import ActorDetail from '../components/ActorDetail';
import NamespaceDetail from '../components/NamespaceDetail';
import ConnectorDetail from '../components/ConnectorDetail';
import ConnectionDetail from '../components/ConnectionDetail';
import KeyDetail from '../components/KeyDetail';
import RateLimitDetail from '../components/RateLimitDetail';

const namespace = {
  apiVersion: API_VERSION,
  kind: 'Namespace',
  metadata: {
    id: 'root.payments',
    namespace: 'root',
    name: 'payments',
    labels: {team: 'payments', 'apxy/ns/team': 'platform'},
    annotations: {owner: 'Payments Platform', runbook: 'go/payments'},
    createdAt: '2026-08-05T15:30:00Z',
    updatedAt: '2026-08-05T15:45:00Z',
  },
  spec: {},
  status: {state: 'active'},
};

const connector = {
  apiVersion: API_VERSION,
  kind: 'Connector',
  metadata: {
    id: 'cxr_payments_stripe',
    generation: 3,
    namespace: 'root.payments',
    name: 'stripe',
    labels: {provider: 'stripe', 'apxy/ns/team': 'payments'},
    annotations: {owner: 'Payments Platform', runbook: 'go/payments/stripe'},
    createdAt: '2026-08-05T15:30:00Z',
    updatedAt: '2026-08-05T15:45:00Z',
  },
  spec: {
    definition: {
      displayName: 'Stripe',
      description: 'Payment processing for the Acme platform.',
      highlight: 'Production connector',
      setupFlow: {steps: []},
    },
  },
  status: {release: {state: 'primary'}},
};

const connection = {
  apiVersion: API_VERSION,
  kind: 'Connection',
  metadata: {
    id: 'cxn_payments_production',
    namespace: 'root.payments',
    name: 'stripe-production',
    labels: {environment: 'production', 'apxy/ns/team': 'payments'},
    annotations: {owner: 'Payments Platform', runbook: 'go/payments/stripe'},
    createdAt: '2026-08-05T15:30:00Z',
    updatedAt: '2026-08-05T15:45:00Z',
  },
  spec: {
    connectorRef: {
      apiVersion: API_VERSION,
      kind: 'Connector',
      id: connector.metadata.id,
      name: connector.metadata.name,
      namespace: connector.metadata.namespace,
      generation: connector.metadata.generation,
    },
  },
  status: {
    lifecycle: {state: 'configured'},
    health: {state: 'healthy'},
    configuration: {configured: true, schema: {}},
  },
};

const key = {
  apiVersion: API_VERSION,
  kind: 'Key',
  metadata: {
    id: 'key_payments_primary',
    namespace: 'root.payments',
    name: 'payments-primary',
    labels: {environment: 'production', 'apxy/ns/team': 'payments'},
    annotations: {owner: 'Payments Platform', rotation: 'quarterly'},
    createdAt: '2026-08-05T15:30:00Z',
    updatedAt: '2026-08-05T15:45:00Z',
  },
  spec: {
    usage: 'data_encryption',
    materialType: 'external',
    desiredState: 'active',
    keyData: {type: 'aes-gcm'},
  },
  status: {state: 'active', keyDataConfigured: true},
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
      namespaceMatcher: 'root.payments.checkout.**',
    },
    selector: {methods: ['GET', 'POST'], requestTypes: ['proxy']},
    bucket: {dimensions: ['actor']},
    algorithm: {tokenBucket: {capacity: 100, refillRate: 20}},
  },
  status: {effectiveMode: 'enforce'},
};

const actor = {
  apiVersion: 'authproxy.net/v1alpha1',
  kind: 'Actor',
  metadata: {
    id: 'act_payments_billing',
    namespace: 'root.payments',
    name: 'billing-service',
    labels: {team: 'payments', 'apxy/act/-/name': 'billing-service'},
    annotations: {owner: 'Payments Platform', runbook: 'go/payments/billing'},
    createdAt: '2026-08-05T15:30:00Z',
    updatedAt: '2026-08-05T15:45:00Z',
  },
  spec: {
    externalId: 'svc_billing',
    permissions: [],
  },
  status: {signingKeyConfigured: true},
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
  render: () => <DetailCanvas><NamespaceDetail namespacePath={namespace.metadata.id}/></DetailCanvas>,
};

export const Connector: Story = {
  render: () => <DetailCanvas><ConnectorDetail connectorId={connector.metadata.id}/></DetailCanvas>,
};

export const Connection: Story = {
  render: () => <DetailCanvas><ConnectionDetail connectionId={connection.metadata.id}/></DetailCanvas>,
};

export const Key: Story = {
  render: () => <DetailCanvas><KeyDetail keyId={key.metadata.id}/></DetailCanvas>,
};

export const RateLimit: Story = {
  render: () => <DetailCanvas><RateLimitDetail rateLimitId={rateLimit.metadata.id}/></DetailCanvas>,
};

export const Actor: Story = {
  render: () => <DetailCanvas><ActorDetail actorId={actor.metadata.id}/></DetailCanvas>,
};

function DetailCanvas({children}: {children: React.ReactNode}) {
  return <Box sx={{maxWidth: 960, minHeight: '100vh', mx: 'auto', bgcolor: 'background.default'}}>{children}</Box>;
}

function responseData(url: string) {
  if (url === `/api/v1/namespaces/${namespace.metadata.id}`) return namespace;
  if (url === `/api/v1/connectors/${connector.metadata.id}`) return connector;
  if (url === `/api/v1/connectors/${connector.metadata.id}/generations`) {
    return {apiVersion: API_VERSION, kind: 'ConnectorList', metadata: {}, items: [connector]};
  }
  if (url === `/api/v1/connectors/${connector.metadata.id}/generations/${connector.metadata.generation}`) return connector;
  if (url === `/api/v1/connections/${connection.metadata.id}`) return connection;
  if (url === `/api/v1/keys/${key.metadata.id}`) return key;
  if (url === `/api/v1/rate-limits/${rateLimit.metadata.id}`) return rateLimit;
  if (url === `/api/v1/actors/${actor.metadata.id}`) return actor;
  return {};
}
