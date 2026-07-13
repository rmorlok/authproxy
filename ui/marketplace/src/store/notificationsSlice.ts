import {createAsyncThunk, createSlice} from '@reduxjs/toolkit';
import type {RootState} from './store';
import {
    Notification,
    ListNotificationsParams,
    notifications,
} from '@authproxy/api';

interface NotificationsState {
    items: Notification[];
    status: 'idle' | 'loading' | 'succeeded' | 'failed';
    error: string | null;
    markingViewed: boolean;
}

const initialState: NotificationsState = {
    items: [],
    status: 'idle',
    error: null,
    markingViewed: false,
};

export const fetchNotificationsAsync = createAsyncThunk(
    'notifications/fetchNotifications',
    async (params?: ListNotificationsParams) => {
        const response = await notifications.list({
            limit: 25,
            include_viewed: true,
            ...params,
        });
        return response.data.items;
    }
);

export const markNotificationsViewedAsync = createAsyncThunk(
    'notifications/markNotificationsViewed',
    async (ids: string[]) => {
        if (ids.length > 0) {
            await notifications.markBatchViewed(ids);
        }
        return ids;
    }
);

export const notificationsSlice = createSlice({
    name: 'notifications',
    initialState,
    reducers: {},
    extraReducers: (builder) => {
        builder
            .addCase(fetchNotificationsAsync.pending, (state) => {
                state.status = 'loading';
                state.error = null;
            })
            .addCase(fetchNotificationsAsync.fulfilled, (state, action) => {
                state.status = 'succeeded';
                state.items = action.payload;
            })
            .addCase(fetchNotificationsAsync.rejected, (state, action) => {
                state.status = 'failed';
                state.error = action.error.message || 'Failed to fetch notifications';
            })
            .addCase(markNotificationsViewedAsync.pending, (state) => {
                state.markingViewed = true;
            })
            .addCase(markNotificationsViewedAsync.fulfilled, (state, action) => {
                state.markingViewed = false;
                const viewedIds = new Set(action.payload);
                state.items = state.items.map((item) => (
                    viewedIds.has(item.id) ? {...item, viewed: true} : item
                ));
            })
            .addCase(markNotificationsViewedAsync.rejected, (state, action) => {
                state.markingViewed = false;
                state.error = action.error.message || 'Failed to mark notifications viewed';
            });
    },
});

export const selectNotifications = (state: RootState) => state.notifications.items;
export const selectNotificationsStatus = (state: RootState) => state.notifications.status;
export const selectNotificationsError = (state: RootState) => state.notifications.error;
export const selectUnviewedNotificationCount = (state: RootState) =>
    state.notifications.items.filter((item) => !item.viewed).length;

export default notificationsSlice.reducer;
