import * as React from 'react';
import {render, screen, waitFor} from '@testing-library/react';
import '@testing-library/jest-dom';
import userEvent from '@testing-library/user-event';
import {Provider} from 'react-redux';
import {combineReducers, configureStore} from '@reduxjs/toolkit';
import {MemoryRouter, Route, Routes, useLocation} from 'react-router-dom';
import ConnectionCard, {ConnectionCardSkeleton} from '../components/ConnectionCard';
import {
    Connection,
    ConnectionState,
    ConnectionHealthState,
    Connector,
    API_VERSION,
    connections,
    tasks,
    PollForTaskResult,
} from '@authproxy/api';
import authReducer from '../store/sessionSlice';
import connectorsReducer from '../store/connectorsSlice';
import connectionsReducer from '../store/connectionsSlice';
import toastsReducer from '../store/toastsSlice';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {completeSetupResponseFixture, connectionFixture, connectorFixture} from '../testing/resources';

vi.mock('@authproxy/api', async () => {
    const actual = await vi.importActual<typeof import('@authproxy/api')>('@authproxy/api');
    return {
        ...actual,
        connections: {
            ...actual.connections,
            disconnect: vi.fn(),
            getSetupStep: vi.fn(),
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

const renderConnectionCard = (
    connection: Connection,
    store = createMockStore(rootInitialState()),
    connector?: Connector,
) => {
    const LocationProbe = () => {
        const location = useLocation();
        return <span data-testid="location">{location.pathname}{location.search}</span>;
    };

    render(
        <MemoryRouter initialEntries={['/connections']}>
            <Provider store={store}>
                <Routes>
                    <Route path="/connections" element={<ConnectionCard connection={connection} connector={connector}/>} />
                    <Route path="/connections/:connectionId" element={<LocationProbe/>} />
                </Routes>
            </Provider>
        </MemoryRouter>
    );

    return store;
};

describe('ConnectionCard', () => {
    const mockConnector: Connector = connectorFixture({
        displayName: 'Google Calendar',
        description: 'Connect to your Google Calendar to manage events and appointments.',
        logo: {publicUrl: 'https://example.com/google-calendar-logo.png'},
        hasConfigure: false,
    });

    const baseConnection: Connection = connectionFixture({
        id: '123e4567-e89b-12d3-a456-426614174000',
        name: 'primary-calendar',
        connector: mockConnector,
        state: ConnectionState.CONFIGURED,
        healthState: ConnectionHealthState.HEALTHY,
    });

    beforeEach(() => {
        vi.mocked(connections.disconnect).mockReset();
        vi.mocked(connections.getSetupStep).mockReset();
        vi.mocked(connections.list).mockReset();
        vi.mocked(connections.reauth).mockReset();
        vi.mocked(tasks.pollForTaskFinalized).mockReset();
        vi.mocked(connections.disconnect).mockResolvedValue({
            data: {
                apiVersion: API_VERSION,
                kind: 'ConnectionDisconnect',
                metadata: {target: {apiVersion: API_VERSION, kind: 'Connection', id: baseConnection.metadata.id}},
                spec: {},
                status: {taskId: 'task-123', connection: {
                    ...baseConnection,
                    status: {...baseConnection.status, lifecycle: {state: ConnectionState.DISCONNECTING}},
                }},
            },
        } as any);
        vi.mocked(connections.list).mockResolvedValue({
            status: 200,
            data: {apiVersion: API_VERSION, kind: 'ConnectionList', metadata: {}, items: []},
        } as any);
        vi.mocked(connections.getSetupStep).mockResolvedValue({data: completeSetupResponseFixture(baseConnection.metadata.id)} as any);
        vi.mocked(connections.reauth).mockResolvedValue({data: completeSetupResponseFixture(baseConnection.metadata.id)} as any);
        vi.mocked(tasks.pollForTaskFinalized).mockResolvedValue({
            result: PollForTaskResult.FINALIZED,
        } as any);
    });

    test('renders connection information correctly with connector details', () => {
        const store = createMockStore(rootInitialState());

        renderConnectionCard(baseConnection, store, mockConnector);

        // Check if the connector name is displayed
        expect(screen.getByText('Google Calendar')).toBeInTheDocument();

        // Check if the connection date is displayed
        expect(screen.getByText(/Connected on/)).toBeInTheDocument();

        expect(screen.queryByText('configured')).not.toBeInTheDocument();
    });

    test('renders with unknown connector fallback when connector missing', () => {
        const store = createMockStore(rootInitialState());
        renderConnectionCard(baseConnection, store);

        // Check if the unknown connector text is displayed
        expect(screen.getByText('Unknown Connector')).toBeInTheDocument();
    });

    test('renders different status labels based on connection state', () => {
        const states = [
            {state: ConnectionState.CONFIGURED, label: /Connected on/},
            {state: ConnectionState.SETUP, label: 'Requires setup'},
            {state: ConnectionState.DISABLED, label: 'Requires reconnection'},
            {state: ConnectionState.DISCONNECTED, label: 'Disconnected'},
        ];

        states.forEach(({state, label}) => {
            const store = createMockStore(rootInitialState());
            const connection = {
                ...baseConnection,
                status: {...baseConnection.status, lifecycle: {state}},
            };

            const {unmount} = render(
                <MemoryRouter>
                    <Provider store={store}>
                        <ConnectionCard connection={connection} connector={mockConnector}/>
                    </Provider>
                </MemoryRouter>
            );

            expect(screen.getByText(label)).toBeInTheDocument();

            unmount();
        });
    });

    test('resumes setup connections from their current step', async () => {
        const store = createMockStore(rootInitialState());
        const user = userEvent.setup();
        const setupConnection = connectionFixture({
            id: baseConnection.metadata.id,
            connector: mockConnector,
            state: ConnectionState.SETUP,
        });
        vi.mocked(connections.getSetupStep).mockResolvedValue({
            data: {
                apiVersion: API_VERSION,
                kind: 'ConnectionSetup',
                metadata: {target: {apiVersion: API_VERSION, kind: 'Connection', id: setupConnection.metadata.id}},
                spec: {},
                status: {
                    type: 'form',
                    stepId: 'calendar',
                    stepTitle: 'Select a Calendar',
                    stepDescription: 'Choose which calendar should be managed.',
                    jsonSchema: {type: 'object'},
                    uiSchema: {type: 'VerticalLayout'},
                },
            },
        } as any);

        renderConnectionCard(setupConnection, store, mockConnector);

        await user.click(screen.getByRole('button', {name: /Resume setup/i}));

        await waitFor(() => {
            expect(connections.getSetupStep).toHaveBeenCalledWith(
                setupConnection.metadata.id,
                window.location.href,
            );
        });
        expect(store.getState().connections.currentFormStep).toMatchObject({
            connectionId: setupConnection.metadata.id,
            stepId: 'calendar',
            stepTitle: 'Select a Calendar',
        });
    });

    test('promotes reauthentication for unhealthy configured connections', async () => {
        const store = createMockStore(rootInitialState());
        const user = userEvent.setup();
        const configurableConnector = connectorFixture({
            displayName: 'Google Calendar',
            description: 'Connect to your Google Calendar to manage events and appointments.',
            logo: {publicUrl: 'https://example.com/google-calendar-logo.png'},
            hasConfigure: true,
        });
        const unhealthyConnection = connectionFixture({
            id: baseConnection.metadata.id,
            connector: configurableConnector,
            healthState: ConnectionHealthState.UNHEALTHY,
        });

        renderConnectionCard(unhealthyConnection, store, configurableConnector);

        expect(screen.getByText('Requires reconnection')).toBeInTheDocument();
        expect(screen.getByText('Reconnection required')).toBeInTheDocument();
        expect(screen.getByRole('button', {name: /Re-authenticate/i})).toBeInTheDocument();
        expect(screen.getByRole('button', {name: /Reconfigure/i})).toBeInTheDocument();
        expect(screen.getByRole('button', {name: /Disconnect/i})).toBeInTheDocument();

        await user.click(screen.getByRole('button', {name: /Re-authenticate/i}));

        await waitFor(() => {
            expect(connections.reauth).toHaveBeenCalledWith(
                unhealthyConnection.metadata.id,
                {returnToUrl: window.location.href},
            );
        });
    });

    test('keeps healthy connection secondary actions in the menu', async () => {
        const store = createMockStore(rootInitialState());
        const user = userEvent.setup();
        const configurableConnector = connectorFixture({
            displayName: 'Google Calendar',
            description: 'Connect to your Google Calendar to manage events and appointments.',
            logo: {publicUrl: 'https://example.com/google-calendar-logo.png'},
            hasConfigure: true,
        });
        const healthyConnection = connectionFixture({
            id: baseConnection.metadata.id,
            connector: configurableConnector,
        });

        renderConnectionCard(healthyConnection, store, configurableConnector);

        expect(screen.getByRole('button', {name: /Reconfigure/i})).toBeInTheDocument();
        expect(screen.queryByRole('button', {name: /Re-authenticate/i})).not.toBeInTheDocument();
        expect(screen.queryByRole('button', {name: /^Disconnect$/i})).not.toBeInTheDocument();

        await user.click(screen.getByRole('button', {name: /Connection actions/i}));

        expect(screen.getByRole('menuitem', {name: /View details/i})).toBeInTheDocument();
        expect(screen.getByRole('menuitem', {name: /Re-authenticate/i})).toBeInTheDocument();
        expect(screen.getByRole('menuitem', {name: /^Disconnect$/i})).toBeInTheDocument();

        await user.click(screen.getByRole('menuitem', {name: /Re-authenticate/i}));

        await waitFor(() => {
            expect(connections.reauth).toHaveBeenCalledWith(
                healthyConnection.metadata.id,
                {returnToUrl: window.location.href},
            );
        });
    });

    test('navigates to connection details from the healthy connection action menu', async () => {
        const store = createMockStore(rootInitialState());
        const user = userEvent.setup();

        renderConnectionCard(baseConnection, store, mockConnector);

        await user.click(screen.getByRole('button', {name: /Connection actions/i}));
        await user.click(screen.getByRole('menuitem', {name: /View details/i}));

        expect(screen.getByTestId('location')).toHaveTextContent(`/connections/${baseConnection.metadata.id}`);
    });

    test('opens disconnect confirmation from healthy connection action menu', async () => {
        const store = createMockStore(rootInitialState());
        const user = userEvent.setup();

        renderConnectionCard(baseConnection, store, mockConnector);

        await user.click(screen.getByRole('button', {name: /Connection actions/i}));
        await user.click(screen.getByRole('menuitem', {name: /^Disconnect$/i}));

        expect(screen.getByRole('heading', {name: /Disconnect Confirmation/i})).toBeInTheDocument();
        await user.click(screen.getByRole('button', {name: /^Disconnect$/i}));

        await waitFor(() => {
            expect(connections.disconnect).toHaveBeenCalledWith(baseConnection.metadata.id);
        });
    });

    test('renders skeleton correctly', () => {
        render(<ConnectionCardSkeleton/>);

        // Check if the skeleton elements are in the document
        const skeletons = document.querySelectorAll('.MuiSkeleton-root');
        expect(skeletons.length).toBeGreaterThan(0);
    });
});
