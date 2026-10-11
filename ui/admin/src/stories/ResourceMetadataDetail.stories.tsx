import * as React from 'react';
import type {Meta, StoryObj} from '@storybook/react';
import Box from '@mui/material/Box';
import {API_VERSION, configureClient, ConnectorReleaseState} from '@authproxy/api';
import type {Connector as ConnectorResource} from '@authproxy/api';
import NamespaceDetail from '../components/NamespaceDetail';
import ConnectorDetail from '../components/ConnectorDetail';
import ConnectorGenerationDetail from '../components/ConnectorGenerationDetail';
import ConnectionDetail from '../components/ConnectionDetail';
import ActorDetail from '../components/ActorDetail';
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

const draftConnectorGeneration: ConnectorResource = {
  ...connector,
  kind: 'Connector',
  metadata: {...connector.metadata, generation: 4},
  spec: {...connector.spec, release: {desiredState: ConnectorReleaseState.DRAFT}},
  status: {release: {state: ConnectorReleaseState.DRAFT}},
};

// A connector whose only generation is a draft. `GET /connectors/:id` returns
// 404 for it because it has no primary generation.
const draftOnlyConnector: ConnectorResource = {
  ...draftConnectorGeneration,
  metadata: {...draftConnectorGeneration.metadata, id: 'cxr_payments_adyen', name: 'adyen', generation: 1},
  spec: {
    ...draftConnectorGeneration.spec,
    definition: {...connector.spec.definition, displayName: 'Adyen', highlight: undefined},
  },
};

// Sentinel returned by responseData for URLs the mock API answers with 404.
const NOT_FOUND = Symbol('not found');

configureClient({
  axiosConfigOverride: {
    adapter: async (config) => {
      const data = responseData(config.url || '', config.params);
      if (data === NOT_FOUND) {
        throw Object.assign(new Error('Request failed with status code 404'), {
          config,
          response: {data: {error: 'not found'}, status: 404, statusText: 'Not Found', headers: {}, config},
        });
      }
      return {
        data,
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      };
    },
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

export const ConnectorWithoutPrimary: Story = {
  render: () => <DetailCanvas><ConnectorDetail connectorId={draftOnlyConnector.metadata.id}/></DetailCanvas>,
};

export const ConnectorGeneration: Story = {
  render: () => <DetailCanvas><ConnectorGenerationDetail connectorGeneration={draftConnectorGeneration}/></DetailCanvas>,
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

function responseData(url: string, params?: {state?: string}) {
  if (url === `/api/v1/connectors/${draftOnlyConnector.metadata.id}`) return NOT_FOUND;
  if (url === `/api/v1/connectors/${draftOnlyConnector.metadata.id}/generations`) {
    const items = !params?.state || params.state === ConnectorReleaseState.DRAFT ? [draftOnlyConnector] : [];
    return {apiVersion: API_VERSION, kind: 'ConnectorList', metadata: {}, items};
  }
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
