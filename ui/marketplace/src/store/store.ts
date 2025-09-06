import {configureStore, ThunkAction, Action} from '@reduxjs/toolkit';
import authReducer from './sessionSlice';
import connectorsReducer from './connectorsSlice';
import connectionsReducer from './connectionsSlice';
import toastsReducer from './toastsSlice';

export const store = configureStore({
    reducer: {
        auth: authReducer,
        connectors: connectorsReducer,
        connections: connectionsReducer,
        toasts: toastsReducer,
    },
});

export type RootState = ReturnType<typeof store.getState>;
export type AppDispatch = typeof store.dispatch;
export type AppThunk<ReturnType = void> = ThunkAction<
    ReturnType,
    RootState,
    unknown,
    Action<string>
>;
