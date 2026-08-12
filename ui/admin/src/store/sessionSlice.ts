import {createAsyncThunk, createSlice} from '@reduxjs/toolkit';
import type {RootState} from './store';
import {
    ApiSessionInitiateRequest,
    ApiSessionInitiateFailureResponse,
    ApiSessionInitiateSuccessResponse,
    session,
    isInitiateSessionSuccessResponse
} from '@authproxy/api';

interface AuthState {
    actorId: string | null;
    status: 'checking' | 'redirecting' | 'authenticated' | 'unauthenticated';
}

const initialState: AuthState = {
    actorId: null,
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
                console.warn('[AuthProxy admin] session initiate returned a redirect', response.data);
                return rejectWithValue(response.data);
            }
        } catch (error: any) {
            // Handle unexpected errors. The SDK already logged the underlying
            // request failure; here we just record what we're going to do next.
            if (error.response?.data && !isInitiateSessionSuccessResponse(error.response.data)) {
                console.warn('[AuthProxy admin] session initiate failed; using server-provided redirect', error.response.data);
                return rejectWithValue(error.response.data);
            }

            const fallback = new URL('error', import.meta.env.VITE_ADMIN_BASE_URL).toString();
            console.error(
                `[AuthProxy admin] session initiate failed with no usable response; falling back to ${fallback}`,
            );
            return rejectWithValue({ redirectUrl: fallback });
        }
    }
);

export const sessionSlice = createSlice({
    name: 'auth',
    initialState,
    reducers: {
        terminate: (state) => {
            state.status = 'checking';
            state.actorId = null;

            setTimeout(async () => {
                await session.terminate();
            }, 0);
        }
    },
    extraReducers: (builder) => {
        builder
            .addCase(initiateSessionAsync.pending, (state) => {
                state.actorId = null;
                state.status = 'checking';
            })
            .addCase(initiateSessionAsync.fulfilled, (state, action) => {
                state.status = 'authenticated';
                state.actorId = action.payload.actorId;
            })
            .addCase(initiateSessionAsync.rejected, (state, action) => {
                state.status = 'unauthenticated';
                state.actorId = null;
                const target = action.payload?.redirectUrl || import.meta.env.VITE_ADMIN_BASE_URL + '/error';
                console.warn(`[AuthProxy admin] not authenticated; redirecting to ${target}`);
                setTimeout(() => {
                    window.location.href = target;
                }, 0);
        });
    },
});

export const {terminate} = sessionSlice.actions;

export const selectAuthStatus = (state: RootState) => state.auth.status;
export const selectActorId = (state: RootState) => state.auth.actorId;

export default sessionSlice.reducer;