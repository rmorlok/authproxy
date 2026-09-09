// @vitest-environment jsdom
import * as React from 'react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ConnectorGenerationDetail from './ConnectorGenerationDetail';
import {ConnectorReleaseState, connectors} from '@authproxy/api';

vi.mock('@uiw/react-codemirror', () => ({default: () => null}));

vi.mock('@authproxy/api', () => ({
  API_VERSION: 'authproxy.net/v1alpha1',
  CONNECTOR_KIND: 'Connector',
  ConnectorReleaseState: {
    DRAFT: 'draft',
    PRIMARY: 'primary',
    ACTIVE: 'active',
    ARCHIVED: 'archived',
  },
  connectors: {
    forceGenerationState: vi.fn(),
    getGeneration: vi.fn(),
    updateGeneration: vi.fn(),
  },
}));

const connectorGeneration = {
  apiVersion: 'authproxy.net/v1alpha1' as const,
  kind: 'Connector' as const,
  metadata: {
    id: 'cxr_stripe',
    name: 'stripe',
    namespace: 'root.payments',
    generation: 4,
    labels: {
      provider: 'stripe',
      'apxy/ns/team': 'platform',
    },
    annotations: {owner: 'Payments Platform'},
    createdAt: '2026-08-05T15:30:00Z',
    updatedAt: '2026-08-05T15:45:00Z',
  },
  spec: {definition: {displayName: 'Stripe'}},
  status: {release: {state: ConnectorReleaseState.DRAFT}},
};

describe('ConnectorGenerationDetail', () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('shows namespace and edits draft metadata through the shared menu', async () => {
    const user = userEvent.setup();
    vi.mocked(connectors.updateGeneration).mockResolvedValue({
      status: 200,
      data: {...connectorGeneration, metadata: {...connectorGeneration.metadata, labels: {...connectorGeneration.metadata.labels, tier: 'internal'}}},
    } as any);
    render(<ConnectorGenerationDetail connectorGeneration={connectorGeneration}/>);

    const namespace = screen.getByText('Namespace').parentElement;
    expect(namespace).not.toBeNull();
    expect(within(namespace!).getByText(connectorGeneration.metadata.namespace)).toBeTruthy();

    await user.click(screen.getByRole('button', {name: 'actions'}));
    expect(screen.getByRole('menuitem', {name: 'Edit annotations…'}).getAttribute('aria-disabled')).not.toBe('true');
    await user.click(screen.getByRole('menuitem', {name: 'Edit labels…'}));

    const dialog = screen.getByRole('dialog', {name: 'Edit connector generation labels'});
    await user.click(within(dialog).getByRole('button', {name: 'Add label'}));
    const keys = within(dialog).getAllByLabelText('Key');
    const values = within(dialog).getAllByLabelText('Value');
    fireEvent.change(keys[keys.length - 1], {target: {value: 'tier'}});
    fireEvent.change(values[values.length - 1], {target: {value: 'internal'}});
    await user.click(within(dialog).getByRole('button', {name: 'Save'}));

    await waitFor(() => expect(connectors.updateGeneration).toHaveBeenCalledWith(
      connectorGeneration.metadata.id,
      connectorGeneration.metadata.generation,
      {
        apiVersion: 'authproxy.net/v1alpha1',
        kind: 'Connector',
        metadata: {labels: {provider: 'stripe', tier: 'internal'}},
        spec: {},
      },
    ));
  });

  it('keeps metadata actions read-only outside the draft generation', async () => {
    const user = userEvent.setup();
    render(<ConnectorGenerationDetail connectorGeneration={{...connectorGeneration, status: {release: {state: ConnectorReleaseState.PRIMARY}}}}/>);

    await user.click(screen.getByRole('button', {name: 'actions'}));
    expect(screen.getByRole('menuitem', {name: 'Edit labels…'}).getAttribute('aria-disabled')).toBe('true');
    expect(screen.getByRole('menuitem', {name: 'Edit annotations…'}).getAttribute('aria-disabled')).toBe('true');
  });
});
