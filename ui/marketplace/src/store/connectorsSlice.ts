import { createAsyncThunk, createSlice } from '@reduxjs/toolkit';
import type { RootState } from './store';
import {connectors, Connector, ListConnectorsParams, connections} from '@authproxy/api';

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
      let allItems: Connector[] = [];
      const params: ListConnectorsParams = {limit: 100};
      let response = await connectors.list(params);
      if(response.status === 200 && response.data.items) {
          allItems = allItems.concat(response.data.items);
      }

      while(response.data.cursor && response.data.cursor !== "") {
          response = await connectors.list({cursor: response.data.cursor});
          if(response.status === 200) {
              allItems = allItems.concat(response.data.items);
          } else {
              return allItems;
          }
      }

      return allItems;
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