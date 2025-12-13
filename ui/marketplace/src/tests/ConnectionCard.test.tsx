import * as React from 'react';
import {render, screen} from '@testing-library/react';
import '@testing-library/jest-dom';
import {Provider} from 'react-redux';
import {combineReducers, configureStore} from '@reduxjs/toolkit';
import ConnectionCard, {ConnectionCardSkeleton} from '../components/ConnectionCard';
import {Connection, ConnectionState, Connector, ConnectorVersionState} from '@authproxy/api';
import authReducer from '../store/sessionSlice';
import connectorsReducer from '../store/connectorsSlice';
import connectionsReducer from '../store/connectionsSlice';
import toastsReducer from '../store/toastsSlice';
import {describe, expect, test} from 'vitest';

// Create a mock store with required reducers
const createMockStore = (preloaded?: Partial<ReturnType<typeof rootInitialState>>) => {
    return configureStore({
        reducer: combineReducers({
            auth: authReducer,
            connectors: connectorsReducer,
            connections: connectionsReducer,
            toasts: toastsReducer,
        }),
        preloadedState: preloaded as any,
    });
};

const rootInitialState = () => ({
    auth: undefined,
    connectors: {items: [], status: 'idle', error: null},
    connections: {
        items: [],
        status: 'idle',
        error: null,
        initiatingConnection: false,
        initiationError: null,
        disconnectingConnection: false,
        disconnectionError: null,
        currentTaskId: null
    },
    toasts: {items: []},
});

describe('ConnectionCard', () => {
    const mockConnector: Connector = {
        id: 'google-calendar',
        namespace: 'root',
        version: 1,
        state: ConnectorVersionState.ACTIVE,
        type: 'oauth2',
        display_name: 'Google Calendar',
        description: 'Connect to your Google Calendar to manage events and appointments.',
        highlight: undefined,
        logo: 'https://example.com/google-calendar-logo.png',
        versions: 1,
        states: [ConnectorVersionState.ACTIVE],
        created_at: '2023-04-01T12:00:00Z',
        updated_at: '2023-04-01T12:00:00Z',
    };

    const baseConnection: Connection = {
        id: '123e4567-e89b-12d3-a456-426614174000',
        namespace: 'root',
        connector: mockConnector,
        state: ConnectionState.CONNECTED,
        created_at: '2023-04-01T12:00:00Z',
        updated_at: '2023-04-01T12:00:00Z',
    };

    test('renders connection information correctly with connector details', () => {
        const store = createMockStore(rootInitialState());

        render(
            <Provider store={store}>
                <ConnectionCard connection={baseConnection}/>
            </Provider>
        );

        // Check if the connector name is displayed
        expect(screen.getByText('Google Calendar')).toBeInTheDocument();

        // Check if the connection date is displayed
        expect(screen.getByText(/Connected on/)).toBeInTheDocument();

        // Check if the status chip label is displayed (state string)
        expect(screen.getByText('connected')).toBeInTheDocument();
    });

    test('renders with unknown connector fallback when connector missing', () => {
        const store = createMockStore(rootInitialState());
        const connWithoutConnector = {...baseConnection, connector: undefined as unknown as any};

        render(
            <Provider store={store}>
                <ConnectionCard connection={connWithoutConnector}/>
            </Provider>
        );

        // Check if the unknown connector text is displayed
        expect(screen.getByText('Unknown Connector')).toBeInTheDocument();
    });

    test('renders different status labels based on connection state', () => {
        const states = [
            {state: ConnectionState.CONNECTED, label: 'connected'},
            {state: ConnectionState.CREATED, label: 'created'},
            {state: ConnectionState.FAILED, label: 'failed'},
            {state: ConnectionState.DISCONNECTED, label: 'disconnected'},
        ];

        states.forEach(({state, label}) => {
            const store = createMockStore(rootInitialState());
            const connection = {...baseConnection, state};

            const {unmount} = render(
                <Provider store={store}>
                    <ConnectionCard connection={connection}/>
                </Provider>
            );

            // Check if the status label is displayed
            expect(screen.getByText(label)).toBeInTheDocument();

            unmount();
        });
    });

    test('renders skeleton correctly', () => {
        render(<ConnectionCardSkeleton/>);

        // Check if the skeleton elements are in the document
        const skeletons = document.querySelectorAll('.MuiSkeleton-root');
        expect(skeletons.length).toBeGreaterThan(0);
    });
});