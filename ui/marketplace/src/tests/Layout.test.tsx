import * as React from 'react';
import {render, screen, waitFor} from '@testing-library/react';
import '@testing-library/jest-dom';
import userEvent from '@testing-library/user-event';
import {Provider} from 'react-redux';
import {combineReducers, configureStore} from '@reduxjs/toolkit';
import {MemoryRouter, Route, Routes, useLocation} from 'react-router-dom';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {
    Notification,
    NotificationLevel,
    NotificationState,
    notifications,
} from '@authproxy/api';
import Layout from '../components/Layout';
import authReducer from '../store/sessionSlice';
import connectorsReducer from '../store/connectorsSlice';
import connectionsReducer from '../store/connectionsSlice';
import notificationsReducer from '../store/notificationsSlice';
import toastsReducer from '../store/toastsSlice';

vi.mock('@authproxy/api', async () => {
    const actual = await vi.importActual<typeof import('@authproxy/api')>('@authproxy/api');
    return {
        ...actual,
        notifications: {
            ...actual.notifications,
            list: vi.fn(),
            markBatchViewed: vi.fn(),
        },
    };
});

const notification: Notification = {
    id: 'ntf_1',
    key: 'connection:cxn_1:auth_required',
    level: NotificationLevel.WARNING,
    state: NotificationState.ACTIVE,
    resource_type: 'connection',
    resource_id: 'cxn_1',
    namespace: 'root',
    title: 'Connection requires re-authentication',
    message: 'Reconnect this connection to continue using it.',
    action_url: '/connections/cxn_1?action=reauth',
    can_action: true,
    viewed: false,
    created_at: '2026-07-12T12:00:00Z',
    updated_at: '2026-07-12T12:00:00Z',
};

function LocationProbe() {
    const location = useLocation();
    return <div data-testid="location">{location.pathname + location.search}</div>;
}

function renderLayout() {
    const store = configureStore({
        reducer: combineReducers({
            auth: authReducer,
            connectors: connectorsReducer,
            connections: connectionsReducer,
            notifications: notificationsReducer,
            toasts: toastsReducer,
        }),
        preloadedState: {
            auth: { actor_id: 'actor_test', status: 'authenticated' },
            connectors: { items: [], status: 'succeeded', error: null },
            connections: {
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
                recentlyCompletedConnectionId: null,
            },
            notifications: {
                items: [],
                status: 'idle',
                error: null,
                markingViewed: false,
            },
            toasts: { items: [] },
        },
    });

    render(
        <MemoryRouter initialEntries={['/']}>
            <Provider store={store}>
                <Routes>
                    <Route element={<Layout />}>
                        <Route path="/" element={<div>Connections page</div>} />
                        <Route path="/connections/:connectionId" element={<LocationProbe />} />
                    </Route>
                </Routes>
            </Provider>
        </MemoryRouter>
    );

    return store;
}

describe('Layout notifications', () => {
    beforeEach(() => {
        vi.mocked(notifications.list).mockReset();
        vi.mocked(notifications.markBatchViewed).mockReset();
        vi.mocked(notifications.list).mockResolvedValue({
            data: {items: [notification], cursor: ''},
        } as any);
        vi.mocked(notifications.markBatchViewed).mockResolvedValue({} as any);
    });

    test('shows notifications, marks them viewed, and follows action URLs', async () => {
        const user = userEvent.setup();
        const store = renderLayout();

        await waitFor(() => {
            expect(notifications.list).toHaveBeenCalledWith({
                limit: 25,
                include_viewed: true,
            });
        });

        await user.click(screen.getByLabelText('Open notifications'));

        expect(await screen.findByText('Connection requires re-authentication')).toBeInTheDocument();
        expect(screen.getByText('Reconnect this connection to continue using it.')).toBeInTheDocument();
        expect(notifications.markBatchViewed).toHaveBeenCalledWith(['ntf_1']);

        await waitFor(() => {
            expect(store.getState().notifications.items[0].viewed).toBe(true);
        });

        await user.click(screen.getByRole('button', {name: 'Open'}));

        expect(await screen.findByTestId('location')).toHaveTextContent('/connections/cxn_1?action=reauth');
    });
});
