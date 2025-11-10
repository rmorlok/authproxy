import * as React from 'react';
import {render, screen} from '@testing-library/react';
import '@testing-library/jest-dom';
import {Provider} from 'react-redux';
import {combineReducers, configureStore} from '@reduxjs/toolkit';
import {MemoryRouter} from 'react-router-dom';
import ConnectionList from '../components/ConnectionList';
import authReducer from '../store/sessionSlice';
import connectorsReducer from '../store/connectorsSlice';
import connectionsReducer from '../store/connectionsSlice';
import toastsReducer from '../store/toastsSlice';
import {Connection, ConnectionState, Connector, ConnectorVersionState} from '@authproxy/api';
import {describe, expect, test} from 'vitest';

function createStore(preloadedState?: any) {
    return configureStore({
        reducer: combineReducers({
            auth: authReducer,
            connectors: connectorsReducer,
            connections: connectionsReducer,
            toasts: toastsReducer,
        }),
        preloadedState,
    });
}

const connector: Connector = {
    id: 'google-calendar',
    version: 1,
    state: ConnectorVersionState.ACTIVE,
    type: 'oauth2',
    display_name: 'Google Calendar',
    description: 'Calendar app',
    highlight: undefined,
    logo: 'https://example.com/logo.png',
    versions: 1,
    states: [ConnectorVersionState.ACTIVE],
    created_at: '2023-04-01T12:00:00Z',
    updated_at: '2023-04-01T12:00:00Z',
};

const makeConnection = (overrides: Partial<Connection> = {}): Connection => ({
    id: 'c-1',
    connector: connector,
    state: ConnectionState.CONNECTED,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
});

describe('ConnectionList', () => {
    test('renders skeletons when loading', () => {
        const store = createStore({
            connectors: {items: [], status: 'succeeded', error: null},
            connections: {
                items: [],
                status: 'loading',
                error: null,
                initiatingConnection: false,
                initiationError: null,
                disconnectingConnection: false,
                disconnectionError: null,
                currentTaskId: null,
            },
            toasts: {items: []},
        });

        render(
            <MemoryRouter>
                <Provider store={store}>
                    <ConnectionList/>
                </Provider>
            </MemoryRouter>
        );

        expect(document.querySelectorAll('.MuiSkeleton-root').length).toBeGreaterThan(0);
    });

    test('shows error alert on failed state', () => {
        const store = createStore({
            connectors: {items: [], status: 'succeeded', error: null},
            connections: {
                items: [],
                status: 'failed',
                error: 'Failed to fetch connections',
                initiatingConnection: false,
                initiationError: null,
                disconnectingConnection: false,
                disconnectionError: null,
                currentTaskId: null,
            },
            toasts: {items: []},
        });

        render(
            <MemoryRouter>
                <Provider store={store}>
                    <ConnectionList/>
                </Provider>
            </MemoryRouter>
        );

        expect(screen.getByText('Failed to fetch connections')).toBeInTheDocument();
    });

    test('shows empty state with call to action when there are no connections', () => {
        const store = createStore({
            connectors: {items: [], status: 'succeeded', error: null},
            connections: {
                items: [],
                status: 'succeeded',
                error: null,
                initiatingConnection: false,
                initiationError: null,
                disconnectingConnection: false,
                disconnectionError: null,
                currentTaskId: null,
            },
            toasts: {items: []},
        });

        render(
            <MemoryRouter>
                <Provider store={store}>
                    <ConnectionList/>
                </Provider>
            </MemoryRouter>
        );

        expect(screen.getByText('No connections yet')).toBeInTheDocument();
        expect(screen.getByRole('link', {name: /Connect an Application/i})).toBeInTheDocument();
    });

    test('renders list of connections when present', () => {
        const items = [makeConnection({id: 'c-1'}), makeConnection({id: 'c-2'})];
        const store = createStore({
            connectors: {items: [connector], status: 'succeeded', error: null},
            connections: {
                items,
                status: 'succeeded',
                error: null,
                initiatingConnection: false,
                initiationError: null,
                disconnectingConnection: false,
                disconnectionError: null,
                currentTaskId: null,
            },
            toasts: {items: []},
        });

        render(
            <MemoryRouter>
                <Provider store={store}>
                    <ConnectionList/>
                </Provider>
            </MemoryRouter>
        );

        expect(screen.getByText('Your Connections')).toBeInTheDocument();
        // The ConnectionCard shows connector display name
        expect(screen.getAllByText('Google Calendar').length).toBeGreaterThanOrEqual(1);
        // And the secondary button should appear
        expect(screen.getByRole('link', {name: /Connect More/i})).toBeInTheDocument();
    });
});
