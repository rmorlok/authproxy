// @vitest-environment jsdom
import * as React from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {cleanup, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {Provider} from 'react-redux';
import {configureStore} from '@reduxjs/toolkit';
import {MemoryRouter, useLocation} from 'react-router-dom';
import namespaceReducer from '../store/namespacesSlice';
import {CommandPaletteProvider, useCommandPalette} from './CommandPalette';
import {SearchResourceCache} from './cache';

const searchResourcesMock = vi.hoisted(() => vi.fn());

vi.mock('@authproxy/api', async (importOriginal) => ({
    ...(await importOriginal<typeof import('@authproxy/api')>()),
    searchResources: searchResourcesMock,
}));

class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
}

function Trigger() {
    const palette = useCommandPalette();
    return (
        <>
            <button onClick={() => palette.open()}>Open search</button>
            <button onClick={() => palette.open('type:namespace')}>Search namespaces</button>
            <button onClick={() => palette.open('type:connection pay')}>Search connections</button>
            <button onClick={() => palette.open('type:bogus')}>Invalid search</button>
        </>
    );
}

function CurrentPath() {
    return <output aria-label="current path">{useLocation().pathname}</output>;
}

function renderPalette({debounceMs = 300}: {debounceMs?: number} = {}) {
    const store = configureStore({
        reducer: {namespaces: namespaceReducer},
        preloadedState: {
            namespaces: {
                currentPath: 'root',
                hasInitialized: true,
                current: null,
                status: 'succeeded' as const,
                error: null,
                children: [],
                childrenHasMore: false,
                childrenStatus: 'succeeded' as const,
                childrenError: null,
            },
        },
    });

    render(
        <Provider store={store}>
            <MemoryRouter initialEntries={['/home']}>
                <CommandPaletteProvider cache={new SearchResourceCache()} debounceMs={debounceMs}>
                    <Trigger />
                    <CurrentPath />
                </CommandPaletteProvider>
            </MemoryRouter>
        </Provider>,
    );
}

