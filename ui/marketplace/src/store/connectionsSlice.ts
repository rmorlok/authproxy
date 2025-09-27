import {createAsyncThunk, createSlice} from '@reduxjs/toolkit';
import type {RootState} from './store';
import {Connection, connections, ConnectionState, DisconnectResponseJson, ListConnectionsParams} from '../api';

interface ConnectionsState {
    items: Connection[];
    status: 'idle' | 'loading' | 'succeeded' | 'failed';
    error: string | null;
    initiatingConnection: boolean;
    initiationError: string | null;
    disconnectingConnection: boolean;
    disconnectionError: string | null;
    currentTaskId: string | null;
}

const initialState: ConnectionsState = {
    items: [],
    status: 'idle',
    error: null,
    initiatingConnection: false,
    initiationError: null,
    disconnectingConnection: false,
    disconnectionError: null,
    currentTaskId: null
};

export const fetchConnectionsAsync = createAsyncThunk(
    'connections/fetchConnections',
    async (state?: string) => {
        let allItems: Connection[] = [];
        const params: ListConnectionsParams = state ? {state: state as ConnectionState, limit: 100} : {limit: 100}
        let response = await connections.list(params);
        if(response.status === 200 && response.data.items) {
            allItems = allItems.concat(response.data.items);
        }

        while(response.data.cursor && response.data.cursor !== "") {
            response = await connections.list({cursor: response.data.cursor});
            if(response.status === 200) {
                allItems = allItems.concat(response.data.items);
            } else {
                return allItems;
            }
        }

        return allItems;
    }
);

export const initiateConnectionAsync = createAsyncThunk(
    'connections/initiateConnection',
    async ({connectorId, returnToUrl}: { connectorId: string, returnToUrl: string }) => {
        const response = await connections.initiate(connectorId, returnToUrl);
        return response.data;
    }
);

export const disconnectConnectionAsync = createAsyncThunk(
    'connections/disconnectConnection',
    async (connectionId: string, {dispatch}): Promise<DisconnectResponseJson> => {
        const response = await connections.disconnect(connectionId);

        return response.data;
    }
);

export const connectionsSlice = createSlice({
    name: 'connections',
    initialState,
    reducers: {
        clearInitiationError: (state) => {
            state.initiationError = null;
        },
        clearDisconnectionError: (state) => {
            state.disconnectionError = null;
        }
    },
    extraReducers: (builder) => {
        builder
            // Fetch connections
            .addCase(fetchConnectionsAsync.pending, (state) => {
                state.status = 'loading';
            })
            .addCase(fetchConnectionsAsync.fulfilled, (state, action) => {
                state.status = 'succeeded';
                state.items = action.payload;
            })
            .addCase(fetchConnectionsAsync.rejected, (state, action) => {
                state.status = 'failed';
                state.error = action.error.message || 'Failed to fetch connections';
            })

            // Initiate connection
            .addCase(initiateConnectionAsync.pending, (state) => {
                state.initiatingConnection = true;
                state.initiationError = null;
            })
            .addCase(initiateConnectionAsync.fulfilled, (state) => {
                state.initiatingConnection = false;
            })
            .addCase(initiateConnectionAsync.rejected, (state, action) => {
                state.initiatingConnection = false;
                state.initiationError = action.error.message || 'Failed to initiate connection';
            })

            // Disconnect connection
            .addCase(disconnectConnectionAsync.pending, (state) => {
                state.disconnectingConnection = true;
                state.disconnectionError = null;
            })
            .addCase(disconnectConnectionAsync.fulfilled, (state, action) => {
                state.disconnectingConnection = false;
                state.currentTaskId = action.payload.task_id;

                // Update the connection in the items array
                const index = state.items.findIndex(conn => conn.id === action.payload.connection.id);
                if (index !== -1) {
                    state.items[index] = action.payload.connection;
                }
            })
            .addCase(disconnectConnectionAsync.rejected, (state, action) => {
                state.disconnectingConnection = false;
                state.disconnectionError = action.error.message || 'Failed to disconnect connection';
            });
    },
});

export const {clearInitiationError, clearDisconnectionError} = connectionsSlice.actions;

// Selectors
export const selectConnections = (state: RootState) => state.connections.items;
export const selectConnectionsStatus = (state: RootState) => state.connections.status;
export const selectConnectionsError = (state: RootState) => state.connections.error;
export const selectInitiatingConnection = (state: RootState) => state.connections.initiatingConnection;
export const selectInitiationError = (state: RootState) => state.connections.initiationError;
export const selectDisconnectingConnection = (state: RootState) => state.connections.disconnectingConnection;
export const selectDisconnectionError = (state: RootState) => state.connections.disconnectionError;
export const selectCurrentTaskId = (state: RootState) => state.connections.currentTaskId;

// Helper selectors
export const selectActiveConnections = (state: RootState) =>
    state.connections.items.filter(conn => conn.state === ConnectionState.CONNECTED);

export default connectionsSlice.reducer;
