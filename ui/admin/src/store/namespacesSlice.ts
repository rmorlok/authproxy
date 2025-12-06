import {createAsyncThunk, createSlice, PayloadAction} from '@reduxjs/toolkit';
import type {RootState, AppThunk} from './store';
import {
    namespaces, ListNamespaceParams, Namespace, ROOT_NAMESPACE_PATH, NamespaceState,
} from '@authproxy/api';

interface NamespacesState {
    currentPath: string;
    hasInitialized: boolean;

    //
    // State related to the current namespace
    //

    current: Namespace | null;
    status: 'idle' | 'loading' | 'succeeded' | 'failed';
    error: string | null;

    //
    // State related to the children of the current namespace
    //

    children: Namespace[];
    childrenStatus: 'idle' | 'loading' | 'succeeded' | 'failed';
    childrenError: string | null;
}

type currentState = Pick<NamespacesState, 'current' | 'status' | 'error'>
type childrenState = Pick<NamespacesState, 'children' | 'childrenStatus' | 'childrenError'>

const initialState: NamespacesState = {
    currentPath: ROOT_NAMESPACE_PATH,
    hasInitialized: false,
    current: null,
    status: 'loading',
    error: null,
    children: [],
    childrenStatus: 'loading',
    childrenError: null,
};

export const loadCurrent = createAsyncThunk<
    currentState,
    string,
    {
        rejectValue: currentState,
    }
>(
    'namespaces/loadCurrent',
    async (_, thunkApi) => {
        const { rejectWithValue, getState } = thunkApi;
        const state = (getState() as RootState).namespaces;

        let current: Namespace = {
            path: state.currentPath,
            state: NamespaceState.ACTIVE,
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString()
        };

        const response = await namespaces.getByPath(state.currentPath);
        if (response.status === 200) {
            current = response.data;
        } else {
            return rejectWithValue({
                status: 'failed',
                current,
                error: "failed to load current namespace",
            });
        }

        return {
            status: 'succeeded',
            current,
            error: null,
        };
    }
);

export const loadCurrentChildren = createAsyncThunk<
    childrenState,
    string,
    {
        rejectValue: childrenState,
    }
>(
    'namespaces/loadCurrentChildren',
    async (_, thunkApi) => {
        const { rejectWithValue, getState } = thunkApi;
        const state = (getState() as RootState).namespaces;

        let allItems: Namespace[] = [];
        const params: ListNamespaceParams = {
            children_of: state.currentPath,
            limit: 100,
        };
        
        let response = await namespaces.list(params);
        if (response.status === 200 && response.data.items) {
            allItems = allItems.concat(response.data.items);
        } else {
            return rejectWithValue({
                childrenStatus: 'failed',
                children: allItems,
                childrenError: "failed to load namespace children",
            });
        }

        while (response.data.cursor && response.data.cursor !== "") {
            response = await namespaces.list({cursor: response.data.cursor});
            if (response.status === 200) {
                allItems = allItems.concat(response.data.items);
            } else {
                return rejectWithValue({
                    childrenStatus: 'failed',
                    children: allItems,
                    childrenError: "failed to load namespace children",
                });
            }
        }

        return {
            childrenStatus: 'succeeded',
            children: allItems,
            childrenError: null,
        };
    }
);

export const namespacesSlice = createSlice({
    name: 'namespaces',
    initialState,
    reducers: {
        setCurrentNamespacePath: (state, action: PayloadAction<string>) => {
            state.currentPath = action.payload;
            state.hasInitialized = true;
        }
    },
    extraReducers: (builder) => {
        builder
            .addCase(loadCurrent.pending, (state) => {
                state.status = 'loading';
            })
            .addCase(loadCurrent.fulfilled, (state, action) => {
                Object.assign(state, action.payload)
            })
            .addCase(loadCurrent.rejected, (state, action) => {
                state.status = 'failed';
                state.error = action.error.message || 'Failed to fetch namespaces';
            })
            .addCase(loadCurrentChildren.pending, (state) => {
                state.childrenStatus = 'loading';
            })
            .addCase(loadCurrentChildren.fulfilled, (state, action) => {
                Object.assign(state, action.payload)
            })
            .addCase(loadCurrentChildren.rejected, (state, action) => {
                state.childrenStatus = 'failed';
                state.childrenError = action.error.message || 'Failed to fetch namespace children';
            });
    },
});

// Selectors
export const selectCurrentNamespacePath = (state: RootState) => state.namespaces.currentPath;
export const selectCurrentNamespace = (state: RootState) => state.namespaces.current;
export const selectCurrentNamespaceChildren = (state: RootState) => state.namespaces.children;
export const selectNamespaceStatus = (state: RootState) => state.namespaces.status;
export const selectNamespaceChildrenStatus = (state: RootState) => state.namespaces.childrenStatus;
export const selectNamespaceError = (state: RootState) => state.namespaces.error;
export const selectHasInitializedNamespace = (state: RootState) => state.namespaces.hasInitialized;

export const setCurrentNamespace = (path: string): AppThunk => (dispatch) => {
    dispatch(namespacesSlice.actions.setCurrentNamespacePath(path));
    dispatch(loadCurrent(path));
    dispatch(loadCurrentChildren(path));
};

export default namespacesSlice.reducer;