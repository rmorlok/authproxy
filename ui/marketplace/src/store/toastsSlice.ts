import {createAction, createAsyncThunk, createSlice, PayloadAction} from '@reduxjs/toolkit';
import type { RootState } from './store';

export interface Toast {
  message: string;
  type: 'success' | 'info' | 'warning' | 'error';
  durationMs?: number;
}

interface ToastState extends Toast {
  id: string;
  removeAt?: number;
  closed: boolean;
}

interface ToastsState {
  items: ToastState[];
}

const initialState: ToastsState = {
  items: [],
};

export const addToast = createAction<Toast>('toasts/addToast');

export const toastsSlice = createSlice({
  name: 'toasts',
  initialState,
  reducers: {
    closeToast: (state, action: PayloadAction<number>) => {
      if (state.items[action.payload]) {
        state.items[action.payload].closed = true;
      }
    }
  },
  extraReducers: (builder) => {
    builder
      // Fetch connections
      .addCase(addToast, (state, action) => {
        const ts: ToastState = {
          ...action.payload,
          id: crypto.randomUUID(),
          closed: false
        }

        if (action.payload.durationMs) {
          ts.removeAt = Date.now() + action.payload.durationMs;
        }

        state.items.push(ts);
      });
  },
});

export const { closeToast } = toastsSlice.actions;

// Selectors
export const selectToasts = (state: RootState) => state.toasts.items;

export default toastsSlice.reducer;
