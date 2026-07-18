import * as React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import userEvent from '@testing-library/user-event';
import { Provider } from 'react-redux';
import { combineReducers, configureStore } from '@reduxjs/toolkit';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, test, vi } from 'vitest';
import {
  Connection,
  ConnectionHealthState,
  ConnectionState,
  Connector,
  ConnectorVersionState,
  connections,
  PollForTaskResult,
  tasks,
} from '@authproxy/api';
import ConnectionDetail from '../components/ConnectionDetail';
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
      cancelSetup: vi.fn(),
      disconnect: vi.fn(),
      getSetupStep: vi.fn(),
      list: vi.fn(),
      reauth: vi.fn(),
      reconfigure: vi.fn(),
      submit: vi.fn(),
    },
    tasks: {
      ...actual.tasks,
      pollForTaskFinalized: vi.fn(),
    },
  };
});

const connector: Connector = {
  id: 'gmail',
  namespace: 'root',
  version: 1,
  state: ConnectorVersionState.ACTIVE,
  display_name: 'GMail',
  description: 'Have the agent respond to your emails without you needing to be involved. Like magic.',
  highlight: 'Respond to email automatically.',
  logo: 'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg"/%3E',
  has_configure: true,
  versions: 1,
  states: [ConnectorVersionState.ACTIVE],
  created_at: '2023-04-01T12:00:00Z',
  updated_at: '2023-04-01T12:00:00Z',
};

const connection: Connection = {
  id: 'c-gmail',
  namespace: 'root',
  connector,
  state: ConnectionState.CONFIGURED,
  health_state: ConnectionHealthState.HEALTHY,
  created_at: '2023-04-01T12:00:00Z',
  updated_at: '2023-04-01T12:00:00Z',
};

const baseConnectionsState = {
  items: [connection],
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
  recentlyCompletedConnectionId: null,
};

function renderConnectionDetail(preloadedState: any = {}, initialEntry = '/connections/c-gmail') {
  const store = configureStore({
    reducer: combineReducers({
      auth: authReducer,
      connectors: connectorsReducer,
      connections: connectionsReducer,
      toasts: toastsReducer,
    }),
    preloadedState: {
      auth: { actor_id: 'actor_test', status: 'authenticated' },
      connectors: { items: [], status: 'succeeded', error: null },
      connections: baseConnectionsState,
      toasts: { items: [] },
      ...preloadedState,
    },
  });

  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Provider store={store}>
        <Routes>
          <Route path="/connections/:connectionId" element={<ConnectionDetail />} />
        </Routes>
      </Provider>
    </MemoryRouter>,
  );

  return store;
}

describe('ConnectionDetail', () => {
  beforeEach(() => {
    vi.mocked(connections.cancelSetup).mockReset();
    vi.mocked(connections.disconnect).mockReset();
    vi.mocked(connections.getSetupStep).mockReset();
    vi.mocked(connections.list).mockReset();
    vi.mocked(connections.reauth).mockReset();
    vi.mocked(connections.reconfigure).mockReset();
    vi.mocked(connections.submit).mockReset();
    vi.mocked(tasks.pollForTaskFinalized).mockReset();
    vi.mocked(connections.cancelSetup).mockResolvedValue({} as any);
    vi.mocked(connections.disconnect).mockResolvedValue({
      data: {
        task_id: 'task-123',
        connection: {
          ...connection,
          state: ConnectionState.DISCONNECTING,
        },
      },
    } as any);
    vi.mocked(connections.getSetupStep).mockResolvedValue({ data: { id: connection.id, type: 'complete' } } as any);
    vi.mocked(connections.list).mockResolvedValue({ status: 200, data: { items: [connection], cursor: '' } } as any);
    vi.mocked(connections.reauth).mockResolvedValue({ data: { id: connection.id, type: 'complete' } } as any);
    vi.mocked(connections.reconfigure).mockResolvedValue({ data: { id: connection.id, type: 'complete' } } as any);
    vi.mocked(connections.submit).mockResolvedValue({ data: { id: connection.id, type: 'complete' } } as any);
    vi.mocked(tasks.pollForTaskFinalized).mockResolvedValue({
      result: PollForTaskResult.FINALIZED,
    } as any);
  });

  test('renders connector detail content with connection status and actions', () => {
    renderConnectionDetail();

    expect(screen.getByRole('link', { name: /Back to connections/i })).toHaveAttribute('href', '/connections');
    expect(screen.getByRole('heading', { name: 'GMail' })).toBeInTheDocument();
    expect(screen.getByText(/Connected on 4\/1\/2023/)).toBeInTheDocument();
    expect(screen.getByText('Have the agent respond to your emails without you needing to be involved. Like magic.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^Connect$/i })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Reconfigure/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Re-authenticate/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^Disconnect$/i })).toBeInTheDocument();
  });

  test('runs connection actions from the detail page', async () => {
    const user = userEvent.setup();
    renderConnectionDetail();

    await user.click(screen.getByRole('button', { name: /Reconfigure/i }));
    expect(connections.reconfigure).toHaveBeenCalledWith(connection.id);

    await user.click(screen.getByRole('button', { name: /Re-authenticate/i }));
    expect(connections.reauth).toHaveBeenCalledWith(connection.id, window.location.href);
  });

  test('disconnects from the detail page after confirmation', async () => {
    const user = userEvent.setup();
    renderConnectionDetail();

    await user.click(screen.getByRole('button', { name: /^Disconnect$/i }));
    expect(screen.getByRole('heading', { name: /Disconnect Confirmation/i })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /^Disconnect$/i }));

    await waitFor(() => {
      expect(connections.disconnect).toHaveBeenCalledWith(connection.id);
    });
  });

  test('offers resume setup for setup-state connections', async () => {
    const user = userEvent.setup();
    const setupConnection = {
      ...connection,
      state: ConnectionState.SETUP,
    };
    renderConnectionDetail({
      connections: {
        ...baseConnectionsState,
        items: [setupConnection],
      },
    });

    expect(screen.getByText('Setup required')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Resume setup/i }));

    await waitFor(() => {
      expect(connections.getSetupStep).toHaveBeenCalledWith(setupConnection.id, window.location.href);
    });
  });

  test('runs re-authentication from notification action URL', async () => {
    renderConnectionDetail({}, '/connections/c-gmail?action=reauth');

    await waitFor(() => {
      expect(connections.reauth).toHaveBeenCalledWith(connection.id, window.location.href);
    });
  });

  test('resumes pending setup from notification action URL', async () => {
    const pendingSetupConnection = {
      ...connection,
      setup_step_id: 'workspace',
    };

    renderConnectionDetail({
      connections: {
        ...baseConnectionsState,
        items: [pendingSetupConnection],
      },
    }, '/connections/c-gmail?action=configure');

    await waitFor(() => {
      expect(connections.getSetupStep).toHaveBeenCalledWith(pendingSetupConnection.id, window.location.href);
    });
  });
});
