// @vitest-environment jsdom
import * as React from 'react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {cleanup, render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Search from './Search';
import AppNavbar from './AppNavbar';

const openCommandPalette = vi.hoisted(() => vi.fn());

vi.mock('../search/CommandPalette', () => ({
    useCommandPalette: () => ({open: openCommandPalette, close: vi.fn()}),
}));
vi.mock('./SideMenuMobile', () => ({default: () => null}));
vi.mock('./ColorModeIconDropdown', () => ({default: () => null}));

describe('command palette triggers', () => {
    afterEach(() => {
        cleanup();
        vi.clearAllMocks();
    });

    it('opens from the desktop search field with click or keyboard', async () => {
        const user = userEvent.setup();
        render(<Search />);
        const trigger = screen.getByRole('textbox', {name: 'Open resource search'});

        await user.click(trigger);
        await user.keyboard('{Enter}');

        expect(openCommandPalette).toHaveBeenCalledTimes(2);
    });

    it('opens from the mobile navigation action', async () => {
        const user = userEvent.setup();
        render(<AppNavbar />);

        await user.click(screen.getByRole('button', {name: 'Open resource search'}));

        expect(openCommandPalette).toHaveBeenCalledTimes(1);
    });
});
