// @vitest-environment jsdom
import * as React from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {cleanup, render, screen, waitFor, within} from '@testing-library/react';
import {MemoryRouter} from 'react-router-dom';
import ConnectorDetail from './ConnectorDetail';
import {ConnectorVersionState, connectors} from '@authproxy/api';

vi.mock('@authproxy/api', () => {
  const connectorApi = {
    archive: vi.fn(),
    disconnectAll: vi.fn(),
    get: vi.fn(),
    listVersions: vi.fn(),
  };

  return {
    ConnectorVersionState: {
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
  id: 'cxr_test',
  version: 4,
  namespace: 'root',
  state: ConnectorVersionState.PRIMARY,
  display_name: 'Example connector',
  description: '',
  logo: '',
  has_configure: false,
  created_at: '2026-07-25T00:00:00.000Z',
  updated_at: '2026-07-25T00:00:00.000Z',
};

const connectorVersions = [
  {id: 'cxr_test', version: 4, state: ConnectorVersionState.PRIMARY, created_at: '2026-07-25T00:00:00.000Z'},
  {id: 'cxr_test', version: 3, state: ConnectorVersionState.ACTIVE, created_at: '2026-07-24T00:00:00.000Z'},
  {id: 'cxr_test', version: 2, state: ConnectorVersionState.ACTIVE, created_at: '2026-07-23T00:00:00.000Z'},
  {id: 'cxr_test', version: 1, state: ConnectorVersionState.ARCHIVED, created_at: '2026-07-22T00:00:00.000Z'},
];

describe('ConnectorDetail', () => {
  beforeEach(() => {
    vi.mocked(connectors.get).mockResolvedValue({status: 200, data: connector} as never);
    vi.mocked(connectors.listVersions).mockResolvedValue({
      status: 200,
      data: {items: connectorVersions},
    } as never);
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('derives available states and the version count from the version list', async () => {
    render(
      <MemoryRouter>
        <ConnectorDetail connectorId={connector.id}/>
      </MemoryRouter>,
    );

    await screen.findByRole('heading', {name: 'Example connector'});
    await waitFor(() => expect(connectors.listVersions).toHaveBeenCalledWith(
      connector.id,
      {limit: 100, order_by: 'version desc'},
    ));

    const states = screen.getByText('Available States').parentElement;
    expect(states).not.toBeNull();
    expect(within(states!).getAllByText(ConnectorVersionState.PRIMARY)).toHaveLength(1);
    expect(within(states!).getAllByText(ConnectorVersionState.ACTIVE)).toHaveLength(1);
    expect(within(states!).getAllByText(ConnectorVersionState.ARCHIVED)).toHaveLength(1);

    const count = screen.getByText('Versions').parentElement;
    expect(count).not.toBeNull();
    expect(within(count!).getByText('4')).toBeTruthy();
  });
});
