import {configureStore} from '@reduxjs/toolkit';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {NamespaceState, namespaces, ROOT_NAMESPACE_PATH} from '@authproxy/api';
import namespaceReducer, {
    isValidNamespacePath,
    normalizeNamespacePath,
    setCurrentNamespace,
} from './namespacesSlice';

vi.mock('@authproxy/api', () => {
    const apiNamespaces = {
        getByPath: vi.fn(),
        list: vi.fn(),
    };

    return {
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
    path: ROOT_NAMESPACE_PATH,
    state: NamespaceState.ACTIVE,
    created_at: '2026-06-20T00:00:00.000Z',
    updated_at: '2026-06-20T00:00:00.000Z',
};

function createStore() {
    return configureStore({
        reducer: {
            namespaces: namespaceReducer,
        },
    });
}

describe('namespacesSlice namespace path handling', () => {
    beforeEach(() => {
        vi.mocked(namespaces.getByPath).mockResolvedValue({status: 200, data: rootNamespace} as any);
        vi.mocked(namespaces.list).mockResolvedValue({status: 200, data: {items: []}} as any);
    });

    afterEach(() => {
        vi.clearAllMocks();
    });

    it('recognizes valid namespace paths', () => {
        expect(isValidNamespacePath(ROOT_NAMESPACE_PATH)).toBe(true);
        expect(isValidNamespacePath('root.platform')).toBe(true);
        expect(isValidNamespacePath('root.platform.team-a')).toBe(true);
    });

    it('normalizes malformed namespace paths to root', () => {
        expect(normalizeNamespacePath('///action/refresh')).toBe(ROOT_NAMESPACE_PATH);
        expect(normalizeNamespacePath('...action.refresh')).toBe(ROOT_NAMESPACE_PATH);
        expect(normalizeNamespacePath('action.refresh')).toBe(ROOT_NAMESPACE_PATH);
        expect(normalizeNamespacePath('root..platform')).toBe(ROOT_NAMESPACE_PATH);
        expect(normalizeNamespacePath(null)).toBe(ROOT_NAMESPACE_PATH);
    });

    it('loads root instead of a malformed namespace path', () => {
        const store = createStore();

        store.dispatch(setCurrentNamespace('///action/refresh') as any);

        expect(store.getState().namespaces.currentPath).toBe(ROOT_NAMESPACE_PATH);
        expect(namespaces.getByPath).toHaveBeenCalledWith(ROOT_NAMESPACE_PATH);
        expect(namespaces.list).toHaveBeenCalledWith({
            children_of: ROOT_NAMESPACE_PATH,
            limit: 100,
        });
    });
});
