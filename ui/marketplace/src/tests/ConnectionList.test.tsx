import * as React from 'react';
import {render, screen} from '@testing-library/react';
import '@testing-library/jest-dom';
import userEvent from '@testing-library/user-event';
import {Provider} from 'react-redux';
import {combineReducers, configureStore} from '@reduxjs/toolkit';
import {MemoryRouter} from 'react-router-dom';
import ConnectionList from '../components/ConnectionList';
import authReducer from '../store/sessionSlice';
import connectorsReducer from '../store/connectorsSlice';
import connectionsReducer from '../store/connectionsSlice';
import toastsReducer from '../store/toastsSlice';
import {Connection, ConnectionState, ConnectionHealthState, Connector, ConnectorVersionState, connections} from '@authproxy/api';
import {beforeEach, describe, expect, test, vi} from 'vitest';

vi.mock('@authproxy/api', async () => {
    const actual = await vi.importActual<typeof import('@authproxy/api')>('@authproxy/api');
    return {
        ...actual,
        connections: {
            ...actual.connections,
            abort: vi.fn(),
            list: vi.fn(),
            retry: vi.fn(),
        },
    };
});

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
    namespace: 'root',
    version: 1,
    state: ConnectorVersionState.ACTIVE,
    display_name: 'Google Calendar',
    description: 'Calendar app',
    highlight: undefined,
    logo: 'https://example.com/logo.png',
    has_configure: false,
    versions: 1,
    states: [ConnectorVersionState.ACTIVE],
    created_at: '2023-04-01T12:00:00Z',
    updated_at: '2023-04-01T12:00:00Z',
};

const makeConnection = (overrides: Partial<Connection> = {}): Connection => ({
    id: 'c-1',
    namespace: 'root',
    connector: connector,
    state: ConnectionState.CONFIGURED,
    health_state: ConnectionHealthState.HEALTHY,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
});

describe('ConnectionList', () => {
    const baseConnectionsState = {
        initiatingConnection: false,
        initiationError: null,
        disconnectingConnection: false,
        disconnectionError: null,
        currentTaskId: null,
        currentFormStep: null,
        submittingForm: false,
        formSubmitError: null,
        verifyingConnectionId: null,
        verifyError: null,
        retryingConnection: false,
    };

    beforeEach(() => {
        vi.mocked(connections.abort).mockReset();
        vi.mocked(connections.list).mockReset();
        vi.mocked(connections.retry).mockReset();
        vi.mocked(connections.abort).mockResolvedValue({} as any);
        vi.mocked(connections.list).mockResolvedValue({
            status: 200,
            data: {items: [], cursor: ''},
        } as any);
        vi.mocked(connections.retry).mockResolvedValue({data: {type: 'complete'}} as any);
    });

    test('renders skeletons when loading', () => {
        const store = createStore({
            connectors: {items: [], status: 'succeeded', error: null},
            connections: {
                ...baseConnectionsState,
                items: [],
                status: 'loading',
                error: null,
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
                ...baseConnectionsState,
                items: [],
                status: 'failed',
                error: 'Failed to fetch connections',
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

    test('shows first-connection guidance and connector discovery when there are no connections', () => {
        const store = createStore({
            connectors: {items: [connector], status: 'succeeded', error: null},
            connections: {
                ...baseConnectionsState,
                items: [],
                status: 'succeeded',
                error: null,
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

        expect(screen.getByText('Connect your first application')).toBeInTheDocument();
        expect(screen.getByText('Available connectors')).toBeInTheDocument();
        expect(screen.getByText('Google Calendar')).toBeInTheDocument();
        expect(screen.getByRole('button', {name: /Connect/i})).toBeInTheDocument();
    });

    test('renders list of connections when present', () => {
        const items = [makeConnection({id: 'c-1'}), makeConnection({id: 'c-2'})];
        const store = createStore({
            connectors: {items: [connector], status: 'succeeded', error: null},
            connections: {
                ...baseConnectionsState,
                items,
                status: 'succeeded',
                error: null,
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

    test('renders polished setup dialog copy and actions', () => {
        const store = createStore({
            connectors: {items: [connector], status: 'succeeded', error: null},
            connections: {
                ...baseConnectionsState,
                items: [makeConnection()],
                status: 'succeeded',
                error: null,
                currentFormStep: {
                    connectionId: 'c-1',
                    stepId: 'calendar',
                    stepTitle: 'Select a Calendar',
                    stepDescription: 'Choose which calendar should be managed.',
                    jsonSchema: {
                        type: 'object',
                        properties: {
                            calendar_id: {
                                type: 'string',
                                title: 'Calendar',
                                enum: ['primary'],
                            },
                        },
                    },
                    uiSchema: {
                        type: 'VerticalLayout',
                        elements: [{type: 'Control', scope: '#/properties/calendar_id'}],
                    },
                },
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

        expect(screen.getByText('Complete setup')).toBeInTheDocument();
        expect(screen.getAllByText('Select a Calendar').length).toBeGreaterThanOrEqual(1);
        expect(screen.getByText('Choose which calendar should be managed.')).toBeInTheDocument();
        expect(screen.getByRole('button', {name: /Cancel setup/i})).toBeInTheDocument();
        expect(screen.getByRole('button', {name: /Save and verify/i})).toBeInTheDocument();
    });

    test('renders verification progress dialog', () => {
        const store = createStore({
            connectors: {items: [connector], status: 'succeeded', error: null},
            connections: {
                ...baseConnectionsState,
                items: [makeConnection()],
                status: 'succeeded',
                error: null,
                verifyingConnectionId: 'c-1',
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

        expect(screen.getByText('Verifying connection')).toBeInTheDocument();
        expect(screen.getByText('Checking credentials')).toBeInTheDocument();
        expect(screen.getByText(/confirming that this connection can reach the provider/i)).toBeInTheDocument();
        expect(screen.getByRole('progressbar')).toBeInTheDocument();
    });

    test('retries verification failure from the retry action', async () => {
        const user = userEvent.setup();
        const store = createStore({
            connectors: {items: [connector], status: 'succeeded', error: null},
            connections: {
                ...baseConnectionsState,
                items: [makeConnection()],
                status: 'succeeded',
                error: null,
                verifyError: {
                    connectionId: 'c-1',
                    message: 'Provider rejected the saved credentials.',
                    canRetry: true,
                },
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

        expect(screen.getByText('Provider check failed')).toBeInTheDocument();
        expect(screen.getByText('Provider rejected the saved credentials.')).toBeInTheDocument();

        await user.click(screen.getByRole('button', {name: /Retry setup/i}));

        expect(connections.retry).toHaveBeenCalledWith('c-1', window.location.href);
    });

    test('cancels verification failure and hides retry when retry is unavailable', async () => {
        const user = userEvent.setup();
        const store = createStore({
            connectors: {items: [connector], status: 'succeeded', error: null},
            connections: {
                ...baseConnectionsState,
                items: [makeConnection()],
                status: 'succeeded',
                error: null,
                verifyError: {
                    connectionId: 'c-1',
                    message: 'Credentials cannot be recovered.',
                    canRetry: false,
                },
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

        expect(screen.queryByRole('button', {name: /Retry setup/i})).not.toBeInTheDocument();
        await user.click(screen.getByRole('button', {name: /Cancel setup/i}));

        expect(connections.abort).toHaveBeenCalledWith('c-1');
    });
});
