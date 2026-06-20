import * as React from 'react';
import {render, screen, waitFor} from '@testing-library/react';
import '@testing-library/jest-dom';
import userEvent from '@testing-library/user-event';
import {Provider} from 'react-redux';
import {combineReducers, configureStore} from '@reduxjs/toolkit';
import {MemoryRouter, useLocation} from 'react-router-dom';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {
    Connector,
    ConnectorVersionState,
    connections,
} from '@authproxy/api';
import ConnectorList from '../components/ConnectorList';
import authReducer from '../store/sessionSlice';
import connectorsReducer from '../store/connectorsSlice';
import connectionsReducer from '../store/connectionsSlice';
import toastsReducer from '../store/toastsSlice';

vi.mock('@authproxy/api', async () => {
    const actual = await vi.importActual<typeof import('@authproxy/api')>('@authproxy/api');
    return {
        ...actual,
        connections: {
            ...actual.connections,
            abort: vi.fn(),
            initiate: vi.fn(),
            list: vi.fn(),
            submit: vi.fn(),
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
    highlight: 'Calendar highlight',
    logo: 'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg"/%3E',
    has_configure: false,
    versions: 1,
    states: [ConnectorVersionState.ACTIVE],
    created_at: '2023-04-01T12:00:00Z',
    updated_at: '2023-04-01T12:00:00Z',
};

const baseConnectionsState = {
    items: [],
    status: 'succeeded',
    error: null,
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

function renderConnectorList(preloadedState: any) {
    const store = createStore({
        auth: { actor_id: 'actor_test', status: 'authenticated' },
        toasts: {items: []},
        ...preloadedState,
    });

    render(
        <MemoryRouter>
            <Provider store={store}>
                <ConnectorList/>
                <LocationDisplay/>
            </Provider>
        </MemoryRouter>,
    );

    return store;
}

function LocationDisplay() {
    const location = useLocation();
    return <div data-testid="location">{location.pathname}</div>;
}

describe('ConnectorList', () => {
    beforeEach(() => {
        vi.mocked(connections.abort).mockReset();
        vi.mocked(connections.initiate).mockReset();
        vi.mocked(connections.list).mockReset();
        vi.mocked(connections.submit).mockReset();
        vi.mocked(connections.abort).mockResolvedValue({} as any);
        vi.mocked(connections.initiate).mockResolvedValue({data: {id: 'c-new', type: 'complete'}} as any);
        vi.mocked(connections.list).mockResolvedValue({status: 200, data: {items: [], cursor: ''}} as any);
        vi.mocked(connections.submit).mockResolvedValue({data: {id: 'c-setup', type: 'complete'}} as any);
    });

    test('renders skeletons while connectors load', () => {
        renderConnectorList({
            connectors: {items: [], status: 'loading', error: null},
            connections: baseConnectionsState,
        });

        expect(document.querySelectorAll('.MuiSkeleton-root').length).toBeGreaterThan(0);
    });

    test('shows connector loading errors', () => {
        renderConnectorList({
            connectors: {items: [], status: 'failed', error: 'Failed to fetch connectors'},
            connections: baseConnectionsState,
        });

        expect(screen.getByText('Failed to fetch connectors')).toBeInTheDocument();
    });

    test('shows an empty state when no connectors are available', () => {
        renderConnectorList({
            connectors: {items: [], status: 'succeeded', error: null},
            connections: baseConnectionsState,
        });

        expect(screen.getByText('No connectors available')).toBeInTheDocument();
    });

    test('initiates a connection from the connect action', async () => {
        const user = userEvent.setup();
        renderConnectorList({
            connectors: {items: [connector], status: 'succeeded', error: null},
            connections: baseConnectionsState,
        });

        await user.click(screen.getByRole('button', {name: /Connect/i}));

        await waitFor(() => {
            expect(connections.initiate).toHaveBeenCalledWith(
                'google-calendar',
                `${window.location.origin}/connections`,
            );
        });
    });

    test('shows connector card copy on the available connectors screen', () => {
        renderConnectorList({
            connectors: {
                items: [
                    connector,
                    {
                        ...connector,
                        id: 'gmail',
                        display_name: 'GMail',
                        highlight: undefined,
                        description: 'Have the agent respond to your emails without you needing to be involved. Like magic.',
                    },
                ],
                status: 'succeeded',
                error: null,
            },
            connections: baseConnectionsState,
        });

        expect(screen.getByText('Calendar highlight')).toBeInTheDocument();
        expect(screen.getByText('Have the agent respond to your emails without you needing to be involved. Like magic.')).toBeInTheDocument();
    });

    test('returns to the connections page when connect completes without setup steps', async () => {
        const user = userEvent.setup();
        renderConnectorList({
            connectors: {items: [connector], status: 'succeeded', error: null},
            connections: baseConnectionsState,
        });

        await user.click(screen.getByRole('button', {name: /Connect/i}));

        await waitFor(() => {
            expect(screen.getByTestId('location')).toHaveTextContent('/connections');
        });
        expect(connections.list).toHaveBeenCalledWith({limit: 100});
    });

    test('navigates to the connector overview from details', async () => {
        const user = userEvent.setup();
        renderConnectorList({
            connectors: {items: [connector], status: 'succeeded', error: null},
            connections: baseConnectionsState,
        });

        await user.click(screen.getByRole('button', {name: /^Details$/i}));

        expect(screen.getByTestId('location')).toHaveTextContent('/connectors/google-calendar');
    });

    test('renders setup dialog and aborts when setup is cancelled', async () => {
        const user = userEvent.setup();
        renderConnectorList({
            connectors: {items: [connector], status: 'succeeded', error: null},
            connections: {
                ...baseConnectionsState,
                currentFormStep: {
                    connectionId: 'c-setup',
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
        });

        expect(screen.getByText('Complete setup')).toBeInTheDocument();
        expect(screen.getAllByText('Select a Calendar').length).toBeGreaterThanOrEqual(1);

        await user.click(screen.getByRole('button', {name: /Cancel setup/i}));

        await waitFor(() => {
            expect(connections.abort).toHaveBeenCalledWith('c-setup');
        });
    });

    test('submits setup form with return URL for OAuth redirects after preconnect', async () => {
        const user = userEvent.setup();
        renderConnectorList({
            connectors: {items: [connector], status: 'succeeded', error: null},
            connections: {
                ...baseConnectionsState,
                currentFormStep: {
                    connectionId: 'c-setup',
                    stepId: 'tenant',
                    stepTitle: 'Choose a tenant',
                    stepDescription: 'Select the tenant before OAuth.',
                    jsonSchema: {
                        type: 'object',
                        required: ['tenant'],
                        properties: {
                            tenant: {
                                type: 'string',
                                title: 'Tenant',
                            },
                        },
                    },
                    uiSchema: {
                        type: 'VerticalLayout',
                        elements: [{type: 'Control', scope: '#/properties/tenant'}],
                    },
                },
            },
        });

        await user.type(screen.getByRole('textbox', {name: /Tenant/i}), 'northwind');
        const submitButton = screen.getByRole('button', {name: /Save and verify/i});
        await waitFor(() => {
            expect(submitButton).toBeEnabled();
        });
        await user.click(submitButton);

        await waitFor(() => {
            expect(connections.submit).toHaveBeenCalledWith(
                'c-setup',
                'tenant',
                expect.any(Object),
                `${window.location.origin}/connections`,
            );
        });
    });
});
