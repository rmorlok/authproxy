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
        API_VERSION: 'authproxy.net/v1alpha1',
        NAMESPACE_KIND: 'Namespace',
        NAMESPACE_PATH_SEPARATOR: '.',
        ROOT_NAMESPACE_PATH: 'root',
        NamespaceState: {
            ACTIVE: 'active',
            DISCONNECTING: 'disconnecting',
            DISCONNECTED: 'disconnected',
        },
        namespaces: apiNamespaces,
    };
});

const rootNamespace = {
    apiVersion: 'authproxy.net/v1alpha1' as const,
    kind: 'Namespace' as const,
    metadata: {
        id: ROOT_NAMESPACE_PATH,
        name: ROOT_NAMESPACE_PATH,
        createdAt: '2026-06-20T00:00:00.000Z',
        updatedAt: '2026-06-20T00:00:00.000Z',
    },
    spec: {},
    status: {state: NamespaceState.ACTIVE},
};

function renderSelector({childrenHasMore = false}: {childrenHasMore?: boolean} = {}) {
    const store = configureStore({
        reducer: {
            namespaces: namespaceReducer,
        },
        preloadedState: {
            namespaces: {
                currentPath: ROOT_NAMESPACE_PATH,
                hasInitialized: true,
                current: rootNamespace,
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
            <NamespaceSelector />
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
            metadata: {...rootNamespace.metadata, id: 'root.team-a', name: 'team-a', namespace: 'root'},
        };
        vi.mocked(namespaces.create).mockResolvedValue({status: 200, data: createdNamespace} as any);
        vi.mocked(namespaces.getByPath).mockResolvedValue({status: 200, data: createdNamespace} as any);

        await user.click(screen.getByRole('combobox', {name: 'Select namespace'}));
        await user.click(await screen.findByText('Add namespace'));
        expect(screen.getByRole('dialog', {name: 'Add namespace'})).toBeTruthy();

        await user.type(screen.getByLabelText('Name'), 'team-a');
        await user.click(screen.getByRole('button', {name: 'Create'}));

        await waitFor(() => {
            expect(namespaces.create).toHaveBeenCalledWith({
                apiVersion: 'authproxy.net/v1alpha1',
                kind: 'Namespace',
                metadata: {name: 'team-a', namespace: 'root'},
                spec: {},
            });
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
});
