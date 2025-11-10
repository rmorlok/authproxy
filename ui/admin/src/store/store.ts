import {configureStore, ThunkAction, Action} from '@reduxjs/toolkit';
import authReducer from './sessionSlice';
import toastsReducer from './toastsSlice';

export const store = configureStore({
    reducer: {
        auth: authReducer,
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
