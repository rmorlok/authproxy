import { client } from './client';
import {Connector} from "./connectors";
import {ListResponse} from "./common";

// Connection models
export enum ConnectionState {
  CREATED = 'created',
  CONNECTED = 'connected',
  FAILED = 'failed',
  DISCONNECTING = 'disconnecting',
  DISCONNECTED = 'disconnected'
}

export interface Connection {
  id: string;
  connector: Connector;
  state: ConnectionState;
  created_at: string;
  updated_at: string;
}

export function canBeDisconnected(connection: Connection): boolean {
  return connection.state !== ConnectionState.DISCONNECTING &&
      connection.state !== ConnectionState.DISCONNECTED;
}

// Request models
export interface InitiateConnectionRequest {
  connector_id: string;
  return_to_url: string;
}

export enum InitiateConnectionResponseType {
  REDIRECT = 'redirect'
}

export interface InitiateConnectionResponse {
  id: string;
  type: InitiateConnectionResponseType;
}

export interface InitiateConnectionRedirectResponse extends InitiateConnectionResponse {
  redirect_url: string;
}

// Disconnect models
export interface DisconnectResponseJson {
  task_id: string;
  connection: Connection;
}

export interface ForceConnectionStateRequest {
    state: ConnectionState;
}

export interface ForceConnectionStateResponse extends Connection {}

/**
 * Parameters used for listing connections.
 * This interface defines the criteria and options available for querying connection data.
 *
 * @interface
 */
export interface ListConnectionsParams {
    state?: ConnectionState;
    cursor?: string;
    limit?: number;
    order_by?: string;
}

/**
 * Get a list of all connections
 * @param params The parameters for filtering and pagination
 * @returns Promise with the list of connections
 */
export const listConnections = (params: ListConnectionsParams) => {
  return client.get<ListResponse<Connection>>('/api/v1/connections', { params });
};

/**
 * Get a specific connection by ID
 * @param id The ID of the connection to get
 * @returns Promise with the connection details
 */
export const getConnection = (id: string) => {
  return client.get<Connection>(`/api/v1/connections/${id}`);
};

/**
 * Initiate a new connection
 * @param connectorId The ID of the connector to connect to
 * @param returnToUrl The URL to return to after the connection is established
 * @returns Promise with the initiation response
 */
export const initiateConnection = (connectorId: string, returnToUrl: string) => {
  const request: InitiateConnectionRequest = {
    connector_id: connectorId,
    return_to_url: returnToUrl
  };

  return client.post<InitiateConnectionRedirectResponse>('/api/v1/connections/_initiate', request);
};

/**
 * Disconnect a connection
 * @param id The ID of the connection to disconnect
 * @returns Promise with the disconnect response
 */
export const disconnectConnection = (id: string) => {
  return client.post<DisconnectResponseJson>(`/api/v1/connections/${id}/_disconnect`);
};

/**
 * Force the state of a connection. Requires admin permissions.
 * @param id The ID of the connection to force state for
 * @param state The state to force
 * @returns Promise with final state of the connection
 */
export const forceConnectionState = (id: string, state: ConnectionState) => {
    const request: ForceConnectionStateRequest = {
        state: state
    };
    return client.put<ForceConnectionStateResponse>(`/api/v1/connections/${id}/_force_state`, request);
};

export const connections = {
  list: listConnections,
  get: getConnection,
  initiate: initiateConnection,
  disconnect: disconnectConnection,
  force_state: forceConnectionState,
};
