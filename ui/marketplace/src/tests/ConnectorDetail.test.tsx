import * as React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import userEvent from '@testing-library/user-event';
import { Provider } from 'react-redux';
import { combineReducers, configureStore } from '@reduxjs/toolkit';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, test, vi } from 'vitest';
import {
  Connector,
  ConnectorVersionState,
  connections,
} from '@authproxy/api';
import ConnectorDetail from '../components/ConnectorDetail';
import authReducer from '../store/sessionSlice';
import connectorsReducer from '../store/connectorsSlice';
import connectionsReducer from '../store/connectionsSlice';
import toastsReducer from '../store/toastsSlice';

vi.mock('@authproxy/api', async () => {
  const actual = await vi.importActual<typeof import('@authproxy/api')>('@authproxy/api');
  return {
    ...actual,
    connections: {
      ...actual.connections,
      abort: vi.fn(),
      initiate: vi.fn(),
      list: vi.fn(),
      submit: vi.fn(),
    },
  };
});

const connector: Connector = {
  id: 'google-calendar',
  namespace: 'root',
  version: 1,
  state: ConnectorVersionState.ACTIVE,
  display_name: 'Google Calendar',
  description: `Google Calendar lets agents coordinate scheduling.

| Capability | Supported |
| --- | --- |
| Find open time | Yes |
| Create events | Yes |`,
  highlight: 'Coordinate meetings from Google Calendar.',
  logo: 'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg"/%3E',
  has_configure: false,
  versions: 1,
  states: [ConnectorVersionState.ACTIVE],
  created_at: '2023-04-01T12:00:00Z',
  updated_at: '2023-04-01T12:00:00Z',
};

const baseConnectionsState = {
  items: [],
  status: 'succeeded',
  error: null,
  initiatingConnection: false,
  initiationError: null,
  disconnectingConnection: false,
  disconnectionError: null,
  currentTaskId: null,
  currentFormStep: null,
  submittingForm: false,
  formSubmitError: null,
  verifyingConnectionId: null,
  verifyError: null,
  retryingConnection: false,
};

function renderConnectorDetail(preloadedState: any, connectorId = 'google-calendar') {
  const store = configureStore({
    reducer: combineReducers({
      auth: authReducer,
      connectors: connectorsReducer,
      connections: connectionsReducer,
      toasts: toastsReducer,
    }),
    preloadedState: {
      auth: { actor_id: 'actor_test', status: 'authenticated' },
      toasts: { items: [] },
      ...preloadedState,
    },
  });

  render(
    <MemoryRouter>
      <Provider store={store}>
        <ConnectorDetail connectorId={connectorId} />
      </Provider>
    </MemoryRouter>,
  );

  return store;
}

describe('ConnectorDetail', () => {
  beforeEach(() => {
    vi.mocked(connections.abort).mockReset();
    vi.mocked(connections.initiate).mockReset();
    vi.mocked(connections.list).mockReset();
    vi.mocked(connections.submit).mockReset();
    vi.mocked(connections.abort).mockResolvedValue({} as any);
    vi.mocked(connections.initiate).mockResolvedValue({ data: { id: 'c-new', type: 'complete' } } as any);
    vi.mocked(connections.list).mockResolvedValue({ status: 200, data: { items: [], cursor: '' } } as any);
    vi.mocked(connections.submit).mockResolvedValue({ data: { id: 'c-setup', type: 'complete' } } as any);
  });

  test('renders the connector overview with full markdown description', () => {
    renderConnectorDetail({
      connectors: { items: [connector], status: 'succeeded', error: null },
      connections: baseConnectionsState,
    });

    expect(screen.getByRole('heading', { name: 'Google Calendar' })).toBeInTheDocument();
    expect(screen.getByText('Coordinate meetings from Google Calendar.')).toBeInTheDocument();
    expect(screen.getByText('Google Calendar lets agents coordinate scheduling.')).toBeInTheDocument();
    expect(screen.getByText('Capability')).toBeInTheDocument();
    expect(screen.getByText('Find open time')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /Back to connectors/i })).toHaveAttribute('href', '/connectors');
  });

  test('initiates a connection from the overview connect action', async () => {
    const user = userEvent.setup();
    renderConnectorDetail({
      connectors: { items: [connector], status: 'succeeded', error: null },
      connections: baseConnectionsState,
    });

    await user.click(screen.getByRole('button', { name: /Connect/i }));

    await waitFor(() => {
      expect(connections.initiate).toHaveBeenCalledWith(
        'google-calendar',
        `${window.location.origin}/connections`,
      );
    });
  });

  test('renders a not found state when the connector is unavailable', () => {
    renderConnectorDetail({
      connectors: { items: [connector], status: 'succeeded', error: null },
      connections: baseConnectionsState,
    }, 'unknown');

    expect(screen.getByText('Connector not found.')).toBeInTheDocument();
  });
});
