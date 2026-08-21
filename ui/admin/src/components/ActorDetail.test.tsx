// @vitest-environment jsdom
import * as React from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {MemoryRouter} from 'react-router-dom';
import ActorDetail from './ActorDetail';
import {actors} from '@authproxy/api';

vi.mock('@authproxy/api', () => ({
  actors: {
    getById: vi.fn(),
    update: vi.fn(),
  },
}));

const actor = {
  id: 'act_payments',
  name: 'payments-service',
  namespace: 'root.payments',
  externalId: 'service:payments',
  permissions: [],
  labels: {
    team: 'payments',
    'apxy/ns/team': 'platform',
  },
  annotations: {
    owner: 'Payments Platform',
  },
  createdAt: '2026-08-05T15:30:00Z',
  updatedAt: '2026-08-05T15:45:00Z',
};

function renderActorDetail() {
  render(
    <MemoryRouter>
      <ActorDetail actorId={actor.id}/>
    </MemoryRouter>,
  );
}

describe('ActorDetail', () => {
  beforeEach(() => {
    vi.mocked(actors.getById).mockResolvedValue({status: 200, data: actor} as any);
    vi.mocked(actors.update).mockResolvedValue({
      status: 200,
      data: {...actor, labels: {...actor.labels, tier: 'internal'}},
    } as any);
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('shows the actor namespace and edits metadata through the shared menu', async () => {
    const user = userEvent.setup();
    renderActorDetail();

    await screen.findByRole('heading', {name: actor.name});
    const namespace = screen.getByText('Namespace').parentElement;
    expect(namespace).not.toBeNull();
    expect(within(namespace!).getByText(actor.namespace)).toBeTruthy();

    await user.click(screen.getByRole('button', {name: 'actions'}));
    expect(screen.getByRole('menuitem', {name: 'Edit annotations…'})).toBeTruthy();
    await user.click(screen.getByRole('menuitem', {name: 'Edit labels…'}));

    const dialog = screen.getByRole('dialog', {name: 'Edit actor labels'});
    expect((within(dialog).getByDisplayValue('apxy/ns/team') as HTMLInputElement).disabled).toBe(true);
    await user.click(within(dialog).getByRole('button', {name: 'Add label'}));

    const keys = within(dialog).getAllByLabelText('Key');
    const values = within(dialog).getAllByLabelText('Value');
    fireEvent.change(keys[keys.length - 1], {target: {value: 'tier'}});
    fireEvent.change(values[values.length - 1], {target: {value: 'internal'}});
    await user.click(within(dialog).getByRole('button', {name: 'Save'}));

    await waitFor(() => expect(actors.update).toHaveBeenCalledWith(actor.id, {
      labels: {team: 'payments', tier: 'internal'},
    }));
    expect(await screen.findByText('tier: internal')).toBeTruthy();
  });
});
