import {configureStore, ThunkAction, Action} from '@reduxjs/toolkit';
import counterReducer from './counterSlice';
import authReducer from './authSlice';
import connectorsReducer from './connectorsSlice';
import connectionsReducer from './connectionsSlice';

export const store = configureStore({
    reducer: {
        counter: counterReducer,
        auth: authReducer,
        connectors: connectorsReducer,
        connections: connectionsReducer,
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
