import {createAsyncThunk, createSlice} from '@reduxjs/toolkit';
import type {RootState} from './store';
import {
    Connection,
    connections,
    ConnectionState,
    DisconnectResponseJson,
    InitiateConnectionFormResponse,
    InitiateConnectionResponse,
    isErrorResponse,
    isFormResponse,
    isVerifyingResponse,
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

// applySetupResponse updates state based on a setup-step response. Verifying responses
// set the polling marker; error responses populate verifyError; form responses populate
// currentFormStep; redirect and complete clear the in-flight setup UI.
function applySetupResponse(state: ConnectionsState, response: InitiateConnectionResponse): void {
    if (isFormResponse(response)) {
        state.currentFormStep = formStepFromResponse(response);
        state.verifyingConnectionId = null;
        state.verifyError = null;
        return;
    }
    if (isVerifyingResponse(response)) {
        state.verifyingConnectionId = response.id;
        state.currentFormStep = null;
        state.verifyError = null;
        return;
    }
    if (isErrorResponse(response)) {
        state.verifyError = {
            connectionId: response.id,
            message: response.error,
            canRetry: response.can_retry,
        };
        state.verifyingConnectionId = null;
        state.currentFormStep = null;
        return;
    }

    // Redirect or complete — clear in-flight UI.
    state.currentFormStep = null;
    state.verifyingConnectionId = null;
    state.verifyError = null;
}

interface VerifyError {
    connectionId: string;
    message: string;
    canRetry: boolean;
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
    verifyingConnectionId: string | null;
    verifyError: VerifyError | null;
    retryingConnection: boolean;
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
    verifyingConnectionId: null,
    verifyError: null,
    retryingConnection: false,
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

export const cancelSetupConnectionAsync = createAsyncThunk(
    'connections/cancelSetupConnection',
    async (connectionId: string) => {
        await connections.cancelSetup(connectionId);
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

export const retryConnectionAsync = createAsyncThunk(
    'connections/retryConnection',
    async ({connectionId, returnToUrl}: { connectionId: string, returnToUrl?: string }) => {
        const response = await connections.retry(connectionId, returnToUrl);
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
        clearVerifyState: (state) => {
            state.verifyingConnectionId = null;
            state.verifyError = null;
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
                state.verifyingConnectionId = null;
                state.verifyError = null;
            })
            .addCase(initiateConnectionAsync.fulfilled, (state, action) => {
                state.initiatingConnection = false;
                applySetupResponse(state, action.payload);
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
                applySetupResponse(state, action.payload);
            })
            .addCase(submitConnectionFormAsync.rejected, (state, action) => {
                state.submittingForm = false;
                state.formSubmitError = action.error.message || 'Failed to submit form';
            })

            // Abort connection
            .addCase(abortConnectionAsync.fulfilled, (state, action) => {
                state.currentFormStep = null;
                state.verifyingConnectionId = null;
                state.verifyError = null;
                state.items = state.items.filter(conn => conn.id !== action.payload);
            })

            // Cancel setup (reconfigure abandonment on a ready connection)
            .addCase(cancelSetupConnectionAsync.fulfilled, (state, action) => {
                state.currentFormStep = null;
                state.verifyingConnectionId = null;
                state.verifyError = null;
                const idx = state.items.findIndex(c => c.id === action.payload);
                if (idx !== -1) {
                    state.items[idx] = {
                        ...state.items[idx],
                        setup_step: undefined,
                        setup_error: undefined,
                    };
                }
            })

            // Get setup step (resume / verify polling)
            .addCase(getSetupStepAsync.fulfilled, (state, action) => {
                applySetupResponse(state, action.payload);
            })

            // Reconfigure connection
            .addCase(reconfigureConnectionAsync.pending, (state) => {
                state.initiatingConnection = true;
                state.initiationError = null;
            })
            .addCase(reconfigureConnectionAsync.fulfilled, (state, action) => {
                state.initiatingConnection = false;
                applySetupResponse(state, action.payload);
            })
            .addCase(reconfigureConnectionAsync.rejected, (state, action) => {
                state.initiatingConnection = false;
                state.initiationError = action.error.message || 'Failed to reconfigure connection';
            })

            // Retry connection
            .addCase(retryConnectionAsync.pending, (state) => {
                state.retryingConnection = true;
                state.verifyError = null;
            })
            .addCase(retryConnectionAsync.fulfilled, (state, action) => {
                state.retryingConnection = false;
                applySetupResponse(state, action.payload);
            })
            .addCase(retryConnectionAsync.rejected, (state, action) => {
                state.retryingConnection = false;
                state.initiationError = action.error.message || 'Failed to retry connection';
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

export const {clearInitiationError, clearDisconnectionError, clearFormStep, clearVerifyState} = connectionsSlice.actions;

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

export const selectVerifyingConnectionId = (state: RootState) => state.connections.verifyingConnectionId;
export const selectVerifyError = (state: RootState) => state.connections.verifyError;
export const selectRetryingConnection = (state: RootState) => state.connections.retryingConnection;

// Helper selectors
export const selectActiveConnections = (state: RootState) =>
    state.connections.items.filter(conn => conn.state === ConnectionState.READY);

export default connectionsSlice.reducer;
