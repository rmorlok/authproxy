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

const connectorGenerations = [
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
      data: {apiVersion: 'authproxy.net/v1alpha1', kind: 'ConnectorList', metadata: {}, items: connectorGenerations},
    } as never);
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('derives available states and the generation count from the generation list', async () => {
    render(
      <MemoryRouter>
        <ConnectorDetail connectorId={connector.metadata.id}/>
      </MemoryRouter>,
    );

    await screen.findByRole('heading', {name: 'example-connector'});
    await waitFor(() => expect(connectors.listGenerations).toHaveBeenCalledWith(
      connector.metadata.id,
      {limit: 100, orderBy: 'generation desc'},
    ));

    const states = screen.getByText('Available States').parentElement;
    expect(states).not.toBeNull();
    expect(within(states!).getAllByText(ConnectorReleaseState.PRIMARY)).toHaveLength(1);
    expect(within(states!).getAllByText(ConnectorReleaseState.ACTIVE)).toHaveLength(1);
    expect(within(states!).getAllByText(ConnectorReleaseState.ARCHIVED)).toHaveLength(1);

    const count = screen.getByText('Generations').parentElement;
    expect(count).not.toBeNull();
    expect(within(count!).getByText('4')).toBeTruthy();

    const namespace = screen.getByText('Namespace').parentElement;
    expect(namespace).not.toBeNull();
    expect(within(namespace!).getByText('root')).toBeTruthy();
  });
  describe('when the connector has no primary generation', () => {
    const notFound = Object.assign(new Error('Request failed with status code 404'), {
      response: {status: 404, data: {error: "connector 'cxr_test' not found"}},
    });
    const draft = {...connector, metadata: {...connector.metadata, generation: 5}, status: {release: {state: ConnectorReleaseState.DRAFT}}};
    const archived = {...connector, metadata: {...connector.metadata, generation: 2}, status: {release: {state: ConnectorReleaseState.ARCHIVED}}};

    // Responds to state-filtered generation queries with the matching
    // generations, and to the unfiltered list with every generation.
    const mockGenerationsByState = (items: typeof connector[]) => {
      vi.mocked(connectors.listGenerations).mockImplementation(((_id: string, params?: {state?: string}) => Promise.resolve({
        status: 200,
        data: {
          apiVersion: 'authproxy.net/v1alpha1',
          kind: 'ConnectorList',
          metadata: {},
          items: params?.state ? items.filter(g => g.status.release.state === params.state) : items,
        },
      })) as never);
    };

    const renderDetail = () => render(
      <MemoryRouter>
        <ConnectorDetail connectorId={connector.metadata.id}/>
      </MemoryRouter>,
    );

    it('falls back to the newest draft generation', async () => {
      vi.mocked(connectors.get).mockRejectedValue(notFound);
      mockGenerationsByState([draft, archived]);

      renderDetail();

      await screen.findByRole('heading', {name: 'example-connector'});
      expect(screen.getByText('This connector has no primary generation. Showing generation 5 (draft).')).toBeTruthy();
      expect(connectors.listGenerations).toHaveBeenCalledWith(
        connector.metadata.id,
        {state: ConnectorReleaseState.DRAFT, limit: 1, orderBy: 'generation desc'},
      );
    });

    it('falls back to an archived generation when nothing higher priority exists', async () => {
      vi.mocked(connectors.get).mockRejectedValue(notFound);
      mockGenerationsByState([archived]);

      renderDetail();

      await screen.findByText('This connector has no primary generation. Showing generation 2 (archived).');
    });

    it('shows the not-found error when no generation exists', async () => {
      vi.mocked(connectors.get).mockRejectedValue(notFound);
      mockGenerationsByState([]);

      renderDetail();

      await screen.findByText("connector 'cxr_test' not found");
    });

    it('does not fall back for errors other than not found', async () => {
      vi.mocked(connectors.get).mockRejectedValue(Object.assign(new Error('boom'), {
        response: {status: 500, data: {error: 'Internal Server Error'}},
      }));
      mockGenerationsByState([draft]);

      renderDetail();

      await screen.findByText('Internal Server Error');
      expect(connectors.listGenerations).not.toHaveBeenCalledWith(
        connector.metadata.id,
        expect.objectContaining({state: expect.anything()}),
      );
    });
  });
});
