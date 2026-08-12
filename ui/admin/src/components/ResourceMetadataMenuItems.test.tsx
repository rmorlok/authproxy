// @vitest-environment jsdom
import * as React from 'react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {cleanup, render, screen, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ResourceMetadataMenuItems from './ResourceMetadataMenuItems';

function renderMenuItems(overrides: Partial<React.ComponentProps<typeof ResourceMetadataMenuItems>> = {}) {
  const onCloseMenu = vi.fn();
  const onRename = vi.fn().mockResolvedValue(undefined);
  const onUpdateLabels = vi.fn().mockResolvedValue(undefined);
  const onUpdateAnnotations = vi.fn().mockResolvedValue(undefined);

  render(
    <ResourceMetadataMenuItems
      resource="connection"
      name="billing"
      labels={{team: 'platform', 'apxy/ns/team': 'inherited-platform'}}
      annotations={{owner: 'payments'}}
      onCloseMenu={onCloseMenu}
      onRename={onRename}
      onUpdateLabels={onUpdateLabels}
      onUpdateAnnotations={onUpdateAnnotations}
      {...overrides}
    />,
  );

  return {onCloseMenu, onRename, onUpdateLabels, onUpdateAnnotations};
}

describe('ResourceMetadataMenuItems', () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('renames a resource from the shared metadata actions', async () => {
    const user = userEvent.setup();
    const {onCloseMenu, onRename} = renderMenuItems();

    await user.click(screen.getByRole('menuitem', {name: 'Rename…'}));
    const dialog = screen.getByRole('dialog', {name: 'Rename connection'});
    await user.clear(within(dialog).getByRole('textbox', {name: 'Name'}));
    await user.type(within(dialog).getByRole('textbox', {name: 'Name'}), 'accounts-payable');
    await user.click(within(dialog).getByRole('button', {name: 'Save'}));

    expect(onCloseMenu).toHaveBeenCalledOnce();
    expect(onRename).toHaveBeenCalledWith('accounts-payable');
  });

  it('keeps propagated labels read-only and excludes them from the update', async () => {
    const user = userEvent.setup();
    const {onUpdateLabels} = renderMenuItems();

    await user.click(screen.getByRole('menuitem', {name: 'Edit labels…'}));
    const dialog = screen.getByRole('dialog', {name: 'Edit connection labels'});
    expect((within(dialog).getByDisplayValue('apxy/ns/team') as HTMLInputElement).disabled).toBe(true);
    expect((within(dialog).getByDisplayValue('inherited-platform') as HTMLInputElement).disabled).toBe(true);

    await user.click(within(dialog).getByRole('button', {name: 'Add label'}));
    const keyInputs = within(dialog).getAllByRole('textbox', {name: 'Key'});
    const valueInputs = within(dialog).getAllByRole('textbox', {name: 'Value'});
    await user.type(keyInputs.at(-1)!, 'environment');
    await user.type(valueInputs.at(-1)!, 'production');
    await user.click(within(dialog).getByRole('button', {name: 'Save'}));

    expect(onUpdateLabels).toHaveBeenCalledWith({
      team: 'platform',
      environment: 'production',
    });
  });

  it('edits annotations from the shared metadata actions', async () => {
    const user = userEvent.setup();
    const {onUpdateAnnotations} = renderMenuItems();

    await user.click(screen.getByRole('menuitem', {name: 'Edit annotations…'}));
    const dialog = screen.getByRole('dialog', {name: 'Edit connection annotations'});
    await user.click(within(dialog).getByRole('button', {name: 'Add annotation'}));
    const keyInputs = within(dialog).getAllByRole('textbox', {name: 'Key'});
    const valueInputs = within(dialog).getAllByRole('textbox', {name: 'Value'});
    await user.type(keyInputs.at(-1)!, 'runbook');
    await user.type(valueInputs.at(-1)!, 'go/payments');
    await user.click(within(dialog).getByRole('button', {name: 'Save'}));

    expect(onUpdateAnnotations).toHaveBeenCalledWith({
      owner: 'payments',
      runbook: 'go/payments',
    });
  });
});
