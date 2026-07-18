// @vitest-environment jsdom
import * as React from 'react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import HomeTimeRangePicker from './HomeTimeRangePicker';

describe('HomeTimeRangePicker', () => {
    afterEach(() => {
        cleanup();
        vi.clearAllMocks();
    });

    it('applies typed Grafana time range expressions', async () => {
        const user = userEvent.setup();
        const onApply = vi.fn();
        render(
            <HomeTimeRangePicker
                value={{from: 'now-24h', to: 'now'}}
                onApply={onApply}
            />,
        );

        await user.click(screen.getByRole('button', {name: 'Last 24 hours'}));
        expect(document.querySelector('button[aria-label="Copy time range"]')).not.toBeNull();
        expect(document.querySelector('button[aria-label="Paste time range"]')).not.toBeNull();
        expect(screen.getByRole('button', {name: 'Select From date range'})).not.toBeNull();
        expect(screen.getByRole('button', {name: 'Select To date range'})).not.toBeNull();

        fireEvent.change(screen.getByLabelText('From'), {target: {value: '2026-07-17 00:00:00'}});
        fireEvent.change(screen.getByLabelText('To'), {target: {value: '2026-07-17 23:59:59'}});

        await user.click(screen.getByRole('button', {name: 'Apply time range'}));
        expect(onApply).toHaveBeenCalledWith({
            from: '2026-07-17 00:00:00',
            to: '2026-07-17 23:59:59',
        });
    });

    it('opens the date range calendar from the field calendar buttons', async () => {
        const user = userEvent.setup();
        render(
            <HomeTimeRangePicker
                value={{from: '2026-07-09 00:00:00', to: '2026-07-10 23:59:59'}}
                onApply={vi.fn()}
            />,
        );

        await user.click(screen.getByRole('button', {name: 'Jul 9, 2026 00:00:00 to Jul 10, 2026 23:59:59'}));
        await user.click(screen.getByRole('button', {name: 'Select From date range'}));

        expect(await screen.findByText('Select a time range')).not.toBeNull();
        expect(screen.getByRole('button', {name: 'Close calendar'})).not.toBeNull();
    });

    it('applies dates selected from the calendar range panel', async () => {
        const user = userEvent.setup();
        const onApply = vi.fn();
        render(
            <HomeTimeRangePicker
                value={{from: '2026-07-09 00:00:00', to: '2026-07-10 23:59:59'}}
                onApply={onApply}
            />,
        );

        await user.click(screen.getByRole('button', {name: 'Jul 9, 2026 00:00:00 to Jul 10, 2026 23:59:59'}));
        await user.click(screen.getByRole('button', {name: 'Select From date range'}));
        await user.click(await screen.findByRole('button', {name: 'Select Jul 8, 2026'}));
        await user.click(screen.getByRole('button', {name: 'Select To date range'}));
        await user.click(screen.getByRole('button', {name: 'Select Jul 12, 2026'}));
        await user.click(screen.getByRole('button', {name: 'Apply time range'}));

        expect(onApply).toHaveBeenCalledWith({
            from: '2026-07-08 00:00:00',
            to: '2026-07-12 23:59:59',
        });
    });

    it('applies quick ranges immediately', async () => {
        const user = userEvent.setup();
        const onApply = vi.fn();
        render(
            <HomeTimeRangePicker
                value={{from: 'now-24h', to: 'now'}}
                onApply={onApply}
            />,
        );

        await user.click(screen.getByRole('button', {name: 'Last 24 hours'}));
        await user.click(await screen.findByText('Last 1 hour'));

        expect(onApply).toHaveBeenCalledWith({from: 'now-1h', to: 'now'});
    });
});
