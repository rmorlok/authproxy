// @vitest-environment jsdom
import * as React from 'react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ConnectorVersionDetail from './ConnectorVersionDetail';
import {ConnectorVersionState, connectors} from '@authproxy/api';

vi.mock('@uiw/react-codemirror', () => ({default: () => null}));

vi.mock('@authproxy/api', () => ({
  ConnectorVersionState: {
    DRAFT: 'draft',
    PRIMARY: 'primary',
    ACTIVE: 'active',
    ARCHIVED: 'archived',
  },
  connectors: {
    forceVersionState: vi.fn(),
    getVersion: vi.fn(),
    updateVersion: vi.fn(),
  },
}));

const connectorVersion = {
  id: 'cxr_stripe',
  name: 'stripe',
  namespace: 'root.payments',
  version: 4,
  state: ConnectorVersionState.DRAFT,
  definition: {displayName: 'Stripe'},
  labels: {
    provider: 'stripe',
    'apxy/ns/team': 'platform',
  },
  annotations: {owner: 'Payments Platform'},
  createdAt: '2026-08-05T15:30:00Z',
  updatedAt: '2026-08-05T15:45:00Z',
};

describe('ConnectorVersionDetail', () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('shows namespace and edits draft metadata through the shared menu', async () => {
    const user = userEvent.setup();
    vi.mocked(connectors.updateVersion).mockResolvedValue({
      status: 200,
      data: {...connectorVersion, labels: {...connectorVersion.labels, tier: 'internal'}},
    } as any);
    render(<ConnectorVersionDetail connectorVersion={connectorVersion}/>);

    const namespace = screen.getByText('Namespace').parentElement;
    expect(namespace).not.toBeNull();
    expect(within(namespace!).getByText(connectorVersion.namespace)).toBeTruthy();

    await user.click(screen.getByRole('button', {name: 'actions'}));
    expect(screen.getByRole('menuitem', {name: 'Edit annotations…'}).getAttribute('aria-disabled')).not.toBe('true');
    await user.click(screen.getByRole('menuitem', {name: 'Edit labels…'}));

    const dialog = screen.getByRole('dialog', {name: 'Edit connector version labels'});
    await user.click(within(dialog).getByRole('button', {name: 'Add label'}));
    const keys = within(dialog).getAllByLabelText('Key');
    const values = within(dialog).getAllByLabelText('Value');
    fireEvent.change(keys[keys.length - 1], {target: {value: 'tier'}});
    fireEvent.change(values[values.length - 1], {target: {value: 'internal'}});
    await user.click(within(dialog).getByRole('button', {name: 'Save'}));

    await waitFor(() => expect(connectors.updateVersion).toHaveBeenCalledWith(
      connectorVersion.id,
      connectorVersion.version,
      {labels: {provider: 'stripe', tier: 'internal'}},
    ));
  });

  it('keeps metadata actions read-only outside the draft version', async () => {
    const user = userEvent.setup();
    render(<ConnectorVersionDetail connectorVersion={{...connectorVersion, state: ConnectorVersionState.PRIMARY}}/>);

    await user.click(screen.getByRole('button', {name: 'actions'}));
    expect(screen.getByRole('menuitem', {name: 'Edit labels…'}).getAttribute('aria-disabled')).toBe('true');
    expect(screen.getByRole('menuitem', {name: 'Edit annotations…'}).getAttribute('aria-disabled')).toBe('true');
  });
});
