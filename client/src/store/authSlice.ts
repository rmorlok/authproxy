import {createAsyncThunk, createSlice, PayloadAction} from '@reduxjs/toolkit';
import type {RootState} from './store';
import type {ApiUser} from "../api";
import {auth} from '../api';

interface AuthState {
    user: ApiUser | null;
    status: 'checking' | 'redirecting' | 'authenticated' | 'unauthenticated';
    providers: string[];
}

const initialState: AuthState = {
    user: null,
    status: 'checking',
    providers: []
};

export const loadProvidersAsync = createAsyncThunk(
    'auth/loadProviders',
    async () => {
        const response = await auth.listProviders();
        return response.data;
    });

export const loadAuthStateAsync = createAsyncThunk(
    'auth/loadState',
    async () => {
    const response = await auth.getUser();
    return response.data;
});

export const authSlice = createSlice({
    name: 'auth',
    initialState,
    reducers: {
        loginWithProvider: (state, action: PayloadAction<string>) => {
            state.status = 'redirecting';
            setTimeout(() => {
                window.location.href = import.meta.env.VITE_LOGIN_URL_TEMPLATE.replace('%PROVIDER%', action.payload)
            }, 0);
        },
        logout: (state) => {
            state.status = 'unauthenticated';
            state.user = null;

            setTimeout(async () => {
                await auth.logout();
            }, 0);
        }
    },
    extraReducers: (builder) => {
        builder
            .addCase(loadAuthStateAsync.pending, (state) => {
                state.status = 'checking';
            })
            .addCase(loadAuthStateAsync.fulfilled, (state, action) => {
                state.status = 'authenticated';
                state.user = action.payload;
            })
            .addCase(loadAuthStateAsync.rejected, (state, action) => {
                state.status = 'unauthenticated';
                state.user = null;
        });
    },
});

export const {loginWithProvider, logout} = authSlice.actions;

// The function below is called a selector and allows us to select a value from
// the state. Selectors can also be defined inline where they're used instead of
// in the slice file. For example: `useSelector((state: RootState) => state.counter.value)`
export const selectAuthStatus = (state: RootState) => state.auth.status;
export const selectUser = (state: RootState) => state.auth.user;

export default authSlice.reducer;