// @vitest-environment jsdom
import * as React from 'react';
import {act, cleanup, render, waitFor} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {configureStore} from '@reduxjs/toolkit';
import {Provider} from 'react-redux';
import {MemoryRouter} from 'react-router-dom';
import {namespaces} from '@authproxy/api';

vi.mock('@mui/x-data-grid', () => ({
    DataGrid: () => null,
}));

vi.mock('nuqs', () => ({
    parseAsInteger: {withDefault: () => ({})},
    parseAsString: {withDefault: () => ({})},
    parseAsStringLiteral: () => ({withDefault: () => ({})}),
    useQueryState: (key: string) => [key === 'page' ? 1 : key === 'pageSize' ? 20 : '', vi.fn()],
}));

vi.mock('@authproxy/api', () => ({
    NAMESPACE_PATH_SEPARATOR: '.',
    ROOT_NAMESPACE_PATH: 'root',
    NamespaceState: {
        ACTIVE: 'active',
        DESTROYING: 'destroying',
        DESTROYED: 'destroyed',
    },
    namespaceAndChildren: (path: string) => `${path}.**`,
    namespaces: {
        list: vi.fn(),
    },
}));

import Namespaces, {columns} from './Namespaces';

const initialNamespaceState = {
    currentPath: 'root',
};

function renderNamespaces() {
    const store = configureStore({
        reducer: {
            namespaces: (state = initialNamespaceState, action: {type: string; payload?: string}) =>
                action.type === 'test/selectNamespace'
                    ? {...state, currentPath: action.payload || 'root'}
                    : state,
        },
    });

    render(
        <Provider store={store}>
            <MemoryRouter>
                <Namespaces/>
            </MemoryRouter>
        </Provider>,
    );

    return store;
}

describe('Namespaces table', () => {
    beforeEach(() => {
        vi.mocked(namespaces.list).mockResolvedValue({
            status: 200,
            data: {items: []},
        } as any);
    });

    afterEach(() => {
        cleanup();
        vi.clearAllMocks();
    });

    it('shows namespace resource fields', () => {
        expect(columns.map(({field}) => field)).toEqual([
            'name',
            'path',
            'state',
            'keyId',
            'labels',
            'createdAt',
            'updatedAt',
        ]);
    });

    it('refetches namespaces at or below the selected namespace', async () => {
        const store = renderNamespaces();

        await waitFor(() => {
            expect(namespaces.list).toHaveBeenCalledWith(expect.objectContaining({namespace: 'root.**'}));
        });

        act(() => {
            store.dispatch({type: 'test/selectNamespace', payload: 'root.platform'});
        });

        await waitFor(() => {
            expect(namespaces.list).toHaveBeenLastCalledWith(expect.objectContaining({
                namespace: 'root.platform.**',
            }));
        });
    });
});
