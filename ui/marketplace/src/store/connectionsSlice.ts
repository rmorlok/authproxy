import {createAsyncThunk, createSlice} from '@reduxjs/toolkit';
import type {RootState} from './store';
import {
    Connection,
    connections,
    ConnectionState,
    DisconnectResponseJson,
    InitiateConnectionFormResponse,
    isFormResponse,
    ListConnectionsParams,
} from '@authproxy/api';

interface FormStep {
    connectionId: string;
    stepId: string;
    stepTitle?: string;
    stepDescription?: string;
    currentStep: number;
    totalSteps: number;
    jsonSchema: Record<string, unknown>;
    uiSchema: Record<string, unknown>;
}

function formStepFromResponse(response: InitiateConnectionFormResponse): FormStep {
    return {
        connectionId: response.id,
        stepId: response.step_id,
        stepTitle: response.step_title,
        stepDescription: response.step_description,
        currentStep: response.current_step,
        totalSteps: response.total_steps,
        jsonSchema: response.json_schema,
        uiSchema: response.ui_schema,
    };
}

interface ConnectionsState {
    items: Connection[];
    status: 'idle' | 'loading' | 'succeeded' | 'failed';
    error: string | null;
    initiatingConnection: boolean;
    initiationError: string | null;
    disconnectingConnection: boolean;
    disconnectionError: string | null;
    currentTaskId: string | null;
    currentFormStep: FormStep | null;
    submittingForm: boolean;
    formSubmitError: string | null;
}

const initialState: ConnectionsState = {
    items: [],
    status: 'idle',
    error: null,
    initiatingConnection: false,
    initiationError: null,
    disconnectingConnection: false,
    disconnectionError: null,
    currentTaskId: null,
    currentFormStep: null,
    submittingForm: false,
    formSubmitError: null,
};

export const fetchConnectionsAsync = createAsyncThunk(
    'connections/fetchConnections',
    async (state?: string) => {
        let allItems: Connection[] = [];
        const params: ListConnectionsParams = state ? {state: state as ConnectionState, limit: 100} : {limit: 100};
        let response = await connections.list(params);
        if(response.status === 200 && response.data.items) {
            allItems = allItems.concat(response.data.items);
        }

        while(response.data.cursor && response.data.cursor !== "") {
            response = await connections.list({cursor: response.data.cursor});
            if(response.status === 200) {
                allItems = allItems.concat(response.data.items);
            } else {
                return allItems;
            }
        }

        return allItems;
    }
);

export const initiateConnectionAsync = createAsyncThunk(
    'connections/initiateConnection',
    async ({connectorId, returnToUrl}: { connectorId: string, returnToUrl: string }) => {
        const response = await connections.initiate(connectorId, returnToUrl);
        return response.data;
    }
);

export const submitConnectionFormAsync = createAsyncThunk(
    'connections/submitConnectionForm',
    async ({connectionId, stepId, data}: { connectionId: string, stepId: string, data: unknown }) => {
        const response = await connections.submit(connectionId, stepId, data);
        return response.data;
    }
);

export const abortConnectionAsync = createAsyncThunk(
    'connections/abortConnection',
    async (connectionId: string) => {
        await connections.abort(connectionId);
        return connectionId;
    }
);

export const getSetupStepAsync = createAsyncThunk(
    'connections/getSetupStep',
    async (connectionId: string) => {
        const response = await connections.getSetupStep(connectionId);
        return response.data;
    }
);

export const reconfigureConnectionAsync = createAsyncThunk(
    'connections/reconfigureConnection',
    async (connectionId: string) => {
        const response = await connections.reconfigure(connectionId);
        return response.data;
    }
);

export const disconnectConnectionAsync = createAsyncThunk(
    'connections/disconnectConnection',
    async (connectionId: string, _): Promise<DisconnectResponseJson> => {
        const response = await connections.disconnect(connectionId);

        return response.data;
    }
);

