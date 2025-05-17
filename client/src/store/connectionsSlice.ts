import { createAsyncThunk, createSlice } from '@reduxjs/toolkit';
import type { RootState } from './store';
import { Connection, ConnectionState } from '../models';
import { connections } from '../api';

interface ConnectionsState {
  items: Connection[];
  status: 'idle' | 'loading' | 'succeeded' | 'failed';
  error: string | null;
  initiatingConnection: boolean;
  initiationError: string | null;
}

const initialState: ConnectionsState = {
  items: [],
  status: 'idle',
  error: null,
  initiatingConnection: false,
  initiationError: null
};

export const fetchConnectionsAsync = createAsyncThunk(
  'connections/fetchConnections',
  async (state?: string) => {
    const response = await connections.list(state);
    return response.data.items;
  }
);

export const initiateConnectionAsync = createAsyncThunk(
  'connections/initiateConnection',
  async ({ connectorId, returnToUrl }: { connectorId: string, returnToUrl: string }) => {
    const response = await connections.initiate(connectorId, returnToUrl);
    return response.data;
  }
);

export const connectionsSlice = createSlice({
  name: 'connections',
  initialState,
  reducers: {
    clearInitiationError: (state) => {
      state.initiationError = null;
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
      });
  },
});

export const { clearInitiationError } = connectionsSlice.actions;

// Selectors
export const selectConnections = (state: RootState) => state.connections.items;
export const selectConnectionsStatus = (state: RootState) => state.connections.status;
export const selectConnectionsError = (state: RootState) => state.connections.error;
export const selectInitiatingConnection = (state: RootState) => state.connections.initiatingConnection;
export const selectInitiationError = (state: RootState) => state.connections.initiationError;

// Helper selectors
export const selectActiveConnections = (state: RootState) => 
  state.connections.items.filter(conn => conn.state === ConnectionState.CONNECTED);

export default connectionsSlice.reducer;