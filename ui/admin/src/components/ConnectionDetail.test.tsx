// @vitest-environment jsdom
import * as React from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {cleanup, render, screen, waitFor, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {MemoryRouter} from 'react-router-dom';
import ConnectionDetail from './ConnectionDetail';
import {
  ConnectionHealthState,
  ConnectionState,
  ConnectorVersionState,
  PollForTaskResult,
  TaskState,
  connections,
  connectors,
  tasks,
} from '@authproxy/api';

vi.mock('@authproxy/api', () => {
  const connectionApi = {
    deleteAnnotation: vi.fn(),
    disconnect: vi.fn(),
    forceState: vi.fn(),
    get: vi.fn(),
    migrateVersion: vi.fn(),
    putAnnotation: vi.fn(),
    update: vi.fn(),
  };
  const connectorApi = {
    listVersions: vi.fn(),
  };
  const taskApi = {
    pollForTaskFinalized: vi.fn(),
  };

  return {
    ConnectionHealthState: {
      HEALTHY: 'healthy',
      UNHEALTHY: 'unhealthy',
    },
    ConnectionState: {
      SETUP: 'setup',
      CONFIGURED: 'configured',
      DISABLED: 'disabled',
      DISCONNECTING: 'disconnecting',
      DISCONNECTED: 'disconnected',
    },
    ConnectorVersionState: {
      DRAFT: 'draft',
      PRIMARY: 'primary',
      ACTIVE: 'active',
      ARCHIVED: 'archived',
    },
    PollForTaskResult: {
      FINALIZED: 'finalized',
      RETRIES_EXHAUSTED: 'retries_exhausted',
      ERROR: 'error',
    },
    TaskState: {
      COMPLETED: 'completed',
      FAILED: 'failed',
    },
    canBeDisconnected: () => true,
    connections: connectionApi,
    connectors: connectorApi,
    tasks: taskApi,
  };
});

const connection = {
  id: 'cxn_test',
  name: 'production-crm',
  namespace: 'root',
  state: ConnectionState.CONFIGURED,
  healthState: ConnectionHealthState.HEALTHY,
  connector: {
    id: 'cxr_test',
    name: 'example-connector',
    version: 2,
    namespace: 'root',
    state: ConnectorVersionState.ACTIVE,
    displayName: 'Example connector',
    description: '',
    logo: '',
    hasConfigure: false,
    createdAt: '2026-07-25T00:00:00.000Z',
    updatedAt: '2026-07-25T00:00:00.000Z',
  },
  createdAt: '2026-07-25T00:00:00.000Z',
  updatedAt: '2026-07-25T00:00:00.000Z',
};

const connectorVersions = [
  {id: 'cxr_test', name: 'example-connector', version: 4, state: ConnectorVersionState.DRAFT},
  {id: 'cxr_test', name: 'example-connector', version: 3, state: ConnectorVersionState.PRIMARY},
  {id: 'cxr_test', name: 'example-connector', version: 2, state: ConnectorVersionState.ACTIVE},
  {id: 'cxr_test', name: 'example-connector', version: 1, state: ConnectorVersionState.ACTIVE},
  {id: 'cxr_test', name: 'example-connector', version: 0, state: ConnectorVersionState.ARCHIVED},
];

function renderConnectionDetail() {
  render(
    <MemoryRouter>
      <ConnectionDetail connectionId={connection.id}/>
    </MemoryRouter>,
  );
}

describe('ConnectionDetail', () => {
  beforeEach(() => {
    vi.mocked(connections.get).mockResolvedValue({status: 200, data: connection} as any);
    vi.mocked(connectors.listVersions).mockResolvedValue({status: 200, data: {items: connectorVersions}} as any);
    vi.mocked(connections.migrateVersion).mockResolvedValue({
      status: 200,
      data: {
        taskId: 'task_test',
        connectionId: connection.id,
        sourceVersion: 2,
        targetVersion: 3,
      },
    } as any);
    vi.mocked(connections.update).mockResolvedValue({
      status: 200,
      data: {...connection, name: 'customer-crm'},
    } as any);
    vi.mocked(tasks.pollForTaskFinalized).mockResolvedValue({
      result: PollForTaskResult.FINALIZED,
      taskInfo: {id: 'task_test', state: TaskState.COMPLETED},
    } as any);
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('filters migration targets and starts a migration to the primary version', async () => {
    const user = userEvent.setup();
    vi.mocked(connections.get)
      .mockResolvedValueOnce({status: 200, data: connection} as any)
      .mockResolvedValueOnce({
        status: 200,
        data: {
          ...connection,
          healthState: ConnectionHealthState.UNHEALTHY,
        },
      } as any);
    renderConnectionDetail();

    await screen.findByText('Connection');
    await user.click(screen.getByRole('button', {name: 'actions'}));
    await user.click(screen.getByRole('menuitem', {name: 'Change version…'}));

    const dialog = await screen.findByRole('dialog', {name: 'Change connection version'});
    const target = within(dialog).getByRole('combobox', {name: 'Target version'}) as HTMLSelectElement;
    expect(target.value).toBe('3');
    expect(within(target).queryByRole('option', {name: 'v4 (draft)'})).toBeNull();
    expect(within(target).queryByRole('option', {name: 'v2 (active)'})).toBeNull();
    expect(within(target).queryByRole('option', {name: 'v0 (archived)'})).toBeNull();

    await user.click(within(dialog).getByRole('button', {name: 'Migrate to v3'}));

    await waitFor(() => {
      expect(connections.migrateVersion).toHaveBeenCalledWith(connection.id, {
        targetVersion: 3,
        timeoutSeconds: 600,
      });
    });
    expect(tasks.pollForTaskFinalized).toHaveBeenCalledWith('task_test', {
      initialDelay: 1000,
      maxDelay: 5000,
      maxAttempts: 140,
      backoffFactor: 1.4,
    });
    expect(await screen.findByText('Migration to v3 completed.')).toBeTruthy();
    expect(await screen.findByText('This connection requires re-authentication before it can be used.')).toBeTruthy();
  });

  it('uses the same flow to roll back to an earlier active version', async () => {
    const user = userEvent.setup();
    renderConnectionDetail();

    await screen.findByText('Connection');
    await user.click(screen.getByRole('button', {name: 'actions'}));
    await user.click(screen.getByRole('menuitem', {name: 'Change version…'}));

    const dialog = await screen.findByRole('dialog', {name: 'Change connection version'});
    await user.selectOptions(within(dialog).getByRole('combobox', {name: 'Target version'}), '1');
    await user.click(within(dialog).getByRole('button', {name: 'Rollback to v1'}));

    await waitFor(() => {
      expect(connections.migrateVersion).toHaveBeenCalledWith(connection.id, {
        targetVersion: 1,
        timeoutSeconds: 600,
      });
    });
    expect(await screen.findByText('Rollback to v1 completed.')).toBeTruthy();
  });

  it('explains when no other connector versions are eligible instead of rendering an empty selector', async () => {
    const user = userEvent.setup();
    vi.mocked(connectors.listVersions).mockResolvedValue({
      status: 200,
      data: {items: [{...connectorVersions[2]}]},
    } as any);
    renderConnectionDetail();

    await screen.findByText('Connection');
    await user.click(screen.getByRole('button', {name: 'actions'}));
    await user.click(screen.getByRole('menuitem', {name: 'Change version…'}));

    const dialog = await screen.findByRole('dialog', {name: 'Change connection version'});
    expect(within(dialog).queryByRole('combobox', {name: 'Target version'})).toBeNull();
    expect(within(dialog).getByRole('alert').textContent).toContain('No other active or primary versions are available.');
    expect(within(dialog).getByRole('button', {name: 'Close'})).toBeTruthy();
    expect(within(dialog).queryByRole('button', {name: 'Change version'})).toBeNull();
  });

  it('renames the connection while keeping its id as the route identity', async () => {
    const user = userEvent.setup();
    renderConnectionDetail();

    await screen.findByRole('heading', {name: 'production-crm'});
    await user.click(screen.getByRole('button', {name: 'Rename connection'}));
    const input = screen.getByRole('textbox', {name: 'Name'});
    await user.clear(input);
    await user.type(input, 'customer-crm');
    await user.click(screen.getByRole('button', {name: 'Save'}));

    await waitFor(() => expect(connections.update).toHaveBeenCalledWith('cxn_test', {name: 'customer-crm'}));
    expect(await screen.findByRole('heading', {name: 'customer-crm'})).toBeTruthy();
    expect(screen.getByText('cxn_test')).toBeTruthy();
  });
});
