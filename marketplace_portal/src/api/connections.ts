import { client } from './client';
import { 
  Connection, 
  ListConnectionsResponse, 
  InitiateConnectionRequest, 
  InitiateConnectionRedirectResponse,
  DisconnectResponseJson,
  TaskInfoJson
} from '../models';

/**
 * Get a list of all connections
 * @param state Optional state filter
 * @param cursor Optional cursor for pagination
 * @param limit Optional limit for pagination
 * @returns Promise with the list of connections
 */
export const listConnections = (state?: string, cursor?: string, limit?: number) => {
  const params: Record<string, string | number> = {};

  if (state) params.state = state;
  if (cursor) params.cursor = cursor;
  if (limit) params.limit = limit;

  return client.get<ListConnectionsResponse>('/api/v1/connections', { params });
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
 * Get task information
 * @param id The ID of the task to get
 * @returns Promise with the task information
 */
export const getTask = (id: string) => {
  return client.get<TaskInfoJson>(`/api/v1/tasks/${id}`);
};

export const connections = {
  list: listConnections,
  get: getConnection,
  initiate: initiateConnection,
  disconnect: disconnectConnection,
  getTask: getTask
};
