import { createAsyncThunk, createSlice } from '@reduxjs/toolkit';
import type { RootState } from './store';
import { connectors, Connector } from '../api';

interface ConnectorsState {
  items: Connector[];
  status: 'idle' | 'loading' | 'succeeded' | 'failed';
  error: string | null;
}

const initialState: ConnectorsState = {
  items: [],
  status: 'idle',
  error: null
};

export const fetchConnectorsAsync = createAsyncThunk(
  'connectors/fetchConnectors',
  async () => {
    const response = await connectors.list();
    return response.data.items;
  }
);

export const connectorsSlice = createSlice({
  name: 'connectors',
  initialState,
  reducers: {
    // No reducers needed for now
  },
  extraReducers: (builder) => {
    builder
      .addCase(fetchConnectorsAsync.pending, (state) => {
        state.status = 'loading';
      })
      .addCase(fetchConnectorsAsync.fulfilled, (state, action) => {
        state.status = 'succeeded';
        state.items = action.payload;
      })
      .addCase(fetchConnectorsAsync.rejected, (state, action) => {
        state.status = 'failed';
        state.error = action.error.message || 'Failed to fetch connectors';
      });
  },
});

// Selectors
export const selectConnectors = (state: RootState) => state.connectors.items;
export const selectConnectorsStatus = (state: RootState) => state.connectors.status;
export const selectConnectorsError = (state: RootState) => state.connectors.error;

export default connectorsSlice.reducer;