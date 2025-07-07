import {createAsyncThunk, createSlice, PayloadAction} from '@reduxjs/toolkit';
import type {RootState} from './store';
import {
    ApiSessionInitiateRequest,
    ApiSessionInitiateFailureResponse,
    ApiSessionInitiateSuccessResponse,
    session,
    isInitiateSessionSuccessResponse
} from '../api';

interface AuthState {
    actor_id: string | null;
    status: 'checking' | 'redirecting' | 'authenticated' | 'unauthenticated';
}

const initialState: AuthState = {
    actor_id: null,
    status: 'checking',
};

export const initiateSessionAsync = createAsyncThunk<
    ApiSessionInitiateSuccessResponse,
    ApiSessionInitiateRequest,
    {
        rejectValue: ApiSessionInitiateFailureResponse,
    }
>(
    'auth/initiateSession',
    async (params, { rejectWithValue }) => {
        try {
            const response = await session.initiate(params);

            if (isInitiateSessionSuccessResponse(response.data)) {
                return response.data;
            } else {
                return rejectWithValue(response.data);
            }
        } catch (error: any) {
            // Handle unexpected errors
            if (error.response?.data && !isInitiateSessionSuccessResponse(error.response.data)) {
                return rejectWithValue(error.response.data);
            }

            // Default error response
            return rejectWithValue({ redirect_url: import.meta.env.VITE_PUBLIC_BASE_URL + '/error' });
        }
    }
);

export const sessionSlice = createSlice({
    name: 'auth',
    initialState,
    reducers: {
        terminate: (state) => {
            state.status = 'unauthenticated';
            state.actor_id = null;

            setTimeout(async () => {
                await session.terminate();
            }, 0);
        }
    },
    extraReducers: (builder) => {
        builder
            .addCase(initiateSessionAsync.pending, (state) => {
                state.actor_id = null;
                state.status = 'checking';
            })
            .addCase(initiateSessionAsync.fulfilled, (state, action) => {
                state.status = 'authenticated';
                state.actor_id = action.payload.actor_id;
            })
            .addCase(initiateSessionAsync.rejected, (state, action) => {
                state.status = 'unauthenticated';
                state.actor_id = null;
                setTimeout(() => {
                    window.location.href = action.payload?.redirect_url || import.meta.env.VITE_PUBLIC_BASE_URL + '/error';
                }, 0);
        });
    },
});

export const {terminate} = sessionSlice.actions;

export const selectAuthStatus = (state: RootState) => state.auth.status;
export const selectActorId = (state: RootState) => state.auth.actor_id;

export default sessionSlice.reducer;