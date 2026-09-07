// @vitest-environment jsdom
import React from 'react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {cleanup, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ActorPermissionsEditor from './ActorPermissionsEditor';

describe('ActorPermissionsEditor', () => {
  afterEach(cleanup);

  it('shows every permission dimension', () => {
    render(
      <ActorPermissionsEditor
        permissions={[{
          namespace: 'root.tenant.**',
          resources: ['connections'],
          resourceIds: ['cxn_example'],
          verbs: ['get', 'proxy'],
        }]}
        onSave={vi.fn()}
      />,
    );

    expect(screen.getByText('root.tenant.**')).toBeTruthy();
    expect(screen.getByText('connections')).toBeTruthy();
    expect(screen.getByText('cxn_example')).toBeTruthy();
    expect(screen.getByText('proxy')).toBeTruthy();
  });

  it('normalizes comma-separated values before saving', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<ActorPermissionsEditor permissions={[]} onSave={onSave}/>);

    await user.click(screen.getByRole('button', {name: 'Edit actor permissions'}));
    await user.click(screen.getByRole('button', {name: 'Add permission'}));
    await user.type(screen.getByRole('textbox', {name: /Namespace/}), 'root.tenant.**');
    await user.type(screen.getByRole('textbox', {name: /Resources/}), 'connections, connectors');
    await user.type(screen.getByRole('textbox', {name: /Verbs/}), 'get, proxy');
    await user.click(screen.getByRole('button', {name: 'Save permissions'}));

    await waitFor(() => expect(onSave).toHaveBeenCalledWith([{
      namespace: 'root.tenant.**',
      resources: ['connections', 'connectors'],
      verbs: ['get', 'proxy'],
    }]));
  });

  it('requires namespace, resources, and verbs', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<ActorPermissionsEditor permissions={[]} onSave={onSave}/>);

    await user.click(screen.getByRole('button', {name: 'Edit actor permissions'}));
    await user.click(screen.getByRole('button', {name: 'Add permission'}));
    await user.click(screen.getByRole('button', {name: 'Save permissions'}));

    expect(await screen.findByText(/Permission 1 requires a namespace/)).toBeTruthy();
    expect(onSave).not.toHaveBeenCalled();
  });
});
