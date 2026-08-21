// @vitest-environment jsdom
import * as React from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {act, cleanup, render, waitFor} from '@testing-library/react';
import {configureStore} from '@reduxjs/toolkit';
import {Provider} from 'react-redux';
import {MemoryRouter} from 'react-router-dom';
import {listActors} from '@authproxy/api';

vi.mock('@mui/x-data-grid', () => ({
    DataGrid: () => null,
}));

vi.mock('nuqs', () => ({
    parseAsInteger: {withDefault: () => ({})},
    parseAsString: {withDefault: () => ({})},
    useQueryState: (key: string) => [key === 'page' ? 1 : key === 'pageSize' ? 20 : '', vi.fn()],
}));

vi.mock('@authproxy/api', () => ({
    NAMESPACE_PATH_SEPARATOR: '.',
    ROOT_NAMESPACE_PATH: 'root',
    NamespaceState: {ACTIVE: 'active'},
    listActors: vi.fn(),
    namespaceAndChildren: (path: string) => `${path}.**`,
    namespaces: {
        getByPath: vi.fn(),
        list: vi.fn(),
    },
}));

import Actors, {columns} from './Actors';

const initialNamespaceState = {
    currentPath: 'root',
    hasInitialized: true,
    current: null,
    status: 'succeeded',
    error: null,
    children: [],
    childrenHasMore: false,
    childrenStatus: 'succeeded',
    childrenError: null,
};

function renderActors() {
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
                <Actors />
            </MemoryRouter>
        </Provider>,
    );

    return store;
}

describe('Actors table columns', () => {
    beforeEach(() => {
        vi.mocked(listActors).mockResolvedValue({
            status: 200,
            data: {items: []},
        } as any);
    });

    afterEach(() => {
        cleanup();
        vi.clearAllMocks();
    });

    it('shows current actor fields', () => {
        expect(columns.map(({field}) => field)).toEqual([
            'name',
            'id',
            'externalId',
            'namespace',
            'createdAt',
            'updatedAt',
        ]);
    });

    it('refetches actors for the selected namespace and its descendants', async () => {
        const store = renderActors();

        await waitFor(() => {
            expect(listActors).toHaveBeenCalledWith(expect.objectContaining({namespace: 'root.**'}));
        });

        act(() => {
            store.dispatch({type: 'test/selectNamespace', payload: 'root.platform'});
        });

        await waitFor(() => {
            expect(listActors).toHaveBeenLastCalledWith(expect.objectContaining({
                namespace: 'root.platform.**',
            }));
        });
    });
});
