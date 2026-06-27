// @vitest-environment jsdom
import * as React from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {cleanup, render, screen, waitFor, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {MemoryRouter} from 'react-router-dom';
import KeyDetail from './KeyDetail';
import {KeyState, keys} from '@authproxy/api';

vi.mock('@authproxy/api', () => {
  const apiKeys = {
    delete: vi.fn(),
    deleteAnnotation: vi.fn(),
    get: vi.fn(),
    putAnnotation: vi.fn(),
    update: vi.fn(),
  };

  return {
    KeyState: {
      ACTIVE: 'active',
      DISABLED: 'disabled',
    },
    keys: apiKeys,
  };
});

const initialKey = {
  id: 'key_test',
  namespace: 'root.dev',
  state: KeyState.ACTIVE,
  key_data: {
    type: 'aws_kms',
    fields: {
      aws_kms_key_id: 'alias/authproxy',
      aws_region: 'us-east-1',
      aws_credentials_type: 'implicit',
    },
  },
  labels: {
    environment: 'dev',
  },
  annotations: {
    owner: 'platform',
  },
  created_at: '2026-06-20T00:00:00.000Z',
  updated_at: '2026-06-20T00:00:00.000Z',
};

function renderKeyDetail() {
  render(
    <MemoryRouter>
      <KeyDetail keyId="key_test" />
    </MemoryRouter>,
  );
}

describe('KeyDetail', () => {
  beforeEach(() => {
    vi.mocked(keys.get).mockResolvedValue({status: 200, data: initialKey} as any);
    vi.mocked(keys.update).mockResolvedValue({
      status: 200,
      data: {
        ...initialKey,
        state: KeyState.DISABLED,
        labels: {
          ...initialKey.labels,
          tier: 'internal',
        },
        annotations: {
          ...initialKey.annotations,
          rotation: 'manual',
        },
      },
    } as any);
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('edits key state, labels, and annotations', async () => {
    const user = userEvent.setup();
    renderKeyDetail();

    await screen.findByText('key_test');
    await user.click(screen.getByRole('button', {name: 'actions'}));
    await user.click(screen.getByRole('menuitem', {name: 'Edit...'}));

    const dialog = screen.getByRole('dialog', {name: 'Edit key'});
    await user.click(within(dialog).getByRole('combobox', {name: 'State'}));
    await user.click(screen.getByRole('option', {name: KeyState.DISABLED}));

    await user.click(within(dialog).getByRole('button', {name: 'Add label'}));
    await user.click(within(dialog).getByRole('button', {name: 'Add annotation'}));

    const keyInputs = within(dialog).getAllByLabelText('Key');
    const valueInputs = within(dialog).getAllByLabelText('Value');
    await user.type(keyInputs[1], 'tier');
    await user.type(valueInputs[1], 'internal');
    await user.type(keyInputs[3], 'rotation');
    await user.type(valueInputs[3], 'manual');

    await user.click(within(dialog).getByRole('button', {name: 'Save'}));

    await waitFor(() => {
      expect(keys.update).toHaveBeenCalledWith('key_test', {
        state: KeyState.DISABLED,
        labels: {
          environment: 'dev',
          tier: 'internal',
        },
        annotations: {
          owner: 'platform',
          rotation: 'manual',
        },
      });
    });
    await waitFor(() => {
      expect(screen.queryByRole('dialog', {name: 'Edit key'})).toBeNull();
    });
    expect(screen.getByText('tier: internal')).toBeTruthy();
    expect(screen.getByText('rotation: manual')).toBeTruthy();
  });

  it('edits key provider configuration', async () => {
    const user = userEvent.setup();
    renderKeyDetail();

    await screen.findByText('key_test');
    await user.click(screen.getByRole('button', {name: 'actions'}));
    await user.click(screen.getByRole('menuitem', {name: 'Edit...'}));

    const dialog = screen.getByRole('dialog', {name: 'Edit key'});
    const keyIdInput = within(dialog).getByLabelText('AWS KMS Key ID');
    await user.clear(keyIdInput);
    await user.type(keyIdInput, 'alias/authproxy-v2');

    await user.click(within(dialog).getByRole('button', {name: 'Save'}));

    await waitFor(() => {
      expect(keys.update).toHaveBeenCalledWith('key_test', {
        state: KeyState.ACTIVE,
        key_data: {
          aws_kms_key_id: 'alias/authproxy-v2',
          aws_region: 'us-east-1',
          aws_credentials: {
            type: 'implicit',
          },
        },
        labels: {
          environment: 'dev',
        },
        annotations: {
          owner: 'platform',
        },
      });
    });
  });
});