describe('CommandPalette', () => {
    beforeEach(() => {
        vi.stubGlobal('ResizeObserver', ResizeObserverStub);
        Element.prototype.scrollIntoView = vi.fn();
        searchResourcesMock.mockResolvedValue({
            status: 200,
            data: {items: [], truncated_types: [], incomplete_types: []},
        } as any);
    });

    afterEach(() => {
        cleanup();
        vi.clearAllMocks();
        vi.unstubAllGlobals();
    });

    it('opens globally, seeds local results, filters commands, and routes direct IDs', async () => {
        const user = userEvent.setup();
        renderPalette();

        await user.keyboard('{Control>}k{/Control}');
        const input = await screen.findByPlaceholderText('Search resources or enter a command…');

        await waitFor(() => expect(searchResourcesMock).toHaveBeenCalledWith(
            expect.objectContaining({mode: 'seed', namespace: 'root.**'}),
            expect.objectContaining({signal: expect.any(AbortSignal)}),
        ));

        await user.type(input, 'work');
        expect(await screen.findByText('Workflows')).toBeTruthy();
        await user.clear(input);
        await user.type(input, 'cxn_customer');
        await user.keyboard('{Enter}');

        expect(screen.getByLabelText('current path').textContent).toBe('/connections/cxn_customer');
        await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    });

    it('cancels superseded remote searches and ignores their stale results', async () => {
        const user = userEvent.setup();
        let staleSignal: AbortSignal | undefined;
        let resolveStale: ((value: any) => void) | undefined;
        const staleResponse = new Promise((resolve) => { resolveStale = resolve; });
        searchResourcesMock.mockImplementation((params, config) => {
            if (params.mode === 'seed') {
                return Promise.resolve({
                    status: 200,
                    data: {items: [], truncated_types: ['connection'], incomplete_types: []},
                });
            }
            if (params.q === 'pay') {
                staleSignal = config?.signal;
                return staleResponse;
            }
            return Promise.resolve({
                status: 200,
                data: {
                    items: [{
                        resource_type: 'connection',
                        resource_id: 'cxn_payroll',
                        namespace: 'root',
                        labels: {name: 'Payroll'},
                        matched_labels: [{key: 'name', value: 'Payroll'}],
                        updated_at: '2026-07-12T00:00:00Z',
                    }],
                    truncated_types: [],
                    incomplete_types: [],
                },
            });
        });
        renderPalette({debounceMs: 50});
        await user.click(screen.getByRole('button', {name: 'Open search'}));
        const input = await screen.findByPlaceholderText('Search resources or enter a command…');

        await user.type(input, 'pay');
        expect(searchResourcesMock.mock.calls.filter(([params]) => params.mode === 'query')).toHaveLength(0);
        await waitFor(() => expect(staleSignal).toBeTruthy());
        await user.type(input, 'r');

        await waitFor(() => expect(staleSignal?.aborted).toBe(true));
        expect(await screen.findByText('Payroll')).toBeTruthy();
        resolveStale?.({
            status: 200,
            data: {
                items: [{
                    resource_type: 'connection',
                    resource_id: 'cxn_stale',
                    namespace: 'root',
                    labels: {name: 'Stale'},
                    matched_labels: [{key: 'name', value: 'Stale'}],
                    updated_at: '2026-07-12T00:00:00Z',
                }],
                truncated_types: [],
                incomplete_types: [],
            },
        });
        await Promise.resolve();
        expect(screen.queryByText('Stale')).toBeNull();
    });

    it('keeps qualifier-only searches local and explains how to search the server', async () => {
        const user = userEvent.setup();
        renderPalette({debounceMs: 0});

        await user.click(screen.getByRole('button', {name: 'Search namespaces'}));
        expect(await screen.findByText(/Showing cached matches only/)).toBeTruthy();
        await waitFor(() => expect(searchResourcesMock).toHaveBeenCalledWith(
            expect.objectContaining({mode: 'seed'}),
            expect.anything(),
        ));
        expect(searchResourcesMock.mock.calls.filter(([params]) => params.mode === 'query')).toHaveLength(0);
    });

    it('queries remotely for system-label selectors even after a complete user-label seed', async () => {
        const user = userEvent.setup();
        renderPalette({debounceMs: 0});
        await user.click(screen.getByRole('button', {name: 'Open search'}));
        await waitFor(() => expect(searchResourcesMock).toHaveBeenCalledWith(
            expect.objectContaining({mode: 'seed'}),
            expect.anything(),
        ));

        const input = await screen.findByPlaceholderText('Search resources or enter a command…');
        await user.type(input, 'label:apxy/cxn/-/ns=root');

        await waitFor(() => expect(searchResourcesMock).toHaveBeenCalledWith(
            expect.objectContaining({
                mode: 'query',
                label_selector: 'apxy/cxn/-/ns=root',
            }),
            expect.anything(),
        ));
    });

    it('supports arrow and Enter selection and restores trigger focus after Escape', async () => {
        const user = userEvent.setup();
        searchResourcesMock.mockResolvedValue({
            status: 200,
            data: {
                items: [{
                    resource_type: 'connection',
                    resource_id: 'cxn_payments',
                    namespace: 'root',
                    labels: {env: 'prod', name: 'Payments'},
                    matched_labels: [],
                    updated_at: '2026-07-12T00:00:00Z',
                }],
                truncated_types: [],
                incomplete_types: [],
            },
        } as any);
        renderPalette();

        await user.click(screen.getByRole('button', {name: 'Search connections'}));
        await screen.findByText('Payments');
        await user.keyboard('{ArrowDown}{Enter}');
        expect(screen.getByLabelText('current path').textContent).toBe('/connections/cxn_payments');
        await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());

        const trigger = screen.getByRole('button', {name: 'Open search'});
        await user.click(trigger);
        await screen.findByRole('dialog');
        await user.keyboard('{Escape}');
        await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
        expect(document.activeElement).toBe(trigger);
    });

    it('shows syntax, local-only, empty, and partial-result states without invalid requests', async () => {
        const user = userEvent.setup();
        searchResourcesMock.mockImplementation((params) => {
            if (params.mode === 'seed') {
                return Promise.resolve({
                    status: 200,
                    data: {items: [], truncated_types: ['connection'], incomplete_types: []},
                });
            }
            return Promise.resolve({
                status: 200,
                data: {items: [], truncated_types: ['connection'], incomplete_types: ['actor']},
            });
        });
        renderPalette({debounceMs: 0});
        await user.click(screen.getByRole('button', {name: 'Invalid search'}));
        const input = await screen.findByPlaceholderText('Search resources or enter a command…');
        await waitFor(() => expect(searchResourcesMock).toHaveBeenCalledWith(
            expect.objectContaining({mode: 'seed'}),
            expect.anything(),
        ));
        expect(await screen.findByText(/Unknown resource type/)).toBeTruthy();
        expect(searchResourcesMock.mock.calls.filter(([params]) => params.mode === 'query')).toHaveLength(0);

        await user.clear(input);
        await user.type(input, 'zz');
        expect(await screen.findByText(/Showing local matches/)).toBeTruthy();

        await user.type(input, 'z');
        await waitFor(() => expect(screen.getByText(/Some resource types did not finish: actor/)).toBeTruthy());
        expect(screen.getByText(/Results are partial for connection/)).toBeTruthy();
    });

    it('keeps commands and cached behavior available when remote search fails', async () => {
        const user = userEvent.setup();
        searchResourcesMock.mockImplementation((params) => {
            if (params.mode === 'seed') {
                return Promise.resolve({
                    status: 200,
                    data: {items: [], truncated_types: ['connection'], incomplete_types: []},
                });
            }
            return Promise.reject(new Error('offline'));
        });
        renderPalette({debounceMs: 0});
        await user.click(screen.getByRole('button', {name: 'Open search'}));
        const input = await screen.findByPlaceholderText('Search resources or enter a command…');
        await user.type(input, 'payments');

        expect(await screen.findByText(/Server search is unavailable/)).toBeTruthy();
        await user.clear(input);
        await user.type(input, 'work');
        expect(await screen.findByText('Workflows')).toBeTruthy();
    });
});