export const connectionsSlice = createSlice({
    name: 'connections',
    initialState,
    reducers: {
        clearInitiationError: (state) => {
            state.initiationError = null;
        },
        clearDisconnectionError: (state) => {
            state.disconnectionError = null;
        },
        clearFormStep: (state) => {
            state.currentFormStep = null;
            state.formSubmitError = null;
        },
    },
    extraReducers: (builder) => {
        builder
            // Fetch connections
            .addCase(fetchConnectionsAsync.pending, (state) => {
                state.status = 'loading';
            })
            .addCase(fetchConnectionsAsync.fulfilled, (state, action) => {
                state.status = 'succeeded';
                state.items = action.payload;
            })
            .addCase(fetchConnectionsAsync.rejected, (state, action) => {
                state.status = 'failed';
                state.error = action.error.message || 'Failed to fetch connections';
            })

            // Initiate connection
            .addCase(initiateConnectionAsync.pending, (state) => {
                state.initiatingConnection = true;
                state.initiationError = null;
                state.currentFormStep = null;
            })
            .addCase(initiateConnectionAsync.fulfilled, (state, action) => {
                state.initiatingConnection = false;
                const response = action.payload;
                if (isFormResponse(response)) {
                    state.currentFormStep = formStepFromResponse(response);
                }
            })
            .addCase(initiateConnectionAsync.rejected, (state, action) => {
                state.initiatingConnection = false;
                state.initiationError = action.error.message || 'Failed to initiate connection';
            })

            // Submit connection form
            .addCase(submitConnectionFormAsync.pending, (state) => {
                state.submittingForm = true;
                state.formSubmitError = null;
            })
            .addCase(submitConnectionFormAsync.fulfilled, (state, action) => {
                state.submittingForm = false;
                const response = action.payload;
                if (isFormResponse(response)) {
                    state.currentFormStep = formStepFromResponse(response);
                } else {
                    state.currentFormStep = null;
                }
            })
            .addCase(submitConnectionFormAsync.rejected, (state, action) => {
                state.submittingForm = false;
                state.formSubmitError = action.error.message || 'Failed to submit form';
            })

            // Abort connection
            .addCase(abortConnectionAsync.fulfilled, (state, action) => {
                state.currentFormStep = null;
                state.items = state.items.filter(conn => conn.id !== action.payload);
            })

            // Get setup step (resume)
            .addCase(getSetupStepAsync.fulfilled, (state, action) => {
                const response = action.payload;
                if (isFormResponse(response)) {
                    state.currentFormStep = formStepFromResponse(response);
                }
            })

            // Reconfigure connection
            .addCase(reconfigureConnectionAsync.pending, (state) => {
                state.initiatingConnection = true;
                state.initiationError = null;
            })
            .addCase(reconfigureConnectionAsync.fulfilled, (state, action) => {
                state.initiatingConnection = false;
                const response = action.payload;
                if (isFormResponse(response)) {
                    state.currentFormStep = formStepFromResponse(response);
                }
            })
            .addCase(reconfigureConnectionAsync.rejected, (state, action) => {
                state.initiatingConnection = false;
                state.initiationError = action.error.message || 'Failed to reconfigure connection';
            })

            // Disconnect connection
            .addCase(disconnectConnectionAsync.pending, (state) => {
                state.disconnectingConnection = true;
                state.disconnectionError = null;
            })
            .addCase(disconnectConnectionAsync.fulfilled, (state, action) => {
                state.disconnectingConnection = false;
                state.currentTaskId = action.payload.task_id;

                // Update the connection in the items array
                const index = state.items.findIndex(conn => conn.id === action.payload.connection.id);
                if (index !== -1) {
                    state.items[index] = action.payload.connection;
                }
            })
            .addCase(disconnectConnectionAsync.rejected, (state, action) => {
                state.disconnectingConnection = false;
                state.disconnectionError = action.error.message || 'Failed to disconnect connection';
            });
    },
});

export const {clearInitiationError, clearDisconnectionError, clearFormStep} = connectionsSlice.actions;

// Selectors
export const selectConnections = (state: RootState) => state.connections.items;
export const selectConnectionsStatus = (state: RootState) => state.connections.status;
export const selectConnectionsError = (state: RootState) => state.connections.error;
export const selectInitiatingConnection = (state: RootState) => state.connections.initiatingConnection;
export const selectInitiationError = (state: RootState) => state.connections.initiationError;
export const selectDisconnectingConnection = (state: RootState) => state.connections.disconnectingConnection;
export const selectDisconnectionError = (state: RootState) => state.connections.disconnectionError;
export const selectCurrentTaskId = (state: RootState) => state.connections.currentTaskId;

export const selectCurrentFormStep = (state: RootState) => state.connections.currentFormStep;
export const selectSubmittingForm = (state: RootState) => state.connections.submittingForm;
export const selectFormSubmitError = (state: RootState) => state.connections.formSubmitError;

// Helper selectors
export const selectActiveConnections = (state: RootState) =>
    state.connections.items.filter(conn => conn.state === ConnectionState.READY);

export default connectionsSlice.reducer;
