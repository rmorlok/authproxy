// @vitest-environment jsdom
import React from 'react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {cleanup, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ResourceNameEditor from './ResourceNameEditor';

describe('ResourceNameEditor', () => {
    afterEach(cleanup);

    it('shows the name as primary text and saves a rename by id-backed callback', async () => {
        const user = userEvent.setup();
        const onRename = vi.fn().mockResolvedValue(undefined);
        render(<ResourceNameEditor name="production-crm" resourceType="Connection" onRename={onRename}/>);

        expect(screen.getByRole('heading', {name: 'production-crm'})).toBeTruthy();
        await user.click(screen.getByRole('button', {name: 'Rename connection'}));
        const input = screen.getByRole('textbox', {name: 'Name'});
        await user.clear(input);
        await user.type(input, 'customer-crm');
        await user.click(screen.getByRole('button', {name: 'Save'}));

        await waitFor(() => expect(onRename).toHaveBeenCalledWith('customer-crm'));
    });

    it('rejects invalid names before calling the API', async () => {
        const user = userEvent.setup();
        const onRename = vi.fn();
        render(<ResourceNameEditor name="valid" resourceType="Key" onRename={onRename}/>);

        await user.click(screen.getByRole('button', {name: 'Rename key'}));
        const input = screen.getByRole('textbox', {name: 'Name'});
        await user.clear(input);
        await user.type(input, '-invalid');
        await user.click(screen.getByRole('button', {name: 'Save'}));

        expect(await screen.findByText(/Use 1–63 characters/)).toBeTruthy();
        expect(onRename).not.toHaveBeenCalled();
    });
});
