// @vitest-environment jsdom
import * as React from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {cleanup, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {Provider} from 'react-redux';
import {configureStore} from '@reduxjs/toolkit';
import namespaceReducer from '../store/namespacesSlice';
import NamespaceSelector, {
    childNamespacePath,
    validateNamespaceLeafName,
} from './NamespaceSelector';
import {NamespaceState, namespaces, ROOT_NAMESPACE_PATH} from '@authproxy/api';
import {MemoryRouter, useLocation} from 'react-router-dom';

const openCommandPalette = vi.hoisted(() => vi.fn());

vi.mock('../search/CommandPalette', () => ({
    useCommandPalette: () => ({open: openCommandPalette}),
}));

vi.mock('@authproxy/api', () => {
    const apiNamespaces = {
        create: vi.fn(),
        getByPath: vi.fn(),
        list: vi.fn(),
    };

    return {
        NAMESPACE_PATH_SEPARATOR: '.',
        ROOT_NAMESPACE_PATH: 'root',
        NamespaceState: {
            ACTIVE: 'active',
            DESTROYING: 'destroying',
            DESTROYED: 'destroyed',
        },
        namespaces: apiNamespaces,
    };
});

const rootNamespace = {
    path: ROOT_NAMESPACE_PATH,
    name: ROOT_NAMESPACE_PATH,
    state: NamespaceState.ACTIVE,
    createdAt: '2026-06-20T00:00:00.000Z',
    updatedAt: '2026-06-20T00:00:00.000Z',
};

function CurrentPath() {
    return <output aria-label="current path">{useLocation().pathname}</output>;
}

function renderSelector({
    childrenHasMore = false,
    currentPath = ROOT_NAMESPACE_PATH,
}: {childrenHasMore?: boolean; currentPath?: string} = {}) {
    const currentNamespace = {
        ...rootNamespace,
        path: currentPath,
        name: currentPath.split('.').at(-1) || currentPath,
    };
    const store = configureStore({
        reducer: {
            namespaces: namespaceReducer,
        },
        preloadedState: {
            namespaces: {
                currentPath,
                hasInitialized: true,
                current: currentNamespace,
                status: 'succeeded' as const,
                error: null,
                children: [],
                childrenHasMore,
                childrenStatus: 'succeeded' as const,
                childrenError: null,
            },
        },
    });

    render(
        <Provider store={store}>
            <MemoryRouter initialEntries={['/home']}>
                <NamespaceSelector />
                <CurrentPath/>
            </MemoryRouter>
        </Provider>,
    );

    return store;
}

describe('NamespaceSelector', () => {
    beforeEach(() => {
        vi.mocked(namespaces.getByPath).mockResolvedValue({status: 200, data: rootNamespace} as any);
        vi.mocked(namespaces.list).mockResolvedValue({status: 200, data: {items: []}} as any);
    });

    afterEach(() => {
        cleanup();
        vi.clearAllMocks();
    });

    it('creates a child namespace from the add namespace action', async () => {
        const user = userEvent.setup();
        const store = renderSelector();
        const createdNamespace = {
            ...rootNamespace,
            path: 'root.team-a',
            name: 'team-a',
        };
        vi.mocked(namespaces.create).mockResolvedValue({status: 200, data: createdNamespace} as any);
        vi.mocked(namespaces.getByPath).mockResolvedValue({status: 200, data: createdNamespace} as any);

        await user.click(screen.getByRole('combobox', {name: 'Select namespace'}));
        await user.click(await screen.findByText('Add namespace'));
        expect(screen.getByRole('dialog', {name: 'Add namespace'})).toBeTruthy();

        await user.type(screen.getByLabelText('Name'), 'team-a');
        await user.click(screen.getByRole('button', {name: 'Create'}));

        await waitFor(() => {
            expect(namespaces.create).toHaveBeenCalledWith({path: 'root.team-a'});
        });
        expect(store.getState().namespaces.currentPath).toBe('root.team-a');
    });

    it('builds and validates child namespace names', () => {
        expect(childNamespacePath('root.platform', 'team-a')).toBe('root.platform.team-a');
        expect(validateNamespaceLeafName('team-a')).toBeNull();
        expect(validateNamespaceLeafName('')).toBe('Name is required');
        expect(validateNamespaceLeafName('team.alpha')).toBe('Enter only the child namespace name');
        expect(validateNamespaceLeafName('-team')).toBe('Use letters, numbers, underscores, and hyphens');
    });

    it('hands off to namespace search when the child page is truncated', async () => {
        const user = userEvent.setup();
        renderSelector({childrenHasMore: true});

        await user.click(screen.getByRole('combobox', {name: 'Select namespace'}));
        await user.click(await screen.findByText('Search more namespaces'));

        expect(openCommandPalette).toHaveBeenCalledWith('type:namespace scope:current ');
    });

    it('opens the selected namespace resource without changing scope', async () => {
        const user = userEvent.setup();
        const store = renderSelector({currentPath: 'root.platform'});

        await user.click(screen.getByRole('combobox', {name: 'Select namespace'}));
        await user.click(await screen.findByText('View Current'));

        expect(screen.getByLabelText('current path').textContent).toBe('/namespaces/root.platform');
        expect(store.getState().namespaces.currentPath).toBe('root.platform');
    });

    it('offers go to root outside root and hides it at root', async () => {
        const user = userEvent.setup();
        const store = renderSelector({currentPath: 'root.platform'});

        await user.click(screen.getByRole('combobox', {name: 'Select namespace'}));
        await user.click(await screen.findByText('Go to root'));

        await waitFor(() => expect(store.getState().namespaces.currentPath).toBe(ROOT_NAMESPACE_PATH));

        cleanup();
        renderSelector();
        await user.click(screen.getByRole('combobox', {name: 'Select namespace'}));
        expect(screen.queryByText('Go to root')).toBeNull();
    });
});
