import * as React from 'react';
import {render, screen, waitFor} from '@testing-library/react';
import '@testing-library/jest-dom';
import userEvent from '@testing-library/user-event';
import {Provider} from 'react-redux';
import {combineReducers, configureStore} from '@reduxjs/toolkit';
import ConnectionCard, {ConnectionCardSkeleton} from '../components/ConnectionCard';
import {
    Connection,
    ConnectionState,
    ConnectionHealthState,
    Connector,
    ConnectorVersionState,
    connections,
    tasks,
    PollForTaskResult,
} from '@authproxy/api';
import authReducer from '../store/sessionSlice';
import connectorsReducer from '../store/connectorsSlice';
import connectionsReducer from '../store/connectionsSlice';
import toastsReducer from '../store/toastsSlice';
import {beforeEach, describe, expect, test, vi} from 'vitest';

vi.mock('@authproxy/api', async () => {
    const actual = await vi.importActual<typeof import('@authproxy/api')>('@authproxy/api');
    return {
        ...actual,
        connections: {
            ...actual.connections,
            disconnect: vi.fn(),
            list: vi.fn(),
            reauth: vi.fn(),
        },
        tasks: {
            ...actual.tasks,
            pollForTaskFinalized: vi.fn(),
        },
    };
});

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
        display_name: 'Google Calendar',
        description: 'Connect to your Google Calendar to manage events and appointments.',
        highlight: undefined,
        logo: 'https://example.com/google-calendar-logo.png',
        has_configure: false,
        versions: 1,
        states: [ConnectorVersionState.ACTIVE],
        created_at: '2023-04-01T12:00:00Z',
        updated_at: '2023-04-01T12:00:00Z',
    };

    const baseConnection: Connection = {
        id: '123e4567-e89b-12d3-a456-426614174000',
        namespace: 'root',
        connector: mockConnector,
        state: ConnectionState.CONFIGURED,
        health_state: ConnectionHealthState.HEALTHY,
        created_at: '2023-04-01T12:00:00Z',
        updated_at: '2023-04-01T12:00:00Z',
    };

    beforeEach(() => {
        vi.mocked(connections.disconnect).mockReset();
        vi.mocked(connections.list).mockReset();
        vi.mocked(connections.reauth).mockReset();
        vi.mocked(tasks.pollForTaskFinalized).mockReset();
        vi.mocked(connections.disconnect).mockResolvedValue({
            data: {
                task_id: 'task-123',
                connection: {
                    ...baseConnection,
                    state: ConnectionState.DISCONNECTING,
                },
            },
        } as any);
        vi.mocked(connections.list).mockResolvedValue({
            status: 200,
            data: {items: [], cursor: ''},
        } as any);
        vi.mocked(connections.reauth).mockResolvedValue({data: {type: 'complete'}} as any);
        vi.mocked(tasks.pollForTaskFinalized).mockResolvedValue({
            result: PollForTaskResult.FINALIZED,
        } as any);
    });

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
        expect(screen.getByText('configured')).toBeInTheDocument();
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
            {state: ConnectionState.CONFIGURED, label: 'configured'},
            {state: ConnectionState.SETUP, label: 'setup'},
            {state: ConnectionState.DISABLED, label: 'disabled'},
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

    test('promotes reauthentication for unhealthy configured connections', async () => {
        const store = createMockStore(rootInitialState());
        const user = userEvent.setup();
        const unhealthyConnection: Connection = {
            ...baseConnection,
            connector: {
                ...mockConnector,
                has_configure: true,
            },
            health_state: ConnectionHealthState.UNHEALTHY,
        };

        render(
            <Provider store={store}>
                <ConnectionCard connection={unhealthyConnection}/>
            </Provider>
        );

        expect(screen.getByText('Needs attention')).toBeInTheDocument();
        expect(screen.getByText('Reauthentication required')).toBeInTheDocument();
        expect(screen.getByText(/Re-authenticate to restore access/i)).toBeInTheDocument();
        expect(screen.getByRole('button', {name: /Re-authenticate/i})).toBeInTheDocument();
        expect(screen.getByRole('button', {name: /Reconfigure/i})).toBeInTheDocument();
        expect(screen.getByRole('button', {name: /Disconnect/i})).toBeInTheDocument();

        await user.click(screen.getByRole('button', {name: /Re-authenticate/i}));

        await waitFor(() => {
            expect(connections.reauth).toHaveBeenCalledWith(
                unhealthyConnection.id,
                window.location.href,
            );
        });
    });

    test('keeps healthy connection secondary actions in the menu', async () => {
        const store = createMockStore(rootInitialState());
        const user = userEvent.setup();
        const healthyConnection: Connection = {
            ...baseConnection,
            connector: {
                ...mockConnector,
                has_configure: true,
            },
        };

        render(
            <Provider store={store}>
                <ConnectionCard connection={healthyConnection}/>
            </Provider>
        );

        expect(screen.getByRole('button', {name: /Reconfigure/i})).toBeInTheDocument();
        expect(screen.queryByRole('button', {name: /Re-authenticate/i})).not.toBeInTheDocument();
        expect(screen.queryByRole('button', {name: /^Disconnect$/i})).not.toBeInTheDocument();

        await user.click(screen.getByRole('button', {name: /Connection actions/i}));

        expect(screen.getByRole('menuitem', {name: /Re-authenticate/i})).toBeInTheDocument();
        expect(screen.getByRole('menuitem', {name: /^Disconnect$/i})).toBeInTheDocument();

        await user.click(screen.getByRole('menuitem', {name: /Re-authenticate/i}));

        await waitFor(() => {
            expect(connections.reauth).toHaveBeenCalledWith(
                healthyConnection.id,
                window.location.href,
            );
        });
    });

    test('opens disconnect confirmation from healthy connection action menu', async () => {
        const store = createMockStore(rootInitialState());
        const user = userEvent.setup();

        render(
            <Provider store={store}>
                <ConnectionCard connection={baseConnection}/>
            </Provider>
        );

        await user.click(screen.getByRole('button', {name: /Connection actions/i}));
        await user.click(screen.getByRole('menuitem', {name: /^Disconnect$/i}));

        expect(screen.getByRole('heading', {name: /Disconnect Confirmation/i})).toBeInTheDocument();
        await user.click(screen.getByRole('button', {name: /^Disconnect$/i}));

        await waitFor(() => {
            expect(connections.disconnect).toHaveBeenCalledWith(baseConnection.id);
        });
    });

    test('renders skeleton correctly', () => {
        render(<ConnectionCardSkeleton/>);

        // Check if the skeleton elements are in the document
        const skeletons = document.querySelectorAll('.MuiSkeleton-root');
        expect(skeletons.length).toBeGreaterThan(0);
    });
});
