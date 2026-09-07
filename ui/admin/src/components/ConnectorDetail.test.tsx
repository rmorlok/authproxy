// @vitest-environment jsdom
import * as React from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {cleanup, render, screen, waitFor, within} from '@testing-library/react';
import {MemoryRouter} from 'react-router-dom';
import ConnectorDetail from './ConnectorDetail';
import {ConnectorReleaseState, connectors} from '@authproxy/api';

vi.mock('@authproxy/api', () => {
  const connectorApi = {
    archive: vi.fn(),
    disconnectAll: vi.fn(),
    get: vi.fn(),
    listGenerations: vi.fn(),
    update: vi.fn(),
  };

  return {
    API_VERSION: 'authproxy.net/v1alpha1',
    CONNECTOR_KIND: 'Connector',
    ConnectorReleaseState: {
      DRAFT: 'draft',
      PRIMARY: 'primary',
      ACTIVE: 'active',
      ARCHIVED: 'archived',
    },
    PollForTaskResult: {
      FINALIZED: 'finalized',
    },
    TaskState: {
      COMPLETED: 'completed',
    },
    connectors: connectorApi,
    tasks: {
      pollForTaskFinalized: vi.fn(),
    },
  };
});

const connector = {
  apiVersion: 'authproxy.net/v1alpha1' as const,
  kind: 'Connector' as const,
  metadata: {
    id: 'cxr_test',
    name: 'example-connector',
    generation: 4,
    namespace: 'root',
    createdAt: '2026-07-25T00:00:00.000Z',
    updatedAt: '2026-07-25T00:00:00.000Z',
  },
  spec: {definition: {displayName: 'Example connector'}},
  status: {release: {state: ConnectorReleaseState.PRIMARY}},
};

const connectorVersions = [
  connector,
  {...connector, metadata: {...connector.metadata, generation: 3, createdAt: '2026-07-24T00:00:00.000Z'}, status: {release: {state: ConnectorReleaseState.ACTIVE}}},
  {...connector, metadata: {...connector.metadata, generation: 2, createdAt: '2026-07-23T00:00:00.000Z'}, status: {release: {state: ConnectorReleaseState.ACTIVE}}},
  {...connector, metadata: {...connector.metadata, generation: 1, createdAt: '2026-07-22T00:00:00.000Z'}, status: {release: {state: ConnectorReleaseState.ARCHIVED}}},
];

describe('ConnectorDetail', () => {
  beforeEach(() => {
    vi.mocked(connectors.get).mockResolvedValue({status: 200, data: connector} as never);
    vi.mocked(connectors.listGenerations).mockResolvedValue({
      status: 200,
      data: {apiVersion: 'authproxy.net/v1alpha1', kind: 'ConnectorList', metadata: {}, items: connectorVersions},
    } as never);
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('derives available states and the version count from the version list', async () => {
    render(
      <MemoryRouter>
        <ConnectorDetail connectorId={connector.metadata.id}/>
      </MemoryRouter>,
    );

    await screen.findByRole('heading', {name: 'example-connector'});
    await waitFor(() => expect(connectors.listGenerations).toHaveBeenCalledWith(
      connector.metadata.id,
      {limit: 100, orderBy: 'version desc'},
    ));

    const states = screen.getByText('Available States').parentElement;
    expect(states).not.toBeNull();
    expect(within(states!).getAllByText(ConnectorReleaseState.PRIMARY)).toHaveLength(1);
    expect(within(states!).getAllByText(ConnectorReleaseState.ACTIVE)).toHaveLength(1);
    expect(within(states!).getAllByText(ConnectorReleaseState.ARCHIVED)).toHaveLength(1);

    const count = screen.getByText('Versions').parentElement;
    expect(count).not.toBeNull();
    expect(within(count!).getByText('4')).toBeTruthy();

    const namespace = screen.getByText('Namespace').parentElement;
    expect(namespace).not.toBeNull();
    expect(within(namespace!).getByText('root')).toBeTruthy();
  });
});
